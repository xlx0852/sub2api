//go:build unit

package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyGrokCredentialFailure_PermanentAccountFailures(t *testing.T) {
	t.Parallel()
	account := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth}

	tests := []struct {
		name   string
		err    error
		reason GatewayFailureReason
	}{
		{name: "missing refresh", err: errGrokOAuthRefreshTokenMissing, reason: GrokCredentialReasonMissing},
		{name: "missing access", err: errGrokOAuthAccessTokenMissing, reason: GrokCredentialReasonMissing},
		{name: "expired access", err: errGrokOAuthAccessTokenExpired, reason: GrokCredentialReasonMissing},
		{name: "invalid_grant", err: errors.New("token refresh failed: invalid_grant"), reason: GrokCredentialReasonRevoked},
		{name: "entitlement", err: errors.New("subscription required"), reason: GrokCredentialReasonEntitlement},
		{name: "proxy miss", err: errGrokOAuthConfiguredProxyMiss, reason: GrokCredentialReasonProxyInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class := classifyGrokCredentialFailure(account, tt.err)
			require.Equal(t, GatewayFailureScopeAccount, class.scope)
			require.Equal(t, NextAccountRetry, class.action)
			require.Equal(t, tt.reason, class.reason)
		})
	}
}

func TestClassifyGrokCredentialFailure_ProviderStop(t *testing.T) {
	t.Parallel()
	account := &Account{ID: 2, Platform: PlatformGrok, Type: AccountTypeOAuth}

	class := classifyGrokCredentialFailure(account, errors.New("invalid_client: client_secret=must-not-leak"))
	require.Equal(t, GatewayFailureScopeProvider, class.scope)
	require.Equal(t, NextAccountStop, class.action)
	require.Equal(t, GrokCredentialReasonProviderConfig, class.reason)

	class = classifyGrokCredentialFailure(account, errGrokOAuthRefreshNotConfigured)
	require.Equal(t, NextAccountStop, class.action)
	require.Equal(t, GrokCredentialReasonProviderConfig, class.reason)
}

func TestClassifyGrokCredentialFailure_TransientDefault(t *testing.T) {
	t.Parallel()
	account := &Account{ID: 3, Platform: PlatformGrok, Type: AccountTypeOAuth}
	class := classifyGrokCredentialFailure(account, errors.New("temporary network glitch"))
	require.Equal(t, GatewayFailureScopeAccount, class.scope)
	require.Equal(t, NextAccountRetry, class.action)
	require.Equal(t, GrokCredentialReasonRefreshTransient, class.reason)
}

func TestWrapGrokOAuthCredentialError_MapsAndDoesNotLeakSecrets(t *testing.T) {
	t.Parallel()
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 4, Platform: PlatformGrok, Type: AccountTypeOAuth}

	err := svc.wrapGrokOAuthCredentialError(account, errors.New("invalid_grant refresh_token=super-secret"))
	var failover *UpstreamFailoverError
	require.True(t, errors.As(err, &failover))
	require.True(t, failover.IsCredentialFailure())
	require.True(t, failover.ShouldRetryNextAccount())
	require.Equal(t, GrokCredentialUnavailableClientMessage, failover.ClientMessage)
	require.NotContains(t, failover.ClientMessage, "super-secret")
	require.NotContains(t, failover.ClientMessage, "refresh_token")
	require.Equal(t, GrokCredentialReasonRevoked, failover.Reason)
}

func TestWrapGrokOAuthCredentialError_ProviderStopDoesNotRetry(t *testing.T) {
	t.Parallel()
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 5, Platform: PlatformGrok, Type: AccountTypeOAuth}

	err := svc.wrapGrokOAuthCredentialError(account, errors.New("invalid_client client_secret=x"))
	var failover *UpstreamFailoverError
	require.True(t, errors.As(err, &failover))
	require.False(t, failover.ShouldRetryNextAccount())
	require.False(t, failover.ShouldReportAccountScheduleFailure())
}

func TestWrapGrokOAuthCredentialError_NonGrokPassthrough(t *testing.T) {
	t.Parallel()
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 6, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	orig := errors.New("plain error")
	require.Equal(t, orig, svc.wrapGrokOAuthCredentialError(account, orig))
}

func TestGetAccessToken_GrokOAuthMapsRefreshFailure(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	cache := &grokTokenCacheForProviderTest{lockResult: true}
	provider := NewGrokTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &tokenRefresherStub{
		err: errors.New("invalid_grant: refresh_token=leaked"),
	})
	svc := &OpenAIGatewayService{grokTokenProvider: provider}
	account := &Account{
		ID:       7,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "expired",
			"refresh_token": "rt",
			"expires_at":    "2000-01-01T00:00:00Z",
		},
	}
	repo.accountsByID = map[int64]*Account{7: account}

	token, kind, err := svc.GetAccessToken(t.Context(), account)
	require.Empty(t, token)
	require.Empty(t, kind)
	var failover *UpstreamFailoverError
	require.True(t, errors.As(err, &failover))
	require.Equal(t, GrokCredentialReasonRevoked, failover.Reason)
	require.Equal(t, GrokCredentialUnavailableClientMessage, failover.ClientMessage)
	require.NotContains(t, failover.Error(), "leaked")
}
