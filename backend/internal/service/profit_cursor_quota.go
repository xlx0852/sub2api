package service

import (
	"context"
	"log/slog"
	"math"
	"time"
)

func cursorProfitSnapshot(usage *UsageInfo, now time.Time) map[string]any {
	if usage == nil || usage.CursorSandUsage == nil || usage.Error != "" || usage.IsForbidden || usage.NeedsReauth {
		return nil
	}
	sand := usage.CursorSandUsage
	start, startErr := time.Parse(time.RFC3339Nano, sand.CurrentPeriodStart)
	end, endErr := time.Parse(time.RFC3339Nano, sand.NextResetTimestampUtc)
	if startErr != nil || endErr != nil || start.After(now) || !end.After(now) || !end.After(start) || math.IsNaN(sand.UsagePercent) || math.IsInf(sand.UsagePercent, 0) || sand.UsagePercent < 0 || sand.UsagePercent > 100 {
		return nil
	}
	minutes := int(end.Sub(start) / time.Minute)
	if minutes <= 0 {
		return nil
	}
	return map[string]any{
		"cursor_sand_reset_at":       end.UTC().Format(time.RFC3339Nano),
		"cursor_sand_window_minutes": minutes,
		"cursor_sand_used_percent":   roundUsagePercent2(sand.UsagePercent),
	}
}

func (s *AccountUsageService) persistCursorProfitWindows(ctx context.Context, account *Account, usage *UsageInfo) {
	if account == nil || account.Platform != PlatformCursor || account.Type != AccountTypeOAuth {
		return
	}
	now := time.Now()
	updates := cursorProfitSnapshot(usage, now)
	if len(updates) == 0 {
		return
	}
	if s.accountRepo != nil {
		if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
			slog.Warn("cursor_profit_snapshot_persist_failed", "account_id", account.ID, "error", err)
			return
		}
	}
	observePlatformQuotaWindowUpdates(ctx, s.quotaWindowLedger, account.ID, account.Platform, updates, now)
}
