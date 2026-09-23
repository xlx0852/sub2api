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

func TestGatewayCandidateSnapshotsCaptureLoadWaitAndQuota(t *testing.T) {
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

	service.reportGatewayCandidateSnapshots(context.Background(), []*Account{quotaAccount}, map[int64]*AccountLoadInfo{
		2: {AccountID: 2, LoadRate: 40, WaitingCount: 3},
	})
	quotaSnapshot := service.gatewayRuntimeStatsSnapshot(quotaAccount.ID, now)

	require.Equal(t, 40.0, quotaSnapshot.LoadRate.Value)
	require.Equal(t, 3.0, quotaSnapshot.QueueDepth.Value)
	require.Equal(t, 0.75, quotaSnapshot.QuotaHeadroom.Value)
	require.False(t, quotaSnapshot.WindowHeadroom.Available)
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
