//go:build unit

package service

import (
	"testing"
	"time"
)

func anchorTestCycle(start time.Time, days int) *AccountSubscriptionCycle {
	return &AccountSubscriptionCycle{StartsAt: start, PeriodDays: days, PeriodFee: 100}
}

func TestBuildProfitWindowAnchor_AutoRenewOffStopsAtCycleEnd(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	cycle := anchorTestCycle(time.Date(2026, 8, 5, 15, 36, 0, 0, time.UTC), 30)
	cfg := &AccountCostConfig{AutoRenew: false}
	a := buildProfitWindowAnchor([]*AccountSubscriptionCycle{cycle}, cfg, now)
	if a.recurringUntil == nil {
		t.Fatalf("expected recurringUntil set when auto_renew=false")
	}
	want := cycle.StartsAt.AddDate(0, 0, 30)
	if !a.recurringUntil.Equal(want) {
		t.Fatalf("recurringUntil=%v want %v", a.recurringUntil, want)
	}
}

func TestBuildProfitWindowAnchor_AutoRenewOnRollsOn(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	cycle := anchorTestCycle(time.Date(2026, 8, 5, 15, 36, 0, 0, time.UTC), 30)
	cfg := &AccountCostConfig{AutoRenew: true}
	a := buildProfitWindowAnchor([]*AccountSubscriptionCycle{cycle}, cfg, now)
	if a.recurringUntil != nil {
		t.Fatalf("expected no recurringUntil when auto_renew=true, got %v", a.recurringUntil)
	}
}

func TestBuildProfitWindowAnchor_CoveredEndUsesLatestCycle(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	old := anchorTestCycle(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), 30) // ends 8/3
	cur := anchorTestCycle(time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), 30) // ends 9/4
	a := buildProfitWindowAnchor([]*AccountSubscriptionCycle{old, cur}, &AccountCostConfig{}, now)
	if len(a.spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(a.spans))
	}
	latest := a.spans[0].end
	for _, s := range a.spans {
		if s.end.After(latest) {
			latest = s.end
		}
	}
	want := cur.StartsAt.AddDate(0, 0, 30)
	if !latest.Equal(want) {
		t.Fatalf("latest span end=%v want %v", latest, want)
	}
}

// Gap: window starting in the unconfigured gap (8/3→8/5) must be dropped.
func TestFillProfitQuotaWindows_DropsWindowStartingInCycleGap(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	old := anchorTestCycle(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), 30) // ends 8/3
	cur := anchorTestCycle(time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), 30) // starts 8/5
	anchor := buildProfitWindowAnchor([]*AccountSubscriptionCycle{old, cur}, &AccountCostConfig{AutoRenew: false}, now)

	// A window anchored at 8/4 (inside the gap) should be dropped.
	start := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)
	summary := &AccountProfitSummary{}
	acc := &Account{ID: 1, SessionWindowStart: &start, SessionWindowEnd: &end}
	fillProfitQuotaWindowsWithAnchor(summary, acc, now, anchor)
	if len(summary.QuotaWindows) != 0 {
		t.Fatalf("expected gap window dropped, got %d", len(summary.QuotaWindows))
	}
}

// Expired cycle + auto_renew=false → projection stops at the latest cycle end.
func TestEffectiveRecurringUntil_ExpiredNoRenewStops(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	cycle := anchorTestCycle(time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), 30) // ends 8/8
	a := buildProfitWindowAnchor([]*AccountSubscriptionCycle{cycle}, &AccountCostConfig{AutoRenew: false}, now)
	got := a.effectiveRecurringUntil(now)
	if got == nil {
		t.Fatalf("expected cutoff at latest cycle end")
	}
	want := cycle.StartsAt.AddDate(0, 0, 30)
	if !got.Equal(want) {
		t.Fatalf("cutoff=%v want %v", got, want)
	}
}

// Active cycle + auto_renew=true → no cutoff (keep rolling).
func TestEffectiveRecurringUntil_ActiveRenewKeepsRolling(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	cycle := anchorTestCycle(time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), 30)
	a := buildProfitWindowAnchor([]*AccountSubscriptionCycle{cycle}, &AccountCostConfig{AutoRenew: true}, now)
	if got := a.effectiveRecurringUntil(now); got != nil {
		t.Fatalf("expected nil cutoff, got %v", got)
	}
}
