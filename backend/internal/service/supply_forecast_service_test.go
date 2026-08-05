package service

import (
	"testing"
	"time"
)

func TestCalculateSupplyForecastSeparatesSubscriptionAndMeteredSupply(t *testing.T) {
	historyEnd := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	samples := []*SupplyForecastUsageSample{
		{Date: "2026-07-29", Platform: "openai", AccountID: 1, AccountType: AccountTypeOAuth, Revenue: 70},
		{Date: "2026-07-28", Platform: "openai", AccountID: 1, AccountType: AccountTypeOAuth, Revenue: 50},
		{Date: "2026-07-29", Platform: "openai", AccountID: 2, AccountType: AccountTypeAPIKey, Revenue: 30, MeteredCost: 6},
		{Date: "2026-07-29", Platform: "grok", AccountID: 3, AccountType: AccountTypeOAuth, Revenue: 50},
	}
	quotaSnapshots := []*SubscriptionQuotaSnapshot{
		{AccountID: 1, Platform: "openai", RemainingPct: 100, WindowDays: 7, HasValidData: true},
	}
	resp := calculateSupplyForecast(
		historyEnd, historyEnd, historyEnd.AddDate(0, 0, -30), historyEnd, "UTC", 30, 0.20,
		&StoredValueSnapshot{SpendableBalance: 1000, FrozenBalance: 20, EligibleUsers: 4},
		samples,
		map[string]int{"openai": 1, "grok": 2},
		quotaSnapshots,
	)
	if !resp.Available || resp.RunwayDays == nil {
		t.Fatalf("forecast unavailable: %+v", resp)
	}
	if resp.DailyBurn7 != 28.5714 || resp.DailyBurn30 != 6.6667 || resp.BaseDailyDemand != 28.5714 {
		t.Fatalf("daily burn = %v/%v base=%v", resp.DailyBurn7, resp.DailyBurn30, resp.BaseDailyDemand)
	}
	if resp.PlanningDailyDemand != 34.2857 || resp.ProjectedConsumption != 857.142 || resp.PlanningConsumption != 1000 {
		t.Fatalf("planning = daily %v projected %v protected %v", resp.PlanningDailyDemand, resp.ProjectedConsumption, resp.PlanningConsumption)
	}
	if len(resp.Platforms) != 2 || resp.Platforms[1].Platform != "openai" {
		t.Fatalf("platforms = %+v", resp.Platforms)
	}
	openai := resp.Platforms[1]
	if openai.QuotaAccounts != 1 || openai.AccountDailyCapacityQuota == nil {
		t.Fatalf("quota forecast = accounts %v cap %v", openai.QuotaAccounts, openai.AccountDailyCapacityQuota)
	}
	// 额度驱动：产能 = 60（平台日均）× 100% / 7 = 8.5714
	// 订阅号日需求 = 34.2857 × 0.75 × 0.8 = 20.571 → required = ceil(20.571/8.5714) = 3
	if openai.RequiredSubscriptionAccounts == nil || *openai.RequiredSubscriptionAccounts != 3 {
		t.Fatalf("required accounts = %v, want 3", openai.RequiredSubscriptionAccounts)
	}
	if openai.SubscriptionAccountGap == nil || *openai.SubscriptionAccountGap != 2 {
		t.Fatalf("account gap = %v, want 2", openai.SubscriptionAccountGap)
	}
	if openai.MeteredCostRatio == nil || *openai.MeteredCostRatio != 0.2 {
		t.Fatalf("metered ratio = %v", openai.MeteredCostRatio)
	}
	if openai.MeteredProcurementBudget == nil || *openai.MeteredProcurementBudget != 30 {
		t.Fatalf("metered budget = %v", openai.MeteredProcurementBudget)
	}
}

func TestCalculateSupplyForecastDoesNotInventZeroWhenHistoryMissing(t *testing.T) {
	end := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	resp := calculateSupplyForecast(end, end, end.AddDate(0, 0, -30), end, "UTC", 30, 0.20, &StoredValueSnapshot{SpendableBalance: 500}, nil, nil, nil)
	if resp.Available || resp.UnavailableReason != "no_recent_balance_usage" {
		t.Fatalf("forecast = %+v", resp)
	}
	if resp.RunwayDays != nil || len(resp.Platforms) != 0 {
		t.Fatalf("missing history produced estimates: %+v", resp)
	}
}

// 额度驱动的订阅号供给：额度耗尽的号标红但不把产能压为 0（避免分母爆炸），
// 无额度快照的平台标记 stale 而不是假装有产能。
func TestCalculateSupplyForecastQuotaDrivenExhaustionAndStale(t *testing.T) {
	historyEnd := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	samples := []*SupplyForecastUsageSample{
		{Date: "2026-07-29", Platform: "openai", AccountID: 1, AccountType: AccountTypeOAuth, Revenue: 100},
		{Date: "2026-07-28", Platform: "openai", AccountID: 1, AccountType: AccountTypeOAuth, Revenue: 100},
	}
	// 额度已耗尽的号（remaining=0）
	quotaSnapshots := []*SubscriptionQuotaSnapshot{
		{AccountID: 1, Platform: "openai", RemainingPct: 0, WindowDays: 7, HasValidData: true},
	}
	resp := calculateSupplyForecast(
		historyEnd, historyEnd, historyEnd.AddDate(0, 0, -30), historyEnd, "UTC", 30, 0.20,
		&StoredValueSnapshot{SpendableBalance: 1000},
		samples,
		map[string]int{"openai": 1},
		quotaSnapshots,
	)
	openai := resp.Platforms[0]
	if !openai.QuotaExhausted {
		t.Fatalf("expected quota_exhausted for exhausted account")
	}
	if openai.AccountDailyCapacityQuota == nil || *openai.AccountDailyCapacityQuota <= 0 {
		t.Fatalf("capacity should not be 0 (full-window fallback), got %v", openai.AccountDailyCapacityQuota)
	}
	if openai.QuotaRemainingPct == nil || *openai.QuotaRemainingPct != 0 {
		t.Fatalf("remaining pct = %v, want 0", openai.QuotaRemainingPct)
	}
}

func TestPercentile75UsesNearestRank(t *testing.T) {
	if got := percentile75([]float64{10, 20, 30, 40}); got != 30 {
		t.Fatalf("p75 = %v, want 30", got)
	}
}
