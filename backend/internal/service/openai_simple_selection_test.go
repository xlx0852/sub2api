package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func simpleTestAccount(id int64, typ string) Account {
	return Account{ID: id, Platform: PlatformOpenAI, Type: typ, Status: StatusActive, Schedulable: true, Concurrency: 5, Priority: 1, GroupIDs: []int64{4}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-6-astra": "gpt-6-astra"}}}
}

func simpleTestService(accounts []Account, cc schedulerTestConcurrencyCache) *OpenAIGatewayService {
	cfg := newSchedulerTestOpenAIWSV2Config()
	return &OpenAIGatewayService{cfg: cfg, accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, cache: &schedulerTestGatewayCache{sessionBindings: map[string]int64{}}, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService("true"), concurrencyService: NewConcurrencyService(cc)}
}

func TestSimpleSelection_DisabledPinDoesNotExcludeOAuth(t *testing.T) {
	a, b := simpleTestAccount(89, AccountTypeAPIKey), simpleTestAccount(132, AccountTypeOAuth)
	a.Schedulable = false
	svc := simpleTestService([]Account{a, b}, schedulerTestConcurrencyCache{})
	require.NoError(t, svc.BindStickySession(context.Background(), int64PtrForTest(4), "session", 89))
	result, _, err := svc.SelectAccountWithScheduler(context.Background(), int64PtrForTest(4), "", "session", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(132), result.Account.ID)
	result.ReleaseFunc()
}

func TestSimpleSelection_BusyPreferenceYieldsToNextAccount(t *testing.T) {
	a, b := simpleTestAccount(89, AccountTypeAPIKey), simpleTestAccount(132, AccountTypeOAuth)
	svc := simpleTestService([]Account{a, b}, schedulerTestConcurrencyCache{acquireResults: map[int64]bool{89: false, 132: true}})
	require.NoError(t, svc.BindStickySession(context.Background(), int64PtrForTest(4), "session", 89))
	result, _, err := svc.SelectAccountWithScheduler(context.Background(), int64PtrForTest(4), "", "session", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(132), result.Account.ID)
	require.True(t, result.Acquired)
	result.ReleaseFunc()
}

func TestSimpleSelection_HardGates(t *testing.T) {
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name   string
		mutate func(*Account)
		req    func(*OpenAIAccountScheduleRequest)
	}{
		{name: "disabled", mutate: func(a *Account) { a.Schedulable = false }},
		{name: "model cooldown", mutate: func(a *Account) {
			a.Extra = map[string]any{"model_rate_limits": map[string]any{"gpt-6-astra": map[string]any{"rate_limit_reset_at": future.Format(time.RFC3339)}}}
		}},
		{name: "wrong group", mutate: func(a *Account) { a.GroupIDs = []int64{9} }},
		{name: "excluded", req: func(r *OpenAIAccountScheduleRequest) { r.ExcludedIDs = map[int64]struct{}{132: {}} }},
		{name: "quota cooldown", mutate: func(a *Account) { a.RateLimitResetAt = &future }},
		{name: "credential class", req: func(r *OpenAIAccountScheduleRequest) { r.RequiredCredentialClass = OpenAICredentialClassAPIKey }},
		{name: "transport", mutate: func(a *Account) { a.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": "off"} }, req: func(r *OpenAIAccountScheduleRequest) {
			r.RequiredTransport = OpenAIUpstreamTransportResponsesWebsocketV2Ingress
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := simpleTestAccount(132, AccountTypeOAuth)
			r := OpenAIAccountScheduleRequest{GroupID: int64PtrForTest(4), Platform: PlatformOpenAI, RequestedModel: "gpt-6-astra"}
			if tt.mutate != nil {
				tt.mutate(&a)
			}
			if tt.req != nil {
				tt.req(&r)
			}
			svc := simpleTestService([]Account{a}, schedulerTestConcurrencyCache{})
			svc.cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
			s := newDefaultOpenAIAccountScheduler(svc, nil).(*defaultOpenAIAccountScheduler)
			require.False(t, s.simpleCandidateDecision(context.Background(), &a, r).Allowed)
		})
	}
}

func TestSimpleSelection_RequestCredentialFailureIsolation(t *testing.T) {
	ctx := WithOpenAICredentialFailoverState(context.Background())
	a := simpleTestAccount(89, AccountTypeAPIKey)
	recordOpenAICredentialFailover(ctx, &a, http.StatusUnauthorized)
	require.Equal(t, OpenAICredentialClassAPIKey, openAICredentialFailoverClass(ctx))
	require.Empty(t, openAICredentialFailoverClass(WithOpenAICredentialFailoverState(context.Background())))
	a.Type = AccountTypeOAuth
	recordOpenAICredentialFailover(ctx, &a, http.StatusForbidden)
	require.Equal(t, OpenAICredentialClassAPIKey, openAICredentialFailoverClass(ctx), "first failed class remains locked")
}

func TestSimpleSelection_PreviousResponseKeepsOriginalAccount(t *testing.T) {
	for _, busy := range []bool{false, true} {
		t.Run(fmt.Sprint(busy), func(t *testing.T) {
			a, b := simpleTestAccount(112, AccountTypeOAuth), simpleTestAccount(132, AccountTypeOAuth)
			svc := simpleTestService([]Account{a, b}, schedulerTestConcurrencyCache{acquireResults: map[int64]bool{112: !busy, 132: true}})
			svc.cfg.Gateway.Scheduling.StickySessionWaitTimeout = time.Second
			svc.cfg.Gateway.Scheduling.StickySessionMaxWaiting = 1
			require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(context.Background(), 4, "resp_original", 112, time.Hour))
			sel, d, err := svc.SelectAccountWithScheduler(context.Background(), int64PtrForTest(4), "resp_original", "", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.Equal(t, int64(112), sel.Account.ID)
			require.True(t, d.StickyPreviousHit)
			if busy {
				require.NotNil(t, sel.WaitPlan)
				require.Equal(t, time.Second, sel.WaitPlan.Timeout)
			} else {
				require.True(t, sel.Acquired)
				sel.ReleaseFunc()
			}
		})
	}
}

func TestSimpleSelection_UnavailableContinuationDoesNotAcquireOtherAccount(t *testing.T) {
	svc := simpleTestService([]Account{simpleTestAccount(132, AccountTypeOAuth)}, schedulerTestConcurrencyCache{})
	sel, _, err := svc.SelectAccountWithScheduler(context.Background(), int64PtrForTest(4), "resp_missing", "", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
	require.ErrorContains(t, err, "continuation_unavailable")
	require.Nil(t, sel)
	sel, _, err = svc.SelectAccountWithSchedulerForCapability(context.Background(), int64PtrForTest(4), "resp_missing", "", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, "", false, true, PlatformOpenAI)
	require.NoError(t, err)
	require.Equal(t, int64(132), sel.Account.ID)
	sel.ReleaseFunc()
}

func TestSimpleSelection_AuthFailureCannotSwitchCredentialClass(t *testing.T) {
	a, b, c := simpleTestAccount(89, AccountTypeAPIKey), simpleTestAccount(132, AccountTypeOAuth), simpleTestAccount(112, AccountTypeOAuth)
	svc := simpleTestService([]Account{a, b, c}, schedulerTestConcurrencyCache{})
	ctx := WithOpenAICredentialFailoverState(context.Background())
	// Exercise the real upstream error handler, including the ignore-401 switch.
	b.Extra = map[string]any{"ignore_upstream_401": true}
	require.True(t, svc.handleOpenAIAccountUpstreamError(ctx, &b, 401, http.Header{}, []byte(`{"error":{"code":"token_invalidated"}}`)))
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(&b))
	sel, _, err := svc.SelectAccountWithScheduler(ctx, int64PtrForTest(4), "", "", "gpt-6-astra", map[int64]struct{}{132: {}}, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, int64(112), sel.Account.ID)
	sel.ReleaseFunc()
	sel, _, err = svc.SelectAccountWithScheduler(ctx, int64PtrForTest(4), "", "", "gpt-6-astra", map[int64]struct{}{132: {}, 112: {}}, OpenAIUpstreamTransportAny, false)
	require.Error(t, err)
	require.Nil(t, sel, "must not fall through to apikey account")
	newCtx := WithOpenAICredentialFailoverState(context.Background())
	sel, _, err = svc.SelectAccountWithScheduler(newCtx, int64PtrForTest(4), "", "", "gpt-6-astra", map[int64]struct{}{132: {}, 112: {}}, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, int64(89), sel.Account.ID)
	sel.ReleaseFunc()
}

func TestSimpleSelection_AllCandidatesAttemptedBeyondTopK(t *testing.T) {
	var accounts []Account
	slots := map[int64]bool{}
	for id := int64(1); id <= 10; id++ {
		a := simpleTestAccount(id, AccountTypeOAuth)
		a.Priority = int(id)
		accounts = append(accounts, a)
		slots[id] = id == 10
	}
	svc := simpleTestService(accounts, schedulerTestConcurrencyCache{acquireResults: slots})
	svc.cfg.Gateway.OpenAIWS.LBTopK = 1
	sel, d, err := svc.SelectAccountWithScheduler(context.Background(), int64PtrForTest(4), "", "fresh", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, int64(10), sel.Account.ID)
	require.Equal(t, 10, d.CandidateCount)
	sel.ReleaseFunc()
}

func TestSimpleSelection_RecheckRejectsChangedDBAccount(t *testing.T) {
	cached := simpleTestAccount(132, AccountTypeOAuth)
	fresh := cached
	fresh.Schedulable = false
	svc := simpleTestService([]Account{fresh}, schedulerTestConcurrencyCache{})
	s := newDefaultOpenAIAccountScheduler(svc, nil).(*defaultOpenAIAccountScheduler)
	req := OpenAIAccountScheduleRequest{GroupID: int64PtrForTest(4), Platform: PlatformOpenAI, RequestedModel: "gpt-6-astra"}
	require.True(t, s.simpleCandidateDecision(context.Background(), &cached, req).Allowed)
	require.Nil(t, s.recheckLoadCandidate(context.Background(), &cached, req))
}

func TestSimpleSelection_PrivacyIsHardGate(t *testing.T) {
	a := simpleTestAccount(132, AccountTypeOAuth)
	svc := simpleTestService([]Account{a}, schedulerTestConcurrencyCache{})
	s := newDefaultOpenAIAccountScheduler(svc, nil).(*defaultOpenAIAccountScheduler)
	req := OpenAIAccountScheduleRequest{GroupID: int64PtrForTest(4), Platform: PlatformOpenAI, RequestedModel: "gpt-6-astra", simpleGroup: &Group{RequirePrivacySet: true}}
	require.Equal(t, "privacy", s.simpleCandidateDecision(context.Background(), &a, req).RejectedPolicy)
}

func TestSimpleSelection_AllBusyProducesBoundedWait(t *testing.T) {
	a := simpleTestAccount(132, AccountTypeOAuth)
	svc := simpleTestService([]Account{a}, schedulerTestConcurrencyCache{acquireResults: map[int64]bool{132: false}})
	svc.cfg.Gateway.Scheduling.FallbackWaitTimeout = 2 * time.Second
	svc.cfg.Gateway.Scheduling.FallbackMaxWaiting = 2
	sel, _, err := svc.SelectAccountWithScheduler(context.Background(), int64PtrForTest(4), "", "new", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, sel.WaitPlan)
	require.Equal(t, 2*time.Second, sel.WaitPlan.Timeout)
	require.Equal(t, 2, sel.WaitPlan.MaxWaiting)
}

func TestSimpleSelection_LoadUsesCapacityNotHistory(t *testing.T) {
	a, b := simpleTestAccount(112, AccountTypeOAuth), simpleTestAccount(132, AccountTypeOAuth)
	svc := simpleTestService([]Account{a, b}, schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{112: {AccountID: 112, LoadRate: 90}, 132: {AccountID: 132, LoadRate: 0}}})
	svc.cfg.Gateway.OpenAIWS.LBTopK = 1
	svc.cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 999
	sel, _, err := svc.SelectAccountWithScheduler(context.Background(), int64PtrForTest(4), "", "new", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.Equal(t, int64(132), sel.Account.ID)
	sel.ReleaseFunc()
}

type simpleModelBudgetCache struct {
	schedulerTestConcurrencyCache
	modelCalls   int
	accountCalls int
}

func (c *simpleModelBudgetCache) AcquireModelSlot(context.Context, string, int, string) (bool, error) {
	c.modelCalls++
	return false, nil
}
func (c *simpleModelBudgetCache) ReleaseModelSlot(context.Context, string, string) error { return nil }
func (c *simpleModelBudgetCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	c.accountCalls++
	return true, nil
}

func TestSimpleSelection_GlobalModelBudgetDoesNotRetryOtherAccounts(t *testing.T) {
	accounts := []Account{simpleTestAccount(132, AccountTypeOAuth), simpleTestAccount(112, AccountTypeOAuth)}
	for i := range accounts {
		accounts[i].Credentials["model_mapping"] = map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}
	}
	svc := simpleTestService(accounts, schedulerTestConcurrencyCache{})
	cache := &simpleModelBudgetCache{}
	svc.concurrencyService = NewConcurrencyService(cache)
	svc.concurrencyService.SetModelConcurrencyLimitProvider(func(context.Context) map[string]int { return map[string]int{"gpt-5.6-sol": 1} })
	sel, _, err := svc.SelectAccountWithScheduler(context.Background(), int64PtrForTest(4), "", "fresh", "gpt-5.6-sol", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, sel)
	require.True(t, sel.ModelLimited)
	require.False(t, sel.Acquired)
	require.Nil(t, sel.WaitPlan)
	require.Equal(t, 1, cache.modelCalls)
	require.Zero(t, cache.accountCalls)
}

func TestSimpleSelection_NonAuthFailureDoesNotLockClass(t *testing.T) {
	ctx := WithOpenAICredentialFailoverState(context.Background())
	a := simpleTestAccount(132, AccountTypeOAuth)
	for _, status := range []int{200, 400, 429, 500, 503} {
		recordOpenAICredentialFailover(ctx, &a, status)
		require.Empty(t, openAICredentialFailoverClass(ctx))
	}
	a.Platform = PlatformGrok
	recordOpenAICredentialFailover(ctx, &a, 401)
	require.Empty(t, openAICredentialFailoverClass(ctx))
}

func TestSimpleSelection_UserPinIsOnlyAnEligiblePreference(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprint(disabled), func(t *testing.T) {
			a, b := simpleTestAccount(89, AccountTypeAPIKey), simpleTestAccount(132, AccountTypeOAuth)
			a.Schedulable = !disabled
			svc := simpleTestService([]Account{a, b}, schedulerTestConcurrencyCache{acquireResults: map[int64]bool{89: false, 132: true}})
			svc.cfg.Gateway.OpenAIUserPinEnabled = true
			svc.cache.(*schedulerTestGatewayCache).sessionBindings[openAIUserPinCacheKey(47)] = 89
			ctx := context.WithValue(context.Background(), ctxkey.OpenAIUserPinAPIKeyID, int64(47))
			sel, _, err := svc.SelectAccountWithScheduler(ctx, int64PtrForTest(4), "", "", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.Equal(t, int64(132), sel.Account.ID)
			sel.ReleaseFunc()
			require.Equal(t, int64(132), svc.getUserPinnedAccountID(ctx, int64PtrForTest(4), 47))
		})
	}
}

func TestSimpleSelection_AuthenticationLockSurvivesProfileRollback(t *testing.T) {
	svc := simpleTestService([]Account{simpleTestAccount(89, AccountTypeAPIKey)}, schedulerTestConcurrencyCache{})
	svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("false")
	ctx := WithOpenAICredentialFailoverState(context.Background())
	a := simpleTestAccount(132, AccountTypeOAuth)
	recordOpenAICredentialFailover(ctx, &a, 401)
	sel, _, err := svc.SelectAccountWithScheduler(ctx, int64PtrForTest(4), "", "", "gpt-6-astra", map[int64]struct{}{132: {}}, OpenAIUpstreamTransportAny, false)
	require.Error(t, err)
	require.Nil(t, sel)
}
