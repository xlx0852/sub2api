//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type alphaSearchCancellationAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r alphaSearchCancellationAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]service.Account, error) {
	return r.accountsForPlatform(platform), nil
}

func (r alphaSearchCancellationAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.accountsForPlatform(platform), nil
}

func (r alphaSearchCancellationAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.accountsForPlatform(platform), nil
}

func (r alphaSearchCancellationAccountRepo) accountsForPlatform(platform string) []service.Account {
	accounts := make([]service.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform {
			accounts = append(accounts, account)
		}
	}
	return accounts
}

type alphaSearchCancellationUpstream struct {
	service.HTTPUpstream
	mu         sync.Mutex
	accountIDs []int64
}

func (u *alphaSearchCancellationUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.accountIDs = append(u.accountIDs, accountID)
	return nil, context.Canceled
}

func (u *alphaSearchCancellationUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.accountIDs...)
}

func TestOpenAIGatewayHandlerAlphaSearch_ContextCanceledDoesNotFailOverOrPenalizeAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(4100)
	accounts := []service.Account{
		{
			ID:          41,
			Name:        "alpha-account-1",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Priority:    0,
			Credentials: map[string]any{"api_key": "sk-test-1"},
		},
		{
			ID:          42,
			Name:        "alpha-account-2",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Priority:    1,
			Credentials: map[string]any{"api_key": "sk-test-2"},
		},
	}
	accountRepo := alphaSearchCancellationAccountRepo{accounts: accounts}
	upstream := &alphaSearchCancellationUpstream{}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	gatewayService := service.NewOpenAIGatewayService(
		accountRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingService.Stop)
	concurrencyService := service.NewConcurrencyService(nil)
	h := NewOpenAIGatewayHandler(
		gatewayService,
		concurrencyService,
		billingService,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil,
		nil,
		nil,
		nil,
		cfg,
	)
	h.maxAccountSwitches = 10

	body := []byte(`{"id":"search-session","model":"gpt-5.6-sol","commands":{"search_query":[{"q":"news"}]}}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      99,
		GroupID: &groupID,
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			Status:   service.StatusActive,
		},
		User: &service.User{ID: 100, Status: service.StatusActive},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100, Concurrency: 0})

	h.AlphaSearch(c)

	require.Equal(t, statusClientClosedRequest, rec.Code)
	require.Equal(t, "api_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Equal(t, "context canceled", gjson.GetBytes(rec.Body.Bytes(), "error.message").String())
	require.Equal(t, []int64{41}, upstream.calls(), "client cancellation must not fail over")

	_, hasUpstreamErrors := c.Get(service.OpsUpstreamErrorsKey)
	require.False(t, hasUpstreamErrors, "client cancellation must not emit an upstream Ops event")
}
