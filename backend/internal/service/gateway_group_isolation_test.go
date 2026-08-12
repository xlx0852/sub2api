//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Part 1: isAccountInGroup 单元测试
// ============================================================================

func TestIsAccountInGroup(t *testing.T) {
	svc := &GatewayService{}
	groupID100 := int64(100)
	groupID200 := int64(200)

	tests := []struct {
		name     string
		account  *Account
		groupID  *int64
		expected bool
	}{
		// groupID == nil（无分组 API Key）
		{
			"nil_groupID_ungrouped_account_nil_groups",
			&Account{ID: 1, AccountGroups: nil},
			nil, true,
		},
		{
			"nil_groupID_ungrouped_account_empty_slice",
			&Account{ID: 2, AccountGroups: []AccountGroup{}},
			nil, true,
		},
		{
			"nil_groupID_grouped_account_single",
			&Account{ID: 3, AccountGroups: []AccountGroup{{GroupID: 100}}},
			nil, false,
		},
		{
			"nil_groupID_grouped_account_multiple",
			&Account{ID: 4, AccountGroups: []AccountGroup{{GroupID: 100}, {GroupID: 200}}},
			nil, false,
		},
		// groupID != nil（有分组 API Key）
		{
			"with_groupID_account_in_group",
			&Account{ID: 5, AccountGroups: []AccountGroup{{GroupID: 100}}},
			&groupID100, true,
		},
		{
			"with_groupID_account_not_in_group",
			&Account{ID: 6, AccountGroups: []AccountGroup{{GroupID: 200}}},
			&groupID100, false,
		},
		{
			"with_groupID_ungrouped_account",
			&Account{ID: 7, AccountGroups: nil},
			&groupID100, false,
		},
		{
			"with_groupID_multi_group_account_match_one",
			&Account{ID: 8, AccountGroups: []AccountGroup{{GroupID: 100}, {GroupID: 200}}},
			&groupID200, true,
		},
		{
			"with_groupID_multi_group_account_no_match",
			&Account{ID: 9, AccountGroups: []AccountGroup{{GroupID: 300}, {GroupID: 400}}},
			&groupID100, false,
		},
		// 防御性边界
		{
			"nil_account_nil_groupID",
			nil,
			nil, false,
		},
		{
			"nil_account_with_groupID",
			nil,
			&groupID100, false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.isAccountInGroup(tt.account, tt.groupID)
			require.Equal(t, tt.expected, got, "isAccountInGroup 结果不符预期")
		})
	}
}

// ============================================================================
// Part 2: 分组隔离端到端调度测试
// ============================================================================

// groupAwareMockAccountRepo 嵌入 mockAccountRepoForPlatform，覆写分组隔离相关方法。
// allAccounts 存储所有账号，分组查询方法按 AccountGroups 字段进行真实过滤。
type groupAwareMockAccountRepo struct {
	*mockAccountRepoForPlatform
	allAccounts []Account
}

// ListSchedulableUngroupedByPlatform 仅返回未分组账号（AccountGroups 为空）
func (m *groupAwareMockAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range m.allAccounts {
		if acc.Platform == platform && acc.IsSchedulable() && len(acc.AccountGroups) == 0 {
			result = append(result, acc)
		}
	}
	return result, nil
}

// ListSchedulableUngroupedByPlatforms 仅返回未分组账号（多平台版本）
func (m *groupAwareMockAccountRepo) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	platformSet := make(map[string]bool, len(platforms))
	for _, p := range platforms {
		platformSet[p] = true
	}
	var result []Account
	for _, acc := range m.allAccounts {
		if platformSet[acc.Platform] && acc.IsSchedulable() && len(acc.AccountGroups) == 0 {
			result = append(result, acc)
		}
	}
	return result, nil
}

// ListSchedulableByGroupIDAndPlatform 返回属于指定分组的账号
func (m *groupAwareMockAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range m.allAccounts {
		if acc.Platform == platform && acc.IsSchedulable() && accountBelongsToGroup(acc, groupID) {
			result = append(result, acc)
		}
	}
	return result, nil
}

// ListSchedulableByGroupIDAndPlatforms 返回属于指定分组的账号（多平台版本）
func (m *groupAwareMockAccountRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	platformSet := make(map[string]bool, len(platforms))
	for _, p := range platforms {
		platformSet[p] = true
	}
	var result []Account
	for _, acc := range m.allAccounts {
		if platformSet[acc.Platform] && acc.IsSchedulable() && accountBelongsToGroup(acc, groupID) {
			result = append(result, acc)
		}
	}
	return result, nil
}

// accountBelongsToGroup 检查账号是否属于指定分组
func accountBelongsToGroup(acc Account, groupID int64) bool {
	for _, ag := range acc.AccountGroups {
		if ag.GroupID == groupID {
			return true
		}
	}
	return false
}

// Verify interface implementation
var _ AccountRepository = (*groupAwareMockAccountRepo)(nil)

// newGroupAwareMockRepo 创建分组感知的 mock repo
func newGroupAwareMockRepo(accounts []Account) *groupAwareMockAccountRepo {
	byID := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		byID[accounts[i].ID] = &accounts[i]
	}
	return &groupAwareMockAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accounts:     accounts,
			accountsByID: byID,
		},
		allAccounts: accounts,
	}
}

func TestGroupIsolation_UngroupedKey_ShouldNotScheduleGroupedAccounts(t *testing.T) {
	// 场景：无分组 API Key（groupID=nil），池中只有已分组账号 → 应返回错误
	ctx := context.Background()

	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Priority: 1, Status: StatusActive, Schedulable: true,
			AccountGroups: []AccountGroup{{GroupID: 100}}},
		{ID: 2, Platform: PlatformOpenAI, Priority: 2, Status: StatusActive, Schedulable: true,
			AccountGroups: []AccountGroup{{GroupID: 200}}},
	}
	repo := newGroupAwareMockRepo(accounts)
	cache := &mockGatewayCacheForPlatform{}

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
	}

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "", nil, PlatformOpenAI)
	require.Error(t, err, "无分组 Key 不应调度到已分组账号")
	require.Nil(t, acc)
}

func TestGroupIsolation_GroupedKey_ShouldNotScheduleUngroupedAccounts(t *testing.T) {
	// 场景：有分组 API Key（groupID=100），池中只有未分组账号 → 应返回错误
	ctx := context.Background()
	groupID := int64(100)

	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Priority: 1, Status: StatusActive, Schedulable: true,
			AccountGroups: nil},
		{ID: 2, Platform: PlatformOpenAI, Priority: 2, Status: StatusActive, Schedulable: true,
			AccountGroups: []AccountGroup{}},
	}
	repo := newGroupAwareMockRepo(accounts)
	cache := &mockGatewayCacheForPlatform{}

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
	}

	acc, err := svc.selectAccountForModelWithPlatform(ctx, &groupID, "", "", nil, PlatformOpenAI)
	require.Error(t, err, "有分组 Key 不应调度到未分组账号")
	require.Nil(t, acc)
}

func TestGroupIsolation_UngroupedKey_ShouldOnlyScheduleUngroupedAccounts(t *testing.T) {
	// 场景：无分组 API Key（groupID=nil），池中有未分组和已分组账号 → 应只选中未分组的
	ctx := context.Background()

	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Priority: 1, Status: StatusActive, Schedulable: true,
			AccountGroups: []AccountGroup{{GroupID: 100}}}, // 已分组，不应被选中
		{ID: 2, Platform: PlatformOpenAI, Priority: 2, Status: StatusActive, Schedulable: true,
			AccountGroups: nil}, // 未分组，应被选中
		{ID: 3, Platform: PlatformOpenAI, Priority: 3, Status: StatusActive, Schedulable: true,
			AccountGroups: []AccountGroup{{GroupID: 200}}}, // 已分组，不应被选中
	}
	repo := newGroupAwareMockRepo(accounts)
	cache := &mockGatewayCacheForPlatform{}

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
	}

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "", nil, PlatformOpenAI)
	require.NoError(t, err, "应成功调度未分组账号")
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID, "应选中未分组的账号 ID=2")
}

func TestGroupIsolation_GroupedKey_ShouldOnlyScheduleMatchingGroupAccounts(t *testing.T) {
	// 场景：有分组 API Key（groupID=100），池中有未分组和多个分组账号 → 应只选中分组 100 内的
	ctx := context.Background()
	groupID := int64(100)

	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Priority: 1, Status: StatusActive, Schedulable: true,
			AccountGroups: nil}, // 未分组，不应被选中
		{ID: 2, Platform: PlatformOpenAI, Priority: 2, Status: StatusActive, Schedulable: true,
			AccountGroups: []AccountGroup{{GroupID: 200}}}, // 属于分组 200，不应被选中
		{ID: 3, Platform: PlatformOpenAI, Priority: 3, Status: StatusActive, Schedulable: true,
			AccountGroups: []AccountGroup{{GroupID: 100}}}, // 属于分组 100，应被选中
	}
	repo := newGroupAwareMockRepo(accounts)
	cache := &mockGatewayCacheForPlatform{}

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
	}

	acc, err := svc.selectAccountForModelWithPlatform(ctx, &groupID, "", "", nil, PlatformOpenAI)
	require.NoError(t, err, "应成功调度分组内账号")
	require.NotNil(t, acc)
	require.Equal(t, int64(3), acc.ID, "应选中分组 100 内的账号 ID=3")
}
