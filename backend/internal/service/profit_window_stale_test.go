//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestProfitWindowFromExtra_StaleResetAtSkipped 验证修复：上游 reset_at 明显早于当前时间
// （如 error 号不再抓取、extra 陈旧）时，不渲染为"当前窗口"。
// 复现线上 #103（GPT-自有 1，error 状态，reset_at 已过期 5 天）。
func TestProfitWindowFromExtra_StaleResetAtSkipped(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	stale := now.Add(-5 * 24 * time.Hour) // 5 天前过期
	w := profitWindowFromExtra(map[string]any{
		"codex_7d_reset_at":       stale.Format(time.RFC3339),
		"codex_7d_window_minutes": int64(10080),
		"codex_7d_used_percent":   float64(3),
	}, "codex_7d", "7d", "7d", 10080, now)
	require.Nil(t, w, "明显过期的 reset_at 不应渲染为当前窗口")
}

// TestProfitWindowFromExtra_FreshResetAtKept 验证正常窗口（end 在未来）不受影响。
func TestProfitWindowFromExtra_FreshResetAtKept(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	future := now.Add(6 * 24 * time.Hour)
	w := profitWindowFromExtra(map[string]any{
		"codex_7d_reset_at":       future.Format(time.RFC3339),
		"codex_7d_window_minutes": int64(10080),
		"codex_7d_used_percent":   float64(0),
	}, "codex_7d", "7d", "7d", 10080, now)
	require.NotNil(t, w)
	require.Equal(t, "7d", w.Kind)
	if w.EndAt == nil || !w.EndAt.Equal(future) {
		t.Fatalf("EndAt=%v want %v", w.EndAt, future)
	}
}

// TestProfitWindowFromExtra_JustPastResetKept 验证容差：reset_at 刚过（< 10% 窗口）
// 仍渲染（正常重置瞬间的上游滞后），不误伤。
func TestProfitWindowFromExtra_JustPastResetKept(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	justPast := now.Add(-5 * time.Minute) // 5 分钟前
	w := profitWindowFromExtra(map[string]any{
		"codex_7d_reset_at":       justPast.Format(time.RFC3339),
		"codex_7d_window_minutes": int64(10080),
	}, "codex_7d", "7d", "7d", 10080, now)
	require.NotNil(t, w, "刚过期的 reset_at 在容差内应保留")
}
