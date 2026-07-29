//go:build unit

package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAccountTestService_TestAccountConnection_GrokUsesXAIResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID:          13,
		Name:        "grok-oauth",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "grok-access-token",
			"refresh_token": "grok-refresh-token",
			"expires_at":    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"model_mapping": map[string]any{
				"grok": "grok-4.3",
			},
		},
	}
	repo := &mockAccountRepoForGemini{
		accountsByID: map[int64]*Account{account.ID: account},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
				"data: {\"type\":\"response.completed\"}\n\n",
		)),
	}}
	svc := &AccountTestService{
		accountRepo:       repo,
		grokTokenProvider: NewGrokTokenProvider(repo, nil),
		httpUpstream:      upstream,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/13/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "grok", "", AccountTestModeDefault)
	require.NoError(t, err)

	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer grok-access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, xai.GrokCLITokenAuthValue, upstream.lastReq.Header.Get(xai.GrokCLITokenAuthHeader))
	require.Equal(t, xai.GrokCLIVersionValue, upstream.lastReq.Header.Get(xai.GrokCLIVersionHeader))
	require.Equal(t, HTTPUpstreamProfileGrok, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Empty(t, upstream.lastReq.Header.Get(grokConversationIDHeader))
	require.Equal(t, "grok-4.3", gjson.GetBytes(upstream.lastBody, "model").String())
	require.NotContains(t, rec.Body.String(), "claude")
	require.Contains(t, rec.Body.String(), `"model":"grok-4.3"`)
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_TestAccountConnection_GrokAPIKeyUsesOfficialResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID:          14,
		Name:        "grok-api-key",
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{
			"api_key":   "xai-api-key",
			"base_url":  xai.DefaultBaseURL,
			"using_api": true,
			"model_mapping": map[string]any{
				"grok": "grok-4.5",
			},
		},
	}
	repo := &mockAccountRepoForGemini{
		accountsByID: map[int64]*Account{account.ID: account},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
				"data: {\"type\":\"response.completed\"}\n\n",
		)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/14/test", nil)

	err := svc.TestAccountConnection(c, account.ID, "grok", "", AccountTestModeDefault)
	require.NoError(t, err)

	require.Equal(t, "https://api.x.ai/v1/responses", upstream.lastReq.URL.String())
	require.Empty(t, upstream.lastReq.Header.Get(grokConversationIDHeader))
	require.Equal(t, "Bearer xai-api-key", upstream.lastReq.Header.Get("Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get(xai.GrokCLITokenAuthHeader))
	require.Empty(t, upstream.lastReq.Header.Get(xai.GrokCLIVersionHeader))
	require.Equal(t, "grok-4.5", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
}
