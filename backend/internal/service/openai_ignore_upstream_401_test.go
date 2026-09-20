package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type ignore401TokenInvalidator struct {
	n int
}

func (r *ignore401TokenInvalidator) InvalidateToken(_ context.Context, _ *Account) error {
	r.n++
	return nil
}

func TestIgnoreUpstream401_FailoversWithoutDisablingAccount(t *testing.T) {
	invalidator := &ignore401TokenInvalidator{}
	rl := NewRateLimitService(nil, nil, &config.Config{}, nil, nil)
	rl.SetTokenCacheInvalidator(invalidator)
	svc := &OpenAIGatewayService{rateLimitService: rl}
	account := &Account{
		ID:       132,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"ignore_upstream_401": true},
		Credentials: map[string]any{
			"refresh_token": "rt-132",
		},
	}
	body := []byte(`{"error":{"code":"token_invalidated","message":"Encountered invalidated oauth token for user, failing request"}}`)

	shouldDisable := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusUnauthorized, http.Header{}, body)

	require.True(t, shouldDisable, "this request should still failover")
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account), "account must stay schedulable")
	require.Equal(t, 1, invalidator.n)
}

func TestIgnoreUpstream401_OffByDefault(t *testing.T) {
	account := &Account{ID: 132, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.False(t, account.IsIgnoreUpstream401Enabled())
}
