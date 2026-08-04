package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFillProfitQuotaWindows_Codex(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	reset5h := now.Add(2 * time.Hour)
	reset7d := now.Add(3 * 24 * time.Hour)
	acc := &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_5h_used_percent":   42.0,
			"codex_5h_reset_at":       reset5h.Format(time.RFC3339),
			"codex_5h_window_minutes": 300,
			"codex_7d_used_percent":   77.5,
			"codex_7d_reset_at":       reset7d.Format(time.RFC3339),
			"codex_7d_window_minutes": 10080,
		},
	}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, acc, now)

	require.NotNil(t, summary.FiveHourUtilization)
	require.InDelta(t, 42.0, *summary.FiveHourUtilization, 0.01)
	require.NotNil(t, summary.SevenDayUtilization)
	require.InDelta(t, 77.5, *summary.SevenDayUtilization, 0.01)
	require.Len(t, summary.QuotaWindows, 2)

	var five *ProfitQuotaWindow
	for i := range summary.QuotaWindows {
		if summary.QuotaWindows[i].Kind == "5h" {
			five = &summary.QuotaWindows[i]
		}
	}
	require.NotNil(t, five)
	require.NotNil(t, five.EndAt)
	require.WithinDuration(t, reset5h, *five.EndAt, time.Second)
	require.NotNil(t, five.StartAt)
	require.WithinDuration(t, reset5h.Add(-5*time.Hour), *five.StartAt, time.Second)
}

func TestFillProfitQuotaWindows_KimiAndSession(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	reset := now.Add(90 * time.Minute)
	start := now.Add(-210 * time.Minute)
	end := now.Add(90 * time.Minute)
	acc := &Account{
		ID:       2,
		Platform: PlatformKimi,
		Extra: map[string]any{
			"kimi_quota_5h_utilization": 91.0,
			"kimi_quota_5h_reset_at":    reset.Format(time.RFC3339),
		},
		SessionWindowStart: &start,
		SessionWindowEnd:   &end,
	}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, acc, now)
	require.NotNil(t, summary.FiveHourUtilization)
	require.InDelta(t, 91.0, *summary.FiveHourUtilization, 0.01)
	require.GreaterOrEqual(t, len(summary.QuotaWindows), 2)
}

func TestFillProfitQuotaWindows_RejectsPathlessEmpty(t *testing.T) {
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, &Account{ID: 3, Extra: map[string]any{}}, time.Now().UTC())
	require.Nil(t, summary.FiveHourUtilization)
	require.Empty(t, summary.QuotaWindows)
}

func TestFillProfitQuotaWindows_GrokBilling(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	acc := &Account{
		ID:       84,
		Platform: PlatformGrok,
		Extra: map[string]any{
			"grok_usage_snapshot": map[string]any{
				"tokens": map[string]any{
					"limit":     int64(53000000),
					"remaining": int64(53000000),
				},
				"updated_at": "2026-08-01T09:44:09Z",
			},
			"grok_billing_snapshot": map[string]any{
				"period_type":   "weekly",
				"period_start":  "2026-07-30T02:33:56.140418+00:00",
				"period_end":    "2026-08-06T02:33:56.140418+00:00",
				"usage_percent": 68.0,
				"product_usage": []any{
					map[string]any{"product": "GrokBuild", "usage_percent": 46.0},
					map[string]any{"product": "GrokChat"},
				},
			},
		},
	}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, acc, now)
	require.Len(t, summary.QuotaWindows, 1)
	w := summary.QuotaWindows[0]
	require.Equal(t, "grok-billing", w.ID)
	require.Equal(t, "7d", w.Kind)
	require.NotNil(t, w.UsedPercent)
	require.InDelta(t, 46.0, *w.UsedPercent, 0.01)
	require.NotNil(t, w.StartAt)
	require.NotNil(t, w.EndAt)
	require.Equal(t, 2026, w.StartAt.Year())
	require.Equal(t, time.July, w.StartAt.Month())
	require.Equal(t, 30, w.StartAt.Day())
}

func TestFillProfitQuotaWindows_GrokFreeUsageFallback(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	acc := &Account{
		ID:       90,
		Platform: PlatformGrok,
		Extra: map[string]any{
			"grok_usage_snapshot": map[string]any{
				"tokens": map[string]any{
					"limit":     int64(1000000),
					"remaining": int64(250000),
				},
				"updated_at": "2026-08-01T09:25:40Z",
			},
		},
	}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, acc, now)
	require.Len(t, summary.QuotaWindows, 1)
	w := summary.QuotaWindows[0]
	require.Equal(t, "24h", w.Kind)
	require.NotNil(t, w.UsedPercent)
	require.InDelta(t, 75.0, *w.UsedPercent, 0.01)
	require.NotNil(t, w.StartAt)
	require.NotNil(t, w.EndAt)
}

func TestFillProfitQuotaWindows_CapsAtSubscriptionCycleEnd(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	reset := now.Add(3 * 24 * time.Hour)
	cycleEnd := now.Add(24 * time.Hour)
	acc := &Account{Extra: map[string]any{
		"codex_7d_used_percent": 50.0,
		"codex_7d_reset_at":     reset.Format(time.RFC3339),
	}}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, acc, now, &cycleEnd)

	require.Len(t, summary.QuotaWindows, 1)
	w := summary.QuotaWindows[0]
	require.NotNil(t, w.RecurringUntilAt)
	require.WithinDuration(t, cycleEnd, *w.RecurringUntilAt, time.Second)
	require.WithinDuration(t, cycleEnd, *w.EndAt, time.Second)
}

func TestQuotaWindowCutoffForCycles_UsesBanEffectiveAt(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	banAt := now.Add(2 * time.Hour)
	cutoff := quotaWindowCutoffForCycles([]*AccountSubscriptionCycle{{
		StartsAt:    now.Add(-24 * time.Hour),
		PeriodDays:  2,
		Termination: &AccountSubscriptionTermination{EffectiveAt: banAt},
	}}, now)
	require.NotNil(t, cutoff)
	require.WithinDuration(t, banAt, *cutoff, time.Second)
}
