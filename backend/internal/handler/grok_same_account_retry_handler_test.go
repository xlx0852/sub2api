package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokRetryHandlerEvents struct {
	mu     sync.Mutex
	events []string
}

func (e *grokRetryHandlerEvents) add(event string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

func (e *grokRetryHandlerEvents) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.events...)
}

type grokRetryHandlerAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
	events   *grokRetryHandlerEvents
}

func (r *grokRetryHandlerAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]service.Account, error) {
	r.events.add("select")
	return r.accountsForPlatform(platform), nil
}

func (r *grokRetryHandlerAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	r.events.add("select")
	return r.accountsForPlatform(platform), nil
}

func (r *grokRetryHandlerAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	r.events.add("select")
	return r.accountsForPlatform(platform), nil
}

func (r *grokRetryHandlerAccountRepo) SetTempUnschedulable(_ context.Context, id int64, _ time.Time, _ string) error {
	r.events.add("penalty:" + strconv.FormatInt(id, 10))
	return nil
}

func (r *grokRetryHandlerAccountRepo) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	return nil
}

func (r *grokRetryHandlerAccountRepo) accountsForPlatform(platform string) []service.Account {
	accounts := make([]service.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform {
			accounts = append(accounts, account)
		}
	}
	return accounts
}

type grokRetryHandlerUpstream struct {
	service.HTTPUpstream
	events  *grokRetryHandlerEvents
	mu      sync.Mutex
	calls   []int64
	headers []http.Header
}

func (u *grokRetryHandlerUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.calls = append(u.calls, accountID)
	u.headers = append(u.headers, req.Header.Clone())
	accountAttempt := 0
	for _, id := range u.calls {
		if id == accountID {
			accountAttempt++
		}
	}
	u.mu.Unlock()
	u.events.add("upstream:" + strconv.FormatInt(accountID, 10))

	if accountID == 101 && accountAttempt <= 2 {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":{"message":"transient"}}`)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(bytes.NewBufferString(
			`{"id":"chatcmpl_retry_ok","object":"chat.completion","model":"grok-4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		)),
	}, nil
}

func (u *grokRetryHandlerUpstream) requestHeaders() []http.Header {
	u.mu.Lock()
	defer u.mu.Unlock()
	headers := make([]http.Header, 0, len(u.headers))
	for _, header := range u.headers {
		headers = append(headers, header.Clone())
	}
	return headers
}

func (u *grokRetryHandlerUpstream) accountIDs() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.calls...)
}

func TestOpenAIGatewayHandlerChatCompletions_GrokRetryPinsAccountThenFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(501)
	events := &grokRetryHandlerEvents{}
	accounts := []service.Account{
		{
			ID: 101, Name: "grok-transient", Platform: service.PlatformGrok,
			Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
			Concurrency: 2, Priority: 0,
			Credentials: map[string]any{
				"access_token": "grok-first",
				"base_url":     "https://example.invalid",
				"model_mapping": map[string]any{
					"grok-4.3": "grok-4.3",
				},
			},
		},
		{
			ID: 102, Name: "grok-healthy", Platform: service.PlatformGrok,
			Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
			Concurrency: 2, Priority: 1,
			Credentials: map[string]any{
				"access_token": "grok-second",
				"base_url":     "https://example.invalid",
				"model_mapping": map[string]any{
					"grok-4.3": "grok-4.4",
				},
			},
		},
	}
	accountRepo := &grokRetryHandlerAccountRepo{accounts: accounts, events: events}
	upstream := &grokRetryHandlerUpstream{events: events}
	require.True(t, accounts[0].IsSchedulableForModel("grok-4.3"))
	require.True(t, accounts[0].IsModelSupported("grok-4.3"))
	require.True(t, accounts[0].SupportsOpenAIEndpointCapability(service.OpenAIEndpointCapabilityChatCompletions))
	cfg := testGrokRetryConfig()
	cfg.RunMode = config.RunModeSimple
	cfg.Gateway.GrokSameAccountRetry.FallbackDelayMS = 0
	cfg.Gateway.MaxAccountSwitches = 3

	var acquiredMu sync.Mutex
	var acquiredIDs []int64
	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) {
			return true, nil
		},
		acquireAccountSlotFn: func(_ context.Context, accountID int64, _ int, _ string) (bool, error) {
			acquiredMu.Lock()
			acquiredIDs = append(acquiredIDs, accountID)
			acquiredMu.Unlock()
			return true, nil
		},
	}
	concurrencyService := service.NewConcurrencyService(cache)
	gatewayService := service.NewOpenAIGatewayService(
		accountRepo, nil, nil, nil, nil, nil, nil, cfg, nil, concurrencyService, nil, nil, nil,
		upstream, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingService.Stop)
	handler := NewOpenAIGatewayHandler(
		gatewayService,
		concurrencyService,
		billingService,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil, cfg,
	)

	apiKey := &service.APIKey{
		ID: 601, GroupID: &groupID,
		User:  &service.User{ID: 701, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformGrok, Status: service.StatusActive},
	}
	body := []byte(`{"model":"grok-4.3","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"high","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	requestContext := context.WithValue(context.Background(), ctxkey.ClientRequestID, "grok-retry-handler-request")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(requestContext)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})

	handler.ChatCompletions(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"content":"ok"`)
	require.Equal(t, []int64{101, 101, 102}, upstream.accountIDs())
	require.Equal(t,
		[]string{"select", "upstream:101", "upstream:101", "penalty:101", "select", "upstream:102"},
		events.snapshot(),
	)
	acquiredMu.Lock()
	require.Equal(t, []int64{101, 101, 102}, acquiredIDs)
	acquiredMu.Unlock()
	require.Equal(t, int32(3), atomic.LoadInt32(&cache.releaseAccountCalled))
	require.Equal(t, int32(1), atomic.LoadInt32(&cache.releaseUserCalled))

	headers := upstream.requestHeaders()
	require.Len(t, headers, 3)
	for _, header := range headers {
		require.NotEmpty(t, header.Get("X-Grok-Req-Id"))
		require.NotEmpty(t, header.Get("X-Grok-Session-Id"))
		require.Equal(t, "sub2api", header.Get("X-Grok-Agent-Id"))
		require.Equal(t, headers[0].Get("X-Grok-Req-Id"), header.Get("X-Grok-Req-Id"))
		require.Equal(t, headers[0].Get("X-Grok-Session-Id"), header.Get("X-Grok-Session-Id"))
	}
	require.Equal(t, "grok-4.3", headers[0].Get("X-Grok-Model-Override"))
	require.Equal(t, "grok-4.3", headers[1].Get("X-Grok-Model-Override"))
	require.Equal(t, "grok-4.4", headers[2].Get("X-Grok-Model-Override"))
	require.Equal(t, "1", headers[0].Get("X-Grok-Turn-Idx"))
	require.Equal(t, "2", headers[1].Get("X-Grok-Turn-Idx"))
	require.Equal(t, "3", headers[2].Get("X-Grok-Turn-Idx"))
}
