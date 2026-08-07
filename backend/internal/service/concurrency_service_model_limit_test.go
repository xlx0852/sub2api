//go:build unit

package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// modelLimitCacheForTest 实现 ModelConcurrencyCache 的测试桩，嵌入账号槽位桩。
type modelLimitCacheForTest struct {
	stubConcurrencyCacheForTest
	modelAcquireResult bool
	modelAcquireErr    error
	modelReleaseErr    error

	modelAcquireCalls atomic.Int64
	modelReleaseCalls atomic.Int64
	releasedModelKeys []string
}

var _ ConcurrencyCache = (*modelLimitCacheForTest)(nil)
var _ ModelConcurrencyCache = (*modelLimitCacheForTest)(nil)

func (c *modelLimitCacheForTest) AcquireModelSlot(_ context.Context, _ string, _ int, _ string) (bool, error) {
	c.modelAcquireCalls.Add(1)
	return c.modelAcquireResult, c.modelAcquireErr
}

func (c *modelLimitCacheForTest) ReleaseModelSlot(_ context.Context, modelKey string, _ string) error {
	c.modelReleaseCalls.Add(1)
	c.releasedModelKeys = append(c.releasedModelKeys, modelKey)
	return c.modelReleaseErr
}

func TestAcquireModelAwareAccountSlot_NoBudget_Passthrough(t *testing.T) {
	// 未配置预算（provider 返回空 map）→ 透传账号槽位逻辑，ModelLimited 恒 false。
	cache := &modelLimitCacheForTest{}
	cache.acquireResult = true
	svc := NewConcurrencyService(cache)
	svc.SetModelConcurrencyLimitProvider(func(ctx context.Context) map[string]int { return nil })

	result, err := svc.AcquireModelAwareAccountSlot(context.Background(), "gpt-5.6-luna", 1, 4)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.False(t, result.ModelLimited)
	require.NotNil(t, result.ReleaseFunc)
	// 未配置预算时不应触碰 model 槽位
	require.Equal(t, int64(0), cache.modelAcquireCalls.Load())
}

func TestAcquireModelAwareAccountSlot_NoBudgetForModel_Passthrough(t *testing.T) {
	// 预算只配置了 gpt-5.6-luna，sol 请求不受影响（模型键折叠后不命中预算）。
	cache := &modelLimitCacheForTest{}
	cache.acquireResult = true
	svc := NewConcurrencyService(cache)
	svc.SetModelConcurrencyLimitProvider(func(ctx context.Context) map[string]int {
		return map[string]int{"gpt-5.6-luna": 8}
	})

	result, err := svc.AcquireModelAwareAccountSlot(context.Background(), "gpt-5.6-sol", 1, 4)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.False(t, result.ModelLimited)
	require.Equal(t, int64(0), cache.modelAcquireCalls.Load())
}

func TestAcquireModelAwareAccountSlot_NonCodexModel_Passthrough(t *testing.T) {
	// 有预算配置时，非 codex 模型请求（如 claude）也必须透传，不触碰 model 槽。
	cache := &modelLimitCacheForTest{}
	cache.acquireResult = true
	svc := NewConcurrencyService(cache)
	svc.SetModelConcurrencyLimitProvider(func(ctx context.Context) map[string]int {
		return map[string]int{"gpt-5.6-luna": 8}
	})

	result, err := svc.AcquireModelAwareAccountSlot(context.Background(), "claude-sonnet-4-5", 1, 4)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.False(t, result.ModelLimited)
	require.Equal(t, int64(0), cache.modelAcquireCalls.Load())
}

func TestAcquireModelAwareAccountSlot_ModelBudgetExceeded(t *testing.T) {
	// 模型预算已满（model 槽获取失败）→ ModelLimited=true，且不碰账号槽。
	cache := &modelLimitCacheForTest{}
	cache.acquireResult = true
	cache.modelAcquireResult = false // 预算满
	svc := NewConcurrencyService(cache)
	svc.SetModelConcurrencyLimitProvider(func(ctx context.Context) map[string]int {
		return map[string]int{"gpt-5.6-luna": 8}
	})

	result, err := svc.AcquireModelAwareAccountSlot(context.Background(), "gpt-5.6-luna-2026-08-01", 1, 4)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.True(t, result.ModelLimited)
	require.Nil(t, result.ReleaseFunc)
	require.Equal(t, int64(1), cache.modelAcquireCalls.Load())
	// 不应触碰账号槽位
	require.Len(t, cache.releasedAccountIDs, 0)
}

func TestAcquireModelAwareAccountSlot_ModelOK_AccountFull_ReleasesModelSlot(t *testing.T) {
	// model 槽抢到、账号槽满 → 反向释放 model 槽。
	cache := &modelLimitCacheForTest{}
	cache.acquireResult = false // 账号槽满
	cache.modelAcquireResult = true
	svc := NewConcurrencyService(cache)
	svc.SetModelConcurrencyLimitProvider(func(ctx context.Context) map[string]int {
		return map[string]int{"gpt-5.6-luna": 8}
	})

	result, err := svc.AcquireModelAwareAccountSlot(context.Background(), "gpt-5.6-luna", 1, 4)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.False(t, result.ModelLimited) // 是账号槽满，不是模型预算满
	require.Equal(t, int64(1), cache.modelAcquireCalls.Load())
	require.Equal(t, int64(1), cache.modelReleaseCalls.Load())
	require.Equal(t, []string{"gpt-5.6-luna"}, cache.releasedModelKeys)
}

func TestAcquireModelAwareAccountSlot_BothAcquired_CompositeRelease(t *testing.T) {
	// model + account 都抢到 → ReleaseFunc 组合释放两者。
	cache := &modelLimitCacheForTest{}
	cache.acquireResult = true
	cache.modelAcquireResult = true
	svc := NewConcurrencyService(cache)
	svc.SetModelConcurrencyLimitProvider(func(ctx context.Context) map[string]int {
		return map[string]int{"gpt-5.6-luna": 8}
	})

	result, err := svc.AcquireModelAwareAccountSlot(context.Background(), "gpt-5.6-luna", 1, 4)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.False(t, result.ModelLimited)
	require.NotNil(t, result.ReleaseFunc)
	require.Equal(t, int64(1), cache.modelAcquireCalls.Load())

	// 组合释放：账号槽 + model 槽都释放
	result.ReleaseFunc()
	require.Len(t, cache.releasedAccountIDs, 1)
	require.Equal(t, int64(1), cache.modelReleaseCalls.Load())
	require.Equal(t, []string{"gpt-5.6-luna"}, cache.releasedModelKeys)
}

func TestAcquireModelAwareAccountSlot_RedisUnsupported_FailOpen(t *testing.T) {
	// cache 未实现 ModelConcurrencyCache（只实现 ConcurrencyCache）→ fail-open 透传账号槽逻辑。
	cache := &stubConcurrencyCacheForTest{}
	cache.acquireResult = true
	svc := NewConcurrencyService(cache)
	svc.SetModelConcurrencyLimitProvider(func(ctx context.Context) map[string]int {
		return map[string]int{"gpt-5.6-luna": 8}
	})

	result, err := svc.AcquireModelAwareAccountSlot(context.Background(), "gpt-5.6-luna", 1, 4)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.False(t, result.ModelLimited)
}

func TestAcquireModelAwareAccountSlot_ModelAcquireError_FailOpen(t *testing.T) {
	// model 槽位 Redis 错误 → fail-open 继续走账号槽位，不因预算功能拒绝请求。
	cache := &modelLimitCacheForTest{}
	cache.acquireResult = true
	cache.modelAcquireErr = errors.New("redis down")
	svc := NewConcurrencyService(cache)
	svc.SetModelConcurrencyLimitProvider(func(ctx context.Context) map[string]int {
		return map[string]int{"gpt-5.6-luna": 8}
	})

	result, err := svc.AcquireModelAwareAccountSlot(context.Background(), "gpt-5.6-luna", 1, 4)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.False(t, result.ModelLimited)
	require.Equal(t, int64(1), cache.modelAcquireCalls.Load())
}
