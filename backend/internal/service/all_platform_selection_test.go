package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestAllPlatformsUsePriorityLoadSelection(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformGrok, PlatformKimi, PlatformAnthropic, PlatformGemini, PlatformAntigravity} {
		t.Run(platform, func(t *testing.T) {
			model := "gpt-6-astra"
			if platform == PlatformAnthropic {
				model = "claude-sonnet-4-6"
			}
			if platform == PlatformGemini || platform == PlatformAntigravity {
				model = "gemini-3.1-pro"
			}
			accounts := []Account{{ID: 1, Platform: platform, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 1}, {ID: 2, Platform: platform, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 1}}
			for i := range accounts {
				accounts[i].Credentials = map[string]any{"model_mapping": map[string]any{model: model}}
			}
			cc := schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{1: {AccountID: 1, LoadRate: 80}, 2: {AccountID: 2, LoadRate: 10}}}
			repo := &legacyReplayAccountRepository{accounts: accounts}
			ctx := context.Background()
			var sel *AccountSelectionResult
			var err error
			if platform == PlatformOpenAI || platform == PlatformGrok || platform == PlatformKimi {
				svc := &OpenAIGatewayService{accountRepo: repo, cfg: &config.Config{}, cache: &schedulerTestGatewayCache{sessionBindings: map[string]int64{}}, concurrencyService: NewConcurrencyService(cc), rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("false")}
				// No opt-in, including when the historical advanced toggle is false.
				sel, _, err = svc.SelectAccountWithSchedulerForCapability(ctx, nil, "", "new", model, nil, OpenAIUpstreamTransportAny, "", false, false, platform)
			} else {
				svc := &GatewayService{accountRepo: repo, cfg: &config.Config{}, concurrencyService: NewConcurrencyService(cc)}
				ctx = context.WithValue(ctx, ctxkey.ForcePlatform, platform)
				sel, err = svc.SelectAccountWithLoadAwareness(ctx, nil, "", model, nil, "", 0)
			}
			require.NoError(t, err)
			require.NotNil(t, sel)
			require.Equal(t, int64(2), sel.Account.ID)
			require.True(t, sel.Acquired)
			sel.ReleaseFunc()
		})
	}
}

func TestAllPlatformsSharedOrderingIgnoresOldScoringSignals(t *testing.T) {
	now := time.Now()
	a, b := &Account{ID: 1, Priority: 1}, &Account{ID: 2, Priority: 1, LastUsedAt: &now, Extra: map[string]any{"codex_7d_used_percent": 100.0}}
	seen := map[int64]bool{}
	for i := 0; i < 100; i++ {
		order := simpleAccountOrder([]*Account{a, b}, map[int64]*AccountLoadInfo{1: {LoadRate: 0}, 2: {LoadRate: 0}})
		seen[order[0].ID] = true
	}
	require.Len(t, seen, 2, "neither unknown quota nor LRU can pin all requests")
}

func TestAllPlatformsGatewayBusyStickyYields(t *testing.T) {
	a, b := simpleTestAccount(1, AccountTypeAPIKey), simpleTestAccount(2, AccountTypeAPIKey)
	for _, acc := range []*Account{&a, &b} {
		acc.Platform = PlatformAnthropic
		acc.GroupIDs = nil
		acc.Credentials = nil
	}
	cache := &legacyReplayGatewayCache{bindings: map[string]int64{"busy": 1}}
	svc := &GatewayService{accountRepo: &legacyReplayAccountRepository{accounts: []Account{a, b}}, cache: cache, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{1: false, 2: true}}), cfg: &config.Config{}}
	sel, err := svc.SelectAccountWithLoadAwareness(context.Background(), nil, "busy", "claude-sonnet-4-6", nil, "", 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), sel.Account.ID)
	require.Nil(t, sel.WaitPlan)
	sel.ReleaseFunc()
}

type allPlatformMutableRepo struct {
	*legacyReplayAccountRepository
	current map[int64]*Account
}

func (r *allPlatformMutableRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if a, ok := r.current[id]; ok {
		return a, nil
	}
	return r.legacyReplayAccountRepository.GetByID(ctx, id)
}

func TestAllPlatformsGatewayRechecksGroupAfterList(t *testing.T) {
	a := simpleTestAccount(1, AccountTypeAPIKey)
	a.Platform = PlatformAnthropic
	a.Credentials = nil
	fresh := a
	fresh.GroupIDs = []int64{9}
	repo := &allPlatformMutableRepo{legacyReplayAccountRepository: &legacyReplayAccountRepository{accounts: []Account{a}}, current: map[int64]*Account{1: &fresh}}
	slots := &legacyReplayConcurrencyCache{}
	svc := &GatewayService{accountRepo: repo, concurrencyService: NewConcurrencyService(slots), cfg: &config.Config{}}
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{ID: 4, Platform: PlatformAnthropic, Status: StatusActive, Hydrated: true})
	sel, err := svc.SelectAccountWithLoadAwareness(ctx, int64PtrForTest(4), "", "claude-sonnet-4-6", nil, "", 0)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Nil(t, sel)
	require.Empty(t, slots.acquireOrder)
}

func TestAllPlatformsGatewayModelRoutingIsExplicitPreference(t *testing.T) {
	for _, busy := range []bool{false, true} {
		t.Run(fmt.Sprint(busy), func(t *testing.T) {
			a, b := simpleTestAccount(1, AccountTypeAPIKey), simpleTestAccount(2, AccountTypeAPIKey)
			for _, acc := range []*Account{&a, &b} {
				acc.Platform = PlatformAnthropic
				acc.Credentials = nil
			}
			a.Priority = 0
			b.Priority = 10
			repo := &legacyReplayAccountRepository{accounts: []Account{a, b}}
			slots := &legacyReplayConcurrencyCache{acquired: map[int64]bool{1: true, 2: !busy}}
			svc := &GatewayService{accountRepo: repo, concurrencyService: NewConcurrencyService(slots), cfg: &config.Config{}}
			group := &Group{ID: 4, Platform: PlatformAnthropic, Status: StatusActive, Hydrated: true, ModelRoutingEnabled: true, ModelRouting: map[string][]int64{"claude-sonnet-4-6": {2}}}
			ctx := context.WithValue(context.Background(), ctxkey.Group, group)
			sel, err := svc.SelectAccountWithLoadAwareness(ctx, int64PtrForTest(4), "", "claude-sonnet-4-6", nil, "", 0)
			require.NoError(t, err)
			if busy {
				require.Equal(t, int64(1), sel.Account.ID)
				require.Equal(t, []int64{2, 1}, slots.acquireOrder)
			} else {
				require.Equal(t, int64(2), sel.Account.ID)
			}
			sel.ReleaseFunc()
		})
	}
}

func TestAllPlatformsGatewayMixedAntigravityOptInRemainsRequired(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			a, b := simpleTestAccount(1, AccountTypeAPIKey), simpleTestAccount(2, AccountTypeAPIKey)
			a.Platform = PlatformGemini
			b.Platform = PlatformAntigravity
			a.Priority = 5
			b.Priority = 0
			for _, acc := range []*Account{&a, &b} {
				acc.Credentials = map[string]any{"model_mapping": map[string]any{"gemini-3.1-pro": "gemini-3.1-pro"}}
			}
			b.Extra = map[string]any{"mixed_scheduling": mixed}
			slots := &legacyReplayConcurrencyCache{acquired: map[int64]bool{1: true, 2: true}}
			svc := &GatewayService{accountRepo: &legacyReplayAccountRepository{accounts: []Account{a, b}}, concurrencyService: NewConcurrencyService(slots), cfg: &config.Config{}}
			ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{ID: 4, Platform: PlatformGemini, Status: StatusActive, Hydrated: true})
			sel, err := svc.SelectAccountWithLoadAwareness(ctx, int64PtrForTest(4), "", "gemini-3.1-pro", nil, "", 0)
			require.NoError(t, err)
			want := int64(1)
			if mixed {
				want = 2
			}
			require.Equal(t, want, sel.Account.ID)
			sel.ReleaseFunc()
		})
	}
}
