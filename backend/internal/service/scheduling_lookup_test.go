package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSchedulingConfigFromNilUsesDefault(t *testing.T) {
	require.Equal(t, scheduling.DefaultConfig(), schedulingConfigFrom(nil))
}

func TestSchedulingConfigFromUsesGatewayScheduling(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 9
	got := schedulingConfigFrom(cfg)
	require.Equal(t, 9, got.StickySessionMaxWaiting)
}

func TestWaitPlanToSchedulingRoundTrip(t *testing.T) {
	require.Nil(t, waitPlanToScheduling(nil))
	got := waitPlanToScheduling(&AccountWaitPlan{
		AccountID:      7,
		MaxConcurrency: 3,
		Timeout:        2 * time.Second,
		MaxWaiting:     4,
	})
	require.Equal(t, &scheduling.WaitPlan{
		AccountID:      7,
		MaxConcurrency: 3,
		Timeout:        2 * time.Second,
		MaxWaiting:     4,
	}, got)
}
