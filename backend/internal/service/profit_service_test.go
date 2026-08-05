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
	storedValue      *StoredValueSnapshot
	forecastSamples        []*SupplyForecastUsageSample
	forecastSupply         map[string]int
	forecastQuotaSnapshots []*SubscriptionQuotaSnapshot
	cycleRevenues    map[int64]float64
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
func (s *profitRepoStub) GetSubscriptionCycle(_ context.Context, id int64) (*AccountSubscriptionCycle, error) {
	for _, cycles := range s.cycles {
		for _, cycle := range cycles {
			if cycle.ID == id {
				return cycle, nil
			}
		}
	}
	return nil, ErrSubscriptionCycleNotFound
}
func (s *profitRepoStub) CreateSubscriptionCycle(_ context.Context, cycle *AccountSubscriptionCycle) (*AccountSubscriptionCycle, error) {
	if cycle.ID == 0 {
		cycle.ID = int64(len(s.cycles[cycle.AccountID]) + 1)
	}
	s.cycles[cycle.AccountID] = append(s.cycles[cycle.AccountID], cycle)
	return cycle, nil
}
func (s *profitRepoStub) DeleteSubscriptionCycle(_ context.Context, id int64) error {
	for accountID, cycles := range s.cycles {
		kept := cycles[:0]
		for _, cycle := range cycles {
			if cycle.ID != id {
				kept = append(kept, cycle)
			}
		}
		s.cycles[accountID] = kept
	}
	return nil
}
func (s *profitRepoStub) CreateSubscriptionTermination(_ context.Context, termination *AccountSubscriptionTermination, initialRefund *AccountSubscriptionRefund) (*SubscriptionTerminationWriteResult, error) {
	termination.ID = 1
	if cycles := s.cycles[termination.AccountID]; len(cycles) > 0 {
		for _, cycle := range cycles {
			if cycle.ID == termination.CycleID {
				cycle.Termination = termination
				if initialRefund != nil {
					initialRefund.ID = 1
					initialRefund.TerminationID = termination.ID
					initialRefund.CycleID = cycle.ID
					initialRefund.AccountID = cycle.AccountID
					cycle.Refunds = append(cycle.Refunds, initialRefund)
				}
			}
		}
	}
	return &SubscriptionTerminationWriteResult{Termination: termination, InitialRefund: initialRefund, DisabledAccountIDs: []int64{termination.AccountID}}, nil
}
func (s *profitRepoStub) CreateSubscriptionRefund(_ context.Context, refund *AccountSubscriptionRefund) (*AccountSubscriptionRefund, error) {
	return refund, nil
}
func (s *profitRepoStub) VoidSubscriptionRefund(_ context.Context, id int64, reason string, voidedAt time.Time) (*AccountSubscriptionRefund, error) {
	return &AccountSubscriptionRefund{ID: id, VoidReason: reason, VoidedAt: &voidedAt}, nil
}
func (s *profitRepoStub) ReverseSubscriptionTermination(_ context.Context, id int64, reason string, reversedAt time.Time) (*AccountSubscriptionTermination, error) {
	for _, cycles := range s.cycles {
		for _, cycle := range cycles {
			if cycle.Termination != nil && cycle.Termination.ID == id {
				cycle.Termination.ReversedAt = &reversedAt
				cycle.Termination.ReversalReason = reason
				return cycle.Termination, nil
			}
		}
	}
	return &AccountSubscriptionTermination{ID: id, ReversalReason: reason, ReversedAt: &reversedAt}, nil
}
func (s *profitRepoStub) GetSubscriptionCycleRevenueBatch(_ context.Context, ranges []SubscriptionCycleUsageRange) (map[int64]float64, error) {
	result := make(map[int64]float64, len(ranges))
	for _, item := range ranges {
		result[item.CycleID] = s.cycleRevenues[item.CycleID]
	}
	return result, nil
}
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
func (s *profitRepoStub) GetStoredValueSnapshot(context.Context) (*StoredValueSnapshot, error) {
	return s.storedValue, nil
}
func (s *profitRepoStub) GetSupplyForecastUsageSamples(context.Context, time.Time, time.Time, string) ([]*SupplyForecastUsageSample, error) {
	return s.forecastSamples, nil
}
func (s *profitRepoStub) GetSchedulableSubscriptionSupply(context.Context) (map[string]int, error) {
	return s.forecastSupply, nil
}
func (s *profitRepoStub) GetSubscriptionQuotaSnapshots(context.Context) ([]*SubscriptionQuotaSnapshot, error) {
	return s.forecastQuotaSnapshots, nil
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

func TestSubscriptionBanRecognizesRemainingCostAndLaterRefund(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	bannedAt := start.AddDate(0, 0, 10)
	refundedAt := bannedAt.AddDate(0, 0, 2)
	cycle := &AccountSubscriptionCycle{
		StartsAt: start, PeriodFee: 900, PeriodDays: 30,
		Termination: &AccountSubscriptionTermination{EffectiveAt: bannedAt, Reason: "upstream_banned"},
		Refunds:     []*AccountSubscriptionRefund{{Amount: 200, ReceivedAt: refundedAt}},
	}
	if got := subscriptionCycleCostForRange(cycle, start, bannedAt); got != 300 {
		t.Fatalf("pre-ban amortization = %v, want 300", got)
	}
	if got := subscriptionCycleCostForRange(cycle, bannedAt, bannedAt.AddDate(0, 0, 1)); got != 600 {
		t.Fatalf("ban impairment = %v, want 600", got)
	}
	if got := subscriptionCycleCostForRange(cycle, refundedAt, refundedAt.AddDate(0, 0, 1)); got != -200 {
		t.Fatalf("refund adjustment = %v, want -200", got)
	}
	if got := subscriptionCycleCostForRange(cycle, start, start.AddDate(0, 0, 30)); got != 700 {
		t.Fatalf("lifecycle cost = %v, want 700", got)
	}
}

func TestSubscriptionBanLossSummaryIncludesRevenueAndRefund(t *testing.T) {
	cycle := &AccountSubscriptionCycle{
		PeriodFee: 865, PeriodDays: 30,
		Termination: &AccountSubscriptionTermination{EffectiveAt: time.Now(), Reason: "upstream_banned"},
		Refunds:     []*AccountSubscriptionRefund{{Amount: 200, ReceivedAt: time.Now()}},
	}
	summary := subscriptionLossSummary(cycle, 300)
	if summary.NetPurchaseCost != 665 || summary.RealizedLoss != 365 || summary.RealizedProfit != -365 {
		t.Fatalf("loss summary = %+v, want net/loss/profit 665/365/-365", summary)
	}
	if summary.RecoveredAmount != 500 || summary.RecoveryProgress != 57.8035 {
		t.Fatalf("recovery = %+v, want amount/progress 500/57.8035", summary)
	}

	fullyRecovered := subscriptionLossSummary(cycle, 900)
	if fullyRecovered.RealizedLoss != 0 || fullyRecovered.RecoveryProgress != 100 || fullyRecovered.RealizedProfit != 235 {
		t.Fatalf("fully recovered summary = %+v", fullyRecovered)
	}
}

func TestTerminatedBillingWindowFreezesAtBanAndUsesNetCost(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	bannedAt := start.AddDate(0, 0, 10)
	cycle := &AccountSubscriptionCycle{
		StartsAt: start, PeriodFee: 865, PeriodDays: 30,
		Termination: &AccountSubscriptionTermination{EffectiveAt: bannedAt, Reason: "upstream_banned"},
		Refunds:     []*AccountSubscriptionRefund{{Amount: 200, ReceivedAt: bannedAt}},
	}
	summary := &AccountProfitSummary{CostType: AccountCostTypeSubscription}
	fillBillingWindowFromRevenue(cycle, summary, 300, start.AddDate(0, 0, 20))
	if summary.BillingWindowProgress == nil || *summary.BillingWindowProgress != 33.3333 {
		t.Fatalf("elapsed progress = %v, want 33.3333", summary.BillingWindowProgress)
	}
	if summary.BillingWindowCost == nil || *summary.BillingWindowCost != 665 {
		t.Fatalf("net cost = %v, want 665", summary.BillingWindowCost)
	}
	if summary.BillingWindowProfit == nil || *summary.BillingWindowProfit != -365 {
		t.Fatalf("profit = %v, want -365", summary.BillingWindowProfit)
	}
	if summary.BillingWindowLoss == nil || *summary.BillingWindowLoss != 365 {
		t.Fatalf("loss = %v, want 365", summary.BillingWindowLoss)
	}
}

func TestGrokMonthlyQuotaResetDoesNotSplitPaymentCycle(t *testing.T) {
	start := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	reset := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	cycle := &AccountSubscriptionCycle{StartsAt: start, PeriodFee: 310, PeriodDays: 31}

	beforeReset := subscriptionCycleCostForRange(cycle, start, reset)
	afterReset := subscriptionCycleCostForRange(cycle, reset, end)
	fullCycle := subscriptionCycleCostForRange(cycle, start, end)
	if beforeReset != 150 || afterReset != 160 || fullCycle != 310 {
		t.Fatalf("Grok cycle costs before=%v after=%v full=%v, want 150/160/310", beforeReset, afterReset, fullCycle)
	}
	if beforeReset+afterReset != fullCycle {
		t.Fatalf("quota reset duplicated or dropped cost: split=%v full=%v", beforeReset+afterReset, fullCycle)
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
	if repo.dailyBatchCalls != 1 || repo.listConfigsCalls != 1 || repo.cycleBatchCalls != 1 {
		t.Fatalf("batch calls daily/config/cycle = %d/%d/%d, want all 1", repo.dailyBatchCalls, repo.listConfigsCalls, repo.cycleBatchCalls)
	}
	if repo.bestBatchCalls != 0 || repo.rangeStatsCalls != 0 {
		t.Fatalf("overview loaded drawer-only best/range stats: %d/%d", repo.bestBatchCalls, repo.rangeStatsCalls)
	}
	if repo.statsBatchCalls != 0 {
		t.Fatalf("overview performed duplicate account stats query: %d", repo.statsBatchCalls)
	}
	if overview.GeneratedAt.IsZero() {
		t.Fatal("overview generated_at must be set")
	}
	if overview.Summary.Accounts[0].WindowEfficiency != nil || overview.Summary.Accounts[0].BillingWindowRevenue != nil {
		t.Fatalf("overview returned drawer-only metrics: %+v", overview.Summary.Accounts[0])
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

func (m *profitAccountRepoStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _, status, _ string, _ int64, _ string) ([]Account, *pagination.PaginationResult, error) {
	switch status {
	case AccountListStatusDeleted:
		out := make([]Account, 0)
		for _, acc := range m.accounts {
			if acc.DeletedAt != nil {
				out = append(out, acc)
			}
		}
		return out, nil, nil
	case AccountListStatusTrash:
		out := make([]Account, 0)
		for _, acc := range m.accounts {
			if acc.DeletedAt != nil || acc.SubscriptionBanned {
				out = append(out, acc)
			}
		}
		return out, nil, nil
	case AccountListStatusBanned:
		out := make([]Account, 0)
		for _, acc := range m.accounts {
			if acc.SubscriptionBanned {
				out = append(out, acc)
			}
		}
		return out, nil, nil
	}
	out := make([]Account, 0, len(m.accounts))
	for _, acc := range m.accounts {
		if acc.DeletedAt == nil && !acc.SubscriptionBanned {
			out = append(out, acc)
		}
	}
	return out, nil, nil
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

func (m *profitAccountRepoStub) SetSchedulable(_ context.Context, id int64, schedulable bool) error {
	if account := m.byID[id]; account != nil {
		account.Schedulable = schedulable
	}
	for i := range m.accounts {
		if m.accounts[i].ID == id {
			m.accounts[i].Schedulable = schedulable
		}
	}
	return nil
}

func (m *profitAccountRepoStub) SetExpiresAt(_ context.Context, id int64, expiresAt *time.Time) error {
	if account := m.byID[id]; account != nil {
		account.ExpiresAt = expiresAt
	}
	for i := range m.accounts {
		if m.accounts[i].ID == id {
			m.accounts[i].ExpiresAt = expiresAt
		}
	}
	return nil
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


func TestDeriveAccountExpiresAtFromCycles(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	cycle := &AccountSubscriptionCycle{ID: 1, AccountID: 9, StartsAt: start, PeriodDays: 30, PeriodFee: 200}
	got := deriveAccountExpiresAtFromCycles([]*AccountSubscriptionCycle{cycle}, now)
	want := start.AddDate(0, 0, 30)
	if got == nil || !got.Equal(want) {
		t.Fatalf("active cycle expires = %v, want %v", got, want)
	}

	termAt := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	cycle.Termination = &AccountSubscriptionTermination{ID: 3, CycleID: 1, AccountID: 9, EffectiveAt: termAt}
	got = deriveAccountExpiresAtFromCycles([]*AccountSubscriptionCycle{cycle}, now)
	if got == nil || !got.Equal(termAt) {
		t.Fatalf("terminated cycle expires = %v, want %v", got, termAt)
	}

	// future cycle preferred over expired terminated history when no active
	futureStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	future := &AccountSubscriptionCycle{ID: 2, AccountID: 9, StartsAt: futureStart, PeriodDays: 30}
	got = deriveAccountExpiresAtFromCycles([]*AccountSubscriptionCycle{cycle, future}, now)
	wantFuture := futureStart.AddDate(0, 0, 30)
	if got == nil || !got.Equal(wantFuture) {
		t.Fatalf("future cycle expires = %v, want %v", got, wantFuture)
	}

	if got := deriveAccountExpiresAtFromCycles(nil, now); got != nil {
		t.Fatalf("empty cycles should clear expires, got %v", got)
	}
}

func TestProfitService_CreateSubscriptionCycleSyncsExpiresAt(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	nowish := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	_ = nowish
	repo := &profitRepoStub{cycles: map[int64][]*AccountSubscriptionCycle{}}
	accounts := &profitAccountRepoStub{
		byID: map[int64]*Account{
			9: {ID: 9, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Name: "o"},
		},
	}
	svc := &ProfitService{profitRepo: repo, accountRepo: accounts}
	// Freeze-ish: create cycle covering "now" — derive uses time.Now(), so use recent start
	start = time.Now().UTC().AddDate(0, 0, -3)
	created, err := svc.CreateSubscriptionCycle(context.Background(), &AccountSubscriptionCycle{
		AccountID: 9, StartsAt: start, PeriodDays: 30, PeriodFee: 200, Currency: "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := start.AddDate(0, 0, 30)
	acc := accounts.byID[9]
	if acc.ExpiresAt == nil || !acc.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v (cycle id %d)", acc.ExpiresAt, want, created.ID)
	}
}

func TestProfitService_IncludesDeletedAccounts(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	deletedAt := start.AddDate(0, 0, -1)
	repo := &profitRepoStub{
		stats: map[int64]*ProfitUsageStats{
			1: {Requests: 10, Revenue: 50, MeteredCost: 20},
			2: {Requests: 4, Revenue: 12, MeteredCost: 3},
		},
	}
	svc := &ProfitService{
		profitRepo: repo,
		accountRepo: &profitAccountRepoStub{accounts: []Account{
			{ID: 1, Name: "live", Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			{ID: 2, Name: "trash", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, DeletedAt: &deletedAt},
		}},
	}
	resp, err := svc.GetSummary(context.Background(), start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Accounts) != 2 {
		t.Fatalf("accounts = %d, want 2 (live + trash)", len(resp.Accounts))
	}
	var trash *AccountProfitSummary
	for _, acc := range resp.Accounts {
		if acc.AccountID == 2 {
			trash = acc
		}
	}
	if trash == nil {
		t.Fatal("deleted account missing from profit summary")
	}
	if !trash.Deleted {
		t.Fatalf("deleted flag = false, want true: %+v", trash)
	}
	if trash.Revenue != 12 || trash.Cost != 3 {
		t.Fatalf("trash summary = %+v", trash)
	}
}

func TestProfitService_IncludesBannedTrashAccounts(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 30)
	bannedAt := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	repo := &profitRepoStub{
		stats: map[int64]*ProfitUsageStats{
			1: {Requests: 10, Revenue: 50, MeteredCost: 0},
			103: {Requests: 20, Revenue: 200, MeteredCost: 0},
		},
		cycles: map[int64][]*AccountSubscriptionCycle{
			103: {{
				ID: 3, AccountID: 103, StartsAt: start, PeriodFee: 865, PeriodDays: 30, Currency: "USD",
				Termination: &AccountSubscriptionTermination{EffectiveAt: bannedAt, Reason: "upstream_banned"},
			}},
		},
	}
	svc := &ProfitService{
		profitRepo: repo,
		accountRepo: &profitAccountRepoStub{accounts: []Account{
			{ID: 1, Name: "live", Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			{ID: 103, Name: "banned", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusError, SubscriptionBanned: true},
		}},
	}
	resp, err := svc.GetSummary(context.Background(), start, end)
	if err != nil {
		t.Fatal(err)
	}
	var banned *AccountProfitSummary
	for _, acc := range resp.Accounts {
		if acc.AccountID == 103 {
			banned = acc
		}
	}
	if banned == nil {
		t.Fatal("banned trash account missing from profit summary")
	}
	if !banned.Deleted {
		t.Fatalf("deleted/trash flag = false, want true: %+v", banned)
	}
	// 查询区间覆盖封禁日：成本应记满整期订阅费 865
	if banned.Cost != 865 {
		t.Fatalf("banned cost = %v, want 865", banned.Cost)
	}
	if banned.Profit != roundMoney(200-865) {
		t.Fatalf("banned profit = %v, want %v", banned.Profit, roundMoney(200-865))
	}
}
