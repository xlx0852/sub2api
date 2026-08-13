// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ProfitRepository 利润分析数据访问端口。
type ProfitRepository interface {
	// GetCurrentUserBalanceTotal 返回当前未删除普通用户持有的总余额（可用余额 + 冻结余额）。
	GetCurrentUserBalanceTotal(ctx context.Context) (float64, error)
	UpsertCostConfig(ctx context.Context, cfg *domain.AccountCostConfig) (*domain.AccountCostConfig, error)
	GetCostConfig(ctx context.Context, accountID int64) (*domain.AccountCostConfig, error)
	ListCostConfigs(ctx context.Context) ([]*domain.AccountCostConfig, error)
	InsertCostConfigsIfAbsent(ctx context.Context, configs []*domain.AccountCostConfig) ([]int64, error)
	DeleteCostConfig(ctx context.Context, accountID int64) error
	ListAutoRenewSubscriptionAccounts(ctx context.Context) ([]*domain.AccountCostConfig, error)
	HasSubscriptionCycleStartingAt(ctx context.Context, accountID int64, startsAt time.Time) (bool, error)
	ListSubscriptionCycles(ctx context.Context, accountID int64) ([]*domain.AccountSubscriptionCycle, error)
	ListSubscriptionCyclesBatch(ctx context.Context, accountIDs []int64) ([]*domain.AccountSubscriptionCycle, error)
	GetSubscriptionCycle(ctx context.Context, id int64) (*domain.AccountSubscriptionCycle, error)
	CreateSubscriptionCycle(ctx context.Context, cycle *domain.AccountSubscriptionCycle) (*domain.AccountSubscriptionCycle, error)
	DeleteSubscriptionCycle(ctx context.Context, id int64) error
	CreateSubscriptionTermination(ctx context.Context, termination *domain.AccountSubscriptionTermination, initialRefund *domain.AccountSubscriptionRefund) (*domain.SubscriptionTerminationWriteResult, error)
	CreateSubscriptionRefund(ctx context.Context, refund *domain.AccountSubscriptionRefund) (*domain.AccountSubscriptionRefund, error)
	VoidSubscriptionRefund(ctx context.Context, id int64, reason string, voidedAt time.Time) (*domain.AccountSubscriptionRefund, error)
	ReverseSubscriptionTermination(ctx context.Context, id int64, reason string, reversedAt time.Time) (*domain.AccountSubscriptionTermination, error)
	GetSubscriptionCycleRevenueBatch(ctx context.Context, ranges []domain.SubscriptionCycleUsageRange) (map[int64]float64, error)

	// GetAccountUsageStatsBatch 批量聚合账号在 [start,end) 的收入与账号侧成本。
	GetAccountUsageStatsBatch(ctx context.Context, accountIDs []int64, start, end time.Time) (map[int64]*domain.ProfitUsageStats, error)
	// GetAccountUsageStatsForRanges 批量聚合每个账号独立时间范围内的收入与成本。
	GetAccountUsageStatsForRanges(ctx context.Context, ranges []domain.ProfitAccountUsageRange) (map[int64]*domain.ProfitUsageStats, error)
	// GetAccountDailyUsageStats 一次聚合账号汇总与每日趋势所需的基础数据。
	GetAccountDailyUsageStats(ctx context.Context, accountIDs []int64, start, end time.Time, tzName string) ([]*domain.ProfitAccountDailyUsagePoint, error)
	// GetDailyUsageStats 按日聚合收入与 API Key 按量成本；accountID 为 nil 表示全部账号。
	// OAuth/Setup Token 等订阅账号只贡献收入，成本由服务层的周期账本补充。
	GetDailyUsageStats(ctx context.Context, accountID *int64, start, end time.Time, tzName string) ([]*domain.ProfitDailyUsagePoint, error)
	// GetBestWindowRevenue 返回账号自 since 以来最佳固定长度窗口的收入（用于自动学习窗口基准）。
	GetBestWindowRevenue(ctx context.Context, accountID int64, since time.Time, windowSeconds int64) (float64, error)
	GetBestWindowRevenueBatch(ctx context.Context, accountIDs []int64, since time.Time, windowSeconds int64) (map[int64]float64, error)
}
