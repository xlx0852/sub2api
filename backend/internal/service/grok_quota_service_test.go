//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
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

type grokQuotaProxyRepo struct {
	proxyRepoStub
	proxies map[int64]*Proxy
	calls   int
}

func (r *grokQuotaProxyRepo) GetByID(_ context.Context, id int64) (*Proxy, error) {
	r.calls++
	return r.proxies[id], nil
}

func TestGrokQuotaServiceProbeUsageMergesBilling(t *testing.T) {
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

	creditsBody := `{"config":{"creditUsagePercent":10,"currentPeriod":{"type":"weekly","start":"2026-07-05T14:38:00Z","end":"2026-07-12T14:38:00Z"},"productUsage":[{"product":"GrokBuild","usagePercent":10},{"product":"Api"},{"product":"GrokChat"}]}}`
	monthlyBody := `{"config":{"monthlyLimit":{"val":15000},"used":{"val":7473},"onDemandCap":{"val":0},"billingPeriodEnd":"2026-08-01T00:00:00Z"}}`

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(creditsBody)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(monthlyBody)),
		},
	}}
	svc := NewGrokQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

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

	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/billing?format=credits", upstream.requests[0].URL.String())
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/billing", upstream.requests[1].URL.String())
	require.Equal(t, "Bearer access-token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "xai-grok-cli", upstream.requests[0].Header.Get("x-xai-token-auth"))
	require.Equal(t, "user-xyz", upstream.requests[0].Header.Get("x-userid"))

	stored := repo.updates[42]
	require.Contains(t, stored, grokBillingSnapshotKey)
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
