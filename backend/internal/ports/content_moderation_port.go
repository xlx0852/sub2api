// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type ContentModerationRepository interface {
	CreateLog(ctx context.Context, log *domain.ContentModerationLog) error
	ListLogs(ctx context.Context, filter domain.ContentModerationLogFilter) ([]domain.ContentModerationLog, *domain.PaginationResult, error)
	SummarizeLogs(ctx context.Context, filter domain.ContentModerationLogFilter) (*domain.ContentModerationLogSummary, error)
	// CountFlaggedByUserSince 统计窗口内计入封号的违规次数（排除 hash_block；
	// excludeCyberPolicy 为 true 时额外排除 cyber_policy 行）。
	CountFlaggedByUserSince(ctx context.Context, userID int64, since time.Time, excludeCyberPolicy bool) (int, error)
	// ListLatestFlaggedLogsByUserIDs 返回每个用户最近一条 flagged 记录（含 cyber_policy）。
	ListLatestFlaggedLogsByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*domain.ContentModerationLog, error)
	CleanupExpiredLogs(ctx context.Context, hitBefore time.Time, nonHitBefore time.Time) (*domain.ContentModerationCleanupResult, error)
	// UpdateLogEmailSent 回写邮件发送结果（F7：CreateLog 先行后补 EmailSent）。
	UpdateLogEmailSent(ctx context.Context, id int64, sent bool) error
}

type ContentModerationHashCache interface {
	RecordFlaggedInputHash(ctx context.Context, inputHash string) error
	HasFlaggedInputHash(ctx context.Context, inputHash string) (bool, error)
	// 干净输入短缓存：命中则跳过 moderation API（fail-open）。
	RecordCleanInputHash(ctx context.Context, inputHash string, ttl time.Duration) error
	HasCleanInputHash(ctx context.Context, inputHash string) (bool, error)
	DeleteCleanInputHash(ctx context.Context, inputHash string) error
	// 高风险用户采样标记：true=强制全量审；false=短负缓存。
	SetElevatedUserSampling(ctx context.Context, userID int64, elevated bool, ttl time.Duration) error
	GetElevatedUserSampling(ctx context.Context, userID int64) (known bool, elevated bool, err error)
	ListElevatedUsers(ctx context.Context) ([]domain.ContentModerationElevatedUserCacheEntry, error)
	DeleteFlaggedInputHash(ctx context.Context, inputHash string) (bool, error)
	ClearFlaggedInputHashes(ctx context.Context) (int64, error)
	CountFlaggedInputHashes(ctx context.Context) (int64, error)
}
