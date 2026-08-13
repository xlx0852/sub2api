// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type AccountRepository interface {
	Create(ctx context.Context, account *domain.Account) error
	GetByID(ctx context.Context, id int64) (*domain.Account, error)
	// GetByIDs fetches accounts by IDs in a single query.
	// It should return all accounts found (missing IDs are ignored).
	GetByIDs(ctx context.Context, ids []int64) ([]*domain.Account, error)
	// ExistsByID 检查账号是否存在，仅返回布尔值，用于删除前的轻量级存在性检查
	ExistsByID(ctx context.Context, id int64) (bool, error)
	// GetByCRSAccountID finds an account previously synced from CRS.
	// Returns (nil, nil) if not found.
	GetByCRSAccountID(ctx context.Context, crsAccountID string) (*domain.Account, error)
	// FindByExtraField 根据 extra 字段中的键值对查找账号
	FindByExtraField(ctx context.Context, key string, value any) ([]domain.Account, error)
	// FindOAuthByPlatformEmail 按平台 + 邮箱查找 OAuth/setup-token 账号。
	// includeDeleted=true 时包含软删除记录，用于同邮箱合并时恢复最近登录账号。
	FindOAuthByPlatformEmail(ctx context.Context, platform, email string, includeDeleted bool) ([]domain.Account, error)
	// ListOAuthIncludingDeleted 返回全部 OAuth/setup-token 母账号（含软删除、error/封禁）。
	ListOAuthIncludingDeleted(ctx context.Context) ([]domain.Account, error)
	// ListCRSAccountIDs returns a map of crs_account_id -> local account ID
	// for all accounts that have been synced from CRS.
	ListCRSAccountIDs(ctx context.Context) (map[string]int64, error)
	Update(ctx context.Context, account *domain.Account) error
	Delete(ctx context.Context, id int64) error
	// HardDelete 真正删除账号（含已软删除），用于回收站去重清理。
	// 调用前应先 ReassignAccountReferences，避免 usage 等外键数据丢失。
	HardDelete(ctx context.Context, id int64) error
	// ReassignAccountReferences 将 fromID 上的业务引用迁移到 toID（usage/cost/logs 等）。
	ReassignAccountReferences(ctx context.Context, fromID, toID int64) error
	// Restore 恢复软删除账号，并清除 error/封禁运行态。
	Restore(ctx context.Context, id int64) error

	List(ctx context.Context, params domain.PaginationParams) ([]domain.Account, *domain.PaginationResult, error)
	ListWithFilters(ctx context.Context, params domain.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]domain.Account, *domain.PaginationResult, error)
	// ListAllWithFilters 返回符合过滤条件的全部账号（不分页），用于账号列表页
	// 计算 OpenAI 调度分数的过滤范围池。
	ListAllWithFilters(ctx context.Context, platform, accountType, status, search string, groupID int64, privacyMode string) ([]domain.Account, error)
	ListByGroup(ctx context.Context, groupID int64) ([]domain.Account, error)
	ListActive(ctx context.Context) ([]domain.Account, error)
	ListOAuthRefreshCandidates(ctx context.Context) ([]domain.Account, error)
	ListByPlatform(ctx context.Context, platform string) ([]domain.Account, error)

	UpdateLastUsed(ctx context.Context, id int64) error
	BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error
	SetError(ctx context.Context, id int64, errorMsg string) error
	ClearError(ctx context.Context, id int64) error
	SetSchedulable(ctx context.Context, id int64, schedulable bool) error
	// SetExpiresAt 更新账号调度过期时间；expiresAt 为 nil 时清空。
	SetExpiresAt(ctx context.Context, id int64, expiresAt *time.Time) error
	AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error)
	BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error

	ListSchedulable(ctx context.Context) ([]domain.Account, error)
	ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]domain.Account, error)
	ListSchedulableByPlatform(ctx context.Context, platform string) ([]domain.Account, error)
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]domain.Account, error)
	ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]domain.Account, error)
	ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]domain.Account, error)
	ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]domain.Account, error)
	ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]domain.Account, error)

	SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error
	SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error
	SetOverloaded(ctx context.Context, id int64, until time.Time) error
	SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error
	ClearTempUnschedulable(ctx context.Context, id int64) error
	ClearRateLimit(ctx context.Context, id int64) error
	ClearAntigravityQuotaScopes(ctx context.Context, id int64) error
	ClearModelRateLimits(ctx context.Context, id int64) error
	UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error
	// UpdateSessionWindowEnd 仅更新 5h 窗口的结束时间，不动 start / status。
	// 用于 active poll 拿到新 ResetsAt 后回写，避免覆盖请求路径上记录的 status。
	UpdateSessionWindowEnd(ctx context.Context, id int64, end time.Time) error
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
	BulkUpdate(ctx context.Context, ids []int64, updates domain.AccountBulkUpdate) (int64, error)
	// IncrementQuotaUsed 原子递增 API Key 账号的配额用量（总/日/周）
	IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error
	// ResetQuotaUsed 重置 API Key 账号所有维度的配额用量为 0
	ResetQuotaUsed(ctx context.Context, id int64) error
	// RevertProxyFallback 将账号的 proxy_id 切回 proxy_fallback_origin_id，并清空 origin 字段。
	// 仅当 proxy_fallback_origin_id IS NOT NULL 时更新，否则视为账号不存在（返回 ErrAccountNotFound）。
	RevertProxyFallback(ctx context.Context, accountID int64) error
	// ListShadowsByParent 返回指定父账号的影子账号；当前实现仅查 quota_dimension='spark'（唯一预设）。
	// ⚠️ 新增影子维度时：须更新此函数（或新增维度专用列举），并检查所有调用点（级联删除/一母一影校验/type 守卫），否则会静默漏掉新维度。
	ListShadowsByParent(ctx context.Context, parentID int64) ([]*domain.Account, error)
}
