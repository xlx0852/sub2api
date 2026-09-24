package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 本文件覆盖"空池 → 强制 DB 复核 → 重试一次"的编排（gateway_empty_pool_recheck.go）。
//
// 关键约束（改动前请先读）：
//   - 这里只用最薄的假实现：假 SchedulerCache + 假 AccountRepository，不触网、不碰 Redis；
//   - 断言重点不是"能不能选中账号"，而是"什么时候会额外读一次 DB"——这决定生产上会不会放大 DB 压力。

// gatewayEmptyPoolFakeCache 只实现快照读写；其余 SchedulerCache 方法继承嵌入接口（被调用即 panic）。
type gatewayEmptyPoolFakeCache struct {
	SchedulerCache

	snapshot []Account
	hit      bool
	setCalls int
}

func (c *gatewayEmptyPoolFakeCache) GetSnapshot(context.Context, SchedulerBucket) ([]*Account, bool, error) {
	if !c.hit {
		return nil, false, nil
	}
	out := make([]*Account, 0, len(c.snapshot))
	for i := range c.snapshot {
		out = append(out, &c.snapshot[i])
	}
	return out, true, nil
}

func (c *gatewayEmptyPoolFakeCache) SetSnapshot(_ context.Context, _ SchedulerBucket, accounts []Account) error {
	c.setCalls++
	c.snapshot = accounts
	c.hit = true
	return nil
}

// gatewayEmptyPoolFakeRepo 统计"回 DB 列表"的次数，用于证明节流/跳过条件生效。
type gatewayEmptyPoolFakeRepo struct {
	AccountRepository

	accounts  []Account
	listCalls int
}

func (r *gatewayEmptyPoolFakeRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]Account, error) {
	r.listCalls++
	return r.accounts, nil
}

func (r *gatewayEmptyPoolFakeRepo) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]Account, error) {
	r.listCalls++
	return r.accounts, nil
}

func (r *gatewayEmptyPoolFakeRepo) ListSchedulableUngroupedByPlatform(context.Context, string) ([]Account, error) {
	r.listCalls++
	return r.accounts, nil
}

func (r *gatewayEmptyPoolFakeRepo) ListSchedulableUngroupedByPlatforms(context.Context, []string) ([]Account, error) {
	r.listCalls++
	return r.accounts, nil
}

func newGatewayEmptyPoolTestService(repo *gatewayEmptyPoolFakeRepo, cache *gatewayEmptyPoolFakeCache) *GatewayService {
	return &GatewayService{
		accountRepo: repo,
		schedulerSnapshot: &SchedulerSnapshotService{
			cache:         cache,
			accountRepo:   repo,
			refreshLastAt: make(map[string]time.Time),
		},
	}
}

func emptyPoolStats() gatewaySelectionPoolStats {
	return gatewaySelectionPoolStats{Platform: PlatformAnthropic, PoolSize: 2, CandidateCount: 0, EmptyPool: true}
}

func TestSelectGatewayAccountWithEmptyPoolRecheckRefreshesAndRetries(t *testing.T) {
	repo := &gatewayEmptyPoolFakeRepo{accounts: []Account{{ID: 140}}}
	cache := &gatewayEmptyPoolFakeCache{}
	svc := newGatewayEmptyPoolTestService(repo, cache)
	groupID := int64(34)

	attempts := 0
	result, err := svc.selectGatewayAccountWithEmptyPoolRecheck(context.Background(), &groupID, "deepseek-flash", nil,
		func() (*AccountSelectionResult, gatewaySelectionPoolStats, error) {
			attempts++
			if attempts == 1 {
				return nil, emptyPoolStats(), ErrNoAvailableAccounts
			}
			// 复核后 DB 视角多出账号 140
			return &AccountSelectionResult{Account: &Account{ID: 140}},
				gatewaySelectionPoolStats{Platform: PlatformAnthropic, PoolSize: 3, CandidateCount: 1}, nil
		})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(140), result.Account.ID)
	require.Equal(t, 2, attempts, "空池必须重试一次选号")
	require.Equal(t, 1, repo.listCalls, "只允许一次 DB 回读")
	require.Equal(t, 1, cache.setCalls, "复核成功应写回快照")
}

func TestSelectGatewayAccountWithEmptyPoolRecheckLogsUnrecovered(t *testing.T) {
	repo := &gatewayEmptyPoolFakeRepo{accounts: []Account{{ID: 81}, {ID: 139}}}
	cache := &gatewayEmptyPoolFakeCache{}
	svc := newGatewayEmptyPoolTestService(repo, cache)
	groupID := int64(34)

	attempts := 0
	result, err := svc.selectGatewayAccountWithEmptyPoolRecheck(context.Background(), &groupID, "deepseek-flash", nil,
		func() (*AccountSelectionResult, gatewaySelectionPoolStats, error) {
			attempts++
			return nil, emptyPoolStats(), ErrNoAvailableAccounts
		})

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrNoAvailableAccounts, "复核后仍失败必须保留原错误分类")
	require.Equal(t, 2, attempts)
	require.Equal(t, 1, repo.listCalls)
}

func TestSelectGatewayAccountWithEmptyPoolRecheckSkipsWhenAccountsExcluded(t *testing.T) {
	repo := &gatewayEmptyPoolFakeRepo{accounts: []Account{{ID: 140}}}
	cache := &gatewayEmptyPoolFakeCache{}
	svc := newGatewayEmptyPoolTestService(repo, cache)
	groupID := int64(34)

	attempts := 0
	_, err := svc.selectGatewayAccountWithEmptyPoolRecheck(context.Background(), &groupID, "deepseek-flash",
		map[int64]struct{}{140: {}},
		func() (*AccountSelectionResult, gatewaySelectionPoolStats, error) {
			attempts++
			return nil, emptyPoolStats(), ErrNoAvailableAccounts
		})

	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Equal(t, 1, attempts, "failover 排除账号时不得重试")
	require.Equal(t, 0, repo.listCalls, "failover 排除账号时不得回读 DB")
}

func TestSelectGatewayAccountWithEmptyPoolRecheckSkipsBusyPool(t *testing.T) {
	repo := &gatewayEmptyPoolFakeRepo{accounts: []Account{{ID: 140}}}
	cache := &gatewayEmptyPoolFakeCache{}
	svc := newGatewayEmptyPoolTestService(repo, cache)
	groupID := int64(34)

	attempts := 0
	_, err := svc.selectGatewayAccountWithEmptyPoolRecheck(context.Background(), &groupID, "deepseek-flash", nil,
		func() (*AccountSelectionResult, gatewaySelectionPoolStats, error) {
			attempts++
			// 候选存在但没有槽位/等待计划：EmptyPool=false
			return nil, gatewaySelectionPoolStats{Platform: PlatformAnthropic, PoolSize: 3, CandidateCount: 3}, ErrNoAvailableAccounts
		})

	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Equal(t, 1, attempts, "忙池不得重试")
	require.Equal(t, 0, repo.listCalls, "忙池不得回读 DB")
}

func TestSelectGatewayAccountWithEmptyPoolRecheckLeavesOtherErrorsUntouched(t *testing.T) {
	repo := &gatewayEmptyPoolFakeRepo{}
	cache := &gatewayEmptyPoolFakeCache{}
	svc := newGatewayEmptyPoolTestService(repo, cache)
	groupID := int64(34)

	sentinel := errors.New("upstream contract broken")
	attempts := 0
	_, err := svc.selectGatewayAccountWithEmptyPoolRecheck(context.Background(), &groupID, "deepseek-flash", nil,
		func() (*AccountSelectionResult, gatewaySelectionPoolStats, error) {
			attempts++
			return nil, emptyPoolStats(), sentinel
		})

	require.ErrorIs(t, err, sentinel)
	require.Equal(t, 1, attempts)
	require.Equal(t, 0, repo.listCalls)
}

func TestRecheckEmptyGatewayPoolThrottlesRepeatedRefreshes(t *testing.T) {
	repo := &gatewayEmptyPoolFakeRepo{accounts: []Account{{ID: 140}}}
	cache := &gatewayEmptyPoolFakeCache{}
	svc := newGatewayEmptyPoolTestService(repo, cache)
	groupID := int64(34)

	require.True(t, svc.recheckEmptyGatewayPool(context.Background(), &groupID, emptyPoolStats(), "deepseek-flash"))
	require.False(t, svc.recheckEmptyGatewayPool(context.Background(), &groupID, emptyPoolStats(), "deepseek-flash"),
		"节流窗口内必须静默放弃复核")
	require.Equal(t, 1, repo.listCalls, "节流窗口内不得再次回读 DB")
}

func TestRecheckEmptyGatewayPoolWithoutSnapshotService(t *testing.T) {
	svc := &GatewayService{}
	groupID := int64(34)
	require.False(t, svc.recheckEmptyGatewayPool(context.Background(), &groupID, emptyPoolStats(), "deepseek-flash"))
}

func TestSelectGatewayAccountWithEmptyPoolRecheckWithoutSnapshotService(t *testing.T) {
	svc := &GatewayService{}
	groupID := int64(34)

	attempts := 0
	_, err := svc.selectGatewayAccountWithEmptyPoolRecheck(context.Background(), &groupID, "deepseek-flash", nil,
		func() (*AccountSelectionResult, gatewaySelectionPoolStats, error) {
			attempts++
			return nil, emptyPoolStats(), ErrNoAvailableAccounts
		})

	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Equal(t, 1, attempts, "没有快照服务时保持既有行为")
}
