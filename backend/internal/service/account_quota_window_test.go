//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memQuotaWindowRepo struct {
	mu   sync.Mutex
	seq  int64
	rows []*AccountQuotaWindow
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
		"kimi_quota_7d_reset_at":      now.Add(7 * 24 * time.Hour).Format(time.RFC3339),
		"kimi_quota_7d_utilization":   0.42,
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

func TestForceResetAccountWindows_Generic(t *testing.T) {
	repo := &memQuotaWindowRepo{}
	l := NewQuotaWindowLedger(repo)
	ctx := context.Background()
	acc := &Account{ID: 22, Platform: PlatformKimi, Extra: map[string]any{
		"kimi_quota_7d_utilization": 0.9,
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
			EndAt: obsEnd, WindowMinutes: 10080, ObservedAt: start.Add(time.Duration(i*7)*time.Minute),
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
