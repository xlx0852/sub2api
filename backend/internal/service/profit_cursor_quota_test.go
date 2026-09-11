package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type profitProviderLedgerStub struct {
	AccountQuotaWindowRepository
	rows []*AccountQuotaWindow
}

func (r profitProviderLedgerStub) ListByAccountExceptKind(context.Context, int64, string, int) ([]*AccountQuotaWindow, error) {
	return r.rows, nil
}

func TestProfitProviderLedgerMergesActualDurationAndFiltersPlatform(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(-24*time.Hour), now.Add(29*24*time.Hour)
	mins, used := 43200, 20.0
	account := &Account{ID: 121, Platform: PlatformCursor, Extra: map[string]any{
		"cursor_sand_reset_at": end.Format(time.RFC3339), "cursor_sand_used_percent": used, "cursor_sand_window_minutes": mins,
	}}
	svc := &ProfitService{quotaWindowRepo: profitProviderLedgerStub{rows: []*AccountQuotaWindow{
		{ID: 1, AccountID: 121, Platform: PlatformCursor, Kind: "7d", StartAt: start, EndAt: end, WindowMinutes: &mins, UsedPercentOpen: &used, IsOpen: true},
		{ID: 2, AccountID: 121, Platform: PlatformOpenAI, Kind: "7d", StartAt: start, EndAt: end, WindowMinutes: &mins, IsOpen: true},
	}}}
	summary := &AccountProfitSummary{}
	svc.fillAccountQuotaWindows(context.Background(), summary, account, now, profitWindowAnchor{})
	require.Len(t, summary.QuotaWindows, 1)
	require.Equal(t, "30d", summary.QuotaWindows[0].Kind)
}

func TestProfitCursorUsesOfficialPeriod(t *testing.T) {
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	start, end := now.Add(-2*24*time.Hour), now.Add(28*24*time.Hour)
	usage := &UsageInfo{CursorSandUsage: &CursorSandUsageStatus{UsagePercent: 25, CurrentPeriodStart: start.Format(time.RFC3339), NextResetTimestampUtc: end.Format(time.RFC3339)}}
	extra := cursorProfitSnapshot(usage, now)
	require.Equal(t, 43200, extra["cursor_sand_window_minutes"])
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, &Account{Platform: PlatformCursor, Extra: extra}, now)
	require.Len(t, summary.QuotaWindows, 1)
	require.Equal(t, "30d", summary.QuotaWindows[0].Kind)
	require.Equal(t, start, *summary.QuotaWindows[0].StartAt)
	usage.CursorSandUsage.CurrentPeriodStart = ""
	require.Nil(t, cursorProfitSnapshot(usage, now))
}

func TestProfitProviderSnapshotIsolation(t *testing.T) {
	now := time.Now()
	extra := map[string]any{
		"codex_7d_used_percent": 10.0, "codex_7d_reset_at": now.Add(time.Hour).Format(time.RFC3339),
		"kimi_quota_7d_utilization": 20.0, "kimi_quota_7d_reset_at": now.Add(time.Hour).Format(time.RFC3339),
		"grok_billing_snapshot": map[string]any{"period_type": "weekly", "period_start": now.Add(-time.Hour).Format(time.RFC3339), "period_end": now.Add(time.Hour).Format(time.RFC3339), "usage_percent": 30.0},
	}
	for _, platform := range []string{PlatformOpenAI, PlatformKimi, PlatformGrok, PlatformCursor, PlatformAntigravity, PlatformAnthropic} {
		t.Run(platform, func(t *testing.T) {
			summary := &AccountProfitSummary{}
			fillProfitQuotaWindows(summary, &Account{Platform: platform, Extra: extra}, now)
			if platform == PlatformCursor || platform == PlatformAntigravity || platform == PlatformAnthropic {
				require.Empty(t, summary.QuotaWindows)
			} else {
				require.Len(t, summary.QuotaWindows, 1)
			}
		})
	}
}
