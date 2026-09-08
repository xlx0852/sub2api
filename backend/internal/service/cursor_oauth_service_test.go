package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cursorOAuthSessionStoreStub struct {
	mu      sync.Mutex
	session *ProviderOAuthSession
}

func (s *cursorOAuthSessionStoreStub) Create(_ context.Context, session *ProviderOAuthSession, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *session
	s.session = &copy
	return nil
}

func (s *cursorOAuthSessionStoreStub) Get(_ context.Context, _ string) (*ProviderOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return nil, nil
	}
	copy := *s.session
	return &copy, nil
}

func (s *cursorOAuthSessionStoreStub) AcquirePollLease(_ context.Context, _ string, now time.Time, _ time.Duration) (*ProviderOAuthSessionLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return &ProviderOAuthSessionLease{}, nil
	}
	copy := *s.session
	if copy.Status != ProviderOAuthSessionPending || copy.NextPollAtUnixMilli > now.UnixMilli() {
		return &ProviderOAuthSessionLease{Session: &copy}, nil
	}
	copy.Version++
	copy.PollLeaseID = "lease"
	s.session = &copy
	leaseCopy := copy
	return &ProviderOAuthSessionLease{Session: &leaseCopy, ID: "lease", Held: true}, nil
}

func (s *cursorOAuthSessionStoreStub) CommitPoll(_ context.Context, lease *ProviderOAuthSessionLease, updated *ProviderOAuthSession) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil || s.session.Status != ProviderOAuthSessionPending || lease == nil || !lease.Held || s.session.Version != lease.Session.Version {
		return false, nil
	}
	copy := *updated
	copy.Version = s.session.Version + 1
	copy.PollLeaseID = ""
	s.session = &copy
	return true, nil
}

func (s *cursorOAuthSessionStoreStub) Cancel(_ context.Context, _ string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return false, nil
	}
	s.session.Status = ProviderOAuthSessionCancelled
	s.session.Payload = nil
	return true, nil
}

func (s *cursorOAuthSessionStoreStub) ConsumeAuthorized(_ context.Context, _ string) (*ProviderOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil || s.session.Status != ProviderOAuthSessionAuthorized {
		return nil, nil
	}
	result := s.session
	s.session = nil
	return result, nil
}

func (s *cursorOAuthSessionStoreStub) Delete(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = nil
	return nil
}

func makeFakeCursorJWT(sub, email string, exp int64) string {
	payload, _ := json.Marshal(map[string]any{
		"sub":   sub,
		"email": email,
		"exp":   exp,
	})
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestCursorOAuthService_Flow(t *testing.T) {
	var pollCalls int
	fakeJWT := makeFakeCursorJWT("grok|user_test_123", "test@example.com", time.Now().Add(60*24*time.Hour).Unix())

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/poll":
			pollCalls++
			if pollCalls == 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"accessToken":  fakeJWT,
				"refreshToken": fakeJWT,
			})
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "refreshed-access",
				"refresh_token": "refreshed-refresh",
				"shouldLogout":  false,
			})
		case "/auth/exchange_user_api_key":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"accessToken":  fakeJWT,
				"refreshToken": fakeJWT,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	store := &cursorOAuthSessionStoreStub{}
	svc := NewCursorOAuthService(nil, store)
	svc.backendURL = ts.URL
	svc.websiteURL = "https://cursor.com"

	currTime := time.Now()
	svc.now = func() time.Time { return currTime }

	ctx := context.Background()

	// 1. Start Authorization
	startRes, err := svc.StartAuthorization(ctx, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, startRes.SessionID)
	assert.Equal(t, ProviderOAuthSessionPending, startRes.Status)
	assert.Contains(t, startRes.LoginURL, "https://cursor.com/loginDeepControl?")

	// 2. Poll 1: 404 pending
	pollRes1, err := svc.GetAuthorizationStatus(ctx, startRes.SessionID)
	require.NoError(t, err)
	assert.Equal(t, ProviderOAuthSessionPending, pollRes1.Status)

	// Advance time past interval
	currTime = currTime.Add(3 * time.Second)

	// 3. Poll 2: 200 authorized
	pollRes2, err := svc.GetAuthorizationStatus(ctx, startRes.SessionID)
	require.NoError(t, err)
	assert.Equal(t, ProviderOAuthSessionAuthorized, pollRes2.Status)

	// 4. Consume
	tokenInfo, err := svc.ConsumeAuthorization(ctx, startRes.SessionID)
	require.NoError(t, err)
	assert.Equal(t, fakeJWT, tokenInfo.AccessToken)
	assert.Equal(t, "test@example.com", tokenInfo.Email)
	assert.Equal(t, "grok|user_test_123", tokenInfo.UserID)
	assert.NotEmpty(t, tokenInfo.MachineID)

	// 5. Build credentials
	creds := svc.BuildAccountCredentials(tokenInfo)
	assert.Equal(t, fakeJWT, creds["access_token"])
	assert.Equal(t, fakeJWT, creds["refresh_token"])
	assert.Equal(t, "test@example.com", creds["email"])
	assert.Equal(t, "Bearer", creds["token_type"])

	// 6. Test Token Refresher
	refresher := NewCursorTokenRefresher(svc)
	account := &Account{
		Platform:    PlatformCursor,
		Type:        AccountTypeOAuth,
		Credentials: creds,
	}
	assert.True(t, refresher.CanRefresh(account))
	refreshedCreds, err := refresher.Refresh(ctx, account)
	require.NoError(t, err)
	assert.Equal(t, "refreshed-access", refreshedCreds["access_token"])
	assert.Equal(t, "refreshed-refresh", refreshedCreds["refresh_token"])

	// 7. Test Exchange User API Key
	exchanged, err := svc.ExchangeUserAPIKey(ctx, "cur_api_key_test", nil)
	require.NoError(t, err)
	assert.Equal(t, fakeJWT, exchanged.AccessToken)
	assert.Equal(t, "test@example.com", exchanged.Email)
}

func TestCursorQuotaFetcher_FetchQuota(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-tok", r.Header.Get("Authorization"))
		assert.NotEmpty(t, r.Header.Get("x-cursor-checksum"))
		assert.Equal(t, "sand", r.Header.Get("x-cursor-client-type"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		switch r.URL.Path {
		case "/aiserver.v1.DashboardService/GetMe":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authId":    "grok|user_999",
				"email":     "lili@example.com",
				"firstName": "Lili",
			})
		case "/aiserver.v1.DashboardService/GetSandUsageStatus":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"usagePercent":               12.5,
				"hasAvailableUsage":          true,
				"currentPeriodStart":         "2026-09-01T00:00:00Z",
				"nextResetTimestampUtc":      "2026-09-08T00:00:00Z",
				"includedUsageSuperGrokPlan": "supergrok",
				"grokPlanLabel":              "SuperGrok",
			})
		case "/aiserver.v1.DashboardService/GetCurrentPeriodUsage":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"displayMessage": "You've used 12.5% of your included usage",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	fetcher := NewCursorQuotaFetcher(nil)
	fetcher.backendURL = ts.URL

	account := &Account{
		Platform: PlatformCursor,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "test-tok",
			"machine_id":   "mach-123",
		},
	}

	assert.True(t, fetcher.CanFetch(account))
	res, err := fetcher.FetchQuota(context.Background(), account, "")
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.UsageInfo)

	assert.Equal(t, "active", res.UsageInfo.Source)
	assert.Equal(t, "SUPERGROK", res.UsageInfo.SubscriptionTier)
	assert.Equal(t, "SuperGrok", res.UsageInfo.SubscriptionTierRaw)
	require.NotNil(t, res.UsageInfo.CursorSandUsage)
	assert.Equal(t, 12.5, res.UsageInfo.CursorSandUsage.UsagePercent)
	assert.True(t, res.UsageInfo.CursorSandUsage.HasAvailableUsage)
	assert.Equal(t, "SuperGrok", res.UsageInfo.CursorSandUsage.GrokPlanLabel)

	require.NotNil(t, res.UsageInfo.SevenDay)
	assert.Equal(t, 12.5, res.UsageInfo.SevenDay.Utilization)
	assert.Equal(t, "7d", res.UsageInfo.SevenDay.WindowLabel)
}
