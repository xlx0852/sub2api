package service

import (
	"errors"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Minimal Grok OAuth credential-failure mapping (Batch 2).
// Maps request-path token acquisition failures into UpstreamFailoverError so
// HTTP/WS account loops can switch accounts (or stop) without leaking secrets.
// Full upstream CAS quarantine / concurrent-refresh recovery is intentionally
// deferred; GrokTokenProvider.markTempUnschedulable already cools accounts.

const GrokCredentialUnavailableClientMessage = "No healthy Grok OAuth account is currently available"

const (
	GrokCredentialReasonRevoked          GatewayFailureReason = "grok_oauth_credential_revoked"
	GrokCredentialReasonMissing          GatewayFailureReason = "grok_oauth_credentials_missing"
	GrokCredentialReasonEntitlement      GatewayFailureReason = "grok_oauth_entitlement_action_required"
	GrokCredentialReasonProxyInvalid     GatewayFailureReason = "grok_oauth_proxy_invalid"
	GrokCredentialReasonRefreshTransient GatewayFailureReason = "grok_oauth_refresh_transient"
	GrokCredentialReasonProviderConfig   GatewayFailureReason = "grok_oauth_provider_config"
	GrokCredentialReasonProviderDown     GatewayFailureReason = "grok_oauth_provider_unavailable"
	GrokCredentialReasonAccountChanged   GatewayFailureReason = "grok_oauth_account_state_changed"
)

type grokCredentialFailureClass struct {
	scope   GatewayFailureScope
	reason  GatewayFailureReason
	action  NextAccountAction
	message string
}

// wrapGrokOAuthCredentialError converts plain Grok OAuth credential errors into
// UpstreamFailoverError. Non-Grok and already-wrapped errors pass through.
func (s *OpenAIGatewayService) wrapGrokOAuthCredentialError(account *Account, err error) error {
	if err == nil {
		return nil
	}
	if account == nil || !account.IsGrokOAuth() {
		return err
	}
	var existing *UpstreamFailoverError
	if errors.As(err, &existing) {
		return err
	}
	class := classifyGrokCredentialFailure(account, err)
	return newGrokCredentialFailoverError(account, class)
}

func newGrokCredentialFailoverError(account *Account, class grokCredentialFailureClass) error {
	if strings.TrimSpace(class.message) == "" {
		class.message = "Grok OAuth credentials are unavailable"
	}
	return &UpstreamFailoverError{
		Stage:             GatewayFailureStageAccountAuth,
		Scope:             class.scope,
		Reason:            class.reason,
		NextAccountAction: class.action,
		ClientStatusCode:  http.StatusServiceUnavailable,
		ClientMessage:     GrokCredentialUnavailableClientMessage,
		// Keep StatusCode 0 so mapUpstreamError does not invent an upstream HTTP status.
	}
}

func classifyGrokCredentialFailure(account *Account, err error) grokCredentialFailureClass {
	stableReason := strings.ToLower(strings.TrimSpace(infraerrors.Reason(err)))
	message := ""
	if err != nil {
		message = strings.ToLower(err.Error())
	}
	contains := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(stableReason, value) || strings.Contains(message, value) {
				return true
			}
		}
		return false
	}
	hasProxy := account != nil && account.ProxyID != nil

	switch {
	case errors.Is(err, errGrokOAuthRefreshTokenMissing),
		errors.Is(err, errGrokOAuthAccessTokenMissing),
		errors.Is(err, errGrokOAuthAccessTokenExpired),
		contains("access_token not found", "access_token expired", "refresh_token is missing"):
		return grokCredentialFailureClass{
			scope:   GatewayFailureScopeAccount,
			reason:  GrokCredentialReasonMissing,
			action:  NextAccountRetry,
			message: "Grok OAuth credentials are missing or expired",
		}
	case contains("invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_reused", "refresh_token_invalidated", "app_session_terminated"):
		return grokCredentialFailureClass{
			scope:   GatewayFailureScopeAccount,
			reason:  GrokCredentialReasonRevoked,
			action:  NextAccountRetry,
			message: "Grok OAuth credentials require account action",
		}
	case contains("entitlement_denied", "subscription required", "no active grok subscription"):
		return grokCredentialFailureClass{
			scope:   GatewayFailureScopeAccount,
			reason:  GrokCredentialReasonEntitlement,
			action:  NextAccountRetry,
			message: "Grok OAuth entitlement requires account action",
		}
	case errors.Is(err, errGrokOAuthConfiguredProxyMiss), contains("proxy_id but proxy is not loaded", "proxy not found"):
		return grokCredentialFailureClass{
			scope:   GatewayFailureScopeAccount,
			reason:  GrokCredentialReasonProxyInvalid,
			action:  NextAccountRetry,
			message: "Grok OAuth account proxy configuration is invalid",
		}
	case errors.Is(err, errGrokOAuthRefreshNotConfigured),
		contains("invalid_client", "unauthorized_client", "invalid_scope", "unknown scope", "not a grok oauth account"):
		return grokCredentialFailureClass{
			scope:   GatewayFailureScopeProvider,
			reason:  GrokCredentialReasonProviderConfig,
			action:  NextAccountStop,
			message: "Grok OAuth provider configuration is unavailable",
		}
	case !hasProxy && contains("status 429", "status 500", "status 502", "status 503", "status 504"):
		return grokCredentialFailureClass{
			scope:   GatewayFailureScopeProvider,
			reason:  GrokCredentialReasonProviderDown,
			action:  NextAccountStop,
			message: "Grok OAuth provider is temporarily unavailable",
		}
	default:
		return grokCredentialFailureClass{
			scope:   GatewayFailureScopeAccount,
			reason:  GrokCredentialReasonRefreshTransient,
			action:  NextAccountRetry,
			message: "Grok OAuth credential refresh is temporarily unavailable",
		}
	}
}
