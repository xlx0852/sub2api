package service

import (
	"context"
	"log/slog"
	"math"
	"time"
)

// antigravityProfitSnapshot keeps each selected bucket's reset and utilization
// together. Profit reads this persisted snapshot instead of probing upstream.
func antigravityProfitSnapshot(usage *UsageInfo, now time.Time) map[string]any {
	updates := map[string]any{}
	if usage == nil || usage.Error != "" || usage.IsForbidden || usage.NeedsReauth {
		return updates
	}
	for _, group := range usage.AntigravityQuotaGroups {
		for _, bucket := range group.Buckets {
			duration, kind := antigravityQuotaWindowDuration(bucket.Window, bucket.Label)
			if kind == "1d" {
				kind = "24h"
			}
			reset, err := time.Parse(time.RFC3339Nano, bucket.ResetTime)
			if duration <= 0 || err != nil || !reset.After(now) || reset.Add(-duration).After(now) || math.IsNaN(bucket.RemainingFraction) || math.IsInf(bucket.RemainingFraction, 0) || bucket.RemainingFraction < 0 || bucket.RemainingFraction > 1 {
				continue
			}
			prefix := "antigravity_" + kind
			used := roundUsagePercent2((1 - bucket.RemainingFraction) * 100)
			if previous, ok := updates[prefix+"_used_percent"].(float64); ok && previous >= used {
				continue
			}
			updates[prefix+"_used_percent"] = used
			updates[prefix+"_reset_at"] = reset.UTC().Format(time.RFC3339Nano)
			updates[prefix+"_window_minutes"] = int(duration.Minutes())
		}
	}
	return updates
}

func (s *AccountUsageService) persistAntigravityProfitWindows(ctx context.Context, account *Account, usage *UsageInfo) {
	if account == nil || account.Platform != PlatformAntigravity || account.Type != AccountTypeOAuth {
		return
	}
	now := time.Now()
	updates := antigravityProfitSnapshot(usage, now)
	if len(updates) == 0 {
		return
	}
	if s.accountRepo != nil {
		if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
			slog.Warn("antigravity_profit_snapshot_persist_failed", "account_id", account.ID, "error", err)
			return
		}
	}
	observePlatformQuotaWindowUpdates(ctx, s.quotaWindowLedger, account.ID, account.Platform, updates, now)
}

func profitWindowsFromAntigravity(account *Account, now time.Time) []ProfitQuotaWindow {
	var windows []ProfitQuotaWindow
	for _, spec := range platformQuotaWindowSpecs[PlatformAntigravity] {
		if window := profitWindowFromExtra(account.Extra, "antigravity_"+spec.Kind, spec.Kind, spec.Kind, spec.DefaultMins, now); window != nil {
			windows = append(windows, *window)
		}
	}
	return windows
}
