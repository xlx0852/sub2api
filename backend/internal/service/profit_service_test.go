package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

func TestAccountCostConfigJSONContract(t *testing.T) {
	payload, err := json.Marshal(&AccountCostConfig{ID: 7, AccountID: 91, CostType: AccountCostTypeSubscription})
	if err != nil {
		t.Fatalf("marshal cost config: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal cost config: %v", err)
	}
	if decoded["account_id"] != float64(91) || decoded["cost_type"] != AccountCostTypeSubscription {
		t.Fatalf("unexpected JSON contract: %s", payload)
	}
	if _, legacy := decoded["AccountID"]; legacy {
		t.Fatalf("legacy field name leaked into JSON: %s", payload)
	}
}

type profitRepoStub struct {
	configs          []*AccountCostConfig
	cycles           map[int64][]*AccountSubscriptionCycle
	stats            map[int64]*ProfitUsageStats
	rangeStats       map[int64]*ProfitUsageStats
	daily            []*ProfitDailyUsagePoint
	dailyByAccount   []*ProfitAccountDailyUsagePoint
	bestWindow       float64
	bestWindows      map[int64]float64
	upserted         *AccountCostConfig
	batchInserted    []*AccountCostConfig
	onUpsert         func(*AccountCostConfig)
	listConfigsCalls int
	cycleBatchCalls  int
	rangeStatsCalls  int
	dailyBatchCalls  int
	bestBatchCalls   int
	statsBatchCalls  int
}

func (s *profitRepoStub) UpsertCostConfig(_ context.Context, cfg *AccountCostConfig) (*AccountCostConfig, error) {
	s.upserted = cfg
	if s.onUpsert != nil {
		s.onUpsert(cfg)
	}
	return cfg, nil
}
func (s *profitRepoStub) GetCostConfig(_ context.Context, accountID int64) (*AccountCostConfig, error) {
	for _, c := range s.configs {
		if c.AccountID == accountID {
			return c, nil
		}
	}
	return nil, nil
}
func (s *profitRepoStub) ListCostConfigs(context.Context) ([]*AccountCostConfig, error) {
	s.listConfigsCalls++
	return s.configs, nil
}
func (s *profitRepoStub) InsertCostConfigsIfAbsent(_ context.Context, configs []*AccountCostConfig) ([]int64, error) {
	s.batchInserted = append(s.batchInserted, configs...)
	ids := make([]int64, 0, len(configs))
	for _, cfg := range configs {
		ids = append(ids, cfg.AccountID)
	}
	return ids, nil
}
func (s *profitRepoStub) DeleteCostConfig(context.Context, int64) error { return nil }
func (s *profitRepoStub) ListSubscriptionCycles(_ context.Context, accountID int64) ([]*AccountSubscriptionCycle, error) {
	return s.cycles[accountID], nil
}
func (s *profitRepoStub) ListSubscriptionCyclesBatch(_ context.Context, accountIDs []int64) ([]*AccountSubscriptionCycle, error) {
	s.cycleBatchCalls++
	cycles := make([]*AccountSubscriptionCycle, 0)
	for _, accountID := range accountIDs {
		cycles = append(cycles, s.cycles[accountID]...)
	}
	return cycles, nil
}
func (s *profitRepoStub) CreateSubscriptionCycle(_ context.Context, cycle *AccountSubscriptionCycle) (*AccountSubscriptionCycle, error) {
	if cycle.ID == 0 {
		cycle.ID = int64(len(s.cycles[cycle.AccountID]) + 1)
	}
	s.cycles[cycle.AccountID] = append(s.cycles[cycle.AccountID], cycle)
	return cycle, nil
}
func (s *profitRepoStub) DeleteSubscriptionCycle(_ context.Context, id int64) error { return nil }
func (s *profitRepoStub) GetAccountUsageStatsBatch(_ context.Context, _ []int64, _, _ time.Time) (map[int64]*ProfitUsageStats, error) {
	s.statsBatchCalls++
	return s.stats, nil
}
func (s *profitRepoStub) GetAccountUsageStatsForRanges(_ context.Context, _ []ProfitAccountUsageRange) (map[int64]*ProfitUsageStats, error) {
	s.rangeStatsCalls++
	if s.rangeStats != nil {
		return s.rangeStats, nil
	}
	return s.stats, nil
}
func (s *profitRepoStub) GetAccountDailyUsageStats(_ context.Context, _ []int64, _, _ time.Time, _ string) ([]*ProfitAccountDailyUsagePoint, error) {
	s.dailyBatchCalls++
	return s.dailyByAccount, nil
}
func (s *profitRepoStub) GetDailyUsageStats(_ context.Context, _ *int64, _, _ time.Time, _ string) ([]*ProfitDailyUsagePoint, error) {
	return s.daily, nil
}
func (s *profitRepoStub) GetBestWindowRevenue(context.Context, int64, time.Time, int64) (float64, error) {
	return s.bestWindow, nil
}
func (s *profitRepoStub) GetBestWindowRevenueBatch(_ context.Context, accountIDs []int64, _ time.Time, _ int64) (map[int64]float64, error) {
	s.bestBatchCalls++
	result := make(map[int64]float64, len(accountIDs))
	for _, accountID := range accountIDs {
		if s.bestWindows != nil {
			result[accountID] = s.bestWindows[accountID]
		} else {
			result[accountID] = s.bestWindow
		}
	}
	return result, nil
}

func TestProfitService_AmortizedSubscriptionCost(t *testing.T) {
	cycle := &AccountSubscriptionCycle{PeriodFee: 200, PeriodDays: 30}
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	cycle.StartsAt = start
	end := start.AddDate(0, 0, 15)
	got := subscriptionCycleCostForRange(cycle, start, end)
	if got != 100 {
		t.Fatalf("amortized = %v, want 100", got)
	}
	// 全周期 = 全额费用
	end = start.AddDate(0, 0, 30)
	if got := subscriptionCycleCostForRange(cycle, start, end); got != 200 {
		t.Fatalf("full period amortized = %v, want 200", got)
	}
	// 未配置费用 → 0
	if got := subscriptionCycleCostForRange(&AccountSubscriptionCycle{StartsAt: start, PeriodDays: 30}, start, end); got != 0 {
		t.Fatalf("zero fee amortized = %v, want 0", got)
	}
}

func TestSubscriptionCyclesDoNotChargeIdleGap(t *testing.T) {
	first := &AccountSubscriptionCycle{StartsAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), PeriodFee: 300, PeriodDays: 30}
	second := &AccountSubscriptionCycle{StartsAt: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), PeriodFee: 300, PeriodDays: 30}
	gapStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	gapEnd := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	if got := subscriptionCyclesCostForRange([]*AccountSubscriptionCycle{first, second}, gapStart, gapEnd); got != 0 {
		t.Fatalf("idle gap cost = %v, want 0", got)
	}
	activeStart := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	if got := subscriptionCyclesCostForRange([]*AccountSubscriptionCycle{first, second}, activeStart, activeStart.AddDate(0, 0, 10)); got != 100 {
		t.Fatalf("active cycle cost = %v, want 100", got)
	}
}

func TestProfitService_WindowEfficiency(t *testing.T) {
	// 配置基准 $50/5h 窗口，周期 5 天 = 24 个窗口，收入 $240 → 平均每窗口 $10 → 效率 20%
	svc := &ProfitService{profitRepo: &profitRepoStub{}}
	svc.profitRepo = &profitRepoStub{bestWindow: 50}
	eff, source := svc.windowEfficiency(context.Background(), 1, 240, 5)
	if eff == nil || *eff != 20 || source != "learned" {
		t.Fatalf("eff = %v source = %v, want 20 learned", eff, source)
	}
	// 无配置基准 → 自动学习
	svc = &ProfitService{profitRepo: &profitRepoStub{bestWindow: 40}}
	eff, source = svc.windowEfficiency(context.Background(), 1, 240, 5)
	if eff == nil || *eff != 25 || source != "learned" {
		t.Fatalf("eff = %v source = %v, want 25 learned", eff, source)
	}
	// 无收入 → nil
	eff, _ = svc.windowEfficiency(context.Background(), 1, 0, 5)
	if eff != nil {
		t.Fatalf("zero revenue eff = %v, want nil", *eff)
	}
}

func TestProfitService_GetTrendAddsSubscriptionAmortization(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	repo := &profitRepoStub{
		cycles: map[int64][]*AccountSubscriptionCycle{1: {{AccountID: 1, StartsAt: start, PeriodFee: 300, PeriodDays: 30}}},
		daily: []*ProfitDailyUsagePoint{
			// 仓储返回的 MeteredCost 只包含 API Key 账号，订阅账号成本由周期账本补一次。
			{Date: "2026-07-01", Revenue: 100, MeteredCost: 20},
		},
	}
	svc := &ProfitService{profitRepo: repo, accountRepo: &profitAccountRepoStub{accounts: []Account{{ID: 1, Type: AccountTypeOAuth}, {ID: 2, Type: AccountTypeAPIKey}}}}
	resp, err := svc.GetTrend(context.Background(), nil, start, start.AddDate(0, 0, 1), "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Points) != 1 {
		t.Fatalf("points = %d, want 1", len(resp.Points))
	}
	// 成本 = metered 20 + 订阅日摊 300/30=10 → 30；利润 = 100-30 = 70
	if resp.Points[0].Cost != 30 || resp.Points[0].Profit != 70 {
		t.Fatalf("cost=%v profit=%v, want 30/70", resp.Points[0].Cost, resp.Points[0].Profit)
	}
	// 按账号过滤时只算该账号的订阅摊销
	accID := int64(1)
	resp, err = svc.GetTrend(context.Background(), &accID, start, start.AddDate(0, 0, 1), "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Points[0].Cost != 30 {
		t.Fatalf("filtered cost=%v, want 30", resp.Points[0].Cost)
	}
}

func TestProfitService_GetTrendUsesAPIKeyCostPlusSubscriptionAmortizationOnce(t *testing.T) {
	start := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	repo := &profitRepoStub{
		cycles: map[int64][]*AccountSubscriptionCycle{
			1: {{AccountID: 1, StartsAt: start, PeriodFee: 214.30, PeriodDays: 1}},
		},
		daily: []*ProfitDailyUsagePoint{
			{Date: "2026-07-28", Revenue: 538.90, MeteredCost: 122.24},
		},
	}
	svc := &ProfitService{
		profitRepo: repo,
		accountRepo: &profitAccountRepoStub{accounts: []Account{
			{ID: 1, Type: AccountTypeOAuth},
			{ID: 2, Type: AccountTypeAPIKey},
		}},
	}

	resp, err := svc.GetTrend(context.Background(), nil, start, start.AddDate(0, 0, 1), "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Points) != 1 {
		t.Fatalf("points = %d, want 1", len(resp.Points))
	}
	point := resp.Points[0]
	if point.Cost != 336.54 || point.Profit != 202.36 {
		t.Fatalf("cost=%v profit=%v, want 336.54/202.36", point.Cost, point.Profit)
	}
}

func TestProfitService_GetOverviewUsesBoundedBatchQueries(t *testing.T) {
	start := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	cycleStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	repo := &profitRepoStub{
		cycles: map[int64][]*AccountSubscriptionCycle{
			1: {{AccountID: 1, StartsAt: cycleStart, PeriodFee: 300, PeriodDays: 30}},
			2: {{AccountID: 2, StartsAt: cycleStart, PeriodFee: 150, PeriodDays: 30}},
		},
		dailyByAccount: []*ProfitAccountDailyUsagePoint{
			{AccountID: 1, Date: "2026-07-28", Requests: 10, Revenue: 100},
			{AccountID: 2, Date: "2026-07-28", Requests: 5, Revenue: 50},
			{AccountID: 3, Date: "2026-07-28", Requests: 7, Revenue: 70, MeteredCost: 20},
		},
		rangeStats:  map[int64]*ProfitUsageStats{1: {Revenue: 500}, 2: {Revenue: 200}},
		bestWindows: map[int64]float64{1: 50, 2: 25},
	}
	accounts := []Account{
		{ID: 1, Type: AccountTypeOAuth},
		{ID: 2, Type: AccountTypeSetupToken},
		{ID: 3, Type: AccountTypeAPIKey},
	}
	for id := int64(4); id <= 100; id++ {
		accounts = append(accounts, Account{ID: id, Type: AccountTypeAPIKey})
	}
	svc := &ProfitService{
		profitRepo:  repo,
		accountRepo: &profitAccountRepoStub{accounts: accounts},
	}

	overview, err := svc.GetOverview(context.Background(), start, start.AddDate(0, 0, 1), "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if overview.Summary.TotalRevenue != 220 || overview.Summary.TotalCost != 35 || overview.Summary.TotalProfit != 185 {
		t.Fatalf("summary = %+v, want revenue/cost/profit 220/35/185", overview.Summary)
	}
	if len(overview.Points) != 1 || overview.Points[0].Cost != 35 || overview.Points[0].Profit != 185 {
		t.Fatalf("points = %+v, want cost/profit 35/185", overview.Points)
	}
	if repo.dailyBatchCalls != 1 || repo.listConfigsCalls != 1 || repo.cycleBatchCalls != 1 || repo.bestBatchCalls != 1 || repo.rangeStatsCalls != 1 {
		t.Fatalf("batch calls daily/config/cycle/best/range = %d/%d/%d/%d/%d, want all 1", repo.dailyBatchCalls, repo.listConfigsCalls, repo.cycleBatchCalls, repo.bestBatchCalls, repo.rangeStatsCalls)
	}
	if repo.statsBatchCalls != 0 {
		t.Fatalf("overview performed duplicate account stats query: %d", repo.statsBatchCalls)
	}
}

func TestProfitService_UpsertDefaults(t *testing.T) {
	repo := &profitRepoStub{}
	svc := &ProfitService{profitRepo: repo, accountRepo: &profitAccountRepoStub{byID: map[int64]*Account{
		9: {ID: 9, Type: AccountTypeAPIKey},
	}}}
	cfg, err := svc.UpsertCostConfig(context.Background(), &AccountCostConfig{AccountID: 9})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CostType != AccountCostTypeMetered || cfg.PeriodDays != 30 || cfg.Currency != "USD" {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
}

// profitAccountRepoStub 内嵌接口，仅实现 GetSummary/Batch 用到的 ListWithFilters。
type profitAccountRepoStub struct {
	AccountRepository
	accounts []Account
	byID     map[int64]*Account
}

func (m *profitAccountRepoStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _, _, _ string, _ int64, _ string) ([]Account, *pagination.PaginationResult, error) {
	return m.accounts, nil, nil
}

func (m *profitAccountRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	if account := m.byID[id]; account != nil {
		return account, nil
	}
	for i := range m.accounts {
		if m.accounts[i].ID == id {
			return &m.accounts[i], nil
		}
	}
	return nil, nil
}

func TestProfitService_OAuthAccountDefaultsToSubscription(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	repo := &profitRepoStub{
		stats: map[int64]*ProfitUsageStats{
			1: {Requests: 10, Revenue: 50, MeteredCost: 30},
			2: {Requests: 5, Revenue: 20, MeteredCost: 8},
		},
		bestWindow: 10,
	}
	svc := &ProfitService{
		profitRepo: repo,
		accountRepo: &profitAccountRepoStub{accounts: []Account{
			{ID: 1, Name: "oauth-acc", Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			{ID: 2, Name: "apikey-acc", Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		}},
	}
	resp, err := svc.GetSummary(context.Background(), start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(resp.Accounts))
	}
	oauth := resp.Accounts[0]
	// OAuth 未配置 → 自动订阅类型、成本 0、未配置标记
	if oauth.CostType != AccountCostTypeSubscription || oauth.Configured || oauth.Cost != 0 {
		t.Fatalf("oauth summary = %+v, want subscription/unconfigured/cost 0", oauth)
	}
	// 未配置订阅号也能用学习基准算窗口效率
	if oauth.WindowEfficiency == nil || oauth.WindowBaselineSource != "learned" {
		t.Fatalf("oauth window efficiency = %v source = %v, want learned", oauth.WindowEfficiency, oauth.WindowBaselineSource)
	}
	apikey := resp.Accounts[1]
	if apikey.CostType != AccountCostTypeMetered || apikey.Cost != 8 {
		t.Fatalf("apikey summary = %+v, want metered/cost 8", apikey)
	}
}

func TestProfitService_GetAccountSummaryScopesToRequestedAccount(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	account := &Account{ID: 9, Name: "drawer-account", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	svc := &ProfitService{
		profitRepo:  &profitRepoStub{stats: map[int64]*ProfitUsageStats{9: {Requests: 12, Revenue: 8, MeteredCost: 2}}},
		accountRepo: &profitAccountRepoStub{byID: map[int64]*Account{9: account}},
	}

	resp, err := svc.GetAccountSummary(context.Background(), 9, start, start.AddDate(0, 0, 30))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Accounts) != 1 || resp.Accounts[0].AccountID != 9 {
		t.Fatalf("accounts = %+v, want only account #9", resp.Accounts)
	}
	if resp.TotalRevenue != 8 || resp.TotalCost != 2 || resp.TotalProfit != 6 {
		t.Fatalf("totals = revenue %v cost %v profit %v, want 8/2/6", resp.TotalRevenue, resp.TotalCost, resp.TotalProfit)
	}
}

func TestProfitService_AuthTypeOverridesLegacyCostType(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	repo := &profitRepoStub{
		cycles: map[int64][]*AccountSubscriptionCycle{1: {{AccountID: 1, StartsAt: start, PeriodFee: 300, PeriodDays: 30}}},
		stats: map[int64]*ProfitUsageStats{
			1: {Revenue: 50, MeteredCost: 40},
			2: {Revenue: 50, MeteredCost: 40},
		},
	}
	svc := &ProfitService{
		profitRepo: repo,
		accountRepo: &profitAccountRepoStub{accounts: []Account{
			{ID: 1, Type: AccountTypeOAuth},
			{ID: 2, Type: AccountTypeAPIKey},
		}},
	}
	resp, err := svc.GetSummary(context.Background(), start, start.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Accounts[0].CostType != AccountCostTypeSubscription || resp.Accounts[0].Cost != 10 {
		t.Fatalf("OAuth legacy config should be subscription: %+v", resp.Accounts[0])
	}
	if resp.Accounts[1].CostType != AccountCostTypeMetered || resp.Accounts[1].Cost != 40 {
		t.Fatalf("API Key legacy config should be metered: %+v", resp.Accounts[1])
	}
}

func TestProfitService_BatchUpsertSkipsConfigured(t *testing.T) {
	repo := &profitRepoStub{
		configs: []*AccountCostConfig{{AccountID: 1, CostType: AccountCostTypeSubscription, PeriodFee: 20}},
	}
	svc := &ProfitService{
		profitRepo: repo,
		accountRepo: &profitAccountRepoStub{accounts: []Account{
			{ID: 1, Name: "configured-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			{ID: 2, Name: "new-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			{ID: 3, Name: "apikey", Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		}},
	}
	result, err := svc.BatchUpsertSubscriptionConfigs(context.Background(), 200, 30, "")
	if err != nil {
		t.Fatal(err)
	}
	// 只有未配置的 OAuth 账号 #2 被写入
	if result.Updated != 1 || len(result.AccountIDs) != 1 || result.AccountIDs[0] != 2 {
		t.Fatalf("result = %+v, want updated=1 ids=[2]", result)
	}
	if len(repo.batchInserted) != 1 || repo.batchInserted[0].PeriodFee != 200 || repo.batchInserted[0].CostType != AccountCostTypeSubscription {
		t.Fatalf("saved = %+v, want one subscription cfg fee=200", repo.batchInserted)
	}
	// 费用非法 → 报错
	if _, err := svc.BatchUpsertSubscriptionConfigs(context.Background(), 0, 30, ""); err == nil {
		t.Fatal("zero fee should fail")
	}
}

func TestProfitService_BillingWindow(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	start := now.AddDate(0, 0, -20)
	end := start.AddDate(0, 0, 30)
	fee := 1200.0
	cycle := &AccountSubscriptionCycle{AccountID: 1, StartsAt: start, PeriodFee: fee, PeriodDays: 30}
	repo := &profitRepoStub{stats: map[int64]*ProfitUsageStats{1: {Requests: 5, Revenue: 800}}}
	svc := &ProfitService{profitRepo: repo}
	summary := &AccountProfitSummary{CostType: AccountCostTypeSubscription}
	svc.fillBillingWindow(context.Background(), 1, cycle, summary)

	if summary.BillingWindowStart == nil || summary.BillingWindowEnd == nil {
		t.Fatal("window start/end should be set")
	}
	if !summary.BillingWindowEnd.Equal(end) {
		t.Fatalf("window end = %v, want %v", summary.BillingWindowEnd, end)
	}
	if !summary.BillingWindowStart.Equal(start) {
		t.Fatalf("window start = %v, want %v", summary.BillingWindowStart, start)
	}
	// 进度 ≈ 20/30 = 66.67%
	if summary.BillingWindowProgress == nil || *summary.BillingWindowProgress < 66 || *summary.BillingWindowProgress > 68 {
		t.Fatalf("progress = %v, want ~66.67", summary.BillingWindowProgress)
	}
	// 窗口收入 800，成本 1200，利润 -400
	if summary.BillingWindowRevenue == nil || *summary.BillingWindowRevenue != 800 {
		t.Fatalf("window revenue = %v, want 800", summary.BillingWindowRevenue)
	}
	if summary.BillingWindowCost == nil || *summary.BillingWindowCost != 1200 {
		t.Fatalf("window cost = %v, want 1200", summary.BillingWindowCost)
	}
	if summary.BillingWindowProfit == nil || *summary.BillingWindowProfit != -400 {
		t.Fatalf("window profit = %v, want -400", summary.BillingWindowProfit)
	}

	// 零成本周期仍然是有效周期，成本为 0，利润等于周期收入。
	zeroFeeSummary := &AccountProfitSummary{CostType: AccountCostTypeSubscription}
	zeroFeeCycle := &AccountSubscriptionCycle{AccountID: 1, StartsAt: start, PeriodFee: 0, PeriodDays: 30}
	svc.fillBillingWindow(context.Background(), 1, zeroFeeCycle, zeroFeeSummary)
	if zeroFeeSummary.BillingWindowCost == nil || *zeroFeeSummary.BillingWindowCost != 0 {
		t.Fatalf("zero-fee window cost = %v, want 0", zeroFeeSummary.BillingWindowCost)
	}
	if zeroFeeSummary.BillingWindowProfit == nil || *zeroFeeSummary.BillingWindowProfit != 800 {
		t.Fatalf("zero-fee window profit = %v, want 800", zeroFeeSummary.BillingWindowProfit)
	}

}

func TestProfitService_UpsertDerivesCostTypeFromAccountAuth(t *testing.T) {
	repo := &profitRepoStub{}
	accountRepo := &profitAccountRepoStub{byID: map[int64]*Account{
		1: {ID: 1, Type: AccountTypeOAuth},
		2: {ID: 2, Type: AccountTypeAPIKey},
	}}
	svc := &ProfitService{profitRepo: repo, accountRepo: accountRepo}
	if _, err := svc.UpsertCostConfig(context.Background(), &AccountCostConfig{AccountID: 1, CostType: AccountCostTypeMetered, PeriodFee: 200}); err != nil {
		t.Fatal(err)
	}
	if repo.upserted.CostType != AccountCostTypeSubscription || repo.upserted.PeriodFee != 200 {
		t.Fatalf("OAuth config = %+v, want subscription", repo.upserted)
	}
	if _, err := svc.UpsertCostConfig(context.Background(), &AccountCostConfig{AccountID: 2, CostType: AccountCostTypeSubscription, PeriodFee: 200}); err != nil {
		t.Fatal(err)
	}
	if repo.upserted.CostType != AccountCostTypeMetered || repo.upserted.PeriodFee != 0 {
		t.Fatalf("API Key config = %+v, want metered with no subscription fields", repo.upserted)
	}
}
