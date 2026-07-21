package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type grokSameAccountRetryState struct {
	cfg          config.GrokSameAccountRetryConfig
	fallbackWait time.Duration
	maxWaiting   int
	retryCount   map[int64]int
	pinned       *service.AccountSelectionResult
	retrying     bool
	deferredErr  *service.UpstreamFailoverError
	attemptCount int
}

func newGrokSameAccountRetryState(cfg *config.Config) *grokSameAccountRetryState {
	state := &grokSameAccountRetryState{retryCount: make(map[int64]int)}
	if cfg == nil {
		return state
	}
	state.cfg = cfg.Gateway.GrokSameAccountRetry
	state.fallbackWait = cfg.Gateway.Scheduling.FallbackWaitTimeout
	state.maxWaiting = cfg.Gateway.Scheduling.FallbackMaxWaiting
	return state
}

func (s *grokSameAccountRetryState) beginAttempt(account *service.Account) int {
	if s == nil || account == nil || account.Platform != service.PlatformGrok {
		return 0
	}
	s.attemptCount++
	return s.attemptCount
}

func (s *grokSameAccountRetryState) schedule(
	ctx context.Context,
	account *service.Account,
	waitPlan *service.AccountWaitPlan,
	failoverErr *service.UpstreamFailoverError,
) (time.Duration, bool) {
	if s == nil || account == nil || account.Platform != service.PlatformGrok || failoverErr == nil ||
		!failoverErr.GrokSameAccountRetry || !s.cfg.Enabled || s.cfg.MaxRetries <= 0 ||
		s.retryCount[account.ID] >= s.cfg.MaxRetries || ctx == nil || ctx.Err() != nil {
		return 0, false
	}

	delay := time.Duration(s.cfg.FallbackDelayMS) * time.Millisecond
	if retryAfter, ok := parseGrokRetryAfter(failoverErr.ResponseHeaders, time.Now()); ok {
		maxDelay := time.Duration(s.cfg.MaxRetryAfterMS) * time.Millisecond
		if retryAfter > maxDelay {
			return 0, false
		}
		delay = retryAfter
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
		return 0, false
	}

	pinnedWaitPlan := &service.AccountWaitPlan{
		AccountID:      account.ID,
		MaxConcurrency: account.Concurrency,
		Timeout:        s.fallbackWait,
		MaxWaiting:     s.maxWaiting,
	}
	if waitPlan != nil {
		copied := *waitPlan
		copied.AccountID = account.ID
		pinnedWaitPlan = &copied
	}
	s.retryCount[account.ID]++
	s.pinned = &service.AccountSelectionResult{Account: account, WaitPlan: pinnedWaitPlan}
	s.deferredErr = failoverErr
	return delay, true
}

func (s *grokSameAccountRetryState) takePinnedSelection() (*service.AccountSelectionResult, bool) {
	if s == nil || s.pinned == nil {
		return nil, false
	}
	selection := s.pinned
	s.pinned = nil
	s.retrying = true
	return selection, true
}

func (s *grokSameAccountRetryState) penaltyForFailedAttempt(current *service.UpstreamFailoverError) *service.UpstreamFailoverError {
	if s == nil || !s.retrying {
		return nil
	}
	s.retrying = false
	if current != nil && current.GrokAccountPenaltyApplied {
		s.deferredErr = nil
		return nil
	}
	if current != nil && current.GrokSameAccountRetry {
		s.deferredErr = current
	}
	penalty := s.deferredErr
	s.deferredErr = nil
	return penalty
}

func (s *grokSameAccountRetryState) cancelScheduledRetryPenalty() *service.UpstreamFailoverError {
	if s == nil || s.pinned == nil {
		return nil
	}
	s.pinned = nil
	penalty := s.deferredErr
	s.deferredErr = nil
	return penalty
}

func (s *grokSameAccountRetryState) finishSuccessfulAttempt() {
	if s == nil {
		return
	}
	s.retrying = false
	s.deferredErr = nil
}

func canStartGrokSameAccountRetry(c *gin.Context, writerSizeBeforeForward int) bool {
	return c != nil && c.Writer != nil && !c.Writer.Written() && c.Writer.Size() == writerSizeBeforeForward
}

func parseGrokRetryAfter(headers http.Header, now time.Time) (time.Duration, bool) {
	if headers == nil {
		return 0, false
	}
	value := strings.TrimSpace(headers.Get("Retry-After"))
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := retryAt.Sub(now)
	if delay < 0 {
		delay = 0
	}
	return delay, true
}
