//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBuildProfitWindowAnchor_CoveredStartUsesEarliestCycle 验证修复：
// 用户记了多笔账（多个成本周期）时，历史配额窗口应投影回最早记账周期起点，
// 而非只回当前活跃周期起点——否则当前周期之前的配额窗口全部不显示。
// 复现线上 #69：记账 06-29 + 08-07，7d 历史窗口（07 月）应可见。
func TestBuildProfitWindowAnchor_CoveredStartUsesEarliestCycle(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	first := anchorTestCycle(time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC), 30) // ends 7/29
	second := anchorTestCycle(time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC), 30) // ends 9/6
	a := buildProfitWindowAnchor([]*AccountSubscriptionCycle{first, second}, &AccountCostConfig{}, now)

	require.NotNil(t, a.coveredStart, "多周期账号必须有 coveredStart")
	if !a.coveredStart.Equal(first.StartsAt) {
		t.Fatalf("coveredStart=%v want earliest cycle start %v", a.coveredStart, first.StartsAt)
	}
}

// TestBuildProfitWindowAnchor_CoveredStartSingleCycle 单周期账号行为不变
// （coveredStart = 该周期起点，历史投影回该周期起点）。
func TestBuildProfitWindowAnchor_CoveredStartSingleCycle(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	cycle := anchorTestCycle(time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC), 30)
	a := buildProfitWindowAnchor([]*AccountSubscriptionCycle{cycle}, &AccountCostConfig{}, now)
	require.NotNil(t, a.coveredStart)
	if !a.coveredStart.Equal(cycle.StartsAt) {
		t.Fatalf("coveredStart=%v want %v", a.coveredStart, cycle.StartsAt)
	}
}
