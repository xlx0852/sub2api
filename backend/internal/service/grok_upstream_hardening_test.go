package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrokUpstreamExplicitlyDisablesRetry(t *testing.T) {
	require.True(t, grokUpstreamExplicitlyDisablesRetry(http.Header{"X-Should-Retry": []string{" false "}}))
	require.True(t, grokUpstreamExplicitlyDisablesRetry(http.Header{"X-Should-Retry": []string{"FALSE"}}))
	require.False(t, grokUpstreamExplicitlyDisablesRetry(http.Header{"X-Should-Retry": []string{"true"}}))
	require.False(t, grokUpstreamExplicitlyDisablesRetry(http.Header{"X-Should-Retry": []string{"0"}}))
	require.False(t, grokUpstreamExplicitlyDisablesRetry(nil))
}

func TestForwardGrokResponsesExplicitNoRetrySuppressesFailoverAndPenalty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"grok","input":"hi","stream":false}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 7001})

	account := grokHardeningOAuthAccount(71, "sent-token", 1)
	repo := &grokHardeningAccountRepo{
		accounts: map[int64]*Account{account.ID: account},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header: http.Header{
			"Content-Type":   []string{"application/json"},
			"X-Should-Retry": []string{"false"},
		},
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"do not retry"}}`)),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream:      upstream,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		accountRepo:       repo,
	}

	result, err := svc.forwardGrokResponses(context.Background(), c, account, body, "grok", false, time.Now())
	require.Nil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Zero(t, repo.tempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	eventsValue, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := eventsValue.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
}

func TestForwardGrokChatExplicitNoRetrySuppressesFailoverAndPenalty(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		wantPath string
	}{
		{
			name:     "native chat",
			body:     []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high","stream":false}`),
			wantPath: "/v1/chat/completions",
		},
		{
			name:     "chat via responses",
			body:     []byte(`{"model":"grok","messages":[{"role":"user","content":"hi"}],"stream":false}`),
			wantPath: "/v1/responses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(tt.body))
			c.Set("api_key", &APIKey{ID: 7003})

			account := grokHardeningOAuthAccount(74, "sent-token", 1)
			if tt.wantPath == "/v1/responses" {
				account.Credentials["model_mapping"] = map[string]any{"grok": "grok-4.5"}
			}
			repo := &grokHardeningAccountRepo{accounts: map[int64]*Account{account.ID: account}}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header: http.Header{
					"Content-Type":   []string{"application/json"},
					"X-Should-Retry": []string{"false"},
				},
				Body: io.NopCloser(strings.NewReader(`{"error":{"message":"do not retry"}}`)),
			}}
			svc := &OpenAIGatewayService{
				httpUpstream:      upstream,
				grokTokenProvider: NewGrokTokenProvider(repo, nil),
				accountRepo:       repo,
			}

			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, tt.body, "", "")
			require.Nil(t, result)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr))
			require.Equal(t, tt.wantPath, upstream.lastReq.URL.Path)
			require.Zero(t, repo.tempUnschedCalls)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
			eventsValue, ok := c.Get(OpsUpstreamErrorsKey)
			require.True(t, ok)
			events, ok := eventsValue.([]*OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.Len(t, events, 1)
		})
	}
}

func TestApplyGrokRequestIdentityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "server-client-request")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	SetGrokUpstreamAttempt(c, 3)

	headers := http.Header{
		grokRequestIDHeader:     []string{"client-request"},
		grokSessionIDHeader:     []string{"client-session"},
		grokTurnIndexHeader:     []string{"999"},
		grokAgentIDHeader:       []string{"client-agent"},
		grokModelOverrideHeader: []string{"client-model"},
	}
	applyGrokRequestIdentityHeaders(headers, c, "isolated-session", "grok-4.3")

	require.Equal(t, generateSessionUUID("grok-request:v1:server-client-request"), headers.Get(grokRequestIDHeader))
	require.Equal(t, "isolated-session", headers.Get(grokSessionIDHeader))
	require.Equal(t, "3", headers.Get(grokTurnIndexHeader))
	require.Equal(t, grokGatewayAgentID, headers.Get(grokAgentIDHeader))
	require.Equal(t, "grok-4.3", headers.Get(grokModelOverrideHeader))
	require.NotContains(t, strings.Join(headers.Values(grokRequestIDHeader), ","), "client-request")
}

func TestGrokUnauthorizedAttributionStatesAndRedaction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	latest := grokHardeningOAuthAccount(72, "latest-secret-token", 2)
	repo := &grokHardeningAccountRepo{
		accounts: map[int64]*Account{latest.ID: latest},
	}
	svc := &OpenAIGatewayService{accountRepo: repo}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	staleSent := describeGrokCredential(grokHardeningOAuthAccount(72, "old-secret-token", 1), "old-secret-token")
	stale := svc.attributeGrokUnauthorized(context.Background(), c, latest, http.StatusUnauthorized, staleSent)
	require.Equal(t, "stale", stale.state)
	require.False(t, shouldPenalizeGrokUnauthorized(http.StatusUnauthorized, stale))

	currentSent := describeGrokCredential(latest, "latest-secret-token")
	current := svc.attributeGrokUnauthorized(context.Background(), c, latest, http.StatusUnauthorized, currentSent)
	require.Equal(t, "current", current.state)
	require.True(t, shouldPenalizeGrokUnauthorized(http.StatusUnauthorized, current))

	unknown := (&OpenAIGatewayService{}).attributeGrokUnauthorized(context.Background(), c, latest, http.StatusUnauthorized, currentSent)
	require.Equal(t, "unknown", unknown.state)
	require.True(t, shouldPenalizeGrokUnauthorized(http.StatusUnauthorized, unknown))

	event := OpsUpstreamErrorEvent{}
	applyGrokUnauthorizedAttribution(&event, stale)
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "old-secret-token")
	require.NotContains(t, string(encoded), "latest-secret-token")
	require.Equal(t, hashSensitiveValueForLog("old-secret-token"), event.SentTokenFingerprint)
	require.Equal(t, hashSensitiveValueForLog("latest-secret-token"), event.CurrentTokenFingerprint)
}

func TestForwardGrokResponsesUnauthorizedCredentialAttribution(t *testing.T) {
	tests := []struct {
		name             string
		sentToken        string
		sentVersion      int64
		latestToken      string
		latestVersion    int64
		wantState        string
		wantStale        bool
		wantPenaltyCalls int
	}{
		{
			name:          "stale token fails over without penalty",
			sentToken:     "old-token",
			sentVersion:   1,
			latestToken:   "new-token",
			latestVersion: 2,
			wantState:     "stale",
			wantStale:     true,
		},
		{
			name:             "current token keeps existing penalty",
			sentToken:        "current-token",
			sentVersion:      3,
			latestToken:      "current-token",
			latestVersion:    3,
			wantState:        "current",
			wantPenaltyCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			body := []byte(`{"model":"grok","input":"hi","stream":false}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Set("api_key", &APIKey{ID: 7002})

			sentAccount := grokHardeningOAuthAccount(73, tt.sentToken, tt.sentVersion)
			latestAccount := grokHardeningOAuthAccount(73, tt.latestToken, tt.latestVersion)
			repo := &grokHardeningAccountRepo{accounts: map[int64]*Account{73: latestAccount}}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"unauthorized"}}`)),
			}}
			svc := &OpenAIGatewayService{
				httpUpstream:      upstream,
				grokTokenProvider: NewGrokTokenProvider(nil, nil),
				accountRepo:       repo,
			}

			result, err := svc.forwardGrokResponses(context.Background(), c, sentAccount, body, "grok", false, time.Now())
			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, tt.wantStale, failoverErr.GrokStaleCredential)
			require.Equal(t, tt.wantPenaltyCalls, repo.tempUnschedCalls)

			eventsValue, ok := c.Get(OpsUpstreamErrorsKey)
			require.True(t, ok)
			events, ok := eventsValue.([]*OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.Len(t, events, 1)
			require.Equal(t, tt.wantState, events[0].TokenState)
			require.Equal(t, "provider", events[0].TokenSource)
			require.NotContains(t, events[0].SentTokenFingerprint, tt.sentToken)
		})
	}
}

func TestGetGrokAccessTokenWithDescriptorResolvesCredentialOwner(t *testing.T) {
	parent := grokHardeningOAuthAccount(80, "parent-token", 4)
	parentID := parent.ID
	shadow := &Account{
		ID:              81,
		ParentAccountID: &parentID,
		Platform:        PlatformGrok,
		Type:            AccountTypeOAuth,
	}
	repo := &grokHardeningAccountRepo{accounts: map[int64]*Account{parent.ID: parent}}
	svc := &OpenAIGatewayService{accountRepo: repo}

	token, kind, descriptor, err := svc.getGrokAccessTokenWithDescriptor(context.Background(), shadow)
	require.NoError(t, err)
	require.Equal(t, "parent-token", token)
	require.Equal(t, "oauth", kind)
	require.Equal(t, parent.ID, descriptor.accountID)
	require.Equal(t, int64(4), descriptor.version)
	require.Equal(t, "account", descriptor.source)
	require.Equal(t, hashSensitiveValueForLog("parent-token"), descriptor.fingerprint)
}

type grokHardeningAccountRepo struct {
	AccountRepository
	accounts           map[int64]*Account
	tempUnschedCalls   int
	tempUnschedAccount int64
}

func (r *grokHardeningAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	return r.accounts[id], nil
}

func (r *grokHardeningAccountRepo) SetTempUnschedulable(_ context.Context, id int64, _ time.Time, _ string) error {
	r.tempUnschedCalls++
	r.tempUnschedAccount = id
	return nil
}

func grokHardeningOAuthAccount(id int64, token string, version int64) *Account {
	return &Account{
		ID:          id,
		Name:        "grok-hardening",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":   token,
			"expires_at":     time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"_token_version": version,
			"base_url":       xai.DefaultBaseURL,
			"model_mapping":  map[string]any{"grok": "grok-4.3"},
		},
	}
}
