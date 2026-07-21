package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testGrokRetryConfig() *config.Config {
	return &config.Config{Gateway: config.GatewayConfig{
		GrokSameAccountRetry: config.GrokSameAccountRetryConfig{
			Enabled:         true,
			MaxRetries:      1,
			Statuses:        []int{429, 502, 503, 504, 529},
			FallbackDelayMS: 500,
			MaxRetryAfterMS: 2000,
		},
		Scheduling: config.GatewaySchedulingConfig{
			FallbackWaitTimeout: 3 * time.Second,
			FallbackMaxWaiting:  7,
		},
	}}
}

func TestGrokSameAccountRetryPinsExactAccountOnce(t *testing.T) {
	state := newGrokSameAccountRetryState(testGrokRetryConfig())
	account := &service.Account{ID: 42, Platform: service.PlatformGrok, Concurrency: 3}
	failoverErr := &service.UpstreamFailoverError{
		StatusCode:           http.StatusServiceUnavailable,
		GrokSameAccountRetry: true,
	}

	delay, ok := state.schedule(context.Background(), account, nil, failoverErr)
	require.True(t, ok)
	require.Equal(t, 500*time.Millisecond, delay)

	selection, ok := state.takePinnedSelection()
	require.True(t, ok)
	require.Same(t, account, selection.Account)
	require.False(t, selection.Acquired)
	require.Equal(t, account.ID, selection.WaitPlan.AccountID)
	require.Equal(t, account.Concurrency, selection.WaitPlan.MaxConcurrency)
	require.Equal(t, 3*time.Second, selection.WaitPlan.Timeout)
	require.Equal(t, 7, selection.WaitPlan.MaxWaiting)

	_, ok = state.schedule(context.Background(), account, nil, failoverErr)
	require.False(t, ok)
	require.Same(t, failoverErr, state.penaltyForFailedAttempt(failoverErr))
}

func TestGrokSameAccountRetryKeepsOriginalPenaltyWhenRetryFailsDifferently(t *testing.T) {
	state := newGrokSameAccountRetryState(testGrokRetryConfig())
	account := &service.Account{ID: 45, Platform: service.PlatformGrok, Concurrency: 1}
	original := &service.UpstreamFailoverError{
		StatusCode:           http.StatusTooManyRequests,
		ResponseHeaders:      http.Header{"Retry-After": []string{"1"}},
		GrokSameAccountRetry: true,
	}
	second := &service.UpstreamFailoverError{
		StatusCode: http.StatusBadGateway,
	}

	_, ok := state.schedule(context.Background(), account, nil, original)
	require.True(t, ok)
	selection, ok := state.takePinnedSelection()
	require.True(t, ok)
	require.Same(t, account, selection.Account)
	require.Same(t, original, state.penaltyForFailedAttempt(second))
	require.Nil(t, state.penaltyForFailedAttempt(second))
}

func TestGrokSameAccountRetryDropsDeferredPenaltyWhenCurrentFailureAlreadyAppliedIt(t *testing.T) {
	state := newGrokSameAccountRetryState(testGrokRetryConfig())
	account := &service.Account{ID: 48, Platform: service.PlatformGrok, Concurrency: 1}
	original := &service.UpstreamFailoverError{
		StatusCode:           http.StatusTooManyRequests,
		GrokSameAccountRetry: true,
	}
	current := &service.UpstreamFailoverError{
		StatusCode:                http.StatusUnauthorized,
		GrokAccountPenaltyApplied: true,
	}

	_, ok := state.schedule(context.Background(), account, nil, original)
	require.True(t, ok)
	_, ok = state.takePinnedSelection()
	require.True(t, ok)
	require.Nil(t, state.penaltyForFailedAttempt(current))
	require.Nil(t, state.penaltyForFailedAttempt(current))
}

func TestGrokSameAccountRetrySuccessClearsDeferredPenalty(t *testing.T) {
	state := newGrokSameAccountRetryState(testGrokRetryConfig())
	account := &service.Account{ID: 46, Platform: service.PlatformGrok, Concurrency: 1}
	original := &service.UpstreamFailoverError{
		StatusCode:           http.StatusServiceUnavailable,
		GrokSameAccountRetry: true,
	}

	_, ok := state.schedule(context.Background(), account, nil, original)
	require.True(t, ok)
	_, ok = state.takePinnedSelection()
	require.True(t, ok)
	state.finishSuccessfulAttempt()
	require.Nil(t, state.penaltyForFailedAttempt(nil))
}

func TestGrokSameAccountRetryReacquiresAndReleasesAccountSlot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var acquired int32
	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(_ context.Context, accountID int64, maxConcurrency int, _ string) (bool, error) {
			require.Equal(t, int64(47), accountID)
			require.Equal(t, 2, maxConcurrency)
			atomic.AddInt32(&acquired, 1)
			return true, nil
		},
	}
	h := &OpenAIGatewayHandler{
		gatewayService:    &service.OpenAIGatewayService{},
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	account := &service.Account{ID: 47, Platform: service.PlatformGrok, Concurrency: 2}
	selection := &service.AccountSelectionResult{
		Account:  account,
		WaitPlan: &service.AccountWaitPlan{AccountID: account.ID, MaxConcurrency: account.Concurrency},
	}
	streamStarted := false

	release, ok := h.acquireResponsesAccountSlot(c, nil, "", selection, false, &streamStarted, zap.NewNop())
	require.True(t, ok)
	release()

	state := newGrokSameAccountRetryState(testGrokRetryConfig())
	failure := &service.UpstreamFailoverError{StatusCode: 503, GrokSameAccountRetry: true}
	_, ok = state.schedule(c.Request.Context(), account, selection.WaitPlan, failure)
	require.True(t, ok)
	pinned, ok := state.takePinnedSelection()
	require.True(t, ok)
	require.Same(t, account, pinned.Account)
	release, ok = h.acquireResponsesAccountSlot(c, nil, "", pinned, false, &streamStarted, zap.NewNop())
	require.True(t, ok)
	release()
	state.finishSuccessfulAttempt()

	require.Equal(t, int32(2), atomic.LoadInt32(&acquired))
	require.Equal(t, int32(2), atomic.LoadInt32(&cache.releaseAccountCalled))
}

func TestGrokSameAccountRetryBoundsRetryAfterAndDeadline(t *testing.T) {
	account := &service.Account{ID: 43, Platform: service.PlatformGrok, Concurrency: 1}
	longRetryAfter := &service.UpstreamFailoverError{
		StatusCode:           http.StatusTooManyRequests,
		ResponseHeaders:      http.Header{"Retry-After": []string{"45"}},
		GrokSameAccountRetry: true,
	}

	_, ok := newGrokSameAccountRetryState(testGrokRetryConfig()).schedule(context.Background(), account, nil, longRetryAfter)
	require.False(t, ok)

	shortRetryAfter := &service.UpstreamFailoverError{
		StatusCode:           http.StatusTooManyRequests,
		ResponseHeaders:      http.Header{"Retry-After": []string{"1"}},
		GrokSameAccountRetry: true,
	}
	delay, ok := newGrokSameAccountRetryState(testGrokRetryConfig()).schedule(context.Background(), account, nil, shortRetryAfter)
	require.True(t, ok)
	require.Equal(t, time.Second, delay)

	shortDeadlineState := newGrokSameAccountRetryState(testGrokRetryConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, ok = shortDeadlineState.schedule(ctx, account, nil, shortRetryAfter)
	require.False(t, ok)
}

func TestGrokSameAccountRetryRejectsCanceledAndExcludedRequests(t *testing.T) {
	account := &service.Account{ID: 44, Platform: service.PlatformGrok, Concurrency: 1}
	eligible := &service.UpstreamFailoverError{StatusCode: 503, GrokSameAccountRetry: true}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, ok := newGrokSameAccountRetryState(testGrokRetryConfig()).schedule(ctx, account, nil, eligible)
	require.False(t, ok)

	nonGrok := *account
	nonGrok.Platform = service.PlatformOpenAI
	_, ok = newGrokSameAccountRetryState(testGrokRetryConfig()).schedule(context.Background(), &nonGrok, nil, eligible)
	require.False(t, ok)

	excluded := &service.UpstreamFailoverError{StatusCode: http.StatusUnauthorized}
	_, ok = newGrokSameAccountRetryState(testGrokRetryConfig()).schedule(context.Background(), account, nil, excluded)
	require.False(t, ok)
}

func TestParseGrokRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	headers := http.Header{"Retry-After": []string{now.Add(time.Second).Format(http.TimeFormat)}}
	delay, ok := parseGrokRetryAfter(headers, now)
	require.True(t, ok)
	require.Equal(t, time.Second, delay)
}

func TestCanStartGrokSameAccountRetryRequiresUntouchedWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	sizeBefore := c.Writer.Size()
	require.True(t, canStartGrokSameAccountRetry(c, sizeBefore))

	_, err := c.Writer.Write([]byte("x"))
	require.NoError(t, err)
	require.False(t, canStartGrokSameAccountRetry(c, sizeBefore))
}
