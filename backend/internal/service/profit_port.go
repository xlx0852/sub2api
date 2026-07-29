package service

import (
	"context"
	"time"
)

// AccountCostConfig 账号成本配置：绑定账号的成本模型，用于利润分析。
//
// 成本口径由账号认证类型决定：OAuth / Setup Token 为订阅制，API Key 为按量付费。
type AccountCostConfig struct {
	ID        int64   `json:"id"`
	AccountID int64   `json:"account_id"`
	CostType  string  `json:"cost_type"`
	PeriodFee float64 `json:"period_fee"`
	// PeriodDays 计费周期长度（天）
	PeriodDays int    `json:"period_days"`
	Currency   string `json:"currency"`
	// WindowBaselineRevenue 5h 窗口满载理论产出基准（美元）。
	// nil 表示按历史最佳窗口收入自动学习。
	WindowBaselineRevenue *float64  `json:"window_baseline_revenue"`
	Notes                 string    `json:"notes"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// AccountSubscriptionCycle 表示一次实际充值形成的独立订阅周期。
// 停用空档不应通过推断的连续续费填补。
type AccountSubscriptionCycle struct {
	ID         int64     `json:"id"`
	AccountID  int64     `json:"account_id"`
	StartsAt   time.Time `json:"starts_at"`
	PeriodFee  float64   `json:"period_fee"`
	PeriodDays int       `json:"period_days"`
	Currency   string    `json:"currency"`
	Notes      string    `json:"notes"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

const (
	// AccountCostTypeSubscription 订阅制（固定周期费用）
	AccountCostTypeSubscription = "subscription"
	// AccountCostTypeMetered 按量付费（成本 = usage_logs 账号侧成本汇总）
	AccountCostTypeMetered = "metered"
)

// ProfitUsageStats 单账号在给定区间的用量聚合（利润口径）。
type ProfitUsageStats struct {
	Requests int64
	// Revenue 收入 = SUM(actual_cost)（用户实扣）
	Revenue float64
	// MeteredCost 账号侧成本 = SUM(COALESCE(account_stats_cost,total_cost) * COALESCE(account_rate_multiplier,1))
	MeteredCost float64
}

// ProfitDailyUsagePoint 按日的收入/API Key 按量成本原始数据点。
// 订阅账号成本不包含在 MeteredCost 中，由服务层按周期账本摊销。
type ProfitDailyUsagePoint struct {
	Date        string
	Revenue     float64
	MeteredCost float64
}

// ProfitAccountDailyUsagePoint 按账号、按日的利润基础聚合。
// 一次扫描可同时派生账号汇总和全局每日趋势。
type ProfitAccountDailyUsagePoint struct {
	AccountID   int64
	Date        string
	Requests    int64
	Revenue     float64
	MeteredCost float64
}

// ProfitAccountUsageRange 描述一个账号独立的统计时间范围。
type ProfitAccountUsageRange struct {
	AccountID int64
	Start     time.Time
	End       time.Time
}

// ProfitRepository 利润分析数据访问端口。
type ProfitRepository interface {
	UpsertCostConfig(ctx context.Context, cfg *AccountCostConfig) (*AccountCostConfig, error)
	GetCostConfig(ctx context.Context, accountID int64) (*AccountCostConfig, error)
	ListCostConfigs(ctx context.Context) ([]*AccountCostConfig, error)
	InsertCostConfigsIfAbsent(ctx context.Context, configs []*AccountCostConfig) ([]int64, error)
	DeleteCostConfig(ctx context.Context, accountID int64) error
	ListSubscriptionCycles(ctx context.Context, accountID int64) ([]*AccountSubscriptionCycle, error)
	ListSubscriptionCyclesBatch(ctx context.Context, accountIDs []int64) ([]*AccountSubscriptionCycle, error)
	CreateSubscriptionCycle(ctx context.Context, cycle *AccountSubscriptionCycle) (*AccountSubscriptionCycle, error)
	DeleteSubscriptionCycle(ctx context.Context, id int64) error

	// GetAccountUsageStatsBatch 批量聚合账号在 [start,end) 的收入与账号侧成本。
	GetAccountUsageStatsBatch(ctx context.Context, accountIDs []int64, start, end time.Time) (map[int64]*ProfitUsageStats, error)
	// GetAccountUsageStatsForRanges 批量聚合每个账号独立时间范围内的收入与成本。
	GetAccountUsageStatsForRanges(ctx context.Context, ranges []ProfitAccountUsageRange) (map[int64]*ProfitUsageStats, error)
	// GetAccountDailyUsageStats 一次聚合账号汇总与每日趋势所需的基础数据。
	GetAccountDailyUsageStats(ctx context.Context, accountIDs []int64, start, end time.Time, tzName string) ([]*ProfitAccountDailyUsagePoint, error)
	// GetDailyUsageStats 按日聚合收入与 API Key 按量成本；accountID 为 nil 表示全部账号。
	// OAuth/Setup Token 等订阅账号只贡献收入，成本由服务层的周期账本补充。
	GetDailyUsageStats(ctx context.Context, accountID *int64, start, end time.Time, tzName string) ([]*ProfitDailyUsagePoint, error)
	// GetBestWindowRevenue 返回账号自 since 以来最佳固定长度窗口的收入（用于自动学习窗口基准）。
	GetBestWindowRevenue(ctx context.Context, accountID int64, since time.Time, windowSeconds int64) (float64, error)
	GetBestWindowRevenueBatch(ctx context.Context, accountIDs []int64, since time.Time, windowSeconds int64) (map[int64]float64, error)
}
