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

type schedulerRolloutDiffScenario struct {
	name   string
	mutate func(accounts []Account, req *OpenAIAccountScheduleRequest, cfg *config.Config, cache *legacyReplayGatewayCache, concurrency *legacyReplayConcurrencyCache)
}

func TestOpenAISchedulerRolloutDifferentialMatrix(t *testing.T) {
	future := func() *time.Time {
		value := time.Now().Add(time.Hour)
		return &value
	}
	scenarios := []schedulerRolloutDiffScenario{
		{name: "sync"},
		{name: "sse", mutate: func(_ []Account, req *OpenAIAccountScheduleRequest, _ *config.Config, _ *legacyReplayGatewayCache, _ *legacyReplayConcurrencyCache) {
			req.RequiredTransport = OpenAIUpstreamTransportHTTPSSE
		}},
		{name: "ws", mutate: func(accounts []Account, req *OpenAIAccountScheduleRequest, cfg *config.Config, _ *legacyReplayGatewayCache, _ *legacyReplayConcurrencyCache) {
			req.RequiredTransport = OpenAIUpstreamTransportResponsesWebsocketV2
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			for i := range accounts {
				accounts[i].Extra["openai_apikey_responses_websockets_v2_enabled"] = true
			}
		}},
		{name: "compact", mutate: func(accounts []Account, req *OpenAIAccountScheduleRequest, _ *config.Config, _ *legacyReplayGatewayCache, _ *legacyReplayConcurrencyCache) {
			req.RequireCompact = true
			accounts[0].Extra["openai_compact_mode"] = OpenAICompactModeForceOff
			accounts[1].Extra["openai_compact_mode"] = OpenAICompactModeForceOn
		}},
		{name: "media", mutate: func(_ []Account, req *OpenAIAccountScheduleRequest, _ *config.Config, _ *legacyReplayGatewayCache, _ *legacyReplayConcurrencyCache) {
			req.RequiredImageCapability = OpenAIImagesCapabilityNative
		}},
		{name: "sticky", mutate: func(_ []Account, req *OpenAIAccountScheduleRequest, _ *config.Config, cache *legacyReplayGatewayCache, _ *legacyReplayConcurrencyCache) {
			req.SessionHash = "rollout-diff-sticky"
			req.StickyAccountID = 52002
			cache.bindings[req.SessionHash] = req.StickyAccountID
		}},
		{name: "excluded", mutate: func(_ []Account, req *OpenAIAccountScheduleRequest, _ *config.Config, _ *legacyReplayGatewayCache, _ *legacyReplayConcurrencyCache) {
			req.ExcludedIDs = map[int64]struct{}{52001: {}}
		}},
		{name: "429", mutate: func(accounts []Account, _ *OpenAIAccountScheduleRequest, _ *config.Config, _ *legacyReplayGatewayCache, _ *legacyReplayConcurrencyCache) {
			accounts[0].RateLimitResetAt = future()
		}},
		{name: "5xx", mutate: func(accounts []Account, _ *OpenAIAccountScheduleRequest, _ *config.Config, _ *legacyReplayGatewayCache, _ *legacyReplayConcurrencyCache) {
			accounts[0].OverloadUntil = future()
		}},
		{name: "credential_failure", mutate: func(accounts []Account, _ *OpenAIAccountScheduleRequest, _ *config.Config, _ *legacyReplayGatewayCache, _ *legacyReplayConcurrencyCache) {
			accounts[0].TempUnschedulableUntil = future()
		}},
		{name: "wait_plan", mutate: func(_ []Account, _ *OpenAIAccountScheduleRequest, _ *config.Config, _ *legacyReplayGatewayCache, concurrency *legacyReplayConcurrencyCache) {
			concurrency.acquired = map[int64]bool{52001: false, 52002: false}
		}},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			off := captureOpenAISchedulerRolloutDiff(t, scheduling.RolloutOff, scenario)
			shadow := captureOpenAISchedulerRolloutDiff(t, scheduling.RolloutShadow, scenario)
			enforce := captureOpenAISchedulerRolloutDiff(t, scheduling.RolloutEnforce, scenario)
			require.Equal(t, off, shadow, "shadow changed live scheduling result or side effects")
			require.Equal(t, off, enforce, "shared scorer diverged from the characterized legacy decision")
		})
	}
}

func captureOpenAISchedulerRolloutDiff(t *testing.T, mode scheduling.RolloutMode, scenario schedulerRolloutDiffScenario) legacyReplayPath {
	t.Helper()
	accounts := []Account{
		{
			ID: 52001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Priority: 0, Status: StatusActive, Schedulable: true, Concurrency: 2,
			Extra: map[string]any{},
		},
		{
			ID: 52002, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 2,
			Extra: map[string]any{},
		},
	}
	req := OpenAIAccountScheduleRequest{
		Platform:           PlatformOpenAI,
		RequestedModel:     "gpt-5.1",
		RequiredTransport:  OpenAIUpstreamTransportAny,
		RequiredCapability: OpenAIEndpointCapabilityChatCompletions,
	}
	cfg := newSchedulerTestOpenAIWSV2Config()
	cfg.RunMode = config.RunModeStandard
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 0.7
	cache := &legacyReplayGatewayCache{bindings: make(map[string]int64)}
	concurrency := &legacyReplayConcurrencyCache{
		loads: map[int64]*AccountLoadInfo{
			52001: {AccountID: 52001, LoadRate: 10, WaitingCount: 0},
			52002: {AccountID: 52002, LoadRate: 70, WaitingCount: 2},
		},
		acquired: map[int64]bool{52001: true, 52002: true},
		waiting:  map[int64]int{52001: 0, 52002: 0},
	}
	if scenario.mutate != nil {
		scenario.mutate(accounts, &req, cfg, cache, concurrency)
	}
	repo := &legacyReplayAccountRepository{accounts: accounts}
	settingRepo := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		openAIAdvancedSchedulerSettingKey:       "true",
		SettingKeySchedulerScoringRollout:       fmt.Sprintf(`{"platforms":{"openai":%q}}`, mode),
		SettingKeyOpenAIAdvancedSchedulerLBTopK: "1",
	}}
	service := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrency),
		settingService:     NewSettingService(settingRepo, cfg),
	}
	scheduler := newDefaultOpenAIAccountScheduler(service, newOpenAIAccountRuntimeStats())

	selection, decision, err := scheduler.Select(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	return legacyReplayPath{
		SelectedID: selection.Account.ID,
		Acquired:   selection.Acquired,
		Layer:      decision.Layer,
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

func TestGatewayScorerDifferentialRecordsAttributableSelectionChange(t *testing.T) {
	accounts := []Account{
		{ID: 53001, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 0, Status: StatusActive, Schedulable: true, Concurrency: 2},
		{ID: 53002, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 2},
		{ID: 53003, Platform: PlatformGemini, Type: AccountTypeAPIKey, Priority: 0, Status: StatusActive, Schedulable: true, Concurrency: 2},
	}
	service := &GatewayService{
		cfg:                 &config.Config{},
		gatewayRuntimeStats: scheduling.NewRuntimeStatsStore(time.Hour, 16, 1, nil),
	}
	for _, account := range accounts[:2] {
		errorRate, ttft := 0.1, 900.0
		require.True(t, service.gatewayRuntimeStats.Report(scheduling.RuntimeFeedback{
			AccountID:   account.ID,
			ErrorSample: &errorRate,
			TTFTMillis:  &ttft,
		}))
	}
	request := scheduling.Request{
		Platform:       PlatformAnthropic,
		RequestedModel: "claude-sonnet-4-6",
	}
	decision := service.evaluateGatewayCandidates(context.Background(), candidatesFromAccountValues(accounts), request)
	comparison := service.evaluateGatewayScoringShadow(
		context.Background(), PlatformAnthropic, nil, "gateway-diff", request.RequestedModel, 0, false, false,
		accounts, decision,
		map[int64]*AccountLoadInfo{
			53001: {AccountID: 53001, LoadRate: 10, WaitingCount: 0},
			53002: {AccountID: 53002, LoadRate: 70, WaitingCount: 2},
		},
		true,
	)

	require.Equal(t, []int64{53001, 53002}, comparison.EligibleCandidateIDs)
	require.Equal(t, "gateway_platform", comparison.PolicyTrace[2].RejectedPolicy)
	require.Equal(t, []int64{53001}, comparison.LegacySelectionCandidates)
	require.Equal(t, int64(53001), firstInt64(comparison.SharedRank), "score rank must remain attributable to priority/load inputs")
	require.ElementsMatch(t, []int64{53001, 53002}, comparison.SharedTopK)
	sharedSelectedID := firstInt64(comparison.SharedSelectionOrder)
	require.Contains(t, comparison.EligibleCandidateIDs, sharedSelectedID)
	require.Equal(t, sharedSelectedID != 53001, comparison.SelectionDiff)
	require.False(t, comparison.HardGateViolation)
	require.False(t, comparison.Fallback)
}
