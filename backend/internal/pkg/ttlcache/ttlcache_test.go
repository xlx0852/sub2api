package ttlcache

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCache_GetSet(t *testing.T) {
	c := New[int](time.Minute)
	c.set("a", 42)
	v, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, 42, v)
}

func TestCache_GetMiss(t *testing.T) {
	c := New[int](time.Minute)
	_, ok := c.Get("missing")
	require.False(t, ok)
}

func TestCache_GetExpired(t *testing.T) {
	c := New[int](50 * time.Millisecond)
	c.set("a", 1)
	time.Sleep(80 * time.Millisecond)
	_, ok := c.Get("a")
	require.False(t, ok, "过期条目应视为 miss")
}

func TestCache_LoadFillsAndCaches(t *testing.T) {
	c := New[int](time.Minute)
	calls := 0
	fn := func(ctx context.Context) (int, error) {
		calls++
		return 7, nil
	}
	v, err := c.Load(context.Background(), "k", fn)
	require.NoError(t, err)
	require.Equal(t, 7, v)

	// 第二次直接命中缓存，fn 不再执行
	v, err = c.Load(context.Background(), "k", fn)
	require.NoError(t, err)
	require.Equal(t, 7, v)
	require.Equal(t, 1, calls)
}

func TestCache_LoadFnErrorNotCached(t *testing.T) {
	c := New[int](time.Minute)
	fnErr := errors.New("boom")
	calls := 0
	fn := func(ctx context.Context) (int, error) {
		calls++
		return 0, fnErr
	}
	_, err := c.Load(context.Background(), "k", fn)
	require.ErrorIs(t, err, fnErr)

	// 失败不缓存：再次调用仍执行 fn
	_, err = c.Load(context.Background(), "k", fn)
	require.ErrorIs(t, err, fnErr)
	require.Equal(t, 2, calls)
}

func TestCache_LoadSingleflight(t *testing.T) {
	c := New[int](time.Minute)
	var calls atomic.Int64
	fn := func(ctx context.Context) (int, error) {
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		return 9, nil
	}
	results := make(chan int, 8)
	for i := 0; i < 8; i++ {
		go func() {
			v, _ := c.Load(context.Background(), "k", fn)
			results <- v
		}()
	}
	for i := 0; i < 8; i++ {
		require.Equal(t, 9, <-results)
	}
	require.Equal(t, int64(1), calls.Load(), "并发 Load 同一 key 应只执行一次 fn")
}

func TestCache_Delete(t *testing.T) {
	c := New[int](time.Minute)
	c.set("a", 1)
	c.Delete("a")
	_, ok := c.Get("a")
	require.False(t, ok)

	// Delete 后再 Load 重新回填
	v, err := c.Load(context.Background(), "a", func(ctx context.Context) (int, error) { return 2, nil })
	require.NoError(t, err)
	require.Equal(t, 2, v)
}

func TestCache_DisabledTTL(t *testing.T) {
	c := New[int](0)
	calls := 0
	fn := func(ctx context.Context) (int, error) {
		calls++
		return 5, nil
	}
	v, err := c.Load(context.Background(), "k", fn)
	require.NoError(t, err)
	require.Equal(t, 5, v)
	v, err = c.Load(context.Background(), "k", fn)
	require.NoError(t, err)
	require.Equal(t, 5, v)
	require.Equal(t, 2, calls, "禁用缓存时每次 Load 都直接调 fn")
}
