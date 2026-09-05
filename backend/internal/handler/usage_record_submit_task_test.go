package handler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageRecordContextPreservesIdentityWithoutParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	parent = context.WithValue(parent, ctxkey.RequestID, " request-1 ")
	parent = context.WithValue(parent, ctxkey.ClientRequestID, " client-1 ")
	groupID := int64(42)
	parent = service.WithUsageSettlementGroupID(parent, &groupID)
	cancel()
	base, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	ctx := usageRecordContext(parent, base)
	require.NoError(t, ctx.Err())
	require.Equal(t, "request-1", ctx.Value(ctxkey.RequestID))
	require.Equal(t, "client-1", ctx.Value(ctxkey.ClientRequestID))
	require.Equal(t, &groupID, service.UsageSettlementGroupIDFromContext(ctx))
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	want, _ := base.Deadline()
	require.Equal(t, want, deadline)
}

type handlerUsageSettlementCoordinator struct {
	selectedGroup *int64
}

func (*handlerUsageSettlementCoordinator) Enabled() bool       { return true }
func (*handlerUsageSettlementCoordinator) Authoritative() bool { return false }
func (c *handlerUsageSettlementCoordinator) EnabledFor(groupID *int64) bool {
	return c.selectedGroup == nil || (groupID != nil && *c.selectedGroup == *groupID)
}
func (*handlerUsageSettlementCoordinator) AuthoritativeFor(*int64) bool { return false }
func (*handlerUsageSettlementCoordinator) Wake()                        {}
func (*handlerUsageSettlementCoordinator) StartAttempt(_ context.Context, attempt *domain.UsageSettlementAttempt) (*domain.UsageSettlementAttempt, bool, error) {
	return attempt, true, nil
}
func (*handlerUsageSettlementCoordinator) ObserveProviderRequestID(_ context.Context, _ string, _ int64, _, _ string) (*domain.UsageSettlementAttempt, error) {
	return &domain.UsageSettlementAttempt{}, nil
}
func (*handlerUsageSettlementCoordinator) HandoffReady(_ context.Context, _ string, _ int64, _ string, _ *domain.UsageSettlementPayloadV1) (*domain.UsageSettlementAttempt, error) {
	return &domain.UsageSettlementAttempt{}, nil
}

func newUsageRecordTestPool(t *testing.T) *service.UsageRecordWorkerPool {
	t.Helper()
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             8,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	return pool
}

func TestGatewayHandlerSubmitUsageRecordTask_WithPool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &GatewayHandler{usageRecordWorkerPool: pool}

	done := make(chan struct{})
	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
	}
}

func TestGatewayHandlerSubmitUsageRecordTask_DurableModeHandsOffSynchronously(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	gateway := &service.GatewayService{}
	gateway.SetUsageSettlementCoordinator(&handlerUsageSettlementCoordinator{})
	h := &GatewayHandler{gatewayService: gateway, usageRecordWorkerPool: pool}
	var called atomic.Bool
	h.submitUsageRecordTask(context.Background(), func(context.Context) { called.Store(true) })
	require.True(t, called.Load())
}

func TestGatewayHandlerSubmitUsageRecordTask_UnselectedGroupKeepsLegacyPoolPath(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	pool.Stop()
	selected, other := int64(91), int64(92)
	gateway := &service.GatewayService{}
	gateway.SetUsageSettlementCoordinator(&handlerUsageSettlementCoordinator{selectedGroup: &selected})
	h := &GatewayHandler{gatewayService: gateway, usageRecordWorkerPool: pool}
	var called atomic.Bool
	parent := service.WithUsageSettlementGroupID(context.Background(), &other)
	h.submitUsageRecordTask(parent, func(context.Context) { called.Store(true) })
	require.False(t, called.Load())
}

func TestGatewayHandlerSubmitUsageRecordTask_WithoutPoolSyncFallback(t *testing.T) {
	h := &GatewayHandler{}
	var called atomic.Bool

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline in fallback context")
		}
		called.Store(true)
	})

	require.True(t, called.Load())
}

func TestGatewayHandlerSubmitUsageRecordTask_NilTask(t *testing.T) {
	h := &GatewayHandler{}
	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), nil)
	})
}

func TestGatewayHandlerSubmitUsageRecordTask_WithoutPool_TaskPanicRecovered(t *testing.T) {
	h := &GatewayHandler{}
	var called atomic.Bool

	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
			panic("usage task panic")
		})
	})

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
	})
	require.True(t, called.Load(), "panic 后后续任务应仍可执行")
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithPool(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	done := make(chan struct{})
	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
	}
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_DurableModeHandsOffSynchronously(t *testing.T) {
	pool := newUsageRecordTestPool(t)
	gateway := &service.OpenAIGatewayService{}
	gateway.SetUsageSettlementCoordinator(&handlerUsageSettlementCoordinator{})
	h := &OpenAIGatewayHandler{gatewayService: gateway, usageRecordWorkerPool: pool}
	var called atomic.Bool
	h.submitMandatoryUsageRecordTask(context.Background(), func(context.Context) { called.Store(true) })
	require.True(t, called.Load())
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithoutPoolSyncFallback(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	var called atomic.Bool

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected deadline in fallback context")
		}
		called.Store(true)
	})

	require.True(t, called.Load())
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_NilTask(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), nil)
	})
}

func TestOpenAIGatewayHandlerSubmitUsageRecordTask_WithoutPool_TaskPanicRecovered(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	var called atomic.Bool

	require.NotPanics(t, func() {
		h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
			panic("usage task panic")
		})
	})

	h.submitUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
	})
	require.True(t, called.Load(), "panic 后后续任务应仍可执行")
}

func TestOpenAIGatewayHandlerSubmitMandatoryUsageRecordTask_DroppedTaskSyncFallback(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	block := make(chan struct{})
	release := make(chan struct{})
	pool.Submit(func(ctx context.Context) {
		close(block)
		<-release
	})
	<-block
	pool.Submit(func(ctx context.Context) {})

	var called atomic.Bool
	h.submitMandatoryUsageRecordTask(context.Background(), func(ctx context.Context) {
		called.Store(true)
	})
	close(release)

	require.True(t, called.Load(), "mandatory usage task must run synchronously when async submit is dropped")
}

func TestOpenAIGatewayHandlerSubmitOpenAIUsageRecordTask_ImageResultUsesMandatoryFallback(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	block := make(chan struct{})
	release := make(chan struct{})
	pool.Submit(func(ctx context.Context) {
		close(block)
		<-release
	})
	<-block
	pool.Submit(func(ctx context.Context) {})

	var called atomic.Bool
	h.submitOpenAIUsageRecordTask(context.Background(), &service.OpenAIForwardResult{ImageCount: 1}, func(ctx context.Context) {
		called.Store(true)
	})
	close(release)

	require.True(t, called.Load(), "image usage task must be mandatory when async submit is dropped")
}

func TestOpenAIGatewayHandlerSubmitOpenAIUsageRecordTask_CompactResultUsesMandatoryFallback(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        "drop",
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)
	h := &OpenAIGatewayHandler{usageRecordWorkerPool: pool}

	block := make(chan struct{})
	release := make(chan struct{})
	pool.Submit(func(ctx context.Context) {
		close(block)
		<-release
	})
	<-block
	pool.Submit(func(ctx context.Context) {})

	var called atomic.Bool
	h.submitOpenAIUsageRecordTask(context.Background(), &service.OpenAIForwardResult{Compact: true}, func(ctx context.Context) {
		called.Store(true)
	})
	close(release)

	require.True(t, called.Load(), "compact usage task must be mandatory when async submit is dropped")
}
