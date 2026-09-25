//go:build unit

package service

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// mockAccountRepoForGemini Gemini 测试用的 mock
type mockAccountRepoForGemini struct {
	accounts           []Account
	accountsByID       map[int64]*Account
	listByGroupFunc    func(ctx context.Context, groupID int64, platforms []string) ([]Account, error)
	listByPlatformFunc func(ctx context.Context, platforms []string) ([]Account, error)
}

func (m *mockAccountRepoForGemini) Restore(ctx context.Context, id int64) error {
	return nil
}

func (m *mockAccountRepoForGemini) GetByID(ctx context.Context, id int64) (*Account, error) {
	if acc, ok := m.accountsByID[id]; ok {
		return acc, nil
	}
	return nil, errors.New("account not found")
}

func (m *mockAccountRepoForGemini) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	var result []*Account
	for _, id := range ids {
		if acc, ok := m.accountsByID[id]; ok {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (m *mockAccountRepoForGemini) ExistsByID(ctx context.Context, id int64) (bool, error) {
	if m.accountsByID == nil {
		return false, nil
	}
	_, ok := m.accountsByID[id]
	return ok, nil
}

func (m *mockAccountRepoForGemini) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range m.accounts {
		if acc.Platform == platform && acc.IsSchedulable() {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (m *mockAccountRepoForGemini) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	// 测试时不区分 groupID，直接按 platform 过滤
	return m.ListSchedulableByPlatform(ctx, platform)
}

// Stub methods to implement AccountRepository interface
func (m *mockAccountRepoForGemini) Create(ctx context.Context, account *Account) error { return nil }
func (m *mockAccountRepoForGemini) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*Account, error) {
	return nil, nil
}

func (m *mockAccountRepoForGemini) FindByExtraField(ctx context.Context, key string, value any) ([]Account, error) {
	return nil, nil
}

func (m *mockAccountRepoForGemini) FindOAuthByPlatformEmail(ctx context.Context, platform, email string, includeDeleted bool) ([]Account, error) {
	return nil, nil
}

func (m *mockAccountRepoForGemini) ListOAuthIncludingDeleted(ctx context.Context) ([]Account, error) {
	return nil, nil
}

func (m *mockAccountRepoForGemini) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) Update(ctx context.Context, account *Account) error { return nil }
func (m *mockAccountRepoForGemini) Delete(ctx context.Context, id int64) error         { return nil }
func (m *mockAccountRepoForGemini) HardDelete(ctx context.Context, id int64) error     { return nil }
func (m *mockAccountRepoForGemini) ReassignAccountReferences(ctx context.Context, fromID, toID int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockAccountRepoForGemini) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockAccountRepoForGemini) ListAllWithFilters(ctx context.Context, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListActive(ctx context.Context) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) UpdateLastUsed(ctx context.Context, id int64) error { return nil }
func (m *mockAccountRepoForGemini) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetError(ctx context.Context, id int64, errorMsg string) error {
	return nil
}
func (m *mockAccountRepoForGemini) ClearError(ctx context.Context, id int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetExpiresAt(ctx context.Context, id int64, expiresAt *time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
}
func (m *mockAccountRepoForGemini) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) ListSchedulable(ctx context.Context) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	if m.listByPlatformFunc != nil {
		return m.listByPlatformFunc(ctx, platforms)
	}
	var result []Account
	platformSet := make(map[string]bool)
	for _, p := range platforms {
		platformSet[p] = true
	}
	for _, acc := range m.accounts {
		if platformSet[acc.Platform] && acc.IsSchedulable() {
			result = append(result, acc)
		}
	}
	return result, nil
}
func (m *mockAccountRepoForGemini) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	if m.listByGroupFunc != nil {
		return m.listByGroupFunc(ctx, groupID, platforms)
	}
	return m.ListSchedulableByPlatforms(ctx, platforms)
}
func (m *mockAccountRepoForGemini) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return m.ListSchedulableByPlatform(ctx, platform)
}
func (m *mockAccountRepoForGemini) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	return m.ListSchedulableByPlatforms(ctx, platforms)
}
func (m *mockAccountRepoForGemini) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return nil
}
func (m *mockAccountRepoForGemini) ClearTempUnschedulable(ctx context.Context, id int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) ClearRateLimit(ctx context.Context, id int64) error { return nil }
func (m *mockAccountRepoForGemini) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) ClearModelRateLimits(ctx context.Context, id int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	return nil
}
func (m *mockAccountRepoForGemini) UpdateSessionWindowEnd(ctx context.Context, id int64, end time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	return nil
}
func (m *mockAccountRepoForGemini) BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	return 0, nil
}

func (m *mockAccountRepoForGemini) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	return nil
}

func (m *mockAccountRepoForGemini) ResetQuotaUsed(ctx context.Context, id int64) error {
	return nil
}

func (m *mockAccountRepoForGemini) RevertProxyFallback(ctx context.Context, accountID int64) error {
	return nil
}

func (m *mockAccountRepoForGemini) ListShadowsByParent(ctx context.Context, parentID int64) ([]*Account, error) {
	return nil, nil
}

// Verify interface implementation
var _ AccountRepository = (*mockAccountRepoForGemini)(nil)

// mockGroupRepoForGemini Gemini 测试用的 group repo mock
type mockGroupRepoForGemini struct {
	groups           map[int64]*Group
	getByIDCalls     int
	getByIDLiteCalls int
}

func (m *mockGroupRepoForGemini) GetByID(ctx context.Context, id int64) (*Group, error) {
	m.getByIDCalls++
	if g, ok := m.groups[id]; ok {
		return g, nil
	}
	return nil, errors.New("group not found")
}

func (m *mockGroupRepoForGemini) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	m.getByIDLiteCalls++
	if g, ok := m.groups[id]; ok {
		return g, nil
	}
	return nil, errors.New("group not found")
}

// Stub methods to implement GroupRepository interface
func (m *mockGroupRepoForGemini) Create(ctx context.Context, group *Group) error { return nil }
func (m *mockGroupRepoForGemini) Update(ctx context.Context, group *Group) error { return nil }
func (m *mockGroupRepoForGemini) Delete(ctx context.Context, id int64) error     { return nil }
func (m *mockGroupRepoForGemini) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, nil
}
func (m *mockGroupRepoForGemini) List(ctx context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockGroupRepoForGemini) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockGroupRepoForGemini) ListActive(ctx context.Context) ([]Group, error) { return nil, nil }
func (m *mockGroupRepoForGemini) ListActiveByPlatform(ctx context.Context, platform string) ([]Group, error) {
	return nil, nil
}
func (m *mockGroupRepoForGemini) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, nil
}
func (m *mockGroupRepoForGemini) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	return 0, 0, nil
}
func (m *mockGroupRepoForGemini) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
}

func (m *mockGroupRepoForGemini) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return nil
}

func (m *mockGroupRepoForGemini) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, nil
}

func (m *mockGroupRepoForGemini) UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	return nil
}

var _ GroupRepository = (*mockGroupRepoForGemini)(nil)

// mockGatewayCacheForGemini Gemini 测试用的 cache mock
type mockGatewayCacheForGemini struct {
	sessionBindings map[string]int64
	deletedSessions map[string]int
}

func (m *mockGatewayCacheForGemini) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	if id, ok := m.sessionBindings[sessionHash]; ok {
		return id, nil
	}
	return 0, errors.New("not found")
}

func (m *mockGatewayCacheForGemini) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if m.sessionBindings == nil {
		m.sessionBindings = make(map[string]int64)
	}
	m.sessionBindings[sessionHash] = accountID
	return nil
}

func (m *mockGatewayCacheForGemini) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	return nil
}

func (m *mockGatewayCacheForGemini) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	if m.sessionBindings == nil {
		return nil
	}
	if m.deletedSessions == nil {
		m.deletedSessions = make(map[string]int)
	}
	m.deletedSessions[sessionHash]++
	delete(m.sessionBindings, sessionHash)
	return nil
}
