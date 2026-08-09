//go:build unit

package service

import (
	"testing"
	"time"
)

func TestFillDrawerWindowFromQuota_UsesQuotaWindowNotPageFilter(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	winStart := time.Date(2026, 8, 7, 2, 9, 0, 0, time.UTC) // 08/07 10:09 CST-ish
	winEnd := time.Date(2026, 8, 14, 2, 9, 0, 0, time.UTC)
	used := 100.0
	mins := 10080
	win := &ProfitQuotaWindow{
		ID: "7d", Label: "7d", Kind: "7d",
		UsedPercent: &used, StartAt: &winStart, EndAt: &winEnd, WindowMinutes: &mins,
	}
	// Page filter would be e.g. last 7 calendar days with huge revenue on summary.Revenue
	summary := &AccountProfitSummary{
		AccountID: 69, CostType: AccountCostTypeSubscription,
		Revenue: 9999, Cost: 999, Requests: 34000,
		QuotaWindows: []ProfitQuotaWindow{*win},
	}
	cycles := []*AccountSubscriptionCycle{{
		AccountID: 69,
		StartsAt:  time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
		PeriodFee: 200,
		PeriodDays: 30,
	}}
	// Window stats: revenue inside the 7d quota window only
	stats := &ProfitUsageStats{Requests: 1200, Revenue: 331.13, MeteredCost: 0}

	fillDrawerWindowFromQuota(summary, cycles, win, stats, AccountCostTypeSubscription, now)

	if summary.DrawerQuotaKind != "7d" || summary.DrawerQuotaStart == nil || summary.DrawerQuotaEnd == nil {
		t.Fatalf("drawer quota missing: %+v", summary)
	}
	if summary.DrawerQuotaRevenue == nil || *summary.DrawerQuotaRevenue != 331.13 {
		t.Fatalf("revenue=%v", summary.DrawerQuotaRevenue)
	}
	if summary.BillingWindowSource == "cycle" && summary.DrawerQuotaRevenue == nil {
		t.Fatal("cycle should keep billing_window, drawer quota separate")
	}
	// Amortized: 200 * (elapsed days / 30). elapsed = now-start ≈ 2.41d → ~16.07
	if summary.DrawerQuotaCost == nil || *summary.DrawerQuotaCost <= 0 || *summary.DrawerQuotaCost >= 200 {
		t.Fatalf("cost=%v want amortized fraction of 200", summary.DrawerQuotaCost)
	}
	if summary.DrawerQuotaProfit == nil {
		t.Fatal("profit nil")
	}
	wantProfit := roundMoney(331.13 - *summary.DrawerQuotaCost)
	if *summary.DrawerQuotaProfit != wantProfit {
		t.Fatalf("profit=%v want %v", *summary.DrawerQuotaProfit, wantProfit)
	}
	if summary.DrawerQuotaRequests == nil || *summary.DrawerQuotaRequests != 1200 {
		t.Fatalf("requests=%v want window 1200 not page 34000", summary.DrawerQuotaRequests)
	}
	if !summary.DrawerQuotaStart.Equal(winStart) || !summary.DrawerQuotaEnd.Equal(winEnd) {
		t.Fatalf("range %v→%v", summary.DrawerQuotaStart, summary.DrawerQuotaEnd)
	}
}

func TestFillDrawerWindowFromQuota_MeteredUsesWindowMeteredCost(t *testing.T) {
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	start := now.Add(-24 * time.Hour)
	end := now.Add(24 * time.Hour)
	win := &ProfitQuotaWindow{Kind: "24h", StartAt: &start, EndAt: &end}
	summary := &AccountProfitSummary{AccountID: 3, CostType: AccountCostTypeMetered}
	stats := &ProfitUsageStats{Requests: 10, Revenue: 50, MeteredCost: 12.5}
	fillDrawerWindowFromQuota(summary, nil, win, stats, AccountCostTypeMetered, now)
	if summary.DrawerQuotaCost == nil || *summary.DrawerQuotaCost != 12.5 {
		t.Fatalf("cost=%v", summary.DrawerQuotaCost)
	}
	if summary.DrawerQuotaProfit == nil || *summary.DrawerQuotaProfit != 37.5 {
		t.Fatalf("profit=%v", summary.DrawerQuotaProfit)
	}
}
