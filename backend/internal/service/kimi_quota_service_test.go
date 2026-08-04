//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type kimiQuotaTokenProviderStub struct {
	getToken     string
	refreshToken string
	getCalls     atomic.Int32
	refreshCalls atomic.Int32
}

type kimiQuotaHTTPRecorder struct {
	responses     []*http.Response
	requests      []*http.Request
	concurrencies []int
}

type kimiWindowStatsUsageRepo struct {
	UsageLogRepository
	stats []*usagestats.AccountStats
	calls int
}

type kimiQuotaSchedulingRepo struct {
	*mockAccountRepoForPlatform
	extraUpdates        map[string]any
	pauseUntil          *time.Time
	pauseReason         string
	clearCalls          int
	rateLimitUntil      *time.Time
	rateLimitCalls      int
	clearRateLimitCalls int
}

func (r *kimiQuotaSchedulingRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.extraUpdates = updates
	return nil
}

func (r *kimiQuotaSchedulingRepo) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, reason string) error {
	r.pauseUntil = &until
	r.pauseReason = reason
	return nil
}

func (r *kimiQuotaSchedulingRepo) ClearTempUnschedulable(_ context.Context, _ int64) error {
	r.clearCalls++
	return nil
}

func (r *kimiQuotaSchedulingRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitCalls++
	r.rateLimitUntil = &resetAt
	return nil
}

func (r *kimiQuotaSchedulingRepo) ClearRateLimit(_ context.Context, _ int64) error {
	r.clearRateLimitCalls++
	return nil
}

func TestSyncKimiQuotaSchedulingStatePausesUntilLatestFullWindow(t *testing.T) {
	now := time.Date(2026, 7, 30, 2, 0, 0, 0, time.UTC)
	fiveHourReset := now.Add(2 * time.Hour)
	sevenDayReset := now.Add(20 * time.Hour)
	account := &Account{
		ID:                      105,
		Platform:                PlatformKimi,
		Type:                    AccountTypeOAuth,
		TempUnschedulableReason: kimiQuotaPauseReasonPrefix + " official window reset",
	}
	repo := &kimiQuotaSchedulingRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{}}

	syncKimiQuotaSchedulingState(context.Background(), repo, account, &KimiQuotaUsage{
		FiveHour: &UsageProgress{Utilization: 100, ResetsAt: &fiveHourReset},
		SevenDay: &UsageProgress{Utilization: 100, ResetsAt: &sevenDayReset},
	}, now)

	// OpenAI-aligned: write rate_limit_reset_at (latest full window), not temp-unsched.
	require.Nil(t, repo.pauseUntil)
	require.Equal(t, 1, repo.rateLimitCalls)
	require.NotNil(t, repo.rateLimitUntil)
	require.Equal(t, sevenDayReset, *repo.rateLimitUntil)
	require.Equal(t, 1, repo.clearCalls) // migrate legacy temp-unsched
	require.Equal(t, float64(100), repo.extraUpdates["kimi_quota_7d_utilization"])
	require.Equal(t, sevenDayReset.UTC().Format(time.RFC3339), repo.extraUpdates[kimiQuotaRateLimitUntilKey])
}

func TestSyncKimiQuotaSchedulingStateClearsRecoveredQuotaPause(t *testing.T) {
	now := time.Now().UTC()
	reset := now.Add(5 * time.Hour)
	account := &Account{
		ID: 105, Platform: PlatformKimi, Type: AccountTypeOAuth,
		TempUnschedulableReason: kimiQuotaPauseReasonPrefix + " official window reset",
		Extra: map[string]any{
			kimiQuotaRateLimitUntilKey: now.Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &kimiQuotaSchedulingRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{}}

	syncKimiQuotaSchedulingState(context.Background(), repo, account, &KimiQuotaUsage{
		FiveHour: &UsageProgress{Utilization: 10, ResetsAt: &reset},
		SevenDay: &UsageProgress{Utilization: 20, ResetsAt: &reset},
	}, now)

	require.Nil(t, repo.pauseUntil)
	require.Equal(t, 1, repo.clearCalls)
	require.Equal(t, 1, repo.clearRateLimitCalls)
	require.Nil(t, repo.extraUpdates[kimiQuotaRateLimitUntilKey])
}

func TestSyncKimiQuotaSchedulingStateDoesNotClearUnrelatedRateLimit(t *testing.T) {
	now := time.Now().UTC()
	reset := now.Add(5 * time.Hour)
	account := &Account{
		ID: 105, Platform: PlatformKimi, Type: AccountTypeOAuth,
		Extra: map[string]any{},
	}
	repo := &kimiQuotaSchedulingRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{}}

	syncKimiQuotaSchedulingState(context.Background(), repo, account, &KimiQuotaUsage{
		FiveHour: &UsageProgress{Utilization: 10, ResetsAt: &reset},
		SevenDay: &UsageProgress{Utilization: 20, ResetsAt: &reset},
	}, now)

	require.Equal(t, 0, repo.clearRateLimitCalls)
}

func (r *kimiWindowStatsUsageRepo) GetAccountWindowStats(context.Context, int64, time.Time) (*usagestats.AccountStats, error) {
	index := r.calls
	r.calls++
	if index >= len(r.stats) {
		return &usagestats.AccountStats{}, nil
	}
	return r.stats[index], nil
}

func (r *kimiQuotaHTTPRecorder) Do(req *http.Request, _ string, _ int64, accountConcurrency int) (*http.Response, error) {
	r.requests = append(r.requests, req)
	r.concurrencies = append(r.concurrencies, accountConcurrency)
	if len(r.responses) == 0 {
		return nil, nil
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	return response, nil
}

func (r *kimiQuotaHTTPRecorder) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return r.Do(req, proxyURL, accountID, accountConcurrency)
}

func (p *kimiQuotaTokenProviderStub) GetAccessToken(context.Context, *Account) (string, error) {
	p.getCalls.Add(1)
	return p.getToken, nil
}

func (p *kimiQuotaTokenProviderStub) ForceRefresh(context.Context, *Account) (string, error) {
	p.refreshCalls.Add(1)
	return p.refreshToken, nil
}

func TestParseKimiQuotaUsage(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	result, err := parseKimiQuotaUsage([]byte(`{
		"subType":"TYPE_PURCHASE",
		"user":{"membership":{"level":"LEVEL_INTERMEDIATE"}},
		"usage":{"limit":"1000","used":"130","remaining":"870","resetTime":"2026-08-01T12:00:00.123456789999Z"},
		"limits":[
			{"window":{"duration":300,"timeUnit":"TIME_UNIT_MINUTE"},"detail":{"limit":100,"used":83,"remaining":17,"resetTime":"2026-07-29T17:00:00Z"}},
			{"window":{"duration":60,"timeUnit":"MINUTE"},"detail":{"limit":10,"used":1,"resetTime":"2026-07-29T13:00:00Z"}}
		]
	}`), now)
	require.NoError(t, err)
	require.Equal(t, "LEVEL_INTERMEDIATE", result.SubscriptionTier)
	require.Equal(t, "TYPE_PURCHASE", result.SubscriptionKind)
	require.NotNil(t, result.FiveHour)
	require.NotNil(t, result.SevenDay)
	require.InDelta(t, 83, result.FiveHour.Utilization, 0.001)
	require.InDelta(t, 13, result.SevenDay.Utilization, 0.001)
	require.Equal(t, 5*60*60, result.FiveHour.RemainingSeconds)
	require.InDelta(t, 5*60*60, result.FiveHour.ResetsAt.Sub(now).Seconds(), 0.001)
	require.Equal(t, time.Date(2026, 8, 1, 12, 0, 0, 123456789, time.UTC), *result.SevenDay.ResetsAt)
}

func TestParseKimiQuotaUsageFallsBackToSubTypeWithoutMembership(t *testing.T) {
	result, err := parseKimiQuotaUsage([]byte(`{
		"subType":"TYPE_PURCHASE",
		"usage":{"limit":100,"used":10,"remaining":90}
	}`), time.Now())
	require.NoError(t, err)
	require.Equal(t, "TYPE_PURCHASE", result.SubscriptionTier)
	require.Equal(t, "TYPE_PURCHASE", result.SubscriptionKind)
}

func TestKimiQuotaServiceQueryUsageRefreshesOnceAfterUnauthorized(t *testing.T) {
	account := &Account{
		ID:          42,
		Platform:    PlatformKimi,
		Type:        AccountTypeOAuth,
		Concurrency: 0,
		Credentials: map[string]any{
			"device_name":  "local-device",
			"device_model": "MacBookPro",
			"device_id":    "device-id",
		},
	}
	repo := &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{account.ID: account}}
	tokens := &kimiQuotaTokenProviderStub{getToken: "stale-token", refreshToken: "fresh-token"}
	upstream := &kimiQuotaHTTPRecorder{responses: []*http.Response{
		{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":"expired"}`))},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{
			"usage":{"limit":1000,"used":130,"remaining":870,"resetTime":"2026-08-01T12:00:00Z"},
			"limits":[{"window":{"duration":300,"timeUnit":"MINUTE"},"detail":{"limit":100,"used":25,"remaining":75,"resetTime":"2026-07-29T17:00:00Z"}}]
		}`))},
	}}
	svc := NewKimiQuotaService(repo, nil, tokens, upstream)

	result, err := svc.QueryUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, 25.0, result.FiveHour.Utilization)
	require.Equal(t, int32(1), tokens.getCalls.Load())
	require.Equal(t, int32(1), tokens.refreshCalls.Load())
	require.Len(t, upstream.requests, 2)
	require.Equal(t, []int{10, 10}, upstream.concurrencies)
	require.Equal(t, "Bearer stale-token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "Bearer fresh-token", upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, "KimiCLI/1.10.6", upstream.requests[1].Header.Get("User-Agent"))
	require.Equal(t, "kimi_cli", upstream.requests[1].Header.Get("X-Msh-Platform"))
	require.Equal(t, "device-id", upstream.requests[1].Header.Get("X-Msh-Device-Id"))
}

func TestKimiQuotaServiceQueryUsageMapsUnauthorized(t *testing.T) {
	account := &Account{ID: 7, Platform: PlatformKimi, Type: AccountTypeOAuth}
	repo := &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{account.ID: account}}
	tokens := &kimiQuotaTokenProviderStub{getToken: "token", refreshToken: ""}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{}`))},
	}}
	svc := NewKimiQuotaService(repo, nil, tokens, upstream)

	_, err := svc.QueryUsage(context.Background(), account.ID)
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, infraerrors.Code(err))
	require.Equal(t, "KIMI_QUOTA_UNAUTHENTICATED", infraerrors.Reason(err))
}

func TestAccountUsageServiceKimiQuotaAddsWindowRequestStats(t *testing.T) {
	now := time.Now()
	fiveHourReset := now.Add(5 * time.Hour)
	sevenDayReset := now.Add(7 * 24 * time.Hour)
	account := &Account{ID: 102, Platform: PlatformKimi, Type: AccountTypeOAuth}
	repo := &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{account.ID: account}}
	querier := &kimiQuotaQuerierStub{usage: &KimiQuotaUsage{
		FiveHour: &UsageProgress{
			Utilization: 0,
			ResetsAt:    &fiveHourReset,
		},
		SevenDay: &UsageProgress{
			Utilization: 5,
			ResetsAt:    &sevenDayReset,
		},
	}}
	usageRepo := &kimiWindowStatsUsageRepo{stats: []*usagestats.AccountStats{
		{Requests: 774, Tokens: 111_300_000, Cost: 114.56, UserCost: 13.75},
		{Requests: 773, Tokens: 111_300_000, Cost: 114.39, UserCost: 13.73},
	}}
	service := &AccountUsageService{
		accountRepo:      repo,
		usageLogRepo:     usageRepo,
		kimiQuotaService: querier,
		cache:            NewUsageCache(),
	}

	usage, err := service.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, 2, usageRepo.calls)
	require.NotNil(t, usage.FiveHour.WindowStats)
	require.EqualValues(t, 774, usage.FiveHour.WindowStats.Requests)
	require.EqualValues(t, 111_300_000, usage.FiveHour.WindowStats.Tokens)
	require.NotNil(t, usage.SevenDay.WindowStats)
	require.EqualValues(t, 773, usage.SevenDay.WindowStats.Requests)
	require.InDelta(t, 13.73, usage.SevenDay.WindowStats.UserCost, 0.001)
}

type kimiQuotaQuerierStub struct {
	queries atomic.Int32
	usage   *KimiQuotaUsage
}

func (q *kimiQuotaQuerierStub) QueryUsage(context.Context, int64) (*KimiQuotaUsage, error) {
	q.queries.Add(1)
	return q.usage, nil
}

func TestAccountUsageServiceKimiQuotaCachesForTenMinutesAndForceBypasses(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	account := &Account{ID: 101, Platform: PlatformKimi, Type: AccountTypeOAuth}
	repo := &mockAccountRepoForPlatform{accountsByID: map[int64]*Account{account.ID: account}}
	querier := &kimiQuotaQuerierStub{usage: &KimiQuotaUsage{
		FiveHour:         &UsageProgress{Utilization: 25, ResetsAt: &resetAt},
		SevenDay:         &UsageProgress{Utilization: 13, ResetsAt: &resetAt},
		SubscriptionTier: "LEVEL_INTERMEDIATE",
		SubscriptionKind: "TYPE_PURCHASE",
		FetchedAt:        time.Now().Unix(),
	}}
	service := &AccountUsageService{
		accountRepo:      repo,
		kimiQuotaService: querier,
		cache:            NewUsageCache(),
	}

	first, err := service.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	second, err := service.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	forced, err := service.GetUsage(context.Background(), account.ID, true)
	require.NoError(t, err)

	require.Equal(t, 25.0, first.FiveHour.Utilization)
	require.Equal(t, 13.0, second.SevenDay.Utilization)
	require.Equal(t, 25.0, forced.FiveHour.Utilization)
	require.Equal(t, "LEVEL_INTERMEDIATE", forced.SubscriptionTier)
	require.Equal(t, "TYPE_PURCHASE", forced.SubscriptionKind)
	require.Equal(t, int32(2), querier.queries.Load())
}

// Keep the test package's HTTPUpstream implementation honest when the
// interface gains a method in the future.
var _ HTTPUpstream = (*httpUpstreamRecorder)(nil)
var _ KimiAccessTokenProvider = (*kimiQuotaTokenProviderStub)(nil)
