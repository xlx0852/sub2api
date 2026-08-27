package service

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type firstOutputStage uint8

const (
	firstOutputStagePreUpstream firstOutputStage = iota
	firstOutputStageConnect
	firstOutputStageUpstream
	firstOutputStageRetry
	firstOutputStageWrite
)

type FirstOutputStageTracker struct {
	mu sync.Mutex

	startedAt time.Time
	lastAt    time.Time
	stage     firstOutputStage
	finished  bool

	preUpstream time.Duration
	connect     time.Duration
	upstream    time.Duration
	retry       time.Duration
	write       time.Duration
}

type firstOutputStageTrackerContextKey struct{}

func NewFirstOutputStageTracker(startedAt time.Time) *FirstOutputStageTracker {
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	return &FirstOutputStageTracker{
		startedAt: startedAt,
		lastAt:    startedAt,
		stage:     firstOutputStagePreUpstream,
	}
}

func WithFirstOutputStageTracker(ctx context.Context, tracker *FirstOutputStageTracker) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tracker == nil {
		return ctx
	}
	return context.WithValue(ctx, firstOutputStageTrackerContextKey{}, tracker)
}

func FirstOutputStageTrackerFromContext(ctx context.Context) *FirstOutputStageTracker {
	if ctx == nil {
		return nil
	}
	tracker, _ := ctx.Value(firstOutputStageTrackerContextKey{}).(*FirstOutputStageTracker)
	return tracker
}

func firstOutputResponseContext(resp *http.Response) context.Context {
	if resp != nil && resp.Request != nil {
		return resp.Request.Context()
	}
	return context.Background()
}

func markFirstOutputConnectionStart(ctx context.Context) {
	if tracker := FirstOutputStageTrackerFromContext(ctx); tracker != nil {
		tracker.transition(firstOutputStageConnect, time.Now(), false)
	}
}

func markFirstOutputRequestWritten(ctx context.Context) {
	if tracker := FirstOutputStageTrackerFromContext(ctx); tracker != nil {
		tracker.transition(firstOutputStageUpstream, time.Now(), false)
	}
}

func markFirstOutputAttemptFailed(ctx context.Context) {
	if tracker := FirstOutputStageTrackerFromContext(ctx); tracker != nil {
		tracker.transition(firstOutputStageRetry, time.Now(), true)
	}
}

func (t *FirstOutputStageTracker) MarkAttemptFailed() {
	if t != nil {
		t.transition(firstOutputStageRetry, time.Now(), true)
	}
}

func markFirstOutputObserved(ctx context.Context) {
	if tracker := FirstOutputStageTrackerFromContext(ctx); tracker != nil {
		tracker.transition(firstOutputStageWrite, time.Now(), false)
	}
}

func markFirstOutputFlushed(ctx context.Context) {
	if tracker := FirstOutputStageTrackerFromContext(ctx); tracker != nil {
		tracker.finish(time.Now())
	}
}

func (t *FirstOutputStageTracker) transition(next firstOutputStage, now time.Time, failedAttempt bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	t.accrueLocked(now, failedAttempt)
	t.stage = next
	t.lastAt = now
}

func (t *FirstOutputStageTracker) finish(now time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	t.accrueLocked(now, false)
	t.lastAt = now
	t.finished = true
}

func (t *FirstOutputStageTracker) accrueLocked(now time.Time, failedAttempt bool) {
	if now.Before(t.lastAt) {
		now = t.lastAt
	}
	delta := now.Sub(t.lastAt)
	if failedAttempt && t.stage == firstOutputStageUpstream {
		t.retry += delta
		return
	}
	switch t.stage {
	case firstOutputStagePreUpstream:
		t.preUpstream += delta
	case firstOutputStageConnect:
		t.connect += delta
	case firstOutputStageUpstream:
		t.upstream += delta
	case firstOutputStageRetry:
		t.retry += delta
	case firstOutputStageWrite:
		t.write += delta
	}
}

func (t *FirstOutputStageTracker) Snapshot() *domain.FirstOutputStages {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.finished {
		return nil
	}
	pre := durationMilliseconds(t.preUpstream)
	connect := durationMilliseconds(t.connect)
	upstream := durationMilliseconds(t.upstream)
	retry := durationMilliseconds(t.retry)
	write := durationMilliseconds(t.write)
	total := durationMilliseconds(t.lastAt.Sub(t.startedAt))
	return &domain.FirstOutputStages{
		GatewayPreUpstreamMs:      &pre,
		UpstreamConnectMs:         &connect,
		UpstreamFirstOutputMs:     &upstream,
		RetryFailoverMs:           &retry,
		GatewayFirstOutputWriteMs: &write,
		GatewayFirstOutputTotalMs: &total,
	}
}

func durationMilliseconds(value time.Duration) int {
	if value <= 0 {
		return 0
	}
	return int(value.Milliseconds())
}
