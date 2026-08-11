//go:build unit

package service

import (
	"testing"
	"time"
)

func anchorTestCycle(start time.Time, days int) *AccountSubscriptionCycle {
	return &AccountSubscriptionCycle{StartsAt: start, PeriodDays: days, PeriodFee: 100}
}

func TestBuildProfitWindowAnchor_ActiveCycleStopsAtRecordedEnd(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	cycle := anchorTestCycle(time.Date(2026, 8, 5, 15, 36, 0, 0, time.UTC), 30)
	a := buildProfitWindowAnchor([]*AccountSubscriptionCycle{cycle}, &AccountCostConfig{AutoRenew: false}, now)
	want := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
	if a.recurringUntil == nil || !a.recurringUntil.Equal(want) {
		t.Fatalf("active cycle must stop at recorded end %v, got %v", want, a.recurringUntil)
	}
}

func TestBuildProfitWindowAnchor_ExpiredStopsAtLatestCycleEnd(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	cycle := anchorTestCycle(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), 30) // ended 7/31
	a := buildProfitWindowAnchor([]*AccountSubscriptionCycle{cycle}, &AccountCostConfig{AutoRenew: true}, now)
	if a.recurringUntil == nil {
		t.Fatalf("expired cycle must cap projection at latest cycle end even if auto_renew=true")
	}
	want := cycle.StartsAt.AddDate(0, 0, 30)
	if !a.recurringUntil.Equal(want) {
		t.Fatalf("recurringUntil=%v want %v", a.recurringUntil, want)
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

func TestBuildProfitWindowAnchor_MergesOverlappingAndAdjacentCycles(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	first := anchorTestCycle(time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC), 30)
	overlapping := anchorTestCycle(time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC), 30)
	adjacent := anchorTestCycle(time.Date(2026, 9, 19, 8, 0, 0, 0, time.UTC), 30)

	a := buildProfitWindowAnchor([]*AccountSubscriptionCycle{adjacent, first, overlapping}, &AccountCostConfig{}, now)

	if len(a.spans) != 1 {
		t.Fatalf("overlapping paid coverage must be one canonical span, got %+v", a.spans)
	}
	if !a.spans[0].start.Equal(first.StartsAt) {
		t.Fatalf("merged start=%v want %v", a.spans[0].start, first.StartsAt)
	}
	wantEnd := adjacent.StartsAt.AddDate(0, 0, adjacent.PeriodDays)
	if !a.spans[0].end.Equal(wantEnd) {
		t.Fatalf("merged end=%v want %v", a.spans[0].end, wantEnd)
	}
}

// A provider window can cross from an unconfigured gap into a paid cycle. Only
// the paid intersection is usable.
func TestFillProfitQuotaWindows_ClipsWindowCrossingIntoCycle(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	old := anchorTestCycle(time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), 30) // ends 8/3
	cur := anchorTestCycle(time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), 30) // starts 8/5
	anchor := buildProfitWindowAnchor([]*AccountSubscriptionCycle{old, cur}, &AccountCostConfig{AutoRenew: false}, now)

	// Window starts in the gap but overlaps the current cycle from 8/5 onward.
	start := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)
	summary := &AccountProfitSummary{}
	acc := &Account{ID: 1, SessionWindowStart: &start, SessionWindowEnd: &end}
	fillProfitQuotaWindowsWithAnchor(summary, acc, now, anchor)
	if len(summary.QuotaWindows) != 1 {
		t.Fatalf("expected one clipped intersection, got %d", len(summary.QuotaWindows))
	}
	if summary.QuotaWindows[0].StartAt == nil || !summary.QuotaWindows[0].StartAt.Equal(cur.StartsAt) {
		t.Fatalf("clipped start=%v want %v", summary.QuotaWindows[0].StartAt, cur.StartsAt)
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

// Active cycle + auto_renew=true still stops at the latest recorded paid end.
func TestEffectiveRecurringUntil_ActiveRenewRequiresRecordedNextCycle(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	cycle := anchorTestCycle(time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), 30)
	a := buildProfitWindowAnchor([]*AccountSubscriptionCycle{cycle}, &AccountCostConfig{AutoRenew: true}, now)
	want := cycle.StartsAt.AddDate(0, 0, cycle.PeriodDays)
	if got := a.effectiveRecurringUntil(now); got == nil || !got.Equal(want) {
		t.Fatalf("cutoff=%v want %v", got, want)
	}
}

// Expired Grok-like window: live upstream end after subscription end must be
// clipped, and recurring_until_at must be stamped for the frontend projector.
func TestFillProfitQuotaWindows_ExpiredNoRenewClipsLiveGrokWindow(t *testing.T) {
	now := time.Date(2026, 8, 9, 4, 0, 0, 0, time.FixedZone("CST", 8*3600))
	cycleStart := time.Date(2026, 7, 9, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	cycleEnd := cycleStart.AddDate(0, 0, 30) // 2026-08-08
	winStart := time.Date(2026, 8, 6, 13, 31, 0, 0, time.FixedZone("CST", 8*3600))
	winEnd := time.Date(2026, 8, 13, 13, 31, 0, 0, time.FixedZone("CST", 8*3600))

	cycle := &AccountSubscriptionCycle{AccountID: 84, StartsAt: cycleStart, PeriodDays: 30, PeriodFee: 699}
	anchor := buildProfitWindowAnchor([]*AccountSubscriptionCycle{cycle}, &AccountCostConfig{AutoRenew: false}, now)

	acc := &Account{
		ID:       84,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"grok_billing_snapshot": map[string]any{
				"period_start": winStart.Format(time.RFC3339),
				"period_end":   winEnd.Format(time.RFC3339),
				"used_percent": 56.0,
			},
		},
	}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindowsWithAnchor(summary, acc, now, anchor)
	if len(summary.QuotaWindows) == 0 {
		t.Fatalf("expected clipped live window, got none")
	}
	w := summary.QuotaWindows[0]
	if w.RecurringUntilAt == nil || !w.RecurringUntilAt.Equal(cycleEnd) {
		t.Fatalf("RecurringUntilAt=%v want %v", w.RecurringUntilAt, cycleEnd)
	}
	if w.EndAt == nil || !w.EndAt.Equal(cycleEnd) {
		t.Fatalf("EndAt=%v want clipped to %v", w.EndAt, cycleEnd)
	}
	if w.EndAt.After(cycleEnd) {
		t.Fatalf("live end %v must not pass cycle end %v", w.EndAt, cycleEnd)
	}
}

func TestDeriveWaitingActivationGaps_BetweenRealWindows(t *testing.T) {
	firstStart := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	firstEnd := time.Date(2026, 8, 8, 1, 0, 0, 0, time.UTC)
	secondStart := time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)
	secondEnd := secondStart.Add(7 * 24 * time.Hour)
	closed := false
	open := true
	gaps := deriveWaitingActivationGaps([]ProfitQuotaWindow{
		{ID: "7d-1", Kind: "7d", StartAt: &firstStart, EndAt: &firstEnd, IsOpen: &closed},
		{ID: "7d-open", Kind: "7d", StartAt: &secondStart, EndAt: &secondEnd, IsOpen: &open},
	}, secondStart.Add(time.Hour))
	if len(gaps) != 1 {
		t.Fatalf("gaps=%d want 1", len(gaps))
	}
	if gaps[0].Status != "waiting_activation" || gaps[0].StartAt == nil || gaps[0].EndAt == nil ||
		!gaps[0].StartAt.Equal(firstEnd) || !gaps[0].EndAt.Equal(secondStart) {
		t.Fatalf("gap=%+v", gaps[0])
	}
}
