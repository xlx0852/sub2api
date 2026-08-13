// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// UserPlatformQuotaRepository 定义 service 层所需的 user × platform quota 数据访问端口。
// repository 包的 userPlatformQuotaRepository 实现此接口。
type UserPlatformQuotaRepository interface {
	// GetByUserPlatform 查询单条配额记录，未找到时返回 (nil, nil)。
	GetByUserPlatform(ctx context.Context, userID int64, platform string) (*domain.UserPlatformQuotaRecord, error)
	// BulkInsertInitial 幂等批量插入初始配额记录（ON CONFLICT DO NOTHING）。
	BulkInsertInitial(ctx context.Context, records []domain.UserPlatformQuotaRecord) error
	// IncrementUsageWithReset 原子地累加用量，若窗口已过期则先重置再累加。
	IncrementUsageWithReset(ctx context.Context, userID int64, platform string, cost float64, now time.Time) error
	// ListByUser 查询用户的所有平台配额记录。
	ListByUser(ctx context.Context, userID int64) ([]domain.UserPlatformQuotaRecord, error)
	// UpsertForUser 全量替换该用户所有平台限额配置（事务内）：
	//   1. 软删除未在 records 中出现的所有 active 行
	//   2. 对 records 中每条：UPDATE 已存在的（含重新激活软删行）；UPDATE 未命中时 INSERT
	//      仅改 *_limit_usd + deleted_at + updated_at，保留 *_usage_usd / *_window_start。
	// records 为空时仅执行步骤 1。
	UpsertForUser(ctx context.Context, userID int64, records []domain.UserPlatformQuotaRecord) error
	// ResetExpiredWindow 重置指定窗口（"daily"|"weekly"|"monthly"）的用量与起始时间。
	// 未命中活跃记录时返回（service-side wrapper of repository.ErrUserPlatformQuotaNotFound）。
	ResetExpiredWindow(ctx context.Context, userID int64, platform string, window string, newStart time.Time) error
	// BatchSnapshotUsage 绝对值覆盖写入整批 usage 快照。FK 违反返回 ErrUserPlatformQuotaFKViolation。
	BatchSnapshotUsage(ctx context.Context, snapshots []domain.UserPlatformQuotaSnapshot, now time.Time) error
}
