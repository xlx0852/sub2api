package service

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type schedulingAcquireProbeCache struct {
	ConcurrencyCache
	acquired atomic.Int64
	released atomic.Int64
}

func (c *schedulingAcquireProbeCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	c.acquired.Add(1)
	return true, nil
}

func (c *schedulingAcquireProbeCache) ReleaseAccountSlot(context.Context, int64, string) error {
	c.released.Add(1)
	return nil
}

func TestSchedulingSlotSkipAcquireIsRequestScoped(t *testing.T) {
	cache := &schedulingAcquireProbeCache{}
	concurrency := &ConcurrencyService{cache: cache}
	gateway := &GatewayService{concurrencyService: concurrency}
	openai := &OpenAIGatewayService{concurrencyService: concurrency}
	entries := map[string]func(context.Context) (*AcquireResult, error){
		"gateway": func(ctx context.Context) (*AcquireResult, error) {
			return gateway.tryAcquireAccountSlot(ctx, "test-model", 7, 3)
		},
		"openai": func(ctx context.Context) (*AcquireResult, error) {
			return openai.tryAcquireAccountSlot(ctx, "test-model", 7, 3)
		},
		"shared": func(ctx context.Context) (*AcquireResult, error) {
			return tryAcquireAccountSlot(ctx, concurrency, "test-model", 7, 3)
		},
	}
	for name, acquire := range entries {
		t.Run(name, func(t *testing.T) {
			beforeAcquire, beforeRelease := cache.acquired.Load(), cache.released.Load()
			result, err := acquire(withSkipAccountAcquire(context.Background()))
			require.NoError(t, err)
			require.True(t, result.Acquired)
			result.ReleaseFunc()
			require.Equal(t, beforeAcquire, cache.acquired.Load())
			require.Equal(t, beforeRelease, cache.released.Load())
			result, err = acquire(context.Background())
			require.NoError(t, err)
			result.ReleaseFunc()
			result.ReleaseFunc()
			require.Equal(t, beforeAcquire+1, cache.acquired.Load())
			require.Equal(t, beforeRelease+1, cache.released.Load())
		})
	}
}

func TestTryAcquireAccountSlotNilConcurrencyFailOpen(t *testing.T) {
	got, err := tryAcquireAccountSlot(context.Background(), nil, "gpt-4", 7, 3)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.True(t, got.Acquired)
	require.False(t, got.ModelLimited)
	require.NotNil(t, got.ReleaseFunc)
	got.ReleaseFunc()
}

func TestOnceReleaseRunsOnlyOnce(t *testing.T) {
	calls := 0
	release := onceRelease(func() { calls++ })
	release()
	release()

	require.Equal(t, 1, calls)
}

func TestGatewayAndOpenAISlotAcquireShareHelper(t *testing.T) {
	gateway := &GatewayService{}
	openai := &OpenAIGatewayService{}

	g, gerr := gateway.tryAcquireAccountSlot(context.Background(), "claude-sonnet", 1, 2)
	o, oerr := openai.tryAcquireAccountSlot(context.Background(), "gpt-4", 1, 2)
	require.NoError(t, gerr)
	require.NoError(t, oerr)
	require.True(t, g.Acquired)
	require.True(t, o.Acquired)
	require.False(t, g.ModelLimited)
	require.False(t, o.ModelLimited)
}
