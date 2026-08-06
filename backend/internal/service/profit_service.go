package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// ProfitService 账号利润分析服务。
//
// 利润公式（按账号、按查询周期）：
// - 收入 = SUM(usage_logs.actual_cost)（用户实扣口径）
// - 成本：
//   - metered 账号 = SUM(COALESCE(account_stats_cost,total_cost) × account_rate_multiplier)
//   - subscription 账号 = period_fee × (周期交集天数 / period_days)
//
// - 利润 = 收入 − 成本；利润率 = 利润 / 收入
//
// 订阅号的"重置"（官方 5h/7d 窗口重置 + 重置次数重置）不增加成本，
// 体现为容量利用率与窗口变现效率：同一订阅费摊出更多收入。
type ProfitService struct {
	profitRepo  ProfitRepository
	accountRepo AccountRepository
}

func NewProfitService(profitRepo ProfitRepository, accountRepo AccountRepository) *ProfitService {
	return &ProfitService{profitRepo: profitRepo, accountRepo: accountRepo}
}

// listAccountsForProfit 返回利润复盘用账号集合：在用 + 回收站。
// 回收站包含软删除与结算封禁（SubscriptionBanned），避免封号亏损漏计。
// 硬删除账号不再出现，但其 usage 若已归并到保留号，会体现在保留号上。
func (s *ProfitService) listAccountsForProfit(ctx context.Context) ([]Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, nil
	}
	params := pagination.PaginationParams{Page: 1, PageSize: 10000}
	live, _, err := s.accountRepo.ListWithFilters(ctx, params, "", "", "", "", 0, "")
	if err != nil {
		return nil, err
	}
	// trash = soft-deleted OR settlement-banned（与账号管理回收站一致）
	trash, _, err := s.accountRepo.ListWithFilters(ctx, params, "", "", AccountListStatusTrash, "", 0, "")
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]Account, len(live)+len(trash))
	order := make([]int64, 0, len(live)+len(trash))
	appendUnique := func(acc Account) {
		if _, exists := byID[acc.ID]; exists {
			// trash 列表带 SubscriptionBanned 标记，优先覆盖
			byID[acc.ID] = acc
			return
		}
		order = append(order, acc.ID)
		byID[acc.ID] = acc
	}
	for _, acc := range live {
		appendUnique(acc)
	}
	for _, acc := range trash {
		appendUnique(acc)
	}
	out := make([]Account, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out, nil
}

// getAccountForProfit 读取单个账号（含回收站软删/封禁），供单账号利润抽屉使用。
func (s *ProfitService) getAccountForProfit(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, fmt.Errorf("account repository unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err == nil && account != nil {
		return account, nil
	}
	params := pagination.PaginationParams{Page: 1, PageSize: 10000}
	trash, _, listErr := s.accountRepo.ListWithFilters(ctx, params, "", "", AccountListStatusTrash, "", 0, "")
	if listErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, listErr
	}
	for i := range trash {
		if trash[i].ID == accountID {
			acc := trash[i]
			return &acc, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("account %d not found", accountID)
}

// ProfitQuotaWindow 是利润页时间轴用的滚动配额窗口快照。
// Start/End 由 reset_at 与 window 时长反推；历史轮次不持久化，仅展示当前窗口。
type ProfitQuotaWindow struct {
	ID            string     `json:"id"`
	Label         string     `json:"label"`
	Kind          string     `json:"kind"` // "5h" | "7d" | "24h" | "session" | "other"
	UsedPercent   *float64   `json:"used_percent,omitempty"`
	StartAt       *time.Time `json:"start_at,omitempty"`
	EndAt         *time.Time `json:"end_at,omitempty"`
	WindowMinutes *int       `json:"window_minutes,omitempty"`
	// RecurringUntilAt caps projected occurrences at the subscription cycle or
	// confirmed ban end; it is absent for non-subscription windows.
	RecurringUntilAt *time.Time `json:"recurring_until_at,omitempty"`
}

// AccountProfitSummary 单账号周期利润汇总。
type AccountProfitSummary struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	Platform    string `json:"platform"`
	AccountType string `json:"account_type"`
	CostType    string `json:"cost_type"`
	Configured  bool   `json:"configured"` // 是否已绑定成本配置
	// Deleted 为 true 表示账号当前在回收站（软删除或结算封禁），历史 usage/亏损仍计入利润复盘。
	Deleted bool `json:"deleted"`

	Requests int64   `json:"requests"`
	Revenue  float64 `json:"revenue"` // SUM(actual_cost)
	Cost     float64 `json:"cost"`
	Profit   float64 `json:"profit"`
	Margin   float64 `json:"margin"` // 利润 / 收入；无收入时为 0

	// 订阅账号窗口数据（当前快照，来自 accounts.extra 的 codex_5h/7d）
	FiveHourUtilization *float64 `json:"five_hour_utilization,omitempty"`
	SevenDayUtilization *float64 `json:"seven_day_utilization,omitempty"`

	// QuotaWindows 供利润页「配额窗口」时间轴渲染；由账号 extra / session 字段推导。
	QuotaWindows []ProfitQuotaWindow `json:"quota_windows,omitempty"`

	// 窗口变现效率 = 周期内平均每 5h 窗口收入 / 基准窗口收入。
	// 基准来自 window_baseline_revenue 或历史最佳 5h 窗口自动学习；无基准时为空。
	WindowEfficiency     *float64 `json:"window_efficiency,omitempty"`
	WindowBaselineSource string   `json:"window_baseline_source,omitempty"` // "configured" / "learned" / ""

	// 当前计费窗口（人工本期起始日优先，subscription_expires_at 回退）
	BillingWindowStart             *time.Time `json:"billing_window_start,omitempty"`
	BillingWindowEnd               *time.Time `json:"billing_window_end,omitempty"`
	BillingWindowProgress          *float64   `json:"billing_window_progress,omitempty"` // 0-100，窗口已过比例
	BillingWindowRevenue           *float64   `json:"billing_window_revenue,omitempty"`  // 窗口内至今收入
	BillingWindowCost              *float64   `json:"billing_window_cost,omitempty"`     // 窗口成本 = 整期订阅费
	BillingWindowProfit            *float64   `json:"billing_window_profit,omitempty"`
	BillingWindowSource            string     `json:"billing_window_source,omitempty"` // manual / subscription_expiry
	RequiresCycleStart             bool       `json:"requires_cycle_start,omitempty"`
	BillingWindowTerminatedAt      *time.Time `json:"billing_window_terminated_at,omitempty"`
	BillingWindowTerminationReason string     `json:"billing_window_termination_reason,omitempty"`
	BillingWindowOriginalCost      *float64   `json:"billing_window_original_cost,omitempty"`
	BillingWindowRefundTotal       *float64   `json:"billing_window_refund_total,omitempty"`
	BillingWindowRecoveredAmount   *float64   `json:"billing_window_recovered_amount,omitempty"`
	BillingWindowRecoveryProgress  *float64   `json:"billing_window_recovery_progress,omitempty"`
	BillingWindowLoss              *float64   `json:"billing_window_loss,omitempty"`

	// 最低保本售卖倍率（仅订阅 OAuth/SetupToken）：
	// break_even_rate = current_effective_rate × period_fee / (full_window_user_revenue × windows_per_period)
	// current_effective_rate ≈ 窗内用户扣费 / 账号标价成本（分组实际倍率）
	// full_window_user_revenue = 窗内用户扣费 / (used_percent/100)
	// windows_per_period = period_days×24×60 / window_minutes
	BreakEvenRate              *float64 `json:"break_even_rate,omitempty"`
	BreakEvenWindowKind        string   `json:"break_even_window_kind,omitempty"`
	BreakEvenWindowMinutes     *int     `json:"break_even_window_minutes,omitempty"`
	BreakEvenUsedPercent       *float64 `json:"break_even_used_percent,omitempty"`
	BreakEvenFullWindowRevenue *float64 `json:"break_even_full_window_revenue,omitempty"`
	BreakEvenWindowsPerPeriod  *float64 `json:"break_even_windows_per_period,omitempty"`
	BreakEvenCapacityRevenue   *float64 `json:"break_even_capacity_revenue,omitempty"`
	BreakEvenCurrentRate       *float64 `json:"break_even_current_rate,omitempty"`
	BreakEvenPeriodFee         *float64 `json:"break_even_period_fee,omitempty"`
	BreakEvenPeriodDays        *int     `json:"break_even_period_days,omitempty"`

	Currency string `json:"currency"`
}

// ProfitSummaryResponse 汇总响应。
type ProfitSummaryResponse struct {
	Start        time.Time               `json:"start"`
	End          time.Time               `json:"end"`
	TotalRevenue float64                 `json:"total_revenue"`
	TotalCost    float64                 `json:"total_cost"`
	TotalProfit  float64                 `json:"total_profit"`
	Accounts     []*AccountProfitSummary `json:"accounts"`
}

// ProfitTrendPoint 趋势数据点（含摊销成本）。
type ProfitTrendPoint struct {
	Date    string  `json:"date"`
	Revenue float64 `json:"revenue"`
	Cost    float64 `json:"cost"`
	Profit  float64 `json:"profit"`
}

// ProfitTrendResponse 趋势响应。
type ProfitTrendResponse struct {
	Points []*ProfitTrendPoint `json:"points"`
}

// ProfitOverviewResponse 为利润页一次返回汇总与趋势，避免重复扫描相同时间范围。
type ProfitOverviewResponse struct {
	GeneratedAt time.Time              `json:"generated_at"`
	Summary     *ProfitSummaryResponse `json:"summary"`
	Points      []*ProfitTrendPoint    `json:"points"`
}

// profitAccountingOptions 控制 loadProfitAccountingData 额外加载哪些运营指标。
// 利润列表只需保本倍率；账号抽屉需要完整计费窗 + 5h 效率。
type profitAccountingOptions struct {
	// BreakEven 当前额度窗外推最低保本售卖倍率
	BreakEven bool
	// BillingWindow 当前订阅周期内收入（经营汇总）
	BillingWindow bool
	// WindowEfficiency 近 30 天最佳 5h 窗效率
	WindowEfficiency bool
}

func (o profitAccountingOptions) any() bool {
	return o.BreakEven || o.BillingWindow || o.WindowEfficiency
}

// fullProfitAccountingOptions 账号抽屉 / GetSummary 全量运营指标。
func fullProfitAccountingOptions() profitAccountingOptions {
	return profitAccountingOptions{BreakEven: true, BillingWindow: true, WindowEfficiency: true}
}

// listProfitAccountingOptions 利润页 overview：只算列表要展示的保本倍率，避免多余 usage 扫描。
func listProfitAccountingOptions() profitAccountingOptions {
	return profitAccountingOptions{BreakEven: true}
}

type profitAccountingData struct {
	configsByAccount     map[int64]*AccountCostConfig
	cyclesByAccount      map[int64][]*AccountSubscriptionCycle
	bestWindowByAccount  map[int64]float64
	windowStatsByAccount map[int64]*ProfitUsageStats
	// 保本倍率用：当前配额窗口快照 + 窗口内 usage 聚合
	breakEvenWindowByAccount map[int64]*ProfitQuotaWindow
	breakEvenStatsByAccount  map[int64]*ProfitUsageStats
	now                      time.Time
	opts                     profitAccountingOptions
}

// GetSummary 计算 [start,end) 内所有账号的利润汇总。
func (s *ProfitService) GetSummary(ctx context.Context, start, end time.Time) (*ProfitSummaryResponse, error) {
	if end.Before(start) {
		start, end = end, start
	}
	accounts, err := s.listAccountsForProfit(ctx)
	if err != nil {
		return nil, err
	}
	return s.summarizeAccounts(ctx, accounts, start, end)
}

// GetAccountSummary returns the financial summary for one account only. It keeps
// the account drawer from loading every account's financial rows just to render
// one account's statistics.
func (s *ProfitService) GetAccountSummary(ctx context.Context, accountID int64, start, end time.Time) (*ProfitSummaryResponse, error) {
	if end.Before(start) {
		start, end = end, start
	}
	account, err := s.getAccountForProfit(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, fmt.Errorf("account %d not found", accountID)
	}
	return s.summarizeAccounts(ctx, []Account{*account}, start, end)
}

func (s *ProfitService) summarizeAccounts(ctx context.Context, accounts []Account, start, end time.Time) (*ProfitSummaryResponse, error) {
	accountIDs := make([]int64, 0, len(accounts))
	for i := range accounts {
		accountIDs = append(accountIDs, accounts[i].ID)
	}
	statsByAccount, err := s.profitRepo.GetAccountUsageStatsBatch(ctx, accountIDs, start, end)
	if err != nil {
		return nil, err
	}
	accounting, err := s.loadProfitAccountingData(ctx, accounts, fullProfitAccountingOptions())
	if err != nil {
		return nil, err
	}
	return s.summarizeAccountsWithData(accounts, start, end, statsByAccount, accounting), nil
}

func (s *ProfitService) summarizeAccountsWithData(accounts []Account, start, end time.Time, statsByAccount map[int64]*ProfitUsageStats, accounting *profitAccountingData) *ProfitSummaryResponse {

	periodDays := end.Sub(start).Hours() / 24
	resp := &ProfitSummaryResponse{Start: start, End: end}
	for i := range accounts {
		acc := &accounts[i]
		stats := statsByAccount[acc.ID]
		if stats == nil {
			stats = &ProfitUsageStats{}
		}
		cfg := accounting.configsByAccount[acc.ID]
		costType := costTypeForAccountType(acc.Type)
		summary := &AccountProfitSummary{
			AccountID:   acc.ID,
			AccountName: acc.Name,
			Platform:    acc.Platform,
			AccountType: acc.Type,
			CostType:    costType,
			Deleted:     acc.DeletedAt != nil || acc.SubscriptionBanned,
			Requests:    stats.Requests,
			Revenue:     roundMoney(stats.Revenue),
			Currency:    "USD",
		}
		// 窗口利用率快照 + 时间轴窗口（OpenAI/Kimi/Grok/Claude session）
		var quotaWindowCutoff *time.Time
		if isSubscriptionAccountType(acc.Type) {
			quotaWindowCutoff = quotaWindowCutoffForCycles(accounting.cyclesByAccount[acc.ID], accounting.now)
		}
		fillProfitQuotaWindows(summary, acc, accounting.now, quotaWindowCutoff)

		cost := stats.MeteredCost
		if costType == AccountCostTypeSubscription {
			cost = 0
			cycles := accounting.cyclesByAccount[acc.ID]
			if len(cycles) == 0 {
				summary.RequiresCycleStart = true
			} else {
				summary.Configured = true
				cost = subscriptionCyclesCostForRange(cycles, start, end)
if financialCycle := financialSubscriptionCycle(cycles, accounting.now); financialCycle != nil {
						summary.Currency = financialCycle.Currency
						if accounting.opts.BillingWindow {
							windowRevenue := 0.0
							if windowStats := accounting.windowStatsByAccount[acc.ID]; windowStats != nil {
								windowRevenue = windowStats.Revenue
							}
							fillBillingWindowFromRevenue(financialCycle, summary, windowRevenue, accounting.now)
						}
						if accounting.opts.BreakEven {
							fillBreakEvenRate(
								summary,
								financialCycle,
								accounting.breakEvenWindowByAccount[acc.ID],
								accounting.breakEvenStatsByAccount[acc.ID],
							)
						}
					}
				}
				if accounting.opts.WindowEfficiency {
					summary.WindowEfficiency, summary.WindowBaselineSource = windowEfficiencyFromBaseline(stats.Revenue, periodDays, accounting.bestWindowByAccount[acc.ID])
				}
		} else if cfg != nil {
			// API Key 账号即使遗留了订阅配置，也必须按历史账号侧成本结算。
			summary.Configured = true
			summary.Currency = cfg.Currency
		}
		summary.Cost = roundMoney(cost)
		summary.Profit = roundMoney(summary.Revenue - summary.Cost)
		if summary.Revenue > 0 {
			summary.Margin = roundMoney(summary.Profit / summary.Revenue * 100)
		}
		resp.TotalRevenue += summary.Revenue
		resp.TotalCost += summary.Cost
		resp.TotalProfit += summary.Profit
		resp.Accounts = append(resp.Accounts, summary)
	}
	resp.TotalRevenue = roundMoney(resp.TotalRevenue)
	resp.TotalCost = roundMoney(resp.TotalCost)
	resp.TotalProfit = roundMoney(resp.TotalProfit)
	return resp
}

func (s *ProfitService) loadProfitAccountingData(ctx context.Context, accounts []Account, opts profitAccountingOptions) (*profitAccountingData, error) {
	data := &profitAccountingData{
		configsByAccount:         make(map[int64]*AccountCostConfig),
		cyclesByAccount:          make(map[int64][]*AccountSubscriptionCycle),
		bestWindowByAccount:      make(map[int64]float64),
		windowStatsByAccount:     make(map[int64]*ProfitUsageStats),
		breakEvenWindowByAccount: make(map[int64]*ProfitQuotaWindow),
		breakEvenStatsByAccount:  make(map[int64]*ProfitUsageStats),
		now:                      time.Now(),
		opts:                     opts,
	}
	if len(accounts) == 1 {
		cfg, err := s.profitRepo.GetCostConfig(ctx, accounts[0].ID)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			data.configsByAccount[cfg.AccountID] = cfg
		}
	} else {
		configs, err := s.profitRepo.ListCostConfigs(ctx)
		if err != nil {
			return nil, err
		}
		for _, cfg := range configs {
			data.configsByAccount[cfg.AccountID] = cfg
		}
	}

	subscriptionIDs := make([]int64, 0, len(accounts))
	for i := range accounts {
		if isSubscriptionAccountType(accounts[i].Type) {
			subscriptionIDs = append(subscriptionIDs, accounts[i].ID)
		}
	}
	cycles, err := s.profitRepo.ListSubscriptionCyclesBatch(ctx, subscriptionIDs)
	if err != nil {
		return nil, err
	}
	for _, cycle := range cycles {
		data.cyclesByAccount[cycle.AccountID] = append(data.cyclesByAccount[cycle.AccountID], cycle)
	}
	if !opts.any() {
		return data, nil
	}

	// 近 30 天最佳 5h 窗：仅抽屉效率指标需要，列表跳过。
	if opts.WindowEfficiency {
		data.bestWindowByAccount, err = s.profitRepo.GetBestWindowRevenueBatch(ctx, subscriptionIDs, data.now.Add(-30*24*time.Hour), 5*3600)
		if err != nil {
			return nil, err
		}
	}

	// 当前订阅周期累计收入：经营汇总用，列表不展示则跳过。
	if opts.BillingWindow {
		ranges := make([]ProfitAccountUsageRange, 0, len(subscriptionIDs))
		for _, accountID := range subscriptionIDs {
			if cycle := financialSubscriptionCycle(data.cyclesByAccount[accountID], data.now); cycle != nil {
				end := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
				if termination := activeCycleTermination(cycle); termination != nil && termination.EffectiveAt.Before(end) {
					end = termination.EffectiveAt
				}
				if data.now.Before(end) {
					end = data.now
				}
				if end.After(cycle.StartsAt) {
					ranges = append(ranges, ProfitAccountUsageRange{AccountID: accountID, Start: cycle.StartsAt, End: end})
				}
			}
		}
		data.windowStatsByAccount, err = s.profitRepo.GetAccountUsageStatsForRanges(ctx, ranges)
		if err != nil {
			return nil, err
		}
	}

	// 保本倍率：额度窗（优先 7d，通常 ≤7 天）内 U/A 聚合，比整期扫描轻。
	if opts.BreakEven {
		breakEvenRanges := make([]ProfitAccountUsageRange, 0, len(subscriptionIDs))
		accountsByID := make(map[int64]*Account, len(accounts))
		for i := range accounts {
			accountsByID[accounts[i].ID] = &accounts[i]
		}
		for _, accountID := range subscriptionIDs {
			acc := accountsByID[accountID]
			if acc == nil {
				continue
			}
			cutoff := quotaWindowCutoffForCycles(data.cyclesByAccount[accountID], data.now)
			tmp := &AccountProfitSummary{}
			fillProfitQuotaWindows(tmp, acc, data.now, cutoff)
			win := pickPreferredBreakEvenWindow(tmp.QuotaWindows)
			if win == nil || win.StartAt == nil {
				continue
			}
			start := *win.StartAt
			end := data.now
			if win.EndAt != nil && win.EndAt.Before(end) {
				end = *win.EndAt
			}
			if !end.After(start) {
				continue
			}
			data.breakEvenWindowByAccount[accountID] = win
			breakEvenRanges = append(breakEvenRanges, ProfitAccountUsageRange{
				AccountID: accountID,
				Start:     start,
				End:       end,
			})
		}
		// 无可用额度窗时不打 DB（空 UNNEST 仍有往返成本）。
		if len(breakEvenRanges) > 0 {
			data.breakEvenStatsByAccount, err = s.profitRepo.GetAccountUsageStatsForRanges(ctx, breakEvenRanges)
			if err != nil {
				return nil, err
			}
		}
	}
	return data, nil
}

// subscriptionCostForRange 订阅费按查询区间与当前付费周期的交集摊销。
// 缺少可靠周期锚点时保留旧的范围摊销，以便管理员补录起始日之前仍能查看趋势。
func subscriptionCycleCostForRange(cycle *AccountSubscriptionCycle, start, end time.Time) float64 {
	if cycle.PeriodFee <= 0 || cycle.PeriodDays <= 0 {
		return subscriptionRefundAdjustmentsForRange(cycle, start, end)
	}
	cycleEnd := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
	regularEnd := cycleEnd
	termination := activeCycleTermination(cycle)
	if termination != nil && termination.EffectiveAt.Before(regularEnd) {
		regularEnd = termination.EffectiveAt
	}
	regularStart := start
	if regularStart.Before(cycle.StartsAt) {
		regularStart = cycle.StartsAt
	}
	queryRegularEnd := end
	if queryRegularEnd.After(regularEnd) {
		queryRegularEnd = regularEnd
	}
	cost := 0.0
	days := queryRegularEnd.Sub(regularStart).Hours() / 24
	if days > 0 {
		cost += cycle.PeriodFee * days / float64(cycle.PeriodDays)
	}
	if termination != nil && !termination.EffectiveAt.Before(start) && termination.EffectiveAt.Before(end) {
		amortizedAtBan := cycle.PeriodFee * termination.EffectiveAt.Sub(cycle.StartsAt).Hours() / (float64(cycle.PeriodDays) * 24)
		if amortizedAtBan < 0 {
			amortizedAtBan = 0
		}
		if amortizedAtBan > cycle.PeriodFee {
			amortizedAtBan = cycle.PeriodFee
		}
		cost += cycle.PeriodFee - amortizedAtBan
	}
	return cost + subscriptionRefundAdjustmentsForRange(cycle, start, end)
}

func subscriptionRefundAdjustmentsForRange(cycle *AccountSubscriptionCycle, start, end time.Time) float64 {
	adjustment := 0.0
	for _, refund := range cycle.Refunds {
		if refund == nil || refund.VoidedAt != nil {
			continue
		}
		if !refund.ReceivedAt.Before(start) && refund.ReceivedAt.Before(end) {
			adjustment -= refund.Amount
		}
	}
	return adjustment
}

func subscriptionCyclesCostForRange(cycles []*AccountSubscriptionCycle, start, end time.Time) float64 {
	cost := 0.0
	for _, cycle := range cycles {
		cost += subscriptionCycleCostForRange(cycle, start, end)
	}
	return cost
}

// controllingSubscriptionCycle 选取驱动账号调度过期时间的成本周期：
// 1) 当前活跃周期 2) 最早的未来周期 3) 最近一条已开始周期。
func controllingSubscriptionCycle(cycles []*AccountSubscriptionCycle, now time.Time) *AccountSubscriptionCycle {
	if active := activeSubscriptionCycle(cycles, now); active != nil {
		return active
	}
	var future *AccountSubscriptionCycle
	var latest *AccountSubscriptionCycle
	for _, cycle := range cycles {
		if cycle == nil {
			continue
		}
		if now.Before(cycle.StartsAt) {
			if future == nil || cycle.StartsAt.Before(future.StartsAt) {
				future = cycle
			}
			continue
		}
		if latest == nil || cycle.StartsAt.After(latest.StartsAt) ||
			(cycle.StartsAt.Equal(latest.StartsAt) && cycle.ID > latest.ID) {
			latest = cycle
		}
	}
	if future != nil {
		return future
	}
	return latest
}

// deriveAccountExpiresAtFromCycles 由成本周期推导 accounts.expires_at。
// 无周期时返回 nil（清空过期时间）。
func deriveAccountExpiresAtFromCycles(cycles []*AccountSubscriptionCycle, now time.Time) *time.Time {
	cycle := controllingSubscriptionCycle(cycles, now)
	if cycle == nil {
		return nil
	}
	end := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
	if termination := activeCycleTermination(cycle); termination != nil && termination.EffectiveAt.Before(end) {
		end = termination.EffectiveAt
	}
	return &end
}

func (s *ProfitService) syncAccountExpiresAtFromCycles(ctx context.Context, accountID int64) error {
	if s == nil || s.accountRepo == nil || accountID <= 0 {
		return nil
	}
	cycles, err := s.profitRepo.ListSubscriptionCycles(ctx, accountID)
	if err != nil {
		return err
	}
	expiresAt := deriveAccountExpiresAtFromCycles(cycles, time.Now())
	return s.accountRepo.SetExpiresAt(ctx, accountID, expiresAt)
}

func activeSubscriptionCycle(cycles []*AccountSubscriptionCycle, now time.Time) *AccountSubscriptionCycle {
	for _, cycle := range cycles {
		end := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
		if termination := activeCycleTermination(cycle); termination != nil && !now.Before(termination.EffectiveAt) {
			continue
		}
		if !now.Before(cycle.StartsAt) && now.Before(end) {
			return cycle
		}
	}
	return nil
}

func financialSubscriptionCycle(cycles []*AccountSubscriptionCycle, now time.Time) *AccountSubscriptionCycle {
	if active := activeSubscriptionCycle(cycles, now); active != nil {
		return active
	}
	for _, cycle := range cycles {
		if termination := activeCycleTermination(cycle); termination != nil && !now.Before(termination.EffectiveAt) {
			return cycle
		}
	}
	return nil
}

func activeCycleTermination(cycle *AccountSubscriptionCycle) *AccountSubscriptionTermination {
	if cycle == nil || cycle.Termination == nil || cycle.Termination.ReversedAt != nil {
		return nil
	}
	return cycle.Termination
}

func subscriptionRefundTotal(cycle *AccountSubscriptionCycle) float64 {
	total := 0.0
	for _, refund := range cycle.Refunds {
		if refund != nil && refund.VoidedAt == nil {
			total += refund.Amount
		}
	}
	return roundMoney(total)
}

func subscriptionLossSummary(cycle *AccountSubscriptionCycle, revenue float64) *AccountSubscriptionLossSummary {
	if activeCycleTermination(cycle) == nil {
		return nil
	}
	purchaseCost := roundMoney(cycle.PeriodFee)
	revenue = roundMoney(revenue)
	refundTotal := subscriptionRefundTotal(cycle)
	netCost := roundMoney(math.Max(0, purchaseCost-refundTotal))
	recovered := roundMoney(math.Min(purchaseCost, revenue+refundTotal))
	progress := 100.0
	if purchaseCost > 0 {
		progress = roundMoney(recovered / purchaseCost * 100)
	}
	profit := roundMoney(revenue - netCost)
	loss := roundMoney(math.Max(0, -profit))
	return &AccountSubscriptionLossSummary{
		PurchaseCost: purchaseCost, RevenueBeforeBan: revenue, RefundTotal: refundTotal,
		NetPurchaseCost: netCost, RecoveredAmount: recovered, RecoveryProgress: progress,
		RealizedProfit: profit, RealizedLoss: loss,
	}
}

// fillBillingWindow 计算当前有效充值周期数据。
// 窗口内收入取 [窗口开始, min(now, 窗口结束)] 的 actual_cost 汇总。
func (s *ProfitService) fillBillingWindow(ctx context.Context, accountID int64, cycle *AccountSubscriptionCycle, summary *AccountProfitSummary) {
	wStart := cycle.StartsAt
	wEnd := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
	now := time.Now()

	// 窗口内收入：窗口开始到 min(now, 窗口结束)
	effectiveEnd := wEnd
	if termination := activeCycleTermination(cycle); termination != nil && termination.EffectiveAt.Before(effectiveEnd) {
		effectiveEnd = termination.EffectiveAt
	}
	if now.Before(effectiveEnd) {
		effectiveEnd = now
	}
	var revenue float64
	if effectiveEnd.After(wStart) {
		stats, err := s.profitRepo.GetAccountUsageStatsBatch(ctx, []int64{accountID}, wStart, effectiveEnd)
		if err == nil && stats[accountID] != nil {
			revenue = stats[accountID].Revenue
		}
	}
	fillBillingWindowFromRevenue(cycle, summary, revenue, now)
}

// pickPreferredBreakEvenWindow 选择用于保本测算的配额窗：
// 优先 7d（周额度），其次 5h / 24h；必须有 used_percent 与可解析时长。
func pickPreferredBreakEvenWindow(windows []ProfitQuotaWindow) *ProfitQuotaWindow {
	if len(windows) == 0 {
		return nil
	}
rank := func(kind string) int {
			switch kind {
			case "7d", "30d":
				// 长滚动窗（周/月）优先用于保本测算
				return 0
			case "5h":
				return 1
			case "24h":
				return 2
			default:
				return 3
			}
		}
	var best *ProfitQuotaWindow
	bestRank := 99
	for i := range windows {
		w := &windows[i]
		if w.UsedPercent == nil || *w.UsedPercent <= 0 {
			continue
		}
		mins := profitWindowMinutes(w)
		if mins <= 0 {
			continue
		}
		r := rank(w.Kind)
		if best == nil || r < bestRank {
			best = w
			bestRank = r
		}
	}
	return best
}

func profitWindowMinutes(w *ProfitQuotaWindow) int {
		if w == nil {
			return 0
		}
		if w.WindowMinutes != nil && *w.WindowMinutes > 0 {
			return *w.WindowMinutes
		}
		switch w.Kind {
		case "5h":
			return 300
		case "7d":
			return 10080
		case "30d":
			return 30 * 24 * 60
		case "24h":
			return 1440
		case "session":
			return 300
	default:
		return 0
	}
}

// fillBreakEvenRate 根据订阅周期 + 当前额度窗利用率 + 窗内扣费，计算最低保本售卖倍率。
// 仅在能观测到有效用户扣费与账号标价成本时写入 BreakEvenRate。
func fillBreakEvenRate(summary *AccountProfitSummary, cycle *AccountSubscriptionCycle, win *ProfitQuotaWindow, stats *ProfitUsageStats) {
	if summary == nil || cycle == nil || win == nil || stats == nil {
		return
	}
	if cycle.PeriodFee <= 0 || cycle.PeriodDays <= 0 {
		return
	}
	used := 0.0
	if win.UsedPercent != nil {
		used = *win.UsedPercent
	}
	if used <= 0 {
		return
	}
	if used > 100 {
		used = 100
	}
	// 利用率过低时外推噪声大，至少 1%
	if used < 1 {
		used = 1
	}
	mins := profitWindowMinutes(win)
	if mins <= 0 {
		return
	}
	if stats.Revenue <= 0 {
		return
	}
	fullWindowRevenue := stats.Revenue * (100.0 / used)
	if fullWindowRevenue <= 0 {
		return
	}
	windowsPerPeriod := float64(cycle.PeriodDays) * 24.0 * 60.0 / float64(mins)
	if windowsPerPeriod < 1 {
		windowsPerPeriod = 1
	}
	capacityRevenue := fullWindowRevenue * windowsPerPeriod
	if capacityRevenue <= 0 {
		return
	}

	fee := roundMoney(cycle.PeriodFee)
	days := cycle.PeriodDays
	fullRevRounded := roundMoney(fullWindowRevenue)
	windowsRounded := roundMoney(windowsPerPeriod)
	capacityRounded := roundMoney(capacityRevenue)
	summary.BreakEvenPeriodFee = &fee
	summary.BreakEvenPeriodDays = &days
	summary.BreakEvenWindowKind = win.Kind
	if summary.BreakEvenWindowKind == "" {
		summary.BreakEvenWindowKind = win.Label
	}
	summary.BreakEvenWindowMinutes = &mins
	usedCopy := used
	summary.BreakEvenUsedPercent = &usedCopy
	summary.BreakEvenFullWindowRevenue = &fullRevRounded
	summary.BreakEvenWindowsPerPeriod = &windowsRounded
	summary.BreakEvenCapacityRevenue = &capacityRounded

	// 有效售卖倍率 ≈ 用户扣费 / 账号标价成本（与 usage 里 U/A 比值一致）
	if stats.MeteredCost <= 0 {
		return
	}
	currentRate := stats.Revenue / stats.MeteredCost
	if currentRate <= 0 {
		return
	}
	be := currentRate * cycle.PeriodFee / capacityRevenue
	if be <= 0 {
		return
	}
	// 倍率保留 4 位，与分组 rate_multiplier 展示一致
	beRounded := math.Round(be*10000) / 10000
	curRounded := math.Round(currentRate*10000) / 10000
	summary.BreakEvenRate = &beRounded
	summary.BreakEvenCurrentRate = &curRounded
}

func fillBillingWindowFromRevenue(cycle *AccountSubscriptionCycle, summary *AccountProfitSummary, revenue float64, now time.Time) {
	wStart := cycle.StartsAt
	wEnd := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
	periodDays := cycle.PeriodDays
	revenue = roundMoney(revenue)
	effectiveNow := now
	if termination := activeCycleTermination(cycle); termination != nil && termination.EffectiveAt.Before(effectiveNow) {
		effectiveNow = termination.EffectiveAt
	}
	progress := effectiveNow.Sub(wStart).Hours() / (float64(periodDays) * 24) * 100
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	progress = roundMoney(progress)

	summary.BillingWindowStart = &wStart
	summary.BillingWindowEnd = &wEnd
	summary.BillingWindowSource = "cycle"
	summary.BillingWindowProgress = &progress
	summary.BillingWindowRevenue = &revenue
	// 周期费用允许为 0。是否存在有效周期由周期账本决定，不能用费用是否大于 0 判断。
	cost := roundMoney(cycle.PeriodFee)
	profit := roundMoney(revenue - cycle.PeriodFee)
	if termination := activeCycleTermination(cycle); termination != nil {
		loss := subscriptionLossSummary(cycle, revenue)
		originalCost := loss.PurchaseCost
		refundTotal := loss.RefundTotal
		recoveredAmount := loss.RecoveredAmount
		recoveryProgress := loss.RecoveryProgress
		confirmedLoss := loss.RealizedLoss
		summary.BillingWindowTerminatedAt = &termination.EffectiveAt
		summary.BillingWindowTerminationReason = termination.Reason
		summary.BillingWindowOriginalCost = &originalCost
		summary.BillingWindowRefundTotal = &refundTotal
		summary.BillingWindowRecoveredAmount = &recoveredAmount
		summary.BillingWindowRecoveryProgress = &recoveryProgress
		summary.BillingWindowLoss = &confirmedLoss
		cost = loss.NetPurchaseCost
		profit = loss.RealizedProfit
	} else if refundTotal := subscriptionRefundTotal(cycle); refundTotal > 0 {
		originalCost := roundMoney(cycle.PeriodFee)
		netCost := roundMoney(math.Max(0, originalCost-refundTotal))
		summary.BillingWindowOriginalCost = &originalCost
		summary.BillingWindowRefundTotal = &refundTotal
		cost = netCost
		profit = roundMoney(revenue - netCost)
	}
	summary.BillingWindowCost = &cost
	summary.BillingWindowProfit = &profit
}

// windowEfficiency 窗口变现效率 = 平均每 5h 窗口收入 / 基准窗口收入。
// 官方重置带来的额外容量会体现为更多窗口数或更高窗口收入，自动反映在效率上。
// cfg 为 nil（未绑定配置的订阅号）时仅使用历史学习基准。
func (s *ProfitService) windowEfficiency(ctx context.Context, accountID int64, revenue, periodDays float64) (*float64, string) {
	if revenue <= 0 || periodDays <= 0 {
		return nil, ""
	}
	since := time.Now().Add(-30 * 24 * time.Hour)
	baseline, err := s.profitRepo.GetBestWindowRevenue(ctx, accountID, since, 5*3600)
	if err != nil || baseline <= 0 {
		return nil, ""
	}
	return windowEfficiencyFromBaseline(revenue, periodDays, baseline)
}

func windowEfficiencyFromBaseline(revenue, periodDays, baseline float64) (*float64, string) {
	if revenue <= 0 || periodDays <= 0 || baseline <= 0 {
		return nil, ""
	}
	windows := periodDays * 24 / 5
	if windows < 1 {
		windows = 1
	}
	eff := (revenue / windows) / baseline
	result := roundMoney(eff * 100)
	return &result, "learned"
}

// GetTrend 返回 [start,end) 按日的收入/成本/利润趋势。
// 订阅费按日摊销叠加到每日成本上。
func (s *ProfitService) GetTrend(ctx context.Context, accountID *int64, start, end time.Time, tzName string) (*ProfitTrendResponse, error) {
	if end.Before(start) {
		start, end = end, start
	}
	if tzName == "" {
		tzName = "Asia/Shanghai"
	}
	points, err := s.profitRepo.GetDailyUsageStats(ctx, accountID, start, end, tzName)
	if err != nil {
		return nil, err
	}

	subscriptionIDs := make([]int64, 0)
	if s.accountRepo != nil && accountID != nil {
		account, getErr := s.getAccountForProfit(ctx, *accountID)
		if getErr != nil {
			return nil, getErr
		}
		if account != nil && isSubscriptionAccountType(account.Type) {
			subscriptionIDs = append(subscriptionIDs, account.ID)
		}
	} else if s.accountRepo != nil {
		accounts, listErr := s.listAccountsForProfit(ctx)
		if listErr != nil {
			return nil, listErr
		}
		for _, account := range accounts {
			if isSubscriptionAccountType(account.Type) {
				subscriptionIDs = append(subscriptionIDs, account.ID)
			}
		}
	}
	subscriptionCycles, err := s.profitRepo.ListSubscriptionCyclesBatch(ctx, subscriptionIDs)
	if err != nil {
		return nil, err
	}
	return buildProfitTrend(points, subscriptionCycles, tzName, start, end), nil
}

func buildProfitTrend(points []*ProfitDailyUsagePoint, subscriptionCycles []*AccountSubscriptionCycle, tzName string, start, end time.Time) *ProfitTrendResponse {
	resp := &ProfitTrendResponse{}
	location, _ := time.LoadLocation(tzName)
	if location == nil {
		location = time.UTC
	}
	pointsByDate := make(map[string]*ProfitDailyUsagePoint, len(points))
	for _, point := range points {
		pointsByDate[point.Date] = point
	}
	localStart := start.In(location)
	dayStart := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, location)
	for dayStart.Before(end) {
		dayEnd := dayStart.AddDate(0, 0, 1)
		date := dayStart.Format(time.DateOnly)
		p := pointsByDate[date]
		if p == nil {
			p = &ProfitDailyUsagePoint{Date: date}
		}
		dailySubscriptionCost := 0.0
		for _, cycle := range subscriptionCycles {
			dailySubscriptionCost += subscriptionCycleCostForRange(cycle, dayStart, dayEnd)
		}
		cost := p.MeteredCost + dailySubscriptionCost
		resp.Points = append(resp.Points, &ProfitTrendPoint{
			Date:    date,
			Revenue: roundMoney(p.Revenue),
			Cost:    roundMoney(cost),
			Profit:  roundMoney(p.Revenue - cost),
		})
		dayStart = dayEnd
	}
	return resp
}

// GetOverview 一次加载利润页汇总、趋势和账号明细。
func (s *ProfitService) GetOverview(ctx context.Context, start, end time.Time, tzName string) (*ProfitOverviewResponse, error) {
	if end.Before(start) {
		start, end = end, start
	}
	if tzName == "" {
		tzName = "Asia/Shanghai"
	}
	accounts, err := s.listAccountsForProfit(ctx)
	if err != nil {
		return nil, err
	}
	accountIDs := make([]int64, 0, len(accounts))
	for i := range accounts {
		accountIDs = append(accountIDs, accounts[i].ID)
	}
	dailyByAccount, err := s.profitRepo.GetAccountDailyUsageStats(ctx, accountIDs, start, end, tzName)
	if err != nil {
		return nil, err
	}
	statsByAccount := make(map[int64]*ProfitUsageStats, len(accounts))
	dailyByDate := make(map[string]*ProfitDailyUsagePoint)
	dateOrder := make([]string, 0)
	for _, point := range dailyByAccount {
		stats := statsByAccount[point.AccountID]
		if stats == nil {
			stats = &ProfitUsageStats{}
			statsByAccount[point.AccountID] = stats
		}
		stats.Requests += point.Requests
		stats.Revenue += point.Revenue
		stats.MeteredCost += point.MeteredCost
		daily := dailyByDate[point.Date]
		if daily == nil {
			daily = &ProfitDailyUsagePoint{Date: point.Date}
			dailyByDate[point.Date] = daily
			dateOrder = append(dateOrder, point.Date)
		}
		daily.Revenue += point.Revenue
		daily.MeteredCost += point.MeteredCost
	}
	dailyPoints := make([]*ProfitDailyUsagePoint, 0, len(dateOrder))
	for _, date := range dateOrder {
		dailyPoints = append(dailyPoints, dailyByDate[date])
	}
// 列表页只加载保本倍率所需查询（额度窗短区间），跳过最佳 5h 窗与整期计费窗聚合。
		accounting, err := s.loadProfitAccountingData(ctx, accounts, listProfitAccountingOptions())
	if err != nil {
		return nil, err
	}
	summary := s.summarizeAccountsWithData(accounts, start, end, statsByAccount, accounting)
	cycles := make([]*AccountSubscriptionCycle, 0)
	for _, accountCycles := range accounting.cyclesByAccount {
		cycles = append(cycles, accountCycles...)
	}
	trend := buildProfitTrend(dailyPoints, cycles, tzName, start, end)
	return &ProfitOverviewResponse{GeneratedAt: time.Now().UTC(), Summary: summary, Points: trend.Points}, nil
}

// --- 配置 CRUD ---

func (s *ProfitService) ListCostConfigs(ctx context.Context) ([]*AccountCostConfig, error) {
	return s.profitRepo.ListCostConfigs(ctx)
}

func (s *ProfitService) UpsertCostConfig(ctx context.Context, cfg *AccountCostConfig) (*AccountCostConfig, error) {
	account, err := s.accountRepo.GetByID(ctx, cfg.AccountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	// 成本口径不可由请求覆盖，始终跟随账号认证类型。
	cfg.CostType = costTypeForAccountType(account.Type)
	if cfg.PeriodDays <= 0 {
		cfg.PeriodDays = 30
	}
	if cfg.Currency == "" {
		cfg.Currency = "USD"
	}
	if cfg.CostType == AccountCostTypeMetered {
		cfg.PeriodFee = 0
	}
	return s.profitRepo.UpsertCostConfig(ctx, cfg)
}

// SetSubscriptionAutoRenew toggles auto-renew on the account cost config.
// Creates a subscription cost-config row if missing (fee/days left at defaults
// until the first manual cycle exists; renew copies from previous cycle).
func (s *ProfitService) SetSubscriptionAutoRenew(ctx context.Context, accountID int64, enabled bool) (*AccountCostConfig, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if !isSubscriptionAccountType(account.Type) {
		return nil, fmt.Errorf("auto renew requires oauth or setup-token account")
	}
	cfg, err := s.profitRepo.GetCostConfig(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &AccountCostConfig{
			AccountID:  accountID,
			CostType:   AccountCostTypeSubscription,
			PeriodDays: 30,
			Currency:   "USD",
		}
	}
	cfg.CostType = AccountCostTypeSubscription
	cfg.AutoRenew = enabled
	if cfg.PeriodDays <= 0 {
		cfg.PeriodDays = 30
	}
	if cfg.Currency == "" {
		cfg.Currency = "USD"
	}
	return s.profitRepo.UpsertCostConfig(ctx, cfg)
}

func (s *ProfitService) DeleteCostConfig(ctx context.Context, accountID int64) error {
	return s.profitRepo.DeleteCostConfig(ctx, accountID)
}

type SubscriptionCycleList struct {
	Cycles                []*AccountSubscriptionCycle `json:"cycles"`
	SubscriptionExpiresAt *time.Time                  `json:"subscription_expires_at,omitempty"`
	OAuthTokenExpiresAt   *time.Time                  `json:"oauth_token_expires_at,omitempty"`
	// AccountExpiresAt 账号调度过期时间（由成本周期驱动同步）
	AccountExpiresAt *time.Time `json:"account_expires_at,omitempty"`
	// AutoRenew 成本配置上的订阅自动续期开关
	AutoRenew bool `json:"auto_renew"`
}

type SubscriptionCycleSettlementResult struct {
	Cycle              *AccountSubscriptionCycle `json:"cycle"`
	DisabledAccountIDs []int64                   `json:"disabled_account_ids,omitempty"`
}

func (s *ProfitService) ListSubscriptionCycles(ctx context.Context, accountID int64) (*SubscriptionCycleList, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	cycles, err := s.profitRepo.ListSubscriptionCycles(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if err := s.populateSubscriptionLossSummaries(ctx, cycles); err != nil {
		return nil, err
	}
	// Keep the account scheduling expiry aligned for historical cycles too. Older
	// cycles were created before expiry synchronization was introduced.
	if err := s.syncAccountExpiresAtFromCycles(ctx, accountID); err != nil {
		return nil, err
	}
	account, err = s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	autoRenew := false
	if cfg, cfgErr := s.profitRepo.GetCostConfig(ctx, accountID); cfgErr != nil {
		return nil, cfgErr
	} else if cfg != nil {
		autoRenew = cfg.AutoRenew
	}
	return &SubscriptionCycleList{
		Cycles:                cycles,
		SubscriptionExpiresAt: account.GetCredentialAsTime("subscription_expires_at"),
		OAuthTokenExpiresAt:   account.GetCredentialAsTime("expires_at"),
		AccountExpiresAt:      account.ExpiresAt,
		AutoRenew:             autoRenew,
	}, nil
}

func (s *ProfitService) CreateSubscriptionCycle(ctx context.Context, cycle *AccountSubscriptionCycle) (*AccountSubscriptionCycle, error) {
	account, err := s.accountRepo.GetByID(ctx, cycle.AccountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if !isSubscriptionAccountType(account.Type) {
		return nil, fmt.Errorf("subscription cycles require oauth or setup-token account")
	}
	if cycle.StartsAt.IsZero() || cycle.PeriodDays <= 0 || cycle.PeriodFee < 0 {
		return nil, fmt.Errorf("invalid subscription cycle")
	}
	if cycle.Currency == "" {
		cycle.Currency = "USD"
	}
	created, err := s.profitRepo.CreateSubscriptionCycle(ctx, cycle)
	if err != nil {
		return nil, err
	}
	if err := s.syncAccountExpiresAtFromCycles(ctx, created.AccountID); err != nil {
		return created, err
	}
	return created, nil
}

func (s *ProfitService) DeleteSubscriptionCycle(ctx context.Context, id int64) error {
	cycle, err := s.profitRepo.GetSubscriptionCycle(ctx, id)
	if err != nil {
		return err
	}
	if err := s.profitRepo.DeleteSubscriptionCycle(ctx, id); err != nil {
		return err
	}
	return s.syncAccountExpiresAtFromCycles(ctx, cycle.AccountID)
}

func (s *ProfitService) CreateSubscriptionTermination(ctx context.Context, termination *AccountSubscriptionTermination, initialRefund *AccountSubscriptionRefund) (*SubscriptionCycleSettlementResult, error) {
	if termination == nil || termination.CycleID <= 0 || termination.EffectiveAt.IsZero() {
		return nil, fmt.Errorf("invalid subscription termination")
	}
	now := time.Now()
	if termination.EffectiveAt.After(now) {
		return nil, fmt.Errorf("termination effective_at cannot be in the future")
	}
	if termination.Reason == "" {
		termination.Reason = "upstream_banned"
	}
	if initialRefund != nil && initialRefund.Amount > 0 {
		if initialRefund.ReceivedAt.IsZero() {
			initialRefund.ReceivedAt = termination.EffectiveAt
		}
		if initialRefund.ReceivedAt.After(now) {
			return nil, fmt.Errorf("refund received_at cannot be in the future")
		}
	}
	writeResult, err := s.profitRepo.CreateSubscriptionTermination(ctx, termination, initialRefund)
	if err != nil {
		return nil, err
	}
	// 事务内的 outbox 保证最终同步；提交后再走账号仓储的快照同步，
	// 缩短封禁确认到调度缓存移除之间的窗口。
	if s.accountRepo != nil {
		for _, accountID := range writeResult.DisabledAccountIDs {
			_ = s.accountRepo.SetSchedulable(ctx, accountID, false)
		}
	}
	if err := s.syncAccountExpiresAtFromCycles(ctx, writeResult.Termination.AccountID); err != nil {
		return nil, err
	}
	cycle, err := s.loadSubscriptionCycle(ctx, writeResult.Termination.AccountID, writeResult.Termination.CycleID)
	if err != nil {
		return nil, err
	}
	return &SubscriptionCycleSettlementResult{Cycle: cycle, DisabledAccountIDs: writeResult.DisabledAccountIDs}, nil
}

func (s *ProfitService) PreviewSubscriptionTermination(ctx context.Context, cycleID int64, effectiveAt time.Time, initialRefundAmount float64) (*AccountSubscriptionLossSummary, error) {
	if cycleID <= 0 || effectiveAt.IsZero() || effectiveAt.After(time.Now()) || initialRefundAmount < 0 {
		return nil, fmt.Errorf("invalid subscription termination preview")
	}
	cycle, err := s.profitRepo.GetSubscriptionCycle(ctx, cycleID)
	if err != nil {
		return nil, err
	}
	cycleEnd := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
	if effectiveAt.Before(cycle.StartsAt) || effectiveAt.After(cycleEnd) {
		return nil, fmt.Errorf("termination effective_at must be inside the subscription cycle")
	}
	if subscriptionRefundTotal(cycle)+initialRefundAmount > cycle.PeriodFee+0.00000001 {
		return nil, ErrSubscriptionRefundExceedsFee
	}
	preview := *cycle
	preview.Termination = &AccountSubscriptionTermination{CycleID: cycle.ID, AccountID: cycle.AccountID, EffectiveAt: effectiveAt, Reason: "upstream_banned"}
	preview.Refunds = append([]*AccountSubscriptionRefund(nil), cycle.Refunds...)
	if initialRefundAmount > 0 {
		preview.Refunds = append(preview.Refunds, &AccountSubscriptionRefund{Amount: initialRefundAmount, ReceivedAt: effectiveAt})
	}
	revenues, err := s.profitRepo.GetSubscriptionCycleRevenueBatch(ctx, []SubscriptionCycleUsageRange{{
		CycleID: cycle.ID, AccountID: cycle.AccountID, Start: cycle.StartsAt, End: effectiveAt,
	}})
	if err != nil {
		return nil, err
	}
	return subscriptionLossSummary(&preview, revenues[cycle.ID]), nil
}

func (s *ProfitService) CreateSubscriptionRefund(ctx context.Context, refund *AccountSubscriptionRefund) (*SubscriptionCycleSettlementResult, error) {
	if refund == nil || refund.TerminationID <= 0 || refund.Amount <= 0 || refund.ReceivedAt.IsZero() {
		return nil, fmt.Errorf("invalid subscription refund")
	}
	if refund.ReceivedAt.After(time.Now()) {
		return nil, fmt.Errorf("refund received_at cannot be in the future")
	}
	created, err := s.profitRepo.CreateSubscriptionRefund(ctx, refund)
	if err != nil {
		return nil, err
	}
	cycle, err := s.loadSubscriptionCycle(ctx, created.AccountID, created.CycleID)
	if err != nil {
		return nil, err
	}
	return &SubscriptionCycleSettlementResult{Cycle: cycle}, nil
}

func (s *ProfitService) VoidSubscriptionRefund(ctx context.Context, id int64, reason string) (*SubscriptionCycleSettlementResult, error) {
	if id <= 0 || reason == "" {
		return nil, fmt.Errorf("refund id and void reason are required")
	}
	refund, err := s.profitRepo.VoidSubscriptionRefund(ctx, id, reason, time.Now())
	if err != nil {
		return nil, err
	}
	cycle, err := s.loadSubscriptionCycle(ctx, refund.AccountID, refund.CycleID)
	if err != nil {
		return nil, err
	}
	return &SubscriptionCycleSettlementResult{Cycle: cycle}, nil
}

func (s *ProfitService) ReverseSubscriptionTermination(ctx context.Context, id int64, reason string) (*SubscriptionCycleSettlementResult, error) {
	if id <= 0 || reason == "" {
		return nil, fmt.Errorf("termination id and reversal reason are required")
	}
	termination, err := s.profitRepo.ReverseSubscriptionTermination(ctx, id, reason, time.Now())
	if err != nil {
		return nil, err
	}
	if err := s.syncAccountExpiresAtFromCycles(ctx, termination.AccountID); err != nil {
		return nil, err
	}
	cycle, err := s.loadSubscriptionCycle(ctx, termination.AccountID, termination.CycleID)
	if err != nil {
		return nil, err
	}
	return &SubscriptionCycleSettlementResult{Cycle: cycle}, nil
}

func (s *ProfitService) loadSubscriptionCycle(ctx context.Context, accountID, cycleID int64) (*AccountSubscriptionCycle, error) {
	cycles, err := s.profitRepo.ListSubscriptionCycles(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if err := s.populateSubscriptionLossSummaries(ctx, cycles); err != nil {
		return nil, err
	}
	for _, cycle := range cycles {
		if cycle.ID == cycleID {
			return cycle, nil
		}
	}
	return nil, ErrSubscriptionCycleNotFound
}

func (s *ProfitService) populateSubscriptionLossSummaries(ctx context.Context, cycles []*AccountSubscriptionCycle) error {
	ranges := make([]SubscriptionCycleUsageRange, 0, len(cycles))
	for _, cycle := range cycles {
		termination := activeCycleTermination(cycle)
		if termination == nil {
			continue
		}
		end := termination.EffectiveAt
		cycleEnd := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
		if end.After(cycleEnd) {
			end = cycleEnd
		}
		if end.After(cycle.StartsAt) {
			ranges = append(ranges, SubscriptionCycleUsageRange{CycleID: cycle.ID, AccountID: cycle.AccountID, Start: cycle.StartsAt, End: end})
		}
	}
	revenues, err := s.profitRepo.GetSubscriptionCycleRevenueBatch(ctx, ranges)
	if err != nil {
		return err
	}
	for _, cycle := range cycles {
		if activeCycleTermination(cycle) != nil {
			cycle.LossSummary = subscriptionLossSummary(cycle, revenues[cycle.ID])
		}
	}
	return nil
}

// isSubscriptionAccountType auth 登陆类账号（OAuth / SetupToken）均为订阅制。
func isSubscriptionAccountType(accountType string) bool {
	return accountType == AccountTypeOAuth || accountType == AccountTypeSetupToken
}

func costTypeForAccountType(accountType string) string {
	if isSubscriptionAccountType(accountType) {
		return AccountCostTypeSubscription
	}
	return AccountCostTypeMetered
}

// BatchUpsertCostConfigResult 批量配置结果。
type BatchUpsertCostConfigResult struct {
	Updated    int     `json:"updated"`
	AccountIDs []int64 `json:"account_ids"`
}

// BatchUpsertSubscriptionConfigs 批量为未配置的订阅类（OAuth/SetupToken）账号绑定订阅费用。
// 已绑定配置的账号跳过，避免覆盖人工调整。
func (s *ProfitService) BatchUpsertSubscriptionConfigs(ctx context.Context, fee float64, periodDays int, currency string) (*BatchUpsertCostConfigResult, error) {
	if fee <= 0 {
		return nil, fmt.Errorf("period fee must be positive")
	}
	if periodDays <= 0 {
		periodDays = 30
	}
	if currency == "" {
		currency = "USD"
	}
	accounts, _, err := s.accountRepo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 10000}, "", "", "", "", 0, "")
	if err != nil {
		return nil, err
	}
	configs, err := s.profitRepo.ListCostConfigs(ctx)
	if err != nil {
		return nil, err
	}
	configured := make(map[int64]bool, len(configs))
	for _, c := range configs {
		configured[c.AccountID] = true
	}
	pending := make([]*AccountCostConfig, 0)
	for i := range accounts {
		acc := &accounts[i]
		if !isSubscriptionAccountType(acc.Type) || configured[acc.ID] {
			continue
		}
		pending = append(pending, &AccountCostConfig{
			AccountID:  acc.ID,
			CostType:   AccountCostTypeSubscription,
			PeriodFee:  fee,
			PeriodDays: periodDays,
			Currency:   currency,
		})
	}
	inserted, err := s.profitRepo.InsertCostConfigsIfAbsent(ctx, pending)
	if err != nil {
		return nil, err
	}
	return &BatchUpsertCostConfigResult{Updated: len(inserted), AccountIDs: inserted}, nil
}

func roundMoney(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func fillProfitQuotaWindows(summary *AccountProfitSummary, acc *Account, now time.Time, cutoff ...*time.Time) {
	if summary == nil || acc == nil {
		return
	}
	windows := make([]ProfitQuotaWindow, 0, 4)

	// OpenAI / Codex rolling windows from extra.
	if w := profitWindowFromExtra(acc.Extra, "codex_5h", "5h", "5h", 300, now); w != nil {
		windows = append(windows, *w)
		summary.FiveHourUtilization = w.UsedPercent
	}
	if w := profitWindowFromExtra(acc.Extra, "codex_7d", "7d", "7d", 10080, now); w != nil {
		windows = append(windows, *w)
		summary.SevenDayUtilization = w.UsedPercent
	}

	// Kimi official rolling windows.
	if w := profitWindowFromKimi(acc.Extra, "5h", "5h", 300, now); w != nil {
		if summary.FiveHourUtilization == nil {
			summary.FiveHourUtilization = w.UsedPercent
		}
		// Avoid duplicate 5h if codex already filled (shouldn't happen cross-platform).
		if !hasProfitWindowKind(windows, "5h") {
			windows = append(windows, *w)
		}
	}
	if w := profitWindowFromKimi(acc.Extra, "7d", "7d", 10080, now); w != nil {
		if summary.SevenDayUtilization == nil {
			summary.SevenDayUtilization = w.UsedPercent
		}
		if !hasProfitWindowKind(windows, "7d") {
			windows = append(windows, *w)
		}
	}

	// Grok quota window: prefer billing period; Free 24h fallback only for
	// official xAI hosts (not third-party apikey relays).
	if w := profitWindowFromGrok(acc, now); w != nil {
		windows = append(windows, *w)
	}

	// Claude / Anthropic session window columns.
	if acc.SessionWindowStart != nil && acc.SessionWindowEnd != nil && !acc.SessionWindowEnd.IsZero() {
		start := acc.SessionWindowStart.UTC()
		end := acc.SessionWindowEnd.UTC()
		if end.After(start) {
			mins := int(end.Sub(start).Minutes())
			w := ProfitQuotaWindow{
				ID:            "session",
				Label:         "session",
				Kind:          "session",
				StartAt:       &start,
				EndAt:         &end,
				WindowMinutes: &mins,
			}
			// Session itself has no used%; leave empty and still render the span.
			windows = append(windows, w)
		}
	}

	var recurringUntil *time.Time
	if len(cutoff) > 0 {
		recurringUntil = cutoff[0]
	}
	if recurringUntil != nil {
		capped := windows[:0]
		for i := range windows {
			w := windows[i]
			if w.StartAt != nil && !w.StartAt.Before(*recurringUntil) {
				continue
			}
			if w.EndAt != nil && w.EndAt.After(*recurringUntil) {
				end := *recurringUntil
				w.EndAt = &end
			}
			until := *recurringUntil
			w.RecurringUntilAt = &until
			if w.StartAt == nil || w.EndAt == nil || w.EndAt.After(*w.StartAt) {
				capped = append(capped, w)
			}
		}
		windows = capped
	}
	if len(windows) > 0 {
		summary.QuotaWindows = windows
	}
}

// quotaWindowCutoffForCycles returns the last instant at which future quota
// windows may be projected for a subscription account.
//
// Only active cycles (or confirmed ban terminations) apply a cutoff. A fully
// expired bookkeeping cycle must NOT wipe live upstream windows — e.g. Codex
// free still reports a 30-day rolling window after our cost cycle ended.
func quotaWindowCutoffForCycles(cycles []*AccountSubscriptionCycle, now time.Time) *time.Time {
	if active := activeSubscriptionCycle(cycles, now); active != nil && active.PeriodDays > 0 {
		end := active.StartsAt.AddDate(0, 0, active.PeriodDays)
		if termination := activeCycleTermination(active); termination != nil && termination.EffectiveAt.Before(end) {
			end = termination.EffectiveAt
		}
		return &end
	}
	// Banned accounts: stop projecting after the ban effective time.
	for _, cycle := range cycles {
		if termination := activeCycleTermination(cycle); termination != nil && !now.Before(termination.EffectiveAt) {
			end := termination.EffectiveAt
			return &end
		}
	}
	return nil
}

func hasProfitWindowKind(windows []ProfitQuotaWindow, kind string) bool {
	for _, w := range windows {
		if w.Kind == kind {
			return true
		}
	}
	return false
}

func profitWindowFromExtra(extra map[string]any, prefix, id, kind string, defaultMinutes int, now time.Time) *ProfitQuotaWindow {
	used := extraFloat(extra, prefix+"_used_percent")
	resetAt := extraTime(extra, prefix+"_reset_at")
	resetAfter := extraInt(extra, prefix+"_reset_after_seconds")
	windowMinutes := extraInt(extra, prefix+"_window_minutes")
	if windowMinutes == nil || *windowMinutes <= 0 {
		if defaultMinutes > 0 {
			m := defaultMinutes
			windowMinutes = &m
		}
	}
	if used == nil && resetAt == nil && resetAfter == nil {
		return nil
	}
	end := resolveProfitWindowEnd(resetAt, resetAfter, now)
	start := resolveProfitWindowStart(end, windowMinutes)
	mins := defaultMinutes
	if windowMinutes != nil && *windowMinutes > 0 {
		mins = *windowMinutes
	}
	resolvedKind, label := classifyQuotaWindow(kind, mins)
	resolvedID := id
	if resolvedKind != kind && (id == kind || id == "") {
		resolvedID = resolvedKind
	}
	return &ProfitQuotaWindow{
		ID:            resolvedID,
		Label:         label,
		Kind:          resolvedKind,
		UsedPercent:   used,
		StartAt:       start,
		EndAt:         end,
		WindowMinutes: windowMinutes,
	}
}

// classifyQuotaWindow 根据实际窗口分钟数生成展示标签，并在明显偏离默认 kind 时重分类。
// 例如 Codex free 长窗可能是 30 天（43200 分钟），不能继续标成 7d。
func classifyQuotaWindow(defaultKind string, minutes int) (kind, label string) {
	label = formatQuotaWindowLabel(minutes)
	if minutes <= 0 {
		return defaultKind, defaultKind
	}
	switch {
	case almostQuotaMinutes(minutes, 300):
		return "5h", label
	case almostQuotaMinutes(minutes, 1440):
		return "24h", label
	case almostQuotaMinutes(minutes, 10080):
		return "7d", label
	case minutes >= 20*24*60: // ≥20 天视作月度滚动窗
		return "30d", label
	case minutes >= 6*24*60 && minutes <= 10*24*60:
		return "7d", label
	default:
		if defaultKind != "" {
			return defaultKind, label
		}
		return "other", label
	}
}

func formatQuotaWindowLabel(minutes int) string {
	if minutes <= 0 {
		return "window"
	}
	if minutes < 90 {
		return fmt.Sprintf("%dm", minutes)
	}
	if minutes < 36*60 {
		h := float64(minutes) / 60.0
		if almostEqualFloat(h, math.Round(h), 0.05) {
			return fmt.Sprintf("%dh", int(math.Round(h)))
		}
		return fmt.Sprintf("%.1fh", h)
	}
	days := float64(minutes) / (24.0 * 60.0)
	if almostEqualFloat(days, math.Round(days), 0.05) {
		return fmt.Sprintf("%dd", int(math.Round(days)))
	}
	return fmt.Sprintf("%.1fd", days)
}

func almostQuotaMinutes(got, want int) bool {
	if want <= 0 {
		return false
	}
	delta := got - want
	if delta < 0 {
		delta = -delta
	}
	// 允许约 5% 偏差（上游窗口偶发非整）
	return delta*20 <= want
}

func almostEqualFloat(a, b, eps float64) bool {
	if a > b {
		return a-b <= eps
	}
	return b-a <= eps
}

func profitWindowFromKimi(extra map[string]any, name, kind string, defaultMinutes int, now time.Time) *ProfitQuotaWindow {
	used := extraFloat(extra, "kimi_quota_"+name+"_utilization")
	resetAt := extraTime(extra, "kimi_quota_"+name+"_reset_at")
	if used == nil && resetAt == nil {
		return nil
	}
	m := defaultMinutes
	end := resolveProfitWindowEnd(resetAt, nil, now)
	start := resolveProfitWindowStart(end, &m)
	return &ProfitQuotaWindow{
		ID:            "kimi-" + name,
		Label:         kind,
		Kind:          kind,
		UsedPercent:   used,
		StartAt:       start,
		EndAt:         end,
		WindowMinutes: &m,
	}
}

func profitWindowFromGrok(acc *Account, now time.Time) *ProfitQuotaWindow {
	if acc == nil || acc.Extra == nil {
		return nil
	}
	// Prefer official billing period (weekly/monthly product quota). This is what
	// the management UI shows as GrokBuild/GrokChat windows. Rate-limit header
	// snapshots often lack reset_at and cannot be placed on a timeline.
	if w := profitWindowFromGrokBilling(acc.Extra, now); w != nil {
		return w
	}
	// Free-tier 24h inference is only meaningful for official xAI Free traffic.
	// Third-party apikey relays (e.g. biuapi.com) may echo limit=1e6 rate-limit
	// headers that are not xAI's Free rolling window — inventing a 24h bar there
	// mislabels the account.
	return profitWindowFromGrokUsageSnapshot(acc.Extra, now, grokQuotaSourceIsOfficial(acc))
}

// grokQuotaSourceIsOfficial reports whether this account's Grok traffic targets
// an official xAI host (api.x.ai / regional / CLI gateway). Empty base_url is
// treated as official because GetGrokBaseURL defaults there.
func grokQuotaSourceIsOfficial(acc *Account) bool {
	if acc == nil || !acc.IsGrok() {
		return false
	}
	return xai.IsOfficialBaseURL(acc.GetGrokBaseURL())
}

func profitWindowFromGrokBilling(extra map[string]any, now time.Time) *ProfitQuotaWindow {
	billing := asStringAnyMap(firstNonNil(
		extra["grok_billing_snapshot"],
		extra["grok_billing"],
	))
	if billing == nil {
		return nil
	}

	startAt := anyToTimeFlexible(firstNonNil(billing["period_start"], billing["billing_period_start"]))
	endAt := anyToTimeFlexible(firstNonNil(billing["period_end"], billing["billing_period_end"]))
	if startAt == nil && endAt == nil {
		return nil
	}

	used := anyToFloat64(firstNonNil(billing["usage_percent"], billing["used_percent"]))
	// Prefer product-specific percent when present (GrokBuild matches the screenshot).
	if products, ok := billing["product_usage"].([]any); ok {
		for _, raw := range products {
			item := asStringAnyMap(raw)
			if item == nil {
				continue
			}
			product := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["product"])))
			if product == "" {
				continue
			}
			if p := anyToFloat64(item["usage_percent"]); p != nil {
				if strings.Contains(product, "grokbuild") || strings.Contains(product, "build") {
					used = p
					break
				}
				if used == nil {
					used = p
				}
			}
		}
	}

	kind := "7d"
	label := "7d"
	periodType := strings.ToLower(strings.TrimSpace(fmt.Sprint(billing["period_type"])))
	switch {
	case strings.Contains(periodType, "month"):
		kind = "other"
		label = "30d"
	case strings.Contains(periodType, "week"):
		kind = "7d"
		label = "7d"
	case startAt != nil && endAt != nil:
		hours := endAt.Sub(*startAt).Hours()
		if hours >= 20*24 && hours <= 40*24 {
			kind = "other"
			label = "30d"
		} else if hours >= 5*24 && hours <= 10*24 {
			kind = "7d"
			label = "7d"
		} else if hours >= 20 && hours <= 30 {
			kind = "24h"
			label = "24h"
		}
	}

	var windowMinutes *int
	if startAt != nil && endAt != nil && endAt.After(*startAt) {
		m := int(endAt.Sub(*startAt).Minutes())
		if m > 0 {
			windowMinutes = &m
		}
	} else if endAt != nil && windowMinutes == nil {
		// Fallback duration for weekly billing when only end is known.
		m := 7 * 24 * 60
		windowMinutes = &m
		startAt = resolveProfitWindowStart(endAt, windowMinutes)
	} else if startAt != nil && endAt == nil && windowMinutes == nil {
		m := 7 * 24 * 60
		windowMinutes = &m
		tmp := startAt.Add(time.Duration(m) * time.Minute)
		endAt = &tmp
	}

	if used == nil && startAt == nil && endAt == nil {
		return nil
	}
	return &ProfitQuotaWindow{
		ID:            "grok-billing",
		Label:         label,
		Kind:          kind,
		UsedPercent:   used,
		StartAt:       startAt,
		EndAt:         endAt,
		WindowMinutes: windowMinutes,
	}
}

func profitWindowFromGrokUsageSnapshot(extra map[string]any, now time.Time, allowFree24hFallback bool) *ProfitQuotaWindow {
	m := asStringAnyMap(extra["grok_usage_snapshot"])
	if m == nil {
		return nil
	}
	tokens := asStringAnyMap(m["tokens"])
	requests := asStringAnyMap(m["requests"])
	if tokens == nil && requests == nil {
		return nil
	}

	source := tokens
	id := "grok-tokens"
	if source == nil {
		source = requests
		id = "grok-requests"
	}

	var used *float64
	limit := anyToInt64(source["limit"])
	remaining := anyToInt64(source["remaining"])
	if limit != nil && *limit > 0 && remaining != nil {
		u := (1.0 - float64(*remaining)/float64(*limit)) * 100
		if u < 0 {
			u = 0
		}
		if u > 100 {
			u = 100
		}
		used = &u
	}
	resetAt := anyToTimeFlexible(source["reset_at"])
	if resetAt == nil {
		if ru := anyToInt64(source["reset_unix"]); ru != nil && *ru > 0 {
			tt := time.Unix(*ru, 0).UTC()
			resetAt = &tt
		}
	}
	// No absolute reset: free-tier rolling token windows are ~24h. Anchor end at
	// next 24h boundary from last observation when possible so the bar still renders.
	// Only invent this timeline for official xAI Free traffic — third-party relays
	// often advertise limit=1e6 without meaning Free's 24h rolling budget.
	if resetAt == nil {
		if !allowFree24hFallback || limit == nil || !xaiIsLikelyFreeRolling(*limit) {
			// Without a real reset (or Free-24h eligibility), do not invent a
			// multi-day timeline from rate-limit headers alone.
			return nil
		}
		observedAt := anyToTimeFlexible(firstNonNil(m["last_headers_seen_at"], m["updated_at"]))
		if observedAt == nil {
			observedAt = &now
		}
		tt := observedAt.UTC().Add(24 * time.Hour)
		resetAt = &tt
	}
	if used == nil && resetAt == nil {
		return nil
	}

	mins := 24 * 60
	if limit != nil && *limit > 0 && !xaiIsLikelyFreeRolling(*limit) {
		mins = 24 * 60
	}
	end := resolveProfitWindowEnd(resetAt, nil, now)
	start := resolveProfitWindowStart(end, &mins)
	return &ProfitQuotaWindow{
		ID:            id,
		Label:         "24h",
		Kind:          "24h",
		UsedPercent:   used,
		StartAt:       start,
		EndAt:         end,
		WindowMinutes: &mins,
	}
}

func asStringAnyMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[string]string:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out
	default:
		// Some DB drivers decode jsonb objects as map[any]any.
		if raw, ok := v.(map[any]any); ok {
			out := make(map[string]any, len(raw))
			for k, val := range raw {
				out[fmt.Sprint(k)] = val
			}
			return out
		}
	}
	return nil
}

func firstNonNil(values ...any) any {
	for _, v := range values {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		return v
	}
	return nil
}

func anyToFloat64(v any) *float64 {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		return &n
	case float32:
		f := float64(n)
		return &f
	case int:
		f := float64(n)
		return &f
	case int32:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return &f
		}
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return nil
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return &f
		}
	}
	return nil
}

func anyToTimeFlexible(v any) *time.Time {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case time.Time:
		t := n.UTC()
		return &t
	case *time.Time:
		if n == nil {
			return nil
		}
		t := n.UTC()
		return &t
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return nil
		}
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05.999999999Z07:00",
			"2006-01-02 15:04:05.999999999Z07:00",
			"2006-01-02 15:04:05Z07:00",
		}
		for _, layout := range layouts {
			if ts, err := time.Parse(layout, s); err == nil {
				t := ts.UTC()
				return &t
			}
		}
		// Some payloads omit timezone; assume UTC.
		if ts, err := time.ParseInLocation("2006-01-02T15:04:05.999999999", s, time.UTC); err == nil {
			t := ts.UTC()
			return &t
		}
		if ts, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.UTC); err == nil {
			t := ts.UTC()
			return &t
		}
	case float64:
		// unix seconds
		if n > 1e9 && n < 1e12 {
			tt := time.Unix(int64(n), 0).UTC()
			return &tt
		}
	case int64:
		if n > 1e9 && n < 1e12 {
			tt := time.Unix(n, 0).UTC()
			return &tt
		}
	}
	return nil
}

func xaiIsLikelyFreeRolling(limit int64) bool {
	switch limit {
	case 1_000_000, 2_000_000:
		return true
	default:
		return false
	}
}

func resolveProfitWindowEnd(resetAt *time.Time, resetAfterSeconds *int, now time.Time) *time.Time {
	if resetAt != nil && !resetAt.IsZero() {
		t := resetAt.UTC()
		return &t
	}
	if resetAfterSeconds != nil && *resetAfterSeconds > 0 {
		t := now.UTC().Add(time.Duration(*resetAfterSeconds) * time.Second)
		return &t
	}
	return nil
}

func resolveProfitWindowStart(end *time.Time, windowMinutes *int) *time.Time {
	if end == nil || windowMinutes == nil || *windowMinutes <= 0 {
		return nil
	}
	t := end.Add(-time.Duration(*windowMinutes) * time.Minute)
	return &t
}

func extraTime(extra map[string]any, key string) *time.Time {
	if extra == nil {
		return nil
	}
	v, ok := extra[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return nil
		}
		if ts, err := time.Parse(time.RFC3339, s); err == nil {
			t := ts.UTC()
			return &t
		}
		if ts, err := time.Parse(time.RFC3339Nano, s); err == nil {
			t := ts.UTC()
			return &t
		}
	case time.Time:
		t := n.UTC()
		return &t
	case *time.Time:
		if n == nil {
			return nil
		}
		t := n.UTC()
		return &t
	}
	return nil
}

func extraInt(extra map[string]any, key string) *int {
	if extra == nil {
		return nil
	}
	v, ok := extra[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case int:
		return &n
	case int32:
		i := int(n)
		return &i
	case int64:
		i := int(n)
		return &i
	case float64:
		i := int(n)
		return &i
	case float32:
		i := int(n)
		return &i
	}
	return nil
}

func anyToInt64(v any) *int64 {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case int:
		i := int64(n)
		return &i
	case int32:
		i := int64(n)
		return &i
	case int64:
		return &n
	case float64:
		i := int64(n)
		return &i
	case float32:
		i := int64(n)
		return &i
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return &i
		}
	}
	return nil
}

func anyToTime(v any) *time.Time {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return nil
		}
		if ts, err := time.Parse(time.RFC3339, s); err == nil {
			t := ts.UTC()
			return &t
		}
	case time.Time:
		t := n.UTC()
		return &t
	}
	return nil
}

func extraFloat(extra map[string]any, key string) *float64 {
	if extra == nil {
		return nil
	}
	v, ok := extra[key]
	if !ok || v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		return &n
	case float32:
		f := float64(n)
		return &f
	case int:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	}
	return nil
}
