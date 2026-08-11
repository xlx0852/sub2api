//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type grokQuotaAccountRepo struct {
	*mockAccountRepoForPlatform
	updates               map[int64]map[string]any
	tempUnschedCalls      int
	lastTempUnschedID     int64
	lastTempUnschedUntil  time.Time
	lastTempUnschedReason string
	clearTempUnschedCalls int
	rateLimitCalls        int
	lastRateLimitUntil    time.Time
	clearRateLimitCalls   int
}

func (r *grokQuotaAccountRepo) ClearTempUnschedulable(_ context.Context, _ int64) error {
	r.clearTempUnschedCalls++
	return nil
}

func TestSyncGrokBillingSchedulingStatePausesUntilWeeklyReset(t *testing.T) {
	now := time.Date(2026, 7, 30, 2, 0, 0, 0, time.UTC)
	reset := now.Add(20 * time.Hour)
	percent := 100.0
	account := &Account{
		ID:                      83,
		Platform:                PlatformGrok,
		Type:                    AccountTypeOAuth,
		TempUnschedulableReason: grokBillingPauseReasonPrefix + " official weekly reset",
	}
	repo := &grokQuotaAccountRepo{}

	syncGrokBillingSchedulingState(context.Background(), repo, account, &xai.BillingSnapshot{
		PeriodType: "weekly", UsagePercent: &percent, PeriodEnd: reset.Format(time.RFC3339Nano),
	}, now)

	// OpenAI-aligned: write rate_limit_reset_at, not temp-unsched.
	require.Equal(t, 1, repo.rateLimitCalls)
	require.WithinDuration(t, reset, repo.lastRateLimitUntil, time.Second)
	require.Equal(t, 0, repo.tempUnschedCalls)
	// Migrate legacy billing temp-unsched off.
	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Contains(t, repo.updates[83], grokBillingRateLimitUntilKey)
}

func TestGrokBillingExhaustionDoesNotUseRetiredCalendarBillingPeriod(t *testing.T) {
	now := time.Date(2026, 8, 9, 2, 0, 0, 0, time.UTC)
	percent := 100.0
	billing := &xai.BillingSnapshot{
		PeriodType:       "weekly",
		UsagePercent:     &percent,
		BillingPeriodEnd: now.Add(20 * time.Hour).Format(time.RFC3339Nano),
	}

	require.True(t, grokBillingExhaustionResetAt(billing, now).IsZero())
}

func TestSyncGrokBillingSchedulingStateClearsRecoveredBillingPause(t *testing.T) {
	percent := 42.0
	account := &Account{
		ID:                      83,
		Platform:                PlatformGrok,
		Type:                    AccountTypeOAuth,
		TempUnschedulableReason: grokBillingPauseReasonPrefix + " official weekly reset",
		Extra: map[string]any{
			grokBillingRateLimitUntilKey: time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{}

	syncGrokBillingSchedulingState(context.Background(), repo, account, &xai.BillingSnapshot{
		PeriodType: "weekly", UsagePercent: &percent,
	}, time.Now())

	require.Equal(t, 1, repo.clearTempUnschedCalls)
	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Nil(t, repo.updates[83][grokBillingRateLimitUntilKey])
}

func TestSyncGrokBillingSchedulingStateDoesNotClearUnrelatedRateLimit(t *testing.T) {
	percent := 42.0
	account := &Account{
		ID:       83,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		// No billing marker → residual 429 cooldown must stay.
		Extra: map[string]any{},
	}
	repo := &grokQuotaAccountRepo{}

	syncGrokBillingSchedulingState(context.Background(), repo, account, &xai.BillingSnapshot{
		PeriodType: "weekly", UsagePercent: &percent,
	}, time.Now())

	require.Equal(t, 0, repo.clearRateLimitCalls)
}

func (r *grokQuotaAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	if r.updates == nil {
		r.updates = make(map[int64]map[string]any)
	}
	r.updates[id] = updates
	return nil
}

func (r *grokQuotaAccountRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.tempUnschedCalls++
	r.lastTempUnschedID = id
	r.lastTempUnschedUntil = until
	r.lastTempUnschedReason = reason
	return nil
}

func (r *grokQuotaAccountRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitCalls++
	r.lastRateLimitUntil = resetAt
	return nil
}

func (r *grokQuotaAccountRepo) ClearRateLimit(_ context.Context, _ int64) error {
	r.clearRateLimitCalls++
	return nil
}

type grokQuotaProxyRepo struct {
	proxyRepoStub
	proxies map[int64]*Proxy
	calls   int
}

func (r *grokQuotaProxyRepo) GetByID(_ context.Context, id int64) (*Proxy, error) {
	r.calls++
	return r.proxies[id], nil
}

func TestGrokQuotaServiceProbeUsageUsesOfficialCreditsPath(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID:          42,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "access-token",
			"sub":          "user-xyz",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{42: account},
		},
	}

	creditsBody := `{"config":{"creditUsagePercent":10,"currentPeriod":{"type":"weekly","start":"2026-07-05T14:38:00Z","end":"2026-07-12T14:38:00Z"},"productUsage":[{"product":"GrokBuild","usagePercent":10},{"product":"Api"},{"product":"GrokChat"}],"monthlyLimit":{"val":15000},"used":{"val":7473},"onDemandCap":{"val":0},"billingPeriodEnd":"2026-08-01T00:00:00Z"}}`

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(creditsBody)),
		},
	}}
	svc := NewGrokQuotaService(
		repo,
		nil,
		NewGrokTokenProvider(repo, nil),
		upstream,
	)

	result, err := svc.ProbeUsage(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "billing_probe", result.Source)
	require.NotNil(t, result.Billing)
	require.Equal(t, "weekly", result.Billing.PeriodType)
	require.NotNil(t, result.Billing.UsagePercent)
	require.InDelta(t, 10, *result.Billing.UsagePercent, 0.001)
	require.Equal(t, "SuperGrok", result.Billing.Plan)
	require.NotNil(t, result.Billing.MonthlyLimitCents)
	require.EqualValues(t, 15000, *result.Billing.MonthlyLimitCents)
	require.Len(t, result.Billing.ProductUsage, 3)

	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/billing?format=credits", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer access-token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "xai-grok-cli", upstream.requests[0].Header.Get("x-xai-token-auth"))
	require.Equal(t, "user-xyz", upstream.requests[0].Header.Get("x-userid"))
	require.Equal(t, "headless", upstream.requests[0].Header.Get("x-grok-client-mode"))
	require.Equal(t, HTTPUpstreamProfileGrok, HTTPUpstreamProfileFromContext(upstream.requests[0].Context()))

	stored := repo.updates[42]
	require.Contains(t, stored, grokBillingSnapshotKey)
}

func TestGrokQuotaServiceProbeUsageKeepsFreshZeroCreditsWindow(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID:       43,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-token",
			"sub":          "user-xyz",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{43: account}}}
	creditsBody := `{"config":{"creditUsagePercent":0,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","start":"2026-07-30T02:33:56Z","end":"2026-08-06T02:33:56Z"}},"subscriptionTier":"SuperGrok Heavy"}`
	upstream := &httpUpstreamRecorder{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(creditsBody)),
	}}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

	result, err := svc.ProbeUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, result.Billing)
	require.NotNil(t, result.Billing.UsagePercent)
	require.InDelta(t, 0, *result.Billing.UsagePercent, 0.001)
	require.Equal(t, "2026-08-06T02:33:56Z", result.Billing.PeriodEnd)
	require.Equal(t, xai.PlanSuperGrokHeavy, result.Billing.Plan)
	require.Len(t, upstream.requests, 1, "a successful credits response must not request legacy billing")
}

func TestGrokQuotaServiceProbeUsageUnauthorized(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID:       7,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "bad",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{7: account},
		},
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
		},
		{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
		},
	}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

	_, err := svc.ProbeUsage(context.Background(), 7)
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
}

func TestGrokQuotaServiceProbeUsagePartialFailureDoesNotPoisonStatus(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID:       9,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{9: account},
		},
	}
	// credits fails with 401; plain billing succeeds with monthly data.
	monthlyBody := `{"config":{"monthlyLimit":{"val":15000},"used":{"val":1000},"billingPeriodEnd":"2026-08-01T00:00:00Z"}}`
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(monthlyBody)),
		},
	}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

	result, err := svc.ProbeUsage(context.Background(), 9)
	require.NoError(t, err)
	require.NotNil(t, result.Billing)
	require.Equal(t, http.StatusOK, result.StatusCode)
	require.Equal(t, http.StatusOK, result.Billing.StatusCode)
	require.Equal(t, "SuperGrok", result.Billing.Plan)

	// Persisted snapshot must not carry the failed side's 401.
	stored, ok := repo.updates[9][grokBillingSnapshotKey].(*xai.BillingSnapshot)
	require.True(t, ok)
	require.Equal(t, http.StatusOK, stored.StatusCode)
}

func TestPickSuccessfulBillingStatus(t *testing.T) {
	t.Parallel()

	okSnap := &xai.BillingSnapshot{Plan: "SuperGrok"}
	require.Equal(t, 200, pickSuccessfulBillingStatus(okSnap, 200, okSnap, 200))
	require.Equal(t, 200, pickSuccessfulBillingStatus(nil, 401, okSnap, 200))
	require.Equal(t, 200, pickSuccessfulBillingStatus(okSnap, 200, nil, 401))
	require.Equal(t, 401, pickSuccessfulBillingStatus(nil, 401, nil, 0))
	require.Equal(t, 403, pickSuccessfulBillingStatus(nil, 0, nil, 403))
}

func TestGrokQuotaServiceResetQuotaUnsupported(t *testing.T) {
	t.Parallel()

	account := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth}
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{1: account},
		},
	}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), &httpUpstreamRecorder{})
	_, err := svc.ResetQuota(context.Background(), 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reset")
}

func TestRefreshGrokBillingSnapshotCoalescesConcurrentUsageReads(t *testing.T) {
	account := &Account{
		ID:       771,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-token",
			"sub":          "user-771",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{account.ID: account}}}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"config":{"creditUsagePercent":0,"currentPeriod":{"type":"weekly","end":"2026-08-06T02:33:56Z"}}}`)),
	}}}
	service := &AccountUsageService{
		accountRepo:      repo,
		grokQuotaFetcher: NewGrokQuotaFetcher(),
		grokQuotaService: NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream),
		cache:            NewUsageCache(),
	}

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			usage, err := service.getGrokUsage(context.Background(), account)
			if err != nil || usage.GrokBilling == nil {
				t.Errorf("getGrokUsage() = %#v, %v", usage, err)
			}
		}()
	}
	wg.Wait()

	require.Len(t, upstream.requests, 1, "concurrent stale reads must share one billing probe")
}

func TestGetGrokUsageForceRefreshBypassesFreshSnapshot(t *testing.T) {
	account := &Account{
		ID:       772,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-token",
			"sub":          "user-772",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
		Extra: map[string]any{
			grokBillingSnapshotKey: &xai.BillingSnapshot{FetchedAt: time.Now().UTC().Format(time.RFC3339), UsagePercent: func() *float64 { value := 54.0; return &value }()},
		},
	}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{account.ID: account}}}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"config":{"creditUsagePercent":0,"currentPeriod":{"type":"weekly","end":"2026-08-06T02:33:56Z"}}}`)),
	}}}
	service := &AccountUsageService{
		accountRepo:      repo,
		grokQuotaFetcher: NewGrokQuotaFetcher(),
		grokQuotaService: NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream),
		cache:            NewUsageCache(),
	}

	usage, err := service.getGrokUsage(context.Background(), account, true)
	require.NoError(t, err)
	require.NotNil(t, usage.GrokBilling)
	require.NotNil(t, usage.GrokBilling.UsagePercent)
	require.InDelta(t, 0, *usage.GrokBilling.UsagePercent, 0.001)
	require.Len(t, upstream.requests, 1, "force refresh must bypass a fresh persisted snapshot")
}
