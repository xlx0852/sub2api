//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAttachFullWindowEstimate_LinearProjection(t *testing.T) {
	stats := &WindowStats{Requests: 23600, Tokens: 3_100_000_000, Cost: 1525.51, UserCost: 183.06}
	attachFullWindowEstimate(stats, 59)
	require.NotNil(t, stats.FullRequests)
	require.NotNil(t, stats.FullTokens)
	require.NotNil(t, stats.FullCost)
	require.NotNil(t, stats.FullUserCost)
	// 23600 / 0.59 ≈ 40000
	require.InDelta(t, 40000, float64(*stats.FullRequests), 50)
	require.InDelta(t, 183.06/0.59, *stats.FullUserCost, 0.5)
	require.InDelta(t, 1525.51/0.59, *stats.FullCost, 1.0)
}

func TestAttachFullWindowEstimate_SkipsZeroOrOver(t *testing.T) {
	stats := &WindowStats{Requests: 100, Tokens: 1000, Cost: 10, UserCost: 1}
	attachFullWindowEstimate(stats, 0)
	require.Nil(t, stats.FullRequests)
	require.Equal(t, "insufficient", stats.FullEstimate.Confidence)
	attachFullWindowEstimate(stats, 120)
	require.Nil(t, stats.FullRequests)
}

func TestAttachFullWindowEstimate_HidesBelowFivePercent(t *testing.T) {
	stats := &WindowStats{Requests: 100, Tokens: 1000, Cost: 20, UserCost: 2}
	attachFullWindowEstimate(stats, 4)
	require.Nil(t, stats.FullCost)
	require.NotNil(t, stats.FullEstimate)
	require.Equal(t, "insufficient", stats.FullEstimate.Method)
}

func TestAttachFullWindowEstimate_UsesIncrementalObservations(t *testing.T) {
	now := time.Now()
	stats := &WindowStats{Requests: 400, Tokens: 4000, Cost: 80, StandardCost: 80, UserCost: 8}
	observations := []*AccountQuotaUsageObservation{
		{ObservedAt: now.Add(-time.Hour), UsedPercent: 10, Requests: 100, Tokens: 1000, AccountCost: 20, StandardCost: 20, UserCost: 2},
		{ObservedAt: now, UsedPercent: 20, Requests: 300, Tokens: 3000, AccountCost: 50, StandardCost: 50, UserCost: 5},
	}
	attachFullWindowEstimateFromObservations(stats, 20, observations)
	require.NotNil(t, stats.FullEstimate)
	require.Equal(t, "incremental", stats.FullEstimate.Method)
	require.Equal(t, "medium", stats.FullEstimate.Confidence)
	require.InDelta(t, 300, *stats.FullCost, 0.01)
	require.Equal(t, int64(2000), *stats.FullRequests)
	require.Less(t, *stats.FullEstimate.LowerCost, *stats.FullCost)
	require.Greater(t, *stats.FullEstimate.UpperCost, *stats.FullCost)
}

func TestAttachFullWindowEstimate_IgnoresDecreasingSamples(t *testing.T) {
	now := time.Now()
	stats := &WindowStats{Requests: 400, Tokens: 4000, Cost: 80, StandardCost: 80, UserCost: 8}
	observations := []*AccountQuotaUsageObservation{
		{ObservedAt: now.Add(-time.Hour), UsedPercent: 10, Requests: 300, Tokens: 3000, AccountCost: 60, StandardCost: 60, UserCost: 6},
		{ObservedAt: now, UsedPercent: 20, Requests: 200, Tokens: 2000, AccountCost: 50, StandardCost: 50, UserCost: 5},
	}
	attachFullWindowEstimateFromObservations(stats, 20, observations)
	require.Equal(t, "cumulative", stats.FullEstimate.Method)
	require.Equal(t, "low", stats.FullEstimate.Confidence)
}

func TestAttachFullWindowEstimates_PerWindow(t *testing.T) {
	usage := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 0, WindowStats: &WindowStats{Requests: 10, Tokens: 100, Cost: 1, UserCost: 0.1}},
		SevenDay: &UsageProgress{Utilization: 50, WindowStats: &WindowStats{Requests: 100, Tokens: 1000, Cost: 20, UserCost: 2}},
	}
	attachFullWindowEstimates(usage)
	require.Nil(t, usage.FiveHour.WindowStats.FullRequests)
	require.NotNil(t, usage.SevenDay.WindowStats.FullRequests)
	require.Equal(t, int64(200), *usage.SevenDay.WindowStats.FullRequests)
	require.InDelta(t, 4.0, *usage.SevenDay.WindowStats.FullUserCost, 0.001)
}
