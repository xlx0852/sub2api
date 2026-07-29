package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
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

// AccountProfitSummary 单账号周期利润汇总。
type AccountProfitSummary struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	Platform    string `json:"platform"`
	AccountType string `json:"account_type"`
	CostType    string `json:"cost_type"`
	Configured  bool   `json:"configured"` // 是否已绑定成本配置

	Requests int64   `json:"requests"`
	Revenue  float64 `json:"revenue"` // SUM(actual_cost)
	Cost     float64 `json:"cost"`
	Profit   float64 `json:"profit"`
	Margin   float64 `json:"margin"` // 利润 / 收入；无收入时为 0

	// 订阅账号窗口数据（当前快照，来自 accounts.extra 的 codex_5h/7d）
	FiveHourUtilization *float64 `json:"five_hour_utilization,omitempty"`
	SevenDayUtilization *float64 `json:"seven_day_utilization,omitempty"`

	// 窗口变现效率 = 周期内平均每 5h 窗口收入 / 基准窗口收入。
	// 基准来自 window_baseline_revenue 或历史最佳 5h 窗口自动学习；无基准时为空。
	WindowEfficiency     *float64 `json:"window_efficiency,omitempty"`
	WindowBaselineSource string   `json:"window_baseline_source,omitempty"` // "configured" / "learned" / ""

	// 当前计费窗口（人工本期起始日优先，subscription_expires_at 回退）
	BillingWindowStart    *time.Time `json:"billing_window_start,omitempty"`
	BillingWindowEnd      *time.Time `json:"billing_window_end,omitempty"`
	BillingWindowProgress *float64   `json:"billing_window_progress,omitempty"` // 0-100，窗口已过比例
	BillingWindowRevenue  *float64   `json:"billing_window_revenue,omitempty"`  // 窗口内至今收入
	BillingWindowCost     *float64   `json:"billing_window_cost,omitempty"`     // 窗口成本 = 整期订阅费
	BillingWindowProfit   *float64   `json:"billing_window_profit,omitempty"`
	BillingWindowSource   string     `json:"billing_window_source,omitempty"` // manual / subscription_expiry
	RequiresCycleStart    bool       `json:"requires_cycle_start,omitempty"`

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

type profitAccountingData struct {
	configsByAccount          map[int64]*AccountCostConfig
	cyclesByAccount           map[int64][]*AccountSubscriptionCycle
	bestWindowByAccount       map[int64]float64
	windowStatsByAccount      map[int64]*ProfitUsageStats
	now                       time.Time
	includeOperationalMetrics bool
}

// GetSummary 计算 [start,end) 内所有账号的利润汇总。
func (s *ProfitService) GetSummary(ctx context.Context, start, end time.Time) (*ProfitSummaryResponse, error) {
	if end.Before(start) {
		start, end = end, start
	}
	accounts, _, err := s.accountRepo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 10000}, "", "", "", "", 0, "")
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
	account, err := s.accountRepo.GetByID(ctx, accountID)
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
	accounting, err := s.loadProfitAccountingData(ctx, accounts, true)
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
			Requests:    stats.Requests,
			Revenue:     roundMoney(stats.Revenue),
			Currency:    "USD",
		}
		// 窗口利用率快照（OpenAI 订阅号）
		summary.FiveHourUtilization = extraFloat(acc.Extra, "codex_5h_used_percent")
		summary.SevenDayUtilization = extraFloat(acc.Extra, "codex_7d_used_percent")

		cost := stats.MeteredCost
		if costType == AccountCostTypeSubscription {
			cost = 0
			cycles := accounting.cyclesByAccount[acc.ID]
			if len(cycles) == 0 {
				summary.RequiresCycleStart = true
			} else {
				summary.Configured = true
				cost = subscriptionCyclesCostForRange(cycles, start, end)
				if accounting.includeOperationalMetrics {
					if activeCycle := activeSubscriptionCycle(cycles, accounting.now); activeCycle != nil {
						summary.Currency = activeCycle.Currency
						windowRevenue := 0.0
						if windowStats := accounting.windowStatsByAccount[acc.ID]; windowStats != nil {
							windowRevenue = windowStats.Revenue
						}
						fillBillingWindowFromRevenue(activeCycle, summary, windowRevenue, accounting.now)
					}
				}
			}
			if accounting.includeOperationalMetrics {
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

func (s *ProfitService) loadProfitAccountingData(ctx context.Context, accounts []Account, includeOperationalMetrics bool) (*profitAccountingData, error) {
	data := &profitAccountingData{
		configsByAccount:          make(map[int64]*AccountCostConfig),
		cyclesByAccount:           make(map[int64][]*AccountSubscriptionCycle),
		bestWindowByAccount:       make(map[int64]float64),
		windowStatsByAccount:      make(map[int64]*ProfitUsageStats),
		now:                       time.Now(),
		includeOperationalMetrics: includeOperationalMetrics,
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
	if !includeOperationalMetrics {
		return data, nil
	}
	data.bestWindowByAccount, err = s.profitRepo.GetBestWindowRevenueBatch(ctx, subscriptionIDs, data.now.Add(-30*24*time.Hour), 5*3600)
	if err != nil {
		return nil, err
	}

	ranges := make([]ProfitAccountUsageRange, 0, len(subscriptionIDs))
	for _, accountID := range subscriptionIDs {
		if cycle := activeSubscriptionCycle(data.cyclesByAccount[accountID], data.now); cycle != nil {
			end := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
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
	return data, nil
}

// subscriptionCostForRange 订阅费按查询区间与当前付费周期的交集摊销。
// 缺少可靠周期锚点时保留旧的范围摊销，以便管理员补录起始日之前仍能查看趋势。
func subscriptionCycleCostForRange(cycle *AccountSubscriptionCycle, start, end time.Time) float64 {
	if cycle.PeriodFee <= 0 || cycle.PeriodDays <= 0 {
		return 0
	}
	cycleEnd := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
	if start.Before(cycle.StartsAt) {
		start = cycle.StartsAt
	}
	if end.After(cycleEnd) {
		end = cycleEnd
	}
	days := end.Sub(start).Hours() / 24
	if days <= 0 {
		return 0
	}
	return cycle.PeriodFee * days / float64(cycle.PeriodDays)
}

func subscriptionCyclesCostForRange(cycles []*AccountSubscriptionCycle, start, end time.Time) float64 {
	cost := 0.0
	for _, cycle := range cycles {
		cost += subscriptionCycleCostForRange(cycle, start, end)
	}
	return cost
}

func activeSubscriptionCycle(cycles []*AccountSubscriptionCycle, now time.Time) *AccountSubscriptionCycle {
	for _, cycle := range cycles {
		end := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
		if !now.Before(cycle.StartsAt) && now.Before(end) {
			return cycle
		}
	}
	return nil
}

// fillBillingWindow 计算当前有效充值周期数据。
// 窗口内收入取 [窗口开始, min(now, 窗口结束)] 的 actual_cost 汇总。
func (s *ProfitService) fillBillingWindow(ctx context.Context, accountID int64, cycle *AccountSubscriptionCycle, summary *AccountProfitSummary) {
	wStart := cycle.StartsAt
	wEnd := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
	now := time.Now()

	// 窗口内收入：窗口开始到 min(now, 窗口结束)
	effectiveEnd := wEnd
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

func fillBillingWindowFromRevenue(cycle *AccountSubscriptionCycle, summary *AccountProfitSummary, revenue float64, now time.Time) {
	wStart := cycle.StartsAt
	wEnd := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
	periodDays := cycle.PeriodDays
	revenue = roundMoney(revenue)
	progress := now.Sub(wStart).Hours() / (float64(periodDays) * 24) * 100
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
		account, getErr := s.accountRepo.GetByID(ctx, *accountID)
		if getErr != nil {
			return nil, getErr
		}
		if account != nil && isSubscriptionAccountType(account.Type) {
			subscriptionIDs = append(subscriptionIDs, account.ID)
		}
	} else if s.accountRepo != nil {
		accounts, _, listErr := s.accountRepo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 10000}, "", "", "", "", 0, "")
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
	return buildProfitTrend(points, subscriptionCycles, tzName), nil
}

func buildProfitTrend(points []*ProfitDailyUsagePoint, subscriptionCycles []*AccountSubscriptionCycle, tzName string) *ProfitTrendResponse {
	resp := &ProfitTrendResponse{}
	location, _ := time.LoadLocation(tzName)
	if location == nil {
		location = time.UTC
	}
	for _, p := range points {
		dayStart, parseErr := time.ParseInLocation(time.DateOnly, p.Date, location)
		if parseErr != nil {
			continue
		}
		dayEnd := dayStart.AddDate(0, 0, 1)
		dailySubscriptionCost := 0.0
		for _, cycle := range subscriptionCycles {
			dailySubscriptionCost += subscriptionCycleCostForRange(cycle, dayStart, dayEnd)
		}
		cost := p.MeteredCost + dailySubscriptionCost
		resp.Points = append(resp.Points, &ProfitTrendPoint{
			Date:    p.Date,
			Revenue: roundMoney(p.Revenue),
			Cost:    roundMoney(cost),
			Profit:  roundMoney(p.Revenue - cost),
		})
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
	accounts, _, err := s.accountRepo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 10000}, "", "", "", "", 0, "")
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
	// 全局页只需要区间财务数据；最佳 5h 窗口和当前周期收入仅供账号抽屉展示。
	accounting, err := s.loadProfitAccountingData(ctx, accounts, false)
	if err != nil {
		return nil, err
	}
	summary := s.summarizeAccountsWithData(accounts, start, end, statsByAccount, accounting)
	cycles := make([]*AccountSubscriptionCycle, 0)
	for _, accountCycles := range accounting.cyclesByAccount {
		cycles = append(cycles, accountCycles...)
	}
	trend := buildProfitTrend(dailyPoints, cycles, tzName)
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

func (s *ProfitService) DeleteCostConfig(ctx context.Context, accountID int64) error {
	return s.profitRepo.DeleteCostConfig(ctx, accountID)
}

type SubscriptionCycleList struct {
	Cycles                []*AccountSubscriptionCycle `json:"cycles"`
	SubscriptionExpiresAt *time.Time                  `json:"subscription_expires_at,omitempty"`
	OAuthTokenExpiresAt   *time.Time                  `json:"oauth_token_expires_at,omitempty"`
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
	return &SubscriptionCycleList{
		Cycles:                cycles,
		SubscriptionExpiresAt: account.GetCredentialAsTime("subscription_expires_at"),
		OAuthTokenExpiresAt:   account.GetCredentialAsTime("expires_at"),
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
	return s.profitRepo.CreateSubscriptionCycle(ctx, cycle)
}

func (s *ProfitService) DeleteSubscriptionCycle(ctx context.Context, id int64) error {
	return s.profitRepo.DeleteSubscriptionCycle(ctx, id)
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
