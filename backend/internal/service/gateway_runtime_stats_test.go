package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/stretchr/testify/require"
)

func TestGatewayCandidateSnapshotsKeepMissingSignalsUnavailable(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	service := &GatewayService{gatewayRuntimeStats: scheduling.NewRuntimeStatsStore(time.Hour, 10, 1, func() time.Time { return now })}
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}

	service.reportGatewayCandidateSnapshots(context.Background(), []*Account{account}, nil)
	snapshot := service.gatewayRuntimeStatsSnapshot(account.ID, now)

	require.False(t, snapshot.LoadRate.Available)
	require.False(t, snapshot.QueueDepth.Available)
	require.False(t, snapshot.QuotaHeadroom.Available)
	require.False(t, snapshot.WindowHeadroom.Available)
}

func TestGatewayCandidateSnapshotsCaptureLoadWaitQuotaAndWindow(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	service := &GatewayService{gatewayRuntimeStats: scheduling.NewRuntimeStatsStore(time.Hour, 10, 1, func() time.Time { return now })}
	quotaAccount := &Account{
		ID:          2,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"quota_limit": 100.0,
			"quota_used":  25.0,
		},
	}
	windowAccount := &Account{
		ID:          3,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"window_cost_limit": 20.0,
		},
	}
	ctx := context.WithValue(context.Background(), windowCostPrefetchContextKey, map[int64]float64{3: 5})
	service.reportGatewayCandidateSnapshots(ctx, []*Account{quotaAccount, windowAccount}, map[int64]*AccountLoadInfo{
		2: {AccountID: 2, LoadRate: 40, WaitingCount: 3},
		3: {AccountID: 3, LoadRate: 20, WaitingCount: 1},
	})
	quotaSnapshot := service.gatewayRuntimeStatsSnapshot(quotaAccount.ID, now)
	windowSnapshot := service.gatewayRuntimeStatsSnapshot(windowAccount.ID, now)

	require.Equal(t, 40.0, quotaSnapshot.LoadRate.Value)
	require.Equal(t, 3.0, quotaSnapshot.QueueDepth.Value)
	require.Equal(t, 0.75, quotaSnapshot.QuotaHeadroom.Value)
	require.False(t, quotaSnapshot.WindowHeadroom.Available)
	require.Equal(t, 0.75, windowSnapshot.WindowHeadroom.Value)
	require.False(t, windowSnapshot.QuotaHeadroom.Available)
}

func TestReportGatewayAccountScheduleResultCapturesErrorAndTTFT(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	service := &GatewayService{gatewayRuntimeStats: scheduling.NewRuntimeStatsStore(time.Hour, 10, 0.5, func() time.Time { return now })}
	ttft := 1200

	service.ReportGatewayAccountScheduleResult(4, false, nil)
	service.ReportGatewayAccountScheduleResult(4, true, &ttft)
	snapshot := service.gatewayRuntimeStatsSnapshot(4, now)

	require.InDelta(t, 0.5, snapshot.ErrorRate.Value, 1e-12)
	require.Equal(t, 1200.0, snapshot.TTFTMillis.Value)
}
