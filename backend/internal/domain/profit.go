// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"errors"
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
	WindowBaselineRevenue *float64 `json:"window_baseline_revenue"`
	// AutoRenew 开启后，上一订阅周期结束后按相同费用/天数自动创建下一周期（类似官方订阅续费）。
	// 仅对 OAuth/SetupToken 订阅账号生效；封禁结算中的周期不会续。
	AutoRenew bool      `json:"auto_renew"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AccountSubscriptionCycle 表示一次实际充值形成的独立订阅周期。
// 停用空档不应通过推断的连续续费填补。
type AccountSubscriptionCycle struct {
	ID          int64                           `json:"id"`
	AccountID   int64                           `json:"account_id"`
	StartsAt    time.Time                       `json:"starts_at"`
	PeriodFee   float64                         `json:"period_fee"`
	PeriodDays  int                             `json:"period_days"`
	Currency    string                          `json:"currency"`
	Notes       string                          `json:"notes"`
	Termination *AccountSubscriptionTermination `json:"termination,omitempty"`
	Refunds     []*AccountSubscriptionRefund    `json:"refunds,omitempty"`
	LossSummary *AccountSubscriptionLossSummary `json:"loss_summary,omitempty"`
	CreatedAt   time.Time                       `json:"created_at"`
	UpdatedAt   time.Time                       `json:"updated_at"`
}

// AccountSubscriptionTermination 是管理员确认的订阅周期提前终止事件。
type AccountSubscriptionTermination struct {
	ID             int64      `json:"id"`
	CycleID        int64      `json:"cycle_id"`
	AccountID      int64      `json:"account_id"`
	EffectiveAt    time.Time  `json:"effective_at"`
	Reason         string     `json:"reason"`
	Notes          string     `json:"notes"`
	ReversedAt     *time.Time `json:"reversed_at,omitempty"`
	ReversalReason string     `json:"reversal_reason,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// AccountSubscriptionRefund 只记录已实际到账的上游退款。
type AccountSubscriptionRefund struct {
	ID            int64      `json:"id"`
	TerminationID int64      `json:"termination_id"`
	CycleID       int64      `json:"cycle_id"`
	AccountID     int64      `json:"account_id"`
	Amount        float64    `json:"amount"`
	Currency      string     `json:"currency"`
	ReceivedAt    time.Time  `json:"received_at"`
	Notes         string     `json:"notes"`
	VoidedAt      *time.Time `json:"voided_at,omitempty"`
	VoidReason    string     `json:"void_reason,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// AccountSubscriptionLossSummary 是由周期、封禁前收入和已到账退款派生的快照。
type AccountSubscriptionLossSummary struct {
	PurchaseCost     float64 `json:"purchase_cost"`
	RevenueBeforeBan float64 `json:"revenue_before_ban"`
	RefundTotal      float64 `json:"refund_total"`
	NetPurchaseCost  float64 `json:"net_purchase_cost"`
	RecoveredAmount  float64 `json:"recovered_amount"`
	RecoveryProgress float64 `json:"recovery_progress"`
	RealizedProfit   float64 `json:"realized_profit"`
	RealizedLoss     float64 `json:"realized_loss"`
}

type SubscriptionTerminationWriteResult struct {
	Termination        *AccountSubscriptionTermination `json:"termination"`
	InitialRefund      *AccountSubscriptionRefund      `json:"initial_refund,omitempty"`
	DisabledAccountIDs []int64                         `json:"disabled_account_ids"`
}

type SubscriptionCycleUsageRange struct {
	CycleID   int64
	AccountID int64
	Start     time.Time
	End       time.Time
}

var (
	ErrSubscriptionCycleNotFound          = errors.New("subscription cycle not found")
	ErrSubscriptionCycleAlreadyTerminated = errors.New("subscription cycle already terminated")
	ErrSubscriptionTerminationNotFound    = errors.New("subscription termination not found")
	ErrSubscriptionTerminationReversed    = errors.New("subscription termination already reversed")
	ErrSubscriptionRefundNotFound         = errors.New("subscription refund not found")
	ErrSubscriptionRefundVoided           = errors.New("subscription refund already voided")
	ErrSubscriptionRefundExceedsFee       = errors.New("subscription refunds exceed cycle fee")
	ErrSubscriptionCycleSettled           = errors.New("settled subscription cycle cannot be deleted")
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
