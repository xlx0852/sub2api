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
	resp := calculateSupplyForecast(
		historyEnd, historyEnd.AddDate(0, 0, -30), historyEnd, "UTC", 30, 0.20,
		&StoredValueSnapshot{SpendableBalance: 1000, FrozenBalance: 20, EligibleUsers: 4},
		samples,
		map[string]int{"openai": 1, "grok": 2},
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
	if openai.RequiredSubscriptionAccounts == nil || *openai.RequiredSubscriptionAccounts != 1 {
		t.Fatalf("required accounts = %v", openai.RequiredSubscriptionAccounts)
	}
	if openai.SubscriptionAccountGap == nil || *openai.SubscriptionAccountGap != 0 {
		t.Fatalf("account gap = %v", openai.SubscriptionAccountGap)
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
	resp := calculateSupplyForecast(end, end.AddDate(0, 0, -30), end, "UTC", 30, 0.20, &StoredValueSnapshot{SpendableBalance: 500}, nil, nil)
	if resp.Available || resp.UnavailableReason != "no_recent_balance_usage" {
		t.Fatalf("forecast = %+v", resp)
	}
	if resp.RunwayDays != nil || len(resp.Platforms) != 0 {
		t.Fatalf("missing history produced estimates: %+v", resp)
	}
}

func TestPercentile75UsesNearestRank(t *testing.T) {
	if got := percentile75([]float64{10, 20, 30, 40}); got != 30 {
		t.Fatalf("p75 = %v, want 30", got)
	}
}
