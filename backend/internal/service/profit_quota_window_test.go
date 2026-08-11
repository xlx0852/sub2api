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

func TestFillProfitQuotaWindows_AnthropicPassiveSevenDayFallback(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	reset := now.Add(5 * 24 * time.Hour)
	acc := &Account{
		ID: 3, Platform: PlatformAnthropic,
		Extra: map[string]any{
			"passive_usage_7d_utilization": 0.42,
			"passive_usage_7d_reset":       reset.Unix(),
		},
	}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, acc, now)
	require.NotNil(t, summary.SevenDayUtilization)
	require.InDelta(t, 42, *summary.SevenDayUtilization, 0.01)
	require.Len(t, summary.QuotaWindows, 1)
	require.Equal(t, "7d", summary.QuotaWindows[0].Kind)
	require.WithinDuration(t, reset, *summary.QuotaWindows[0].EndAt, time.Second)
}

func TestFillProfitQuotaWindows_GrokUsesTopLevelUsagePercent(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	acc := &Account{ID: 84, Platform: PlatformGrok, Extra: map[string]any{
		"grok_billing_snapshot": map[string]any{
			"period_type":   "unknown",
			"period_start":  now.Add(-10 * 24 * time.Hour).Format(time.RFC3339),
			"period_end":    now.Add(20 * 24 * time.Hour).Format(time.RFC3339),
			"usage_percent": 100.0,
			"product_usage": []any{map[string]any{"product": "GrokBuild", "usage_percent": 86.0}},
		},
	}}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, acc, now)
	require.Len(t, summary.QuotaWindows, 1)
	require.Equal(t, "30d", summary.QuotaWindows[0].Kind)
	require.InDelta(t, 100, *summary.QuotaWindows[0].UsedPercent, 0.01)
}

func TestFillProfitQuotaWindows_GrokIgnoresRetiredCalendarMonthFields(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	acc := &Account{ID: 84, Platform: PlatformGrok, Extra: map[string]any{
		"grok_billing_snapshot": map[string]any{
			"period_type":          "monthly",
			"billing_period_start": "2026-08-01T00:00:00Z",
			"billing_period_end":   "2026-09-01T00:00:00Z",
			"used_percent":         42.0,
		},
	}}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, acc, now)
	require.Empty(t, summary.QuotaWindows)
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
	require.InDelta(t, 68.0, *w.UsedPercent, 0.01)
	require.NotNil(t, w.StartAt)
	require.NotNil(t, w.EndAt)
	require.Equal(t, 2026, w.StartAt.Year())
	require.Equal(t, time.July, w.StartAt.Month())
	require.Equal(t, 30, w.StartAt.Day())
}

func TestFillProfitQuotaWindows_GrokFreeUsageFallback(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	// Official Free traffic (empty base_url → api.x.ai) may invent a 24h bar.
	acc := &Account{
		ID:       90,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
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

// Third-party apikey relays often echo limit=1e6 rate-limit headers that are
// NOT xAI Free's 24h rolling budget. Do not invent a 24h timeline for them.
func TestFillProfitQuotaWindows_GrokRelayApikeyNoFake24h(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	acc := &Account{
		ID:       90,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-relay",
			"base_url": "https://api.biuapi.com",
		},
		Extra: map[string]any{
			"grok_usage_snapshot": map[string]any{
				"tokens": map[string]any{
					"limit":     int64(1000000),
					"remaining": int64(1000000),
				},
				"requests": map[string]any{
					"limit":     int64(21),
					"remaining": int64(21),
				},
				"updated_at": "2026-08-05T02:50:10Z",
			},
		},
	}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, acc, now)
	require.Empty(t, summary.QuotaWindows)
}

// When the relay (or official host) provides an explicit reset, still render.
func TestFillProfitQuotaWindows_GrokRelayWithExplicitReset(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	resetAt := now.Add(6 * time.Hour)
	acc := &Account{
		ID:       91,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-relay",
			"base_url": "https://api.biuapi.com",
		},
		Extra: map[string]any{
			"grok_usage_snapshot": map[string]any{
				"tokens": map[string]any{
					"limit":     int64(1000000),
					"remaining": int64(500000),
					"reset_at":  resetAt.Format(time.RFC3339),
				},
				"updated_at": "2026-08-01T09:00:00Z",
			},
		},
	}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, acc, now)
	require.Len(t, summary.QuotaWindows, 1)
	w := summary.QuotaWindows[0]
	require.Equal(t, "24h", w.Kind)
	require.NotNil(t, w.UsedPercent)
	require.InDelta(t, 50.0, *w.UsedPercent, 0.01)
	require.NotNil(t, w.EndAt)
	require.WithinDuration(t, resetAt.UTC(), w.EndAt.UTC(), time.Second)
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

func TestQuotaWindowCutoffForCycles_ExpiredCycleNoCutoff(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	// Bookkeeping cycle ended Jul 29; live free quota may still roll for 30d.
	cutoff := quotaWindowCutoffForCycles([]*AccountSubscriptionCycle{{
		StartsAt:   time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC),
		PeriodDays: 30,
	}}, now)
	require.Nil(t, cutoff)
}

func TestFillProfitQuotaWindows_ExpiredCycleKeepsLive30d(t *testing.T) {
	now := time.Date(2026, 8, 6, 9, 30, 0, 0, time.FixedZone("CST", 8*3600))
	reset30d := time.Date(2026, 8, 28, 13, 51, 58, 0, time.FixedZone("CST", 8*3600))
	acc := &Account{
		ID:       69,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_7d_used_percent":        100.0,
			"codex_7d_reset_at":            reset30d.Format(time.RFC3339),
			"codex_7d_window_minutes":      43200.0,
			"codex_primary_window_minutes": 43200.0,
		},
	}
	// Simulate former bug path: expired cycle end as cutoff.
	cycleEnd := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	summary := &AccountProfitSummary{}
	// With expired cutoff no longer applied by quotaWindowCutoffForCycles; pass nil.
	fillProfitQuotaWindows(summary, acc, now, nil)
	require.NotEmpty(t, summary.QuotaWindows)

	var longWin *ProfitQuotaWindow
	for i := range summary.QuotaWindows {
		if summary.QuotaWindows[i].Kind == "30d" {
			longWin = &summary.QuotaWindows[i]
		}
	}
	require.NotNil(t, longWin, "live 30d window must survive expired bookkeeping cycle")
	require.Equal(t, "30d", longWin.Label)
	require.NotNil(t, longWin.WindowMinutes)
	require.Equal(t, 43200, *longWin.WindowMinutes)

	// Even if a stale cutoff is passed, keep the live window (start after cycle end).
	summary2 := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary2, acc, now, &cycleEnd)
	// Current backend still caps/drops when cutoff is forced; expired-cycle path
	// should avoid supplying cutoff. Assert the preferred helper returns nil cutoff.
	require.Nil(t, quotaWindowCutoffForCycles([]*AccountSubscriptionCycle{{
		StartsAt:   time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC),
		PeriodDays: 30,
	}}, now))
	_ = summary2
}

func TestFillProfitQuotaWindows_CodexFree30d(t *testing.T) {
	now := time.Date(2026, 8, 6, 9, 22, 0, 0, time.UTC)
	reset30d := now.Add(24 * 24 * time.Hour)
	acc := &Account{
		ID:       69,
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			"codex_7d_used_percent":        100.0,
			"codex_7d_reset_at":            reset30d.Format(time.RFC3339),
			"codex_7d_window_minutes":      43200,
			"codex_primary_window_minutes": 43200,
		},
	}
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, acc, now)

	require.NotNil(t, summary.SevenDayUtilization)
	require.InDelta(t, 100.0, *summary.SevenDayUtilization, 0.01)
	require.NotEmpty(t, summary.QuotaWindows)

	var longWin *ProfitQuotaWindow
	for i := range summary.QuotaWindows {
		if summary.QuotaWindows[i].WindowMinutes != nil && *summary.QuotaWindows[i].WindowMinutes == 43200 {
			longWin = &summary.QuotaWindows[i]
		}
	}
	require.NotNil(t, longWin)
	require.Equal(t, "30d", longWin.Kind)
	require.Equal(t, "30d", longWin.Label)
	require.NotNil(t, longWin.StartAt)
	require.NotNil(t, longWin.EndAt)
	require.WithinDuration(t, reset30d.Add(-30*24*time.Hour), *longWin.StartAt, time.Minute)
}

func TestClassifyQuotaWindow(t *testing.T) {
	kind, label := classifyQuotaWindow("7d", 43200)
	require.Equal(t, "30d", kind)
	require.Equal(t, "30d", label)

	kind, label = classifyQuotaWindow("7d", 10080)
	require.Equal(t, "7d", kind)
	require.Equal(t, "7d", label)

	kind, label = classifyQuotaWindow("5h", 300)
	require.Equal(t, "5h", kind)
	require.Equal(t, "5h", label)
}
