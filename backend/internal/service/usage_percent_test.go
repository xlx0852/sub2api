package service

import (
	"math"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoundUsagePercent2(t *testing.T) {
	t.Parallel()

	require.Equal(t, 12.35, roundUsagePercent2(12.345))
	require.Equal(t, 12.34, roundUsagePercent2(12.344))
	require.Equal(t, 7.0, roundUsagePercent2(7))
	require.Equal(t, 0.42, roundUsagePercent2(0.42))
	require.True(t, math.IsNaN(roundUsagePercent2(math.NaN())))
}

func TestRoundUsageRatio2(t *testing.T) {
	t.Parallel()

	require.Equal(t, 0.1235, roundUsageRatio2(0.12345))
	require.Equal(t, 0.15, roundUsageRatio2(0.15))
}

func TestParseCodexRateLimitHeadersRoundsUsedPercent(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "12.345")
	headers.Set("x-codex-secondary-used-percent", "7.144")
	headers.Set("x-codex-primary-over-secondary-limit-percent", "3.14159")

	snapshot := ParseCodexRateLimitHeaders(headers)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.PrimaryUsedPercent)
	require.Equal(t, 12.35, *snapshot.PrimaryUsedPercent)
	require.NotNil(t, snapshot.SecondaryUsedPercent)
	require.Equal(t, 7.14, *snapshot.SecondaryUsedPercent)
	require.NotNil(t, snapshot.PrimaryOverSecondaryPercent)
	require.InDelta(t, 3.14159, *snapshot.PrimaryOverSecondaryPercent, 1e-9)
}
