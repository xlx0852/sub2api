package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCCFallbackTargetPreservesKimiTokenError(t *testing.T) {
	svc := &OpenAIGatewayService{kimiTokenProvider: &KimiTokenProvider{}}
	account := &Account{
		ID: 42, Platform: PlatformKimi, Type: AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "expired-test-token",
			"expires_at":   time.Now().Add(-time.Hour).Format(time.RFC3339),
		},
	}
	_, _, tokenErr := svc.GetAccessToken(context.Background(), account)
	require.Error(t, tokenErr)
	token, target, err := svc.resolveCCFallbackTarget(context.Background(), account)
	require.Empty(t, token)
	require.Empty(t, target)
	require.EqualError(t, err, tokenErr.Error())
}
