package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const (
	grokTokenCacheSkew             = 5 * time.Minute
	grokRequestRefreshTimeout      = 8 * time.Second
	grokTokenProviderLogComponent  = "grok_token_provider"
	grokTempUnschedulableErrorCode = "token_refresh_failed"
)

var (
	errGrokOAuthConfiguredProxyMiss  = errors.New("grok oauth account has proxy_id but proxy is not loaded")
	errGrokOAuthRefreshTokenMissing  = errors.New("grok oauth refresh_token is missing")
	errGrokOAuthAccessTokenMissing   = errors.New("grok oauth access_token is missing")
	errGrokOAuthAccessTokenExpired   = errors.New("grok oauth access_token is expired")
	errGrokOAuthRefreshNotConfigured = errors.New("grok oauth refresh is not configured")
)

type GrokTokenCache = GeminiTokenCache

type GrokTokenProvider struct {
	accountRepo      AccountRepository
	tokenCache       GrokTokenCache
	refreshAPI       *OAuthRefreshAPI
	executor         OAuthRefreshExecutor
	refreshPolicy    ProviderRefreshPolicy
	tempUnschedCache TempUnschedCache
}

func NewGrokTokenProvider(
	accountRepo AccountRepository,
	tokenCache GrokTokenCache,
) *GrokTokenProvider {
	return &GrokTokenProvider{
		accountRepo:   accountRepo,
		tokenCache:    tokenCache,
		refreshPolicy: AntigravityProviderRefreshPolicy(),
	}
}

func (p *GrokTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

func (p *GrokTokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
}

func (p *GrokTokenProvider) SetTempUnschedCache(cache TempUnschedCache) {
	p.tempUnschedCache = cache
}

func (p *GrokTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformGrok || account.Type != AccountTypeOAuth {
		return "", errors.New("not a grok oauth account")
	}

	cacheKey := GrokTokenCacheKey(account)
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
		}
	}

	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= grokTokenRefreshSkew
	if needsRefresh && strings.TrimSpace(account.GetGrokRefreshToken()) == "" {
		if expiresAt == nil || !time.Now().Before(*expiresAt) {
			return "", errors.New("grok access_token expired and refresh_token is missing")
		}
		needsRefresh = false
	}
	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		refreshCtx, cancel := context.WithTimeout(ctx, grokRequestRefreshTimeout)
		defer cancel()
		result, err := p.refreshAPI.RefreshIfNeeded(refreshCtx, account, p.executor, grokTokenRefreshSkew)
		if err != nil {
			p.markTempUnschedulable(account, err)
			if p.refreshPolicy.OnRefreshError == ProviderRefreshErrorReturn {
				return "", err
			}
		} else if !result.LockHeld && result.Account != nil {
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
		}
	}

	accessToken := account.GetGrokAccessToken()
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
	}

	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			accessToken = latestAccount.GetGrokAccessToken()
			if strings.TrimSpace(accessToken) == "" {
				return "", errors.New("access_token not found after version check")
			}
		} else {
			ttl := 30 * time.Minute
			if expiresAt != nil {
				until := time.Until(*expiresAt)
				switch {
				case until > grokTokenCacheSkew:
					ttl = until - grokTokenCacheSkew
				case until > 0:
					ttl = until
				default:
					ttl = time.Minute
				}
			}
			_ = p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
		}
	}

	return accessToken, nil
}

// GetAccessTokenForManualTest returns an access token for an admin-initiated
// "test connection" probe. Unlike GetAccessToken it does not mark the account
// temporarily unschedulable on refresh failure, and it surfaces concrete
// credential/refresh errors so operators can diagnose cooled-down accounts
// without polluting production scheduling state (#4598).
func (p *GrokTokenProvider) GetAccessTokenForManualTest(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformGrok || account.Type != AccountTypeOAuth {
		return "", errors.New("not a grok oauth account")
	}
	if account.ProxyID != nil && account.Proxy == nil {
		return "", errGrokOAuthConfiguredProxyMiss
	}
	if strings.TrimSpace(account.GetGrokRefreshToken()) == "" {
		return "", errGrokOAuthRefreshTokenMissing
	}

	accessToken := strings.TrimSpace(account.GetGrokAccessToken())
	expiresAt := account.GetCredentialAsTime("expires_at")
	tokenValid := accessToken != "" && expiresAt != nil && time.Now().Before(*expiresAt)
	if accessToken != "" && expiresAt != nil && time.Until(*expiresAt) > grokTokenRefreshSkew {
		return accessToken, nil
	}

	if p.refreshAPI == nil || p.executor == nil {
		if tokenValid {
			return accessToken, nil
		}
		return "", errGrokOAuthRefreshNotConfigured
	}

	// Manual tests must not call markTempUnschedulable on refresh failure.
	refreshCtx, cancel := context.WithTimeout(ctx, grokRequestRefreshTimeout)
	defer cancel()
	result, err := p.refreshAPI.RefreshIfNeeded(refreshCtx, account, p.executor, grokTokenRefreshSkew)
	if err != nil {
		if tokenValid {
			return accessToken, nil
		}
		return "", err
	}
	if result != nil && result.LockHeld {
		if tokenValid {
			return accessToken, nil
		}
		return "", errors.New("token refresh is already in progress on another worker; retry in a few seconds")
	}
	if result != nil && result.Account != nil {
		account = result.Account
	}

	accessToken = strings.TrimSpace(account.GetGrokAccessToken())
	if accessToken == "" {
		return "", errGrokOAuthAccessTokenMissing
	}
	if latestExpiry := account.GetCredentialAsTime("expires_at"); latestExpiry != nil && !time.Now().Before(*latestExpiry) {
		return "", errGrokOAuthAccessTokenExpired
	}
	return accessToken, nil
}

func (p *GrokTokenProvider) markTempUnschedulable(account *Account, refreshErr error) {
	if p == nil || p.accountRepo == nil || account == nil {
		return
	}
	now := time.Now()
	until := now.Add(tokenRefreshTempUnschedDuration)
	redactedErr := "unknown error"
	if refreshErr != nil {
		redactedErr = logredact.RedactText(refreshErr.Error())
	}
	if isNonRetryableRefreshError(refreshErr) {
		if err := p.accountRepo.SetError(context.Background(), account.ID, "grok token refresh failed (non-retryable): "+redactedErr); err != nil {
			slog.Warn(grokTokenProviderLogComponent+".set_error_status_failed", "account_id", account.ID, "error", err)
		}
		return
	}
	reason := "grok token refresh failed on request path: " + redactedErr
	bgCtx := context.Background()
	if err := p.accountRepo.SetTempUnschedulable(bgCtx, account.ID, until, reason); err != nil {
		slog.Warn(grokTokenProviderLogComponent+".set_temp_unschedulable_failed", "account_id", account.ID, "error", err)
		return
	}
	if p.tempUnschedCache != nil {
		state := &TempUnschedState{
			UntilUnix:       until.Unix(),
			TriggeredAtUnix: now.Unix(),
			ErrorMessage:    grokTempUnschedulableErrorCode + ": " + reason,
		}
		if err := p.tempUnschedCache.SetTempUnsched(bgCtx, account.ID, state); err != nil {
			slog.Warn(grokTokenProviderLogComponent+".temp_unsched_cache_set_failed", "account_id", account.ID, "error", err)
		}
	}
}

func GrokTokenCacheKey(account *Account) string {
	if account == nil {
		return "grok:account:0"
	}
	return "grok:account:" + strconv.FormatInt(account.ID, 10)
}
