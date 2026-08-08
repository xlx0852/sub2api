//go:build unit

package service

import (
	"testing"
	"time"
)

func TestBuildProfitTrendWithAccounts_StacksByAccountAndAmortizes(t *testing.T) {
	start := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 2) // two days: 8/2, 8/3
	accounts := []Account{
		{ID: 1, Name: "GPT-A"},
		{ID: 2, Name: "GROK-B"},
		{ID: 3, Name: "API-C"},
	}
	daily := []*ProfitAccountDailyUsagePoint{
		{AccountID: 1, Date: "2026-08-02", Revenue: 100},
		{AccountID: 2, Date: "2026-08-02", Revenue: 40},
		{AccountID: 3, Date: "2026-08-02", Revenue: 20, MeteredCost: 5},
		{AccountID: 1, Date: "2026-08-03", Revenue: 80},
		{AccountID: 3, Date: "2026-08-03", Revenue: 10, MeteredCost: 2},
	}
	// Acc1: $300 / 30d = $10/day; Acc2: $150 / 30d = $5/day
	cycles := []*AccountSubscriptionCycle{
		{AccountID: 1, StartsAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), PeriodFee: 300, PeriodDays: 30},
		{AccountID: 2, StartsAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), PeriodFee: 150, PeriodDays: 30},
	}

	resp := buildProfitTrendWithAccounts(daily, accounts, cycles, "UTC", start, end)
	if len(resp.Points) != 2 {
		t.Fatalf("points=%d want 2", len(resp.Points))
	}
	d0 := resp.Points[0]
	if d0.Date != "2026-08-02" {
		t.Fatalf("date0=%s", d0.Date)
	}
	// revenue 100+40+20=160; cost 10+5+5=20; profit 140
	if d0.Revenue != 160 || d0.Cost != 20 || d0.Profit != 140 {
		t.Fatalf("day0 totals rev/cost/profit=%v/%v/%v want 160/20/140", d0.Revenue, d0.Cost, d0.Profit)
	}
	if len(d0.Accounts) != 3 {
		t.Fatalf("day0 accounts=%d want 3: %+v", len(d0.Accounts), d0.Accounts)
	}
	// Sorted by revenue desc: 1,2,3
	if d0.Accounts[0].AccountID != 1 || d0.Accounts[0].Cost != 10 || d0.Accounts[0].Revenue != 100 {
		t.Fatalf("top slice=%+v", d0.Accounts[0])
	}
	if d0.Accounts[1].AccountID != 2 || d0.Accounts[1].Cost != 5 {
		t.Fatalf("second slice=%+v", d0.Accounts[1])
	}
	if d0.Accounts[2].AccountID != 3 || d0.Accounts[2].Cost != 5 || d0.Accounts[2].Revenue != 20 {
		t.Fatalf("metered slice=%+v", d0.Accounts[2])
	}

	d1 := resp.Points[1]
	// Acc2 has no usage on 8/3 but still has $5 sub cost → should appear
	found2 := false
	for _, a := range d1.Accounts {
		if a.AccountID == 2 {
			found2 = true
			if a.Revenue != 0 || a.Cost != 5 {
				t.Fatalf("idle sub account slice=%+v", a)
			}
		}
	}
	if !found2 {
		t.Fatalf("expected subscription account 2 on idle day, got %+v", d1.Accounts)
	}
	// rev 80+10=90; cost 10+5+2=17
	if d1.Revenue != 90 || d1.Cost != 17 {
		t.Fatalf("day1 totals rev/cost=%v/%v want 90/17", d1.Revenue, d1.Cost)
	}
}
