package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGatewayEnforceUsesSharedOrderAfterRuntimeStatsWarm(t *testing.T) {
	off := captureGatewayScoringEnforceSelection(t, scheduling.RolloutOff)
	shadow := captureGatewayScoringEnforceSelection(t, scheduling.RolloutShadow)
	enforce := captureGatewayScoringEnforceSelection(t, scheduling.RolloutEnforce)

	require.Equal(t, int64(61002), off.SelectedID)
	require.Equal(t, off, shadow, "shadow must keep the legacy live selection and side effects")
	require.Equal(t, int64(61001), enforce.SelectedID, "enforce must use the warmed shared score order")
	require.Equal(t, []int64{61001}, enforce.SideEffects.AcquireOrder)
	require.Equal(t, 1, enforce.SideEffects.LoadBatchCalls)
}

func TestGatewayEnforceFallsBackWhenRuntimeStatsAreNotWarm(t *testing.T) {
	off := captureLegacyGatewayReplayWithRollout(t, scheduling.RolloutOff)
	enforce := captureLegacyGatewayReplayWithRollout(t, scheduling.RolloutEnforce)
	require.Equal(t, off, enforce)
}

func TestGatewayEnforcePreservesStickyLivePath(t *testing.T) {
	off := captureGatewayStickyReplayWithRollout(t, scheduling.RolloutOff)
	enforce := captureGatewayStickyReplayWithRollout(t, scheduling.RolloutEnforce)
	require.Equal(t, off, enforce)
}

func captureGatewayScoringEnforceSelection(t *testing.T, mode scheduling.RolloutMode) legacyReplayPath {
	t.Helper()
	accounts := []Account{
		{ID: 61001, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 2},
		{ID: 61002, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 2},
	}
	repo := &legacyReplayAccountRepository{accounts: accounts}
	cache := &legacyReplayGatewayCache{}
	concurrency := &legacyReplayConcurrencyCache{
		loads: map[int64]*AccountLoadInfo{
			61001: {AccountID: 61001, LoadRate: 80, WaitingCount: 2},
			61002: {AccountID: 61002, LoadRate: 10, WaitingCount: 0},
		},
		acquired: map[int64]bool{61001: true, 61002: true},
		waiting:  map[int64]int{61001: 0, 61002: 0},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 2
	settingRepo := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		SettingKeySchedulerScoringRollout: fmt.Sprintf(`{"platforms":{"anthropic":%q}}`, mode),
	}}
	stats := scheduling.NewRuntimeStatsStore(time.Hour, 16, 1, nil)
	service := &GatewayService{
		accountRepo:         repo,
		cache:               cache,
		cfg:                 cfg,
		concurrencyService:  NewConcurrencyService(concurrency),
		settingService:      NewSettingService(settingRepo, cfg),
		gatewayRuntimeStats: stats,
	}
	goodError, goodTTFT := 0.0, 100.0
	badError, badTTFT := 1.0, 5000.0
	require.True(t, stats.Report(scheduling.RuntimeFeedback{AccountID: 61001, ErrorSample: &goodError, TTFTMillis: &goodTTFT}))
	require.True(t, stats.Report(scheduling.RuntimeFeedback{AccountID: 61002, ErrorSample: &badError, TTFTMillis: &badTTFT}))

	selection, err := service.SelectAccountWithLoadAwareness(
		context.Background(), nil, "gateway-enforce-fresh", "claude-sonnet-4-6", nil, "", 0,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	return legacyReplayPath{
		SelectedID: selection.Account.ID,
		Acquired:   selection.Acquired,
		WaitPlan:   legacyReplayWaitPlanFrom(selection.WaitPlan),
		SideEffects: legacyReplaySideEffects{
			AcquireOrder:   append([]int64(nil), concurrency.acquireOrder...),
			ReleaseCalls:   concurrency.releaseCalls,
			LoadBatchCalls: concurrency.loadBatchCalls,
			StickySets:     cache.setCalls,
			StickyRefresh:  cache.refreshCalls,
			StickyDeletes:  cache.deleteCalls,
		},
	}
}
