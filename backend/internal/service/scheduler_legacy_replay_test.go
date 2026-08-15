package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type legacyReplayAccountRepository struct {
	AccountRepository
	accounts []Account
}

func (r *legacyReplayAccountRepository) GetByID(_ context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
		}
	}
	return nil, ErrAccountNotFound
}

func (r *legacyReplayAccountRepository) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	byID := make(map[int64]*Account, len(r.accounts))
	for i := range r.accounts {
		byID[r.accounts[i].ID] = &r.accounts[i]
	}
	result := make([]*Account, 0, len(ids))
	for _, id := range ids {
		if account := byID[id]; account != nil {
			result = append(result, account)
		}
	}
	return result, nil
}

func (r *legacyReplayAccountRepository) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	return legacyReplayAccountsForPlatform(r.accounts, platform, nil), nil
}

func (r *legacyReplayAccountRepository) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

func (r *legacyReplayAccountRepository) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	return legacyReplayAccountsForPlatform(r.accounts, platform, &groupID), nil
}

func (r *legacyReplayAccountRepository) ListSchedulableByPlatforms(_ context.Context, platforms []string) ([]Account, error) {
	return legacyReplayAccountsForPlatforms(r.accounts, platforms, nil), nil
}

func (r *legacyReplayAccountRepository) ListSchedulableUngroupedByPlatforms(_ context.Context, platforms []string) ([]Account, error) {
	return legacyReplayAccountsForPlatforms(r.accounts, platforms, nil), nil
}

func (r *legacyReplayAccountRepository) ListSchedulableByGroupIDAndPlatforms(_ context.Context, groupID int64, platforms []string) ([]Account, error) {
	return legacyReplayAccountsForPlatforms(r.accounts, platforms, &groupID), nil
}

func legacyReplayAccountsForPlatform(accounts []Account, platform string, groupID *int64) []Account {
	return legacyReplayAccountsForPlatforms(accounts, []string{platform}, groupID)
}

func legacyReplayAccountsForPlatforms(accounts []Account, platforms []string, groupID *int64) []Account {
	platformSet := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		platformSet[platform] = struct{}{}
	}
	result := make([]Account, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		if _, ok := platformSet[account.Platform]; !ok || !account.IsSchedulable() {
			continue
		}
		if groupID != nil && !openAIStickyAccountMatchesGroup(&account, groupID) {
			continue
		}
		result = append(result, account)
	}
	return result
}

type legacyReplayGatewayCache struct {
	bindings     map[string]int64
	setCalls     int
	refreshCalls int
	deleteCalls  int
}

func (c *legacyReplayGatewayCache) GetSessionAccountID(_ context.Context, _ int64, sessionHash string) (int64, error) {
	if accountID := c.bindings[sessionHash]; accountID > 0 {
		return accountID, nil
	}
	return 0, ErrAccountNotFound
}

func (c *legacyReplayGatewayCache) SetSessionAccountID(_ context.Context, _ int64, sessionHash string, accountID int64, _ time.Duration) error {
	c.setCalls++
	if c.bindings == nil {
		c.bindings = make(map[string]int64)
	}
	c.bindings[sessionHash] = accountID
	return nil
}

func (c *legacyReplayGatewayCache) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	c.refreshCalls++
	return nil
}

func (c *legacyReplayGatewayCache) DeleteSessionAccountID(_ context.Context, _ int64, sessionHash string) error {
	c.deleteCalls++
	delete(c.bindings, sessionHash)
	return nil
}

type legacyReplayConcurrencyCache struct {
	ConcurrencyCache
	loads          map[int64]*AccountLoadInfo
	acquired       map[int64]bool
	waiting        map[int64]int
	acquireOrder   []int64
	releaseCalls   int
	loadBatchCalls int
}

func (c *legacyReplayConcurrencyCache) AcquireAccountSlot(_ context.Context, accountID int64, _ int, _ string) (bool, error) {
	c.acquireOrder = append(c.acquireOrder, accountID)
	return c.acquired[accountID], nil
}

func (c *legacyReplayConcurrencyCache) ReleaseAccountSlot(_ context.Context, _ int64, _ string) error {
	c.releaseCalls++
	return nil
}

func (c *legacyReplayConcurrencyCache) GetAccountWaitingCount(_ context.Context, accountID int64) (int, error) {
	return c.waiting[accountID], nil
}

func (c *legacyReplayConcurrencyCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	c.loadBatchCalls++
	result := make(map[int64]*AccountLoadInfo, len(accounts))
	for _, account := range accounts {
		if load := c.loads[account.ID]; load != nil {
			copy := *load
			result[account.ID] = &copy
			continue
		}
		result[account.ID] = &AccountLoadInfo{AccountID: account.ID}
	}
	return result, nil
}

type legacyReplayPolicyResult struct {
	AccountID      int64  `json:"account_id"`
	Allowed        bool   `json:"allowed"`
	RejectedPolicy string `json:"rejected_policy,omitempty"`
}

type legacyReplayWaitPlan struct {
	AccountID      int64 `json:"account_id"`
	MaxConcurrency int   `json:"max_concurrency"`
	TimeoutMS      int64 `json:"timeout_ms"`
	MaxWaiting     int   `json:"max_waiting"`
}

type legacyReplaySideEffects struct {
	AcquireOrder   []int64 `json:"acquire_order"`
	ReleaseCalls   int     `json:"release_calls"`
	LoadBatchCalls int     `json:"load_batch_calls"`
	StickySets     int     `json:"sticky_sets"`
	StickyRefresh  int     `json:"sticky_refresh"`
	StickyDeletes  int     `json:"sticky_deletes"`
}

type legacyReplayPath struct {
	RawCandidates []int64                    `json:"raw_candidates"`
	Policies      []legacyReplayPolicyResult `json:"policies"`
	SelectedID    int64                      `json:"selected_id"`
	Acquired      bool                       `json:"acquired"`
	Layer         string                     `json:"layer,omitempty"`
	WaitPlan      *legacyReplayWaitPlan      `json:"wait_plan,omitempty"`
	SideEffects   legacyReplaySideEffects    `json:"side_effects"`
}

type legacyReplayFixture struct {
	Gateway legacyReplayPath `json:"gateway"`
	OpenAI  legacyReplayPath `json:"openai"`
}

func TestLegacySchedulerReplayFixture(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	actual := legacyReplayFixture{
		Gateway: captureLegacyGatewayReplay(t),
		OpenAI:  captureLegacyOpenAIReplay(t),
	}
	actualJSON, err := json.MarshalIndent(actual, "", "  ")
	require.NoError(t, err)

	expectedJSON, err := os.ReadFile("testdata/scheduler_legacy_replay.json")
	require.NoError(t, err)
	require.JSONEq(t, string(expectedJSON), string(actualJSON), "legacy replay changed:\n%s", actualJSON)
}

func captureLegacyGatewayReplay(t *testing.T) legacyReplayPath {
	return captureLegacyGatewayReplayWithRollout(t, scheduling.RolloutOff)
}

func captureLegacyGatewayReplayWithRollout(t *testing.T, mode scheduling.RolloutMode) legacyReplayPath {
	t.Helper()
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	raw := []Account{
		{ID: 1001, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 2, LastUsedAt: legacyReplayTimePtr(now.Add(-2 * time.Hour))},
		{ID: 1002, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 3, LastUsedAt: legacyReplayTimePtr(now.Add(-time.Hour))},
		{ID: 1003, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 0, Status: StatusActive, Schedulable: true, Concurrency: 1},
		{ID: 1004, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 0, Status: StatusActive, Schedulable: false, Concurrency: 1},
		{ID: 1005, Platform: PlatformGemini, Type: AccountTypeAPIKey, Priority: 0, Status: StatusActive, Schedulable: true, Concurrency: 1},
	}
	policyRequest := scheduling.Request{
		Platform:       PlatformAnthropic,
		RequestedModel: "claude-sonnet-4-6",
		ExcludedIDs:    map[int64]struct{}{1003: {}},
	}
	policies := []scheduling.CandidatePolicy{
		scheduling.NotExcludedPolicy,
		scheduling.PlatformPolicy,
		scheduling.GroupMembershipPolicy,
		scheduling.SchedulablePolicy,
		scheduling.SchedulableForModelPolicy,
	}

	repo := &legacyReplayAccountRepository{accounts: raw[:2]}
	cache := &legacyReplayGatewayCache{}
	concurrency := &legacyReplayConcurrencyCache{
		loads: map[int64]*AccountLoadInfo{
			1001: {AccountID: 1001, LoadRate: 80, WaitingCount: 2},
			1002: {AccountID: 1002, LoadRate: 20, WaitingCount: 0},
		},
		acquired: map[int64]bool{1001: false, 1002: false},
		waiting:  map[int64]int{1001: 0, 1002: 0},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.FallbackSelectionMode = "last_used"
	cfg.Gateway.Scheduling.FallbackWaitTimeout = 9 * time.Second
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 4
	cfg.Gateway.OpenAIWS.LBTopK = 2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 0.7
	settingRepo := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		SettingKeySchedulerScoringRollout: fmt.Sprintf(`{"platforms":{"anthropic":%q}}`, mode),
	}}
	service := &GatewayService{
		accountRepo:        repo,
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrency),
		settingService:     NewSettingService(settingRepo, cfg),
	}

	selection, err := service.SelectAccountWithLoadAwareness(
		context.Background(), nil, "legacy-replay-gateway", "claude-sonnet-4-6", nil, "", 0,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)

	return legacyReplayPath{
		RawCandidates: legacyReplayAccountIDs(raw),
		Policies:      legacyReplayPolicyResults(raw, policyRequest, policies),
		SelectedID:    selection.Account.ID,
		Acquired:      selection.Acquired,
		WaitPlan:      legacyReplayWaitPlanFrom(selection.WaitPlan),
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

func captureLegacyOpenAIReplay(t *testing.T) legacyReplayPath {
	return captureOpenAIReplayWithRollout(t, scheduling.RolloutOff)
}

func captureOpenAIReplayWithRollout(t *testing.T, mode scheduling.RolloutMode) legacyReplayPath {
	t.Helper()
	groupID := int64(77)
	raw := []Account{
		{ID: 2001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 2, GroupIDs: []int64{groupID}},
		{ID: 2002, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 3, GroupIDs: []int64{groupID}},
		{ID: 2003, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Priority: 0, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}},
		{ID: 2004, Platform: PlatformGrok, Type: AccountTypeAPIKey, Priority: 0, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}},
		{ID: 2005, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Priority: 0, Status: StatusActive, Schedulable: false, Concurrency: 1, GroupIDs: []int64{groupID}},
	}
	policyRequest := scheduling.Request{
		Platform:            PlatformOpenAI,
		GroupID:             &groupID,
		IncludeFlatGroupIDs: true,
		RequestedModel:      "gpt-5.1",
		ExcludedIDs:         map[int64]struct{}{2003: {}},
	}
	policies := []scheduling.CandidatePolicy{
		scheduling.NotExcludedPolicy,
		scheduling.PlatformPolicy,
		scheduling.GroupMembershipPolicy,
		scheduling.SchedulablePolicy,
		scheduling.SchedulableForModelPolicy,
	}

	repo := &legacyReplayAccountRepository{accounts: raw[:2]}
	cache := &legacyReplayGatewayCache{}
	concurrency := &legacyReplayConcurrencyCache{
		loads: map[int64]*AccountLoadInfo{
			2001: {AccountID: 2001, LoadRate: 75, WaitingCount: 2},
			2002: {AccountID: 2002, LoadRate: 10, WaitingCount: 0},
		},
		acquired: map[int64]bool{2001: false, 2002: false},
		waiting:  map[int64]int{2001: 0, 2002: 0},
	}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.Scheduling.FallbackWaitTimeout = 11 * time.Second
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 5
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 1
	settingRepo := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		openAIAdvancedSchedulerSettingKey: "true",
		SettingKeySchedulerScoringRollout: fmt.Sprintf(`{"platforms":{"openai":%q}}`, mode),
	}}
	service := &OpenAIGatewayService{
		accountRepo:        repo,
		cache:              cache,
		cfg:                cfg,
		rateLimitService:   &RateLimitService{settingService: NewSettingService(settingRepo, cfg)},
		concurrencyService: NewConcurrencyService(concurrency),
	}

	selection, decision, err := service.SelectAccountWithScheduler(
		context.Background(), &groupID, "", "legacy-replay-openai", "gpt-5.1", nil,
		OpenAIUpstreamTransportAny, false,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)

	return legacyReplayPath{
		RawCandidates: legacyReplayAccountIDs(raw),
		Policies:      legacyReplayPolicyResults(raw, policyRequest, policies),
		SelectedID:    selection.Account.ID,
		Acquired:      selection.Acquired,
		Layer:         string(decision.Layer),
		WaitPlan:      legacyReplayWaitPlanFrom(selection.WaitPlan),
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

func TestOpenAIShadowReplayHasNoAdditionalSchedulingSideEffects(t *testing.T) {
	off := captureOpenAIReplayWithRollout(t, scheduling.RolloutOff)
	shadow := captureOpenAIReplayWithRollout(t, scheduling.RolloutShadow)
	require.Equal(t, off, shadow)
}

func TestOpenAISingleAccountWithoutRequestedModelFailsIdenticallyAcrossRolloutModes(t *testing.T) {
	groupID := int64(20)
	for _, mode := range []scheduling.RolloutMode{
		scheduling.RolloutOff,
		scheduling.RolloutShadow,
		scheduling.RolloutEnforce,
	} {
		t.Run(string(mode), func(t *testing.T) {
			repo := &legacyReplayAccountRepository{accounts: []Account{{
				ID:          2020,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				GroupIDs:    []int64{groupID},
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"},
				},
			}}}
			cache := &legacyReplayGatewayCache{}
			concurrency := &legacyReplayConcurrencyCache{}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			settingRepo := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
				openAIAdvancedSchedulerSettingKey: "true",
				SettingKeySchedulerScoringRollout: fmt.Sprintf(`{"platforms":{"openai":%q}}`, mode),
			}}
			service := &OpenAIGatewayService{
				accountRepo:        repo,
				cache:              cache,
				cfg:                cfg,
				rateLimitService:   &RateLimitService{settingService: NewSettingService(settingRepo, cfg)},
				concurrencyService: NewConcurrencyService(concurrency),
			}

			selection, _, err := service.SelectAccountWithSchedulerForCapability(
				context.Background(), &groupID, "", "group-20-luna", "gpt-5.6-luna", nil,
				OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions,
				false, false,
			)
			require.Nil(t, selection)
			require.EqualError(t, err, "no available OpenAI accounts supporting model: gpt-5.6-luna")
			require.Empty(t, concurrency.acquireOrder)
			require.Zero(t, concurrency.loadBatchCalls)
			require.Zero(t, cache.setCalls)
			require.Zero(t, cache.refreshCalls)
			require.Zero(t, cache.deleteCalls)
		})
	}
}

func TestGatewayShadowReplayHasNoAdditionalSchedulingSideEffects(t *testing.T) {
	off := captureLegacyGatewayReplayWithRollout(t, scheduling.RolloutOff)
	shadow := captureLegacyGatewayReplayWithRollout(t, scheduling.RolloutShadow)
	require.Equal(t, off, shadow)
}

func TestGatewayStickyShadowHasNoAdditionalSchedulingSideEffects(t *testing.T) {
	off := captureGatewayStickyReplayWithRollout(t, scheduling.RolloutOff)
	shadow := captureGatewayStickyReplayWithRollout(t, scheduling.RolloutShadow)
	require.Equal(t, off, shadow)
}

func captureGatewayStickyReplayWithRollout(t *testing.T, mode scheduling.RolloutMode) legacyReplayPath {
	t.Helper()
	const sessionHash = "gateway-shadow-sticky"
	accounts := []Account{
		{ID: 1101, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 2},
		{ID: 1102, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 0, Status: StatusActive, Schedulable: true, Concurrency: 2},
	}
	repo := &legacyReplayAccountRepository{accounts: accounts}
	cache := &legacyReplayGatewayCache{bindings: map[string]int64{sessionHash: 1101}}
	concurrency := &legacyReplayConcurrencyCache{acquired: map[int64]bool{1101: true}}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	settingRepo := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		SettingKeySchedulerScoringRollout: fmt.Sprintf(`{"platforms":{"anthropic":%q}}`, mode),
	}}
	service := &GatewayService{
		accountRepo:        repo,
		cache:              cache,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrency),
		settingService:     NewSettingService(settingRepo, cfg),
	}

	selection, err := service.SelectAccountWithLoadAwareness(context.Background(), nil, sessionHash, "claude-sonnet-4-6", nil, "", 0)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	return legacyReplayPath{
		SelectedID: selection.Account.ID,
		Acquired:   selection.Acquired,
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

func TestGatewayShadowComparisonKeepsHardPoliciesAheadOfScoring(t *testing.T) {
	accounts := []Account{
		{ID: 101, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Priority: 1, Status: StatusActive, Schedulable: true},
		{ID: 102, Platform: PlatformGemini, Type: AccountTypeAPIKey, Priority: 0, Status: StatusActive, Schedulable: true},
	}
	allowed := []*Account{&accounts[0]}
	service := &GatewayService{
		cfg:                 &config.Config{},
		gatewayRuntimeStats: scheduling.NewRuntimeStatsStore(time.Hour, 16, 1, nil),
	}
	errorRate, ttft := 0.2, 1200.0
	require.True(t, service.gatewayRuntimeStats.Report(scheduling.RuntimeFeedback{AccountID: 101, ErrorSample: &errorRate, TTFTMillis: &ttft}))
	comparison := service.evaluateGatewayScoringShadow(
		context.Background(), PlatformAnthropic, nil, "shadow-session", "claude-sonnet-4-6", 0, false, false,
		accounts,
		scheduling.CandidateSetDecision{
			AllowedAccounts: allowed,
			Trace: []scheduling.CandidatePolicyTrace{
				{AccountID: 101, Allowed: true},
				{AccountID: 102, RejectedPolicy: "gateway_platform"},
			},
		},
		map[int64]*AccountLoadInfo{101: {AccountID: 101, LoadRate: 40}}, true,
	)

	require.Equal(t, []int64{101, 102}, comparison.CandidateIDs)
	require.Equal(t, []int64{101}, comparison.EligibleCandidateIDs)
	require.Equal(t, []int64{101}, comparison.SharedSelectionOrder)
	require.Equal(t, []int64{101}, comparison.LegacySelectionCandidates)
	require.False(t, comparison.SelectionDiff)
	require.False(t, comparison.HardGateViolation)
	require.False(t, comparison.Fallback)
	require.Len(t, comparison.RuntimeSignals, 1)
	require.True(t, comparison.RuntimeSignals[0].ErrorRateAvailable)
	require.True(t, comparison.RuntimeSignals[0].TTFTAvailable)
	require.Equal(t, int64(1), service.schedulerShadowMetrics.snapshot().EvaluationTotal)
}

func TestSchedulerShadowCountersTrackOpsFields(t *testing.T) {
	var counters schedulerShadowCounters
	first := counters.record(3, true, true, false, false)
	require.Equal(t, schedulerShadowCountersSnapshot{
		EvaluationTotal:     1,
		CandidateTotal:      3,
		SelectionDiffTotal:  1,
		AffinityEscapeTotal: 1,
	}, first)
	second := counters.record(2, false, false, true, true)
	require.Equal(t, int64(2), second.EvaluationTotal)
	require.Equal(t, int64(5), second.CandidateTotal)
	require.Equal(t, int64(1), second.SelectionDiffTotal)
	require.Equal(t, int64(1), second.AffinityEscapeTotal)
	require.Equal(t, int64(1), second.HardGateViolationTotal)
	require.Equal(t, int64(1), second.FallbackTotal)
}

func legacyReplayPolicyResults(accounts []Account, req scheduling.Request, policies []scheduling.CandidatePolicy) []legacyReplayPolicyResult {
	result := make([]legacyReplayPolicyResult, 0, len(accounts))
	for i := range accounts {
		decision := scheduling.EvaluateCandidate(context.Background(), &accounts[i], req, policies...)
		result = append(result, legacyReplayPolicyResult{
			AccountID:      accounts[i].ID,
			Allowed:        decision.Allowed,
			RejectedPolicy: decision.RejectedPolicy,
		})
	}
	return result
}

func legacyReplayAccountIDs(accounts []Account) []int64 {
	result := make([]int64, 0, len(accounts))
	for i := range accounts {
		result = append(result, accounts[i].ID)
	}
	return result
}

func legacyReplayWaitPlanFrom(plan *AccountWaitPlan) *legacyReplayWaitPlan {
	if plan == nil {
		return nil
	}
	return &legacyReplayWaitPlan{
		AccountID:      plan.AccountID,
		MaxConcurrency: plan.MaxConcurrency,
		TimeoutMS:      plan.Timeout.Milliseconds(),
		MaxWaiting:     plan.MaxWaiting,
	}
}

func legacyReplayTimePtr(value time.Time) *time.Time {
	return &value
}

func TestSharedScorerMatchesLegacyOpenAIScoreAndTieBreak(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	setLegacyReplayRuntimeStat(stats, 3001, 0.2, 1200)
	setLegacyReplayRuntimeStat(stats, 3002, 0.6, 4200)
	setLegacyReplayRuntimeStat(stats, 3003, 0.2, 1200)

	accounts := []*Account{
		{ID: 3001, Priority: 1},
		{ID: 3002, Priority: 3},
		{ID: 3003, Priority: 1},
	}
	loads := map[int64]*AccountLoadInfo{
		3001: {AccountID: 3001, LoadRate: 20, WaitingCount: 1},
		3002: {AccountID: 3002, LoadRate: 80, WaitingCount: 4},
		3003: {AccountID: 3003, LoadRate: 20, WaitingCount: 1},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 3
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 0.7
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 0.8
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 0.5
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.PreviousResponse = 5
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.SessionSticky = 3
	service := &OpenAIGatewayService{cfg: cfg}
	scheduler := &defaultOpenAIAccountScheduler{service: service, stats: stats}
	req := OpenAIAccountScheduleRequest{
		SessionHash:             "scorer-characterization",
		StickyWeighted:          true,
		StickyPreviousAccountID: 3001,
		StickyAccountID:         3003,
		PreviousResponseCanMove: true,
	}

	legacyPlan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), req, accounts, loads)
	require.InDelta(t, 8.465, legacyPlan.candidates[0].score, 1e-12)
	require.InDelta(t, 0.520, legacyPlan.candidates[1].score, 1e-12)
	require.InDelta(t, 6.465, legacyPlan.candidates[2].score, 1e-12)
	snapshots := make([]scheduling.CandidateSnapshot, 0, len(legacyPlan.candidates))
	for _, candidate := range legacyPlan.candidates {
		snapshot := scheduling.CandidateSnapshot{
			AccountID: candidate.account.ID,
			Priority:  candidate.priority,
			Signals: scheduling.CandidateSignals{
				LoadRate:   scheduling.AvailableSignal(float64(candidate.loadInfo.LoadRate), time.Time{}),
				QueueDepth: scheduling.AvailableSignal(float64(candidate.loadInfo.WaitingCount), time.Time{}),
				ErrorRate:  scheduling.AvailableSignal(candidate.errorRate, time.Time{}),
			},
			Affinity: scheduling.AffinitySignals{
				PreviousResponse: candidate.account.ID == req.StickyPreviousAccountID,
				SessionSticky:    candidate.account.ID == req.StickyAccountID,
			},
		}
		if candidate.hasTTFT {
			snapshot.Signals.TTFTMillis = scheduling.AvailableSignal(candidate.ttft, time.Time{})
		}
		snapshots = append(snapshots, snapshot)
	}
	weights := scheduling.ScoreWeights{
		Priority:         cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority,
		Load:             cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load,
		Queue:            cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue,
		ErrorRate:        cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate,
		TTFT:             cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT,
		PreviousResponse: cfg.Gateway.OpenAIWS.SchedulerScoreWeights.PreviousResponse,
		SessionSticky:    cfg.Gateway.OpenAIWS.SchedulerScoreWeights.SessionSticky,
	}
	shared := scheduling.ScoreCandidates(snapshots, weights, scheduling.ScoreOptions{StickyWeighted: true})
	require.Len(t, shared, len(legacyPlan.candidates))
	sharedByID := make(map[int64]scheduling.ScoredCandidate, len(shared))
	for _, candidate := range shared {
		sharedByID[candidate.Candidate.AccountID] = candidate
	}
	for _, candidate := range legacyPlan.candidates {
		require.InDelta(t, candidate.score, sharedByID[candidate.account.ID].Score(), 1e-12, "account %d", candidate.account.ID)
	}

	legacyRanked := selectTopKOpenAICandidatesShared(legacyPlan.candidates, len(legacyPlan.candidates))
	sharedRanked := append([]scheduling.ScoredCandidate(nil), shared...)
	scheduling.SortScoredCandidates(sharedRanked)
	require.Equal(t, legacyReplayOpenAIIDs(legacyRanked), legacyReplaySharedIDs(sharedRanked))

	legacyTopK := selectTopKOpenAICandidatesShared(legacyPlan.candidates, 3)
	sharedTopK := scheduling.SelectTopK(shared, 3)
	require.Equal(t, legacyReplayOpenAIIDs(legacyTopK), legacyReplaySharedIDs(sharedTopK))
	require.Equal(t, legacyReplayOpenAIIDs(legacyTopK), legacyReplayOpenAIIDs(selectTopKOpenAICandidatesShared(legacyPlan.candidates, 3)))
	legacyWeighted := buildOpenAIWeightedSelectionOrderShared(legacyTopK, req)
	require.Equal(t, []int64{3001, 3003, 3002}, legacyReplayOpenAIIDs(legacyWeighted))
	seed := scheduling.DeriveSelectionSeed(scheduling.SelectionSeedInput{
		StickyKey:          req.SessionHash,
		PreviousResponseID: req.PreviousResponseID,
		RequestedModel:     req.RequestedModel,
		GroupID:            req.GroupID,
	})
	rng := scheduling.NewSelectionRNG(seed)
	sharedWeighted := scheduling.BuildWeightedSelectionOrder(sharedTopK, &rng)
	require.Equal(t, legacyReplayOpenAIIDs(legacyWeighted), legacyReplaySharedIDs(sharedWeighted))
	require.Equal(t, legacyReplayOpenAIIDs(legacyWeighted), legacyReplayOpenAIIDs(buildOpenAIWeightedSelectionOrderShared(legacyTopK, req)))

	for _, testCase := range []struct {
		name string
		mode scheduling.RolloutMode
	}{
		{name: "off", mode: scheduling.RolloutOff},
		{name: "shadow", mode: scheduling.RolloutShadow},
		{name: "enforce", mode: scheduling.RolloutEnforce},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repo := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
				SettingKeySchedulerScoringRollout: fmt.Sprintf(`{"platforms":{"openai":%q}}`, testCase.mode),
			}}
			modeService := &OpenAIGatewayService{
				cfg:            cfg,
				settingService: NewSettingService(repo, cfg),
			}
			modeScheduler := &defaultOpenAIAccountScheduler{service: modeService, stats: stats}
			modePlan := modeScheduler.buildOpenAIAccountLoadPlan(context.Background(), req, accounts, loads)

			require.Equal(t, testCase.mode, modePlan.scoringMode)
			require.Equal(t, []int64{3001, 3003, 3002}, legacyReplayOpenAIIDs(modePlan.selectionOrder))
			for i, expected := range []float64{8.465, 0.520, 6.465} {
				require.InDelta(t, expected, modePlan.candidates[i].score, 1e-12)
			}
			if testCase.mode == scheduling.RolloutShadow {
				require.NotNil(t, modePlan.shadowComparison)
				require.Equal(t, []int64{3001, 3002, 3003}, modePlan.shadowComparison.CandidateIDs)
				require.Equal(t, []int64{3001, 3003, 3002}, modePlan.shadowComparison.LegacyTopK)
				require.Equal(t, modePlan.shadowComparison.LegacyTopK, modePlan.shadowComparison.SharedTopK)
				require.Equal(t, []int64{3001, 3003, 3002}, modePlan.shadowComparison.LegacySelectionOrder)
				require.Equal(t, modePlan.shadowComparison.LegacySelectionOrder, modePlan.shadowComparison.SharedSelectionOrder)
				require.Zero(t, modePlan.shadowComparison.ScoreDiffCount)
				require.False(t, modePlan.shadowComparison.SelectionDiff)
				require.False(t, modePlan.shadowComparison.HardGateViolation)
			} else {
				require.Nil(t, modePlan.shadowComparison)
			}
		})
	}
}

func TestOpenAIShadowHardGateViolationDetection(t *testing.T) {
	require.False(t, hasOpenAIShadowSelectionOutsideEligible([]int64{3, 1}, []int64{1, 2, 3}))
	require.True(t, hasOpenAIShadowSelectionOutsideEligible([]int64{3, 4}, []int64{1, 2, 3}))
}

func setLegacyReplayRuntimeStat(stats *openAIAccountRuntimeStats, accountID int64, errorRate, ttft float64) {
	stat := stats.loadOrCreate(accountID)
	stat.errorRateEWMABits.Store(math.Float64bits(errorRate))
	stat.ttftEWMABits.Store(math.Float64bits(ttft))
}

func legacyReplayOpenAIIDs(candidates []openAIAccountCandidateScore) []int64 {
	result := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.account.ID)
	}
	return result
}

func legacyReplaySharedIDs(candidates []scheduling.ScoredCandidate) []int64 {
	result := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.Candidate.AccountID)
	}
	return result
}
