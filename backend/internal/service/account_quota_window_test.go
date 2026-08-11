//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

type memQuotaWindowRepo struct {
	mu           sync.Mutex
	seq          int64
	rows         []*AccountQuotaWindow
	observations []*AccountQuotaUsageObservation
}

func (m *memQuotaWindowRepo) HasObservation(_ context.Context, quotaWindowID int64, usedPercent float64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, observation := range m.observations {
		if observation.QuotaWindowID == quotaWindowID && observation.UsedPercent == usedPercent {
			return true, nil
		}
	}
	return false, nil
}

func (m *memQuotaWindowRepo) InsertObservation(_ context.Context, observation *AccountQuotaUsageObservation) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.observations {
		if existing.QuotaWindowID == observation.QuotaWindowID && existing.UsedPercent == observation.UsedPercent {
			return false, nil
		}
	}
	cp := *observation
	m.observations = append(m.observations, &cp)
	return true, nil
}

func (m *memQuotaWindowRepo) ListObservations(_ context.Context, quotaWindowID int64, limit int) ([]*AccountQuotaUsageObservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*AccountQuotaUsageObservation, 0)
	for _, observation := range m.observations {
		if observation.QuotaWindowID != quotaWindowID {
			continue
		}
		cp := *observation
		result = append(result, &cp)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

type quotaObservationStatsStub struct {
	calls int
	stats *usagestats.AccountStats
}

func (s *quotaObservationStatsStub) GetAccountWindowStatsRange(_ context.Context, _ int64, _, _ time.Time) (*usagestats.AccountStats, error) {
	s.calls++
	return s.stats, nil
}

func (m *memQuotaWindowRepo) ListByAccount(_ context.Context, accountID int64, kind string, limit int) ([]*AccountQuotaWindow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*AccountQuotaWindow
	for i := len(m.rows) - 1; i >= 0; i-- {
		r := m.rows[i]
		if r.AccountID != accountID {
			continue
		}
		if kind != "" && r.Kind != kind {
			continue
		}
		cp := *r
		out = append(out, &cp)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
func (m *memQuotaWindowRepo) ListOpenByAccount(ctx context.Context, accountID int64) ([]*AccountQuotaWindow, error) {
	all, _ := m.ListByAccount(ctx, accountID, "", 100)
	var out []*AccountQuotaWindow
	for _, r := range all {
		if r.IsOpen {
			out = append(out, r)
		}
	}
	return out, nil
}
func (m *memQuotaWindowRepo) GetOpen(_ context.Context, accountID int64, kind string) (*AccountQuotaWindow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.AccountID == accountID && r.Kind == kind && r.IsOpen {
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *memQuotaWindowRepo) InsertOpen(_ context.Context, w *AccountQuotaWindow) (*AccountQuotaWindow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	cp := *w
	cp.ID = m.seq
	cp.IsOpen = true
	cp.CreatedAt = time.Now()
	cp.UpdatedAt = cp.CreatedAt
	m.rows = append(m.rows, &cp)
	out := cp
	return &out, nil
}
func (m *memQuotaWindowRepo) UpsertOpenRefresh(_ context.Context, accountID int64, kind string, endAt time.Time, used *float64, windowMinutes *int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.AccountID == accountID && r.Kind == kind && r.IsOpen {
			r.EndAt = endAt
			if used != nil {
				r.UsedPercentOpen = used
			}
			if windowMinutes != nil {
				r.WindowMinutes = windowMinutes
			}
			r.UpdatedAt = time.Now()
			return nil
		}
	}
	return nil
}
func (m *memQuotaWindowRepo) CloseOpen(_ context.Context, accountID int64, kind string, endAt time.Time, reason string, used *float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.AccountID == accountID && r.Kind == kind && r.IsOpen {
			r.IsOpen = false
			r.EndAt = endAt
			r.ClosedReason = reason
			r.UsedPercentClose = used
			r.UpdatedAt = time.Now()
		}
	}
	return nil
}
func (m *memQuotaWindowRepo) CloseAndOpen(_ context.Context, closeID int64, closeEnd time.Time, closeReason string, closeUsed *float64, open *AccountQuotaWindow) (*AccountQuotaWindow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.ID == closeID && r.IsOpen {
			r.IsOpen = false
			r.EndAt = closeEnd
			r.ClosedReason = closeReason
			r.UsedPercentClose = closeUsed
			r.UpdatedAt = time.Now()
		}
	}
	m.seq++
	cp := *open
	cp.ID = m.seq
	cp.IsOpen = true
	cp.CreatedAt = time.Now()
	cp.UpdatedAt = cp.CreatedAt
	m.rows = append(m.rows, &cp)
	out := cp
	return &out, nil
}

func TestQuotaWindowLedger_CapturesOnlyChangedUtilization(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	ledger := NewQuotaWindowLedger(repo)
	statsReader := &quotaObservationStatsStub{stats: &usagestats.AccountStats{
		Requests: 100, Tokens: 1000, Cost: 20, StandardCost: 20, UserCost: 2,
	}}
	ledger.SetObservationStatsReader(statsReader)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	used := 10.0
	observe := func(at time.Time, percent *float64) {
		if err := ledger.ObserveUpstream(ctx, QuotaWindowObservation{
			AccountID: 69, Platform: PlatformOpenAI, Kind: "7d",
			EndAt: at.Add(7 * 24 * time.Hour), WindowMinutes: 10080,
			UsedPercent: percent, ObservedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	observe(t0, &used)
	observe(t0.Add(time.Minute), &used)
	if statsReader.calls != 1 || len(repo.observations) != 1 {
		t.Fatalf("duplicate percentage should not be aggregated: calls=%d observations=%d", statsReader.calls, len(repo.observations))
	}
	used = 11
	observe(t0.Add(2*time.Minute), &used)
	if statsReader.calls != 2 || len(repo.observations) != 2 {
		t.Fatalf("changed percentage should be sampled: calls=%d observations=%d", statsReader.calls, len(repo.observations))
	}
}

func TestQuotaWindowLedger_ObserveDetectsPassiveReset(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	l := NewQuotaWindowLedger(repo)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	// seed open week ending Aug 8
	if err := l.ObserveUpstream(ctx, QuotaWindowObservation{
		AccountID: 69, Platform: PlatformOpenAI, Kind: "7d",
		EndAt: t0.Add(7 * 24 * time.Hour), WindowMinutes: 10080, ObservedAt: t0,
	}); err != nil {
		t.Fatal(err)
	}
	open, _ := repo.GetOpen(ctx, 69, "7d")
	if open == nil || !open.StartAt.Equal(t0) {
		t.Fatalf("seed open=%+v", open)
	}
	// passive reset: new end jumps to Aug 15 from "now" Aug 8 12:00, start becomes Aug 8 12:00
	now := t0.Add(7*24*time.Hour + 12*time.Hour)
	newEnd := now.Add(7 * 24 * time.Hour)
	used := 5.0
	if err := l.ObserveUpstream(ctx, QuotaWindowObservation{
		AccountID: 69, Platform: PlatformOpenAI, Kind: "7d",
		EndAt: newEnd, WindowMinutes: 10080, UsedPercent: &used, ObservedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.ListByAccount(ctx, 69, "7d", 10)
	if len(rows) < 2 {
		t.Fatalf("want closed+open, got %d", len(rows))
	}
	// newest first in ListByAccount implementation of mem is reverse append order - check open
	open, _ = repo.GetOpen(ctx, 69, "7d")
	if open == nil || !open.EndAt.Equal(newEnd) {
		t.Fatalf("new open=%+v", open)
	}
	// closed row exists
	var closed *AccountQuotaWindow
	for _, r := range rows {
		if !r.IsOpen {
			closed = r
			break
		}
	}
	if closed == nil || closed.ClosedReason != QuotaWindowCloseObserved {
		t.Fatalf("closed=%+v", closed)
	}
}

func TestQuotaWindowLedger_ForceResetCard(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	l := NewQuotaWindowLedger(repo)
	ctx := context.Background()
	start := time.Now().Add(-3 * 24 * time.Hour).UTC().Truncate(time.Second)
	_, _ = repo.InsertOpen(ctx, &AccountQuotaWindow{
		AccountID: 1, Platform: PlatformOpenAI, Kind: "7d",
		StartAt: start, EndAt: start.Add(7 * 24 * time.Hour), WindowMinutes: qwIntPtr(10080),
		Source: QuotaWindowSourceObserved, IsOpen: true,
	})
	if err := l.ForceResetCard(ctx, 1, PlatformOpenAI, "7d", 10080, qwFloatPtr(88)); err != nil {
		t.Fatal(err)
	}
	open, _ := repo.GetOpen(ctx, 1, "7d")
	if open == nil || open.Source != QuotaWindowSourceResetCard {
		t.Fatalf("open=%+v", open)
	}
	if open.StartAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatalf("reset start should be ~now, got %v", open.StartAt)
	}
	rows, _ := repo.ListByAccount(ctx, 1, "7d", 10)
	var closed *AccountQuotaWindow
	for _, r := range rows {
		if !r.IsOpen {
			closed = r
		}
	}
	if closed == nil || closed.ClosedReason != QuotaWindowCloseResetCard {
		t.Fatalf("closed=%+v", closed)
	}
}

func qwFloatPtr(v float64) *float64 { return &v }

func TestObservePlatformQuotaWindowUpdates_KimiAndGrok(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	l := NewQuotaWindowLedger(repo)
	ctx := context.Background()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

	// Kimi 7d
	observePlatformQuotaWindowUpdates(ctx, l, 10, PlatformKimi, map[string]any{
		"kimi_quota_7d_reset_at":    now.Add(7 * 24 * time.Hour).Format(time.RFC3339),
		"kimi_quota_7d_utilization": 42.0,
	}, now)
	open, _ := repo.GetOpen(ctx, 10, "7d")
	if open == nil || open.Platform != PlatformKimi {
		t.Fatalf("kimi open=%+v", open)
	}
	if open.UsedPercentOpen == nil || *open.UsedPercentOpen < 41 || *open.UsedPercentOpen > 43 {
		t.Fatalf("kimi used%%=%v want ~42", open.UsedPercentOpen)
	}

	// Grok billing nested
	observePlatformQuotaWindowUpdates(ctx, l, 11, PlatformGrok, map[string]any{
		grokBillingSnapshotKey: map[string]any{
			"period_start":  now.Add(-3 * 24 * time.Hour).Format(time.RFC3339),
			"period_end":    now.Add(4 * 24 * time.Hour).Format(time.RFC3339),
			"usage_percent": 56.0,
		},
	}, now)
	gopen, _ := repo.GetOpen(ctx, 11, "7d")
	if gopen == nil || gopen.Platform != PlatformGrok {
		t.Fatalf("grok open=%+v", gopen)
	}
}

func TestObservePlatformQuotaWindowUpdates_GrokStructUsesBuildPercentAndExplicitDuration(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	ledger := NewQuotaWindowLedger(repo)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	total := 100.0
	build := 86.0
	snapshot := &xai.BillingSnapshot{
		PeriodType: "unknown", UsagePercent: &total,
		PeriodStart:  now.Add(-10 * 24 * time.Hour).Format(time.RFC3339),
		PeriodEnd:    now.Add(20 * 24 * time.Hour).Format(time.RFC3339),
		ProductUsage: []xai.BillingProductUsage{{Product: "GrokBuild", UsagePercent: &build}},
	}
	observePlatformQuotaWindowUpdates(context.Background(), ledger, 83, PlatformGrok, map[string]any{
		grokBillingSnapshotKey: snapshot,
	}, now)
	open, _ := repo.GetOpen(context.Background(), 83, "30d")
	if open == nil {
		t.Fatal("monthly Grok struct snapshot must create a 30d ledger window")
	}
	if open.UsedPercentOpen == nil || *open.UsedPercentOpen != 86 {
		t.Fatalf("used_percent_open=%v want GrokBuild 86", open.UsedPercentOpen)
	}
}

func TestObservePlatformQuotaWindowUpdates_GrokIgnoresRetiredCalendarBillingPeriod(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	ledger := NewQuotaWindowLedger(repo)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	observePlatformQuotaWindowUpdates(context.Background(), ledger, 83, PlatformGrok, map[string]any{
		grokBillingSnapshotKey: map[string]any{
			"period_type":          "monthly",
			"billing_period_start": "2026-08-01T00:00:00Z",
			"billing_period_end":   "2026-09-01T00:00:00Z",
			"used_percent":         42.0,
		},
	}, now)
	rows, _ := repo.ListByAccount(context.Background(), 83, "", 10)
	if len(rows) != 0 {
		t.Fatalf("retired calendar billing period must not create quota windows: %+v", rows)
	}
}

func TestForceResetAccountWindows_Generic(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	l := NewQuotaWindowLedger(repo)
	ctx := context.Background()
	acc := &Account{ID: 22, Platform: PlatformKimi, Extra: map[string]any{
		"kimi_quota_7d_utilization": 90.0,
	}}
	// seed
	_, _ = repo.InsertOpen(ctx, &AccountQuotaWindow{
		AccountID: 22, Platform: PlatformKimi, Kind: "7d",
		StartAt: time.Now().Add(-48 * time.Hour), EndAt: time.Now().Add(5 * 24 * time.Hour),
		WindowMinutes: qwIntPtr(10080), IsOpen: true,
	})
	ForceResetAccountWindows(ctx, l, acc, nil)
	open, _ := repo.GetOpen(ctx, 22, "7d")
	if open == nil || open.Source != QuotaWindowSourceResetCard {
		t.Fatalf("open after force=%+v", open)
	}
}

func TestQuotaWindowLedger_BackwardEndDoesNotCut(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	l := NewQuotaWindowLedger(repo)
	ctx := context.Background()
	start := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)
	_, _ = repo.InsertOpen(ctx, &AccountQuotaWindow{
		AccountID: 69, Platform: PlatformOpenAI, Kind: "7d",
		StartAt: start, EndAt: end, WindowMinutes: qwIntPtr(10080),
		Source: QuotaWindowSourceObserved, IsOpen: true,
	})
	// Observed end earlier than open end (upstream tighter view / late snapshot).
	obsEnd := end.Add(-2 * 24 * time.Hour)
	if err := l.ObserveUpstream(ctx, QuotaWindowObservation{
		AccountID: 69, Platform: PlatformOpenAI, Kind: "7d",
		EndAt: obsEnd, WindowMinutes: 10080, ObservedAt: end.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.ListByAccount(ctx, 69, "7d", 10)
	if len(rows) != 1 {
		t.Fatalf("backward end must not cut history, got %d rows", len(rows))
	}
	open, _ := repo.GetOpen(ctx, 69, "7d")
	if open == nil || !open.EndAt.Equal(obsEnd) {
		t.Fatalf("open should refresh end to %v, got %+v", obsEnd, open)
	}
}

func TestQuotaWindowLedger_ForwardEndCuts(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	l := NewQuotaWindowLedger(repo)
	ctx := context.Background()
	start := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)
	_, _ = repo.InsertOpen(ctx, &AccountQuotaWindow{
		AccountID: 70, Platform: PlatformOpenAI, Kind: "7d",
		StartAt: start, EndAt: end, WindowMinutes: qwIntPtr(10080),
		Source: QuotaWindowSourceObserved, IsOpen: true,
	})
	// New cycle truly starts after the old one (no overlap): end far enough that
	// end-7d > open.end.
	newEnd := end.Add(8 * 24 * time.Hour)
	obsAt := end.Add(24 * time.Hour)
	if err := l.ObserveUpstream(ctx, QuotaWindowObservation{
		AccountID: 70, Platform: PlatformOpenAI, Kind: "7d",
		EndAt: newEnd, WindowMinutes: 10080, ObservedAt: obsAt,
	}); err != nil {
		t.Fatal(err)
	}
	rows, _ := repo.ListByAccount(ctx, 70, "7d", 10)
	if len(rows) < 2 {
		t.Fatalf("forward end should close+open, got %d", len(rows))
	}
}

func TestQuotaWindowLedger_DriftSameCycleRefreshes(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	l := NewQuotaWindowLedger(repo)
	ctx := context.Background()
	start := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)
	_, _ = repo.InsertOpen(ctx, &AccountQuotaWindow{
		AccountID: 80, Platform: PlatformOpenAI, Kind: "7d",
		StartAt: start, EndAt: end, WindowMinutes: qwIntPtr(10080),
		Source: QuotaWindowSourceObserved, IsOpen: true,
	})
	// Same upstream cycle reported slightly earlier/later due to reset_after+now drift.
	for i := 1; i <= 6; i += 1 {
		obsEnd := end.Add(time.Duration(i*7) * time.Minute)
		if err := l.ObserveUpstream(ctx, QuotaWindowObservation{
			AccountID: 80, Platform: PlatformOpenAI, Kind: "7d",
			EndAt: obsEnd, WindowMinutes: 10080, ObservedAt: start.Add(time.Duration(i*7) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	rows, _ := repo.ListByAccount(ctx, 80, "7d", 20)
	if len(rows) != 1 {
		t.Fatalf("relative drift must not cut history, got %d rows", len(rows))
	}
	open, _ := repo.GetOpen(ctx, 80, "7d")
	if open == nil {
		t.Fatal("no open")
	}
	// end should track latest observation, not stick
	if !open.EndAt.After(end) {
		t.Fatalf("open end not refreshed: %+v", open)
	}
}

func TestQuotaWindowLedger_EarlyResetCutsEvenWhenPeriodsOverlap(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	l := NewQuotaWindowLedger(repo)
	ctx := context.Background()
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)
	oldUsed := 72.0
	_, _ = repo.InsertOpen(ctx, &AccountQuotaWindow{
		AccountID: 91, Platform: PlatformOpenAI, Kind: "7d",
		StartAt: start, EndAt: end, WindowMinutes: qwIntPtr(10080),
		UsedPercentOpen: &oldUsed, Source: QuotaWindowSourceObserved, IsOpen: true,
	})

	activatedAt := start.Add(4 * 24 * time.Hour)
	newEnd := activatedAt.Add(7 * 24 * time.Hour)
	newUsed := 1.0
	if err := l.ObserveUpstream(ctx, QuotaWindowObservation{
		AccountID: 91, Platform: PlatformOpenAI, Kind: "7d",
		EndAt: newEnd, WindowMinutes: 10080, UsedPercent: &newUsed, ObservedAt: activatedAt,
	}); err != nil {
		t.Fatal(err)
	}

	rows, _ := repo.ListByAccount(ctx, 91, "7d", 10)
	if len(rows) != 2 {
		t.Fatalf("early overlapping reset must create two real windows, got %d", len(rows))
	}
	open, _ := repo.GetOpen(ctx, 91, "7d")
	if open == nil || !open.StartAt.Equal(activatedAt) || !open.EndAt.Equal(newEnd) {
		t.Fatalf("new open=%+v", open)
	}
	var closed *AccountQuotaWindow
	for _, row := range rows {
		if !row.IsOpen {
			closed = row
			break
		}
	}
	if closed == nil || !closed.EndAt.Equal(activatedAt) || closed.ClosedReason != QuotaWindowCloseObserved {
		t.Fatalf("closed=%+v", closed)
	}
}

func TestQuotaWindowLedger_FirstUseAfterExpiredWindowLeavesActivationGap(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	l := NewQuotaWindowLedger(repo)
	ctx := context.Background()
	start := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)
	oldUsed := 85.0
	_, _ = repo.InsertOpen(ctx, &AccountQuotaWindow{
		AccountID: 92, Platform: PlatformOpenAI, Kind: "7d",
		StartAt: start, EndAt: end, WindowMinutes: qwIntPtr(10080),
		UsedPercentOpen: &oldUsed, Source: QuotaWindowSourceObserved, IsOpen: true,
	})

	zero := 0.0
	if err := l.ObserveUpstream(ctx, QuotaWindowObservation{
		AccountID: 92, Platform: PlatformOpenAI, Kind: "7d",
		EndAt: end, WindowMinutes: 10080, UsedPercent: &zero, ObservedAt: end,
	}); err != nil {
		t.Fatal(err)
	}
	if open, _ := repo.GetOpen(ctx, 92, "7d"); open != nil {
		t.Fatalf("provider clear must leave the account waiting without an open row: %+v", open)
	}
	if !l.IsWaitingActivation(ctx, 92, "7d", end.Add(time.Minute)) {
		t.Fatal("cleared quota must suppress inference probes until first use")
	}

	activatedAt := end.Add(8 * time.Hour)
	newEnd := activatedAt.Add(7 * 24 * time.Hour)
	newUsed := 0.5
	if err := l.ObserveUpstream(ctx, QuotaWindowObservation{
		AccountID: 92, Platform: PlatformOpenAI, Kind: "7d",
		EndAt: newEnd, WindowMinutes: 10080, UsedPercent: &newUsed, ObservedAt: activatedAt,
	}); err != nil {
		t.Fatal(err)
	}

	rows, _ := repo.ListByAccount(ctx, 92, "7d", 10)
	if len(rows) != 2 {
		t.Fatalf("first use after expiry must create a new row, got %d", len(rows))
	}
	var closed *AccountQuotaWindow
	for _, row := range rows {
		if !row.IsOpen {
			closed = row
			break
		}
	}
	if closed == nil || !closed.EndAt.Equal(end) {
		t.Fatalf("old window must close at its known provider reset, got %+v", closed)
	}
	open, _ := repo.GetOpen(ctx, 92, "7d")
	if open == nil || !open.StartAt.Equal(activatedAt) {
		t.Fatalf("new window must start on first use, got %+v", open)
	}
	if l.IsWaitingActivation(ctx, 92, "7d", activatedAt.Add(time.Minute)) {
		t.Fatal("active successor must leave waiting-activation state")
	}
}
