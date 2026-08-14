package scheduling

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestStickyWaitPlanUsesStickyLimits(t *testing.T) {
	cfg := config.GatewaySchedulingConfig{
		StickySessionWaitTimeout: 12 * time.Second,
		StickySessionMaxWaiting:  4,
		FallbackWaitTimeout:      30 * time.Second,
		FallbackMaxWaiting:       100,
	}
	got := StickyWaitPlan(7, 3, cfg)
	require.Equal(t, WaitPlan{
		AccountID:      7,
		MaxConcurrency: 3,
		Timeout:        12 * time.Second,
		MaxWaiting:     4,
	}, got)
}

func TestFallbackWaitPlanUsesFallbackLimits(t *testing.T) {
	cfg := config.GatewaySchedulingConfig{
		StickySessionWaitTimeout: 45 * time.Second,
		StickySessionMaxWaiting:  3,
		FallbackWaitTimeout:      9 * time.Second,
		FallbackMaxWaiting:       11,
	}
	got := FallbackWaitPlan(8, 2, cfg)
	require.Equal(t, WaitPlan{
		AccountID:      8,
		MaxConcurrency: 2,
		Timeout:        9 * time.Second,
		MaxWaiting:     11,
	}, got)
}

func TestDefaultConfigMatchesLegacyNilConfigFallback(t *testing.T) {
	cfg := DefaultConfig()
	require.Equal(t, 3, cfg.StickySessionMaxWaiting)
	require.Equal(t, 45*time.Second, cfg.StickySessionWaitTimeout)
	require.Equal(t, 30*time.Second, cfg.FallbackWaitTimeout)
	require.Equal(t, 100, cfg.FallbackMaxWaiting)
	require.True(t, cfg.LoadBatchEnabled)
	require.Equal(t, 30*time.Second, cfg.SlotCleanupInterval)
}
