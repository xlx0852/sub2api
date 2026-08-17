package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpsServiceGetOpenAICompatibilityFallbackMetrics(t *testing.T) {
	svc := &OpsService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.SessionHashReadOldFallback = true
	svc.cfg.Gateway.OpenAIWS.SessionHashDualWriteOld = false

	got := svc.GetOpenAICompatibilityFallbackMetrics()

	require.Equal(t, "process", got.Scope)
	require.True(t, got.ReadOldFallbackEnabled)
	require.False(t, got.DualWriteOldEnabled)
	require.Equal(t, 600, got.LegacyTTLMaxSeconds)
	require.False(t, got.ObservedAt.IsZero())
	require.Equal(t, SnapshotOpenAICompatibilityFallbackMetrics(), got.Metrics)
}
