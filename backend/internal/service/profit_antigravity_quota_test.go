package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProfitAntigravitySnapshotAndWindows(t *testing.T) {
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	reset := now.Add(48 * time.Hour).Format(time.RFC3339)
	usage := &UsageInfo{AntigravityQuotaGroups: []AntigravityQuotaGroup{{Buckets: []AntigravityQuotaBucket{
		{Window: "7d", RemainingFraction: .6, ResetTime: reset},
		{Window: "7d", RemainingFraction: .8, ResetTime: now.Add(time.Hour).Format(time.RFC3339)},
		{Window: "5h", RemainingFraction: .5, ResetTime: now.Add(time.Hour).Format(time.RFC3339)},
		{Window: "unknown", RemainingFraction: .1, ResetTime: reset},
	}}}}
	extra := antigravityProfitSnapshot(usage, now)
	require.Equal(t, 40.0, extra["antigravity_7d_used_percent"])
	require.Equal(t, reset, extra["antigravity_7d_reset_at"])
	account := &Account{ID: 118, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Extra: extra}
	summary := &AccountProfitSummary{}
	svc := &ProfitService{}
	svc.fillAccountQuotaWindows(context.Background(), summary, account, now, profitWindowAnchor{})
	require.Len(t, summary.QuotaWindows, 1)
	require.Equal(t, "7d", summary.QuotaWindows[0].Kind)
	require.Equal(t, 40.0, *summary.SevenDayUtilization)
	require.Equal(t, now.Add(48*time.Hour), *summary.QuotaWindows[0].EndAt)
	// Other providers must not inherit stale Antigravity fields.
	account.Platform = PlatformOpenAI
	other := &AccountProfitSummary{}
	svc.fillAccountQuotaWindows(context.Background(), other, account, now, profitWindowAnchor{})
	require.Empty(t, other.QuotaWindows)
}

func TestProfitAntigravityRejectsInvalidSnapshots(t *testing.T) {
	now := time.Now()
	for _, reset := range []string{"", "invalid", now.Add(-time.Hour).Format(time.RFC3339), now.Add(8 * 24 * time.Hour).Format(time.RFC3339)} {
		usage := &UsageInfo{AntigravityQuotaGroups: []AntigravityQuotaGroup{{Buckets: []AntigravityQuotaBucket{{Window: "7d", RemainingFraction: .4, ResetTime: reset}}}}}
		require.Empty(t, antigravityProfitSnapshot(usage, now))
	}
}
