package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

const cursorRequestRefreshTimeout = 10 * time.Second

type CursorTokenProvider struct {
	accountRepo AccountRepository
	tokenCache  GeminiTokenCache
	refreshAPI  *OAuthRefreshAPI
	executor    OAuthRefreshExecutor
}

func NewCursorTokenProvider(accountRepo AccountRepository, tokenCache GeminiTokenCache) *CursorTokenProvider {
	return &CursorTokenProvider{accountRepo: accountRepo, tokenCache: tokenCache}
}

func (p *CursorTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI, p.executor = api, executor
}

func (p *CursorTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil || account.Platform != PlatformCursor || account.Type != AccountTypeOAuth {
		return "", errors.New("not a Cursor OAuth account")
	}
	cacheKey := CursorTokenCacheKey(account)
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
		}
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= cursorTokenRefreshSkew
	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		refreshCtx, cancel := context.WithTimeout(ctx, cursorRequestRefreshTimeout)
		defer cancel()
		result, err := p.refreshAPI.RefreshIfNeeded(refreshCtx, account, p.executor, cursorTokenRefreshSkew)
		if err != nil {
			return "", err
		}
		if result != nil && result.Account != nil {
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
		}
	}
	accessToken := strings.TrimSpace(account.GetCredential("access_token"))
	if accessToken == "" {
		return "", errors.New("cursor access token is missing")
	}
	if expiresAt != nil && !time.Now().Before(*expiresAt) {
		return "", errors.New("cursor access token is expired")
	}
	if latest, stale := CheckTokenVersion(ctx, account, p.accountRepo); stale && latest != nil {
		account = latest
		accessToken = strings.TrimSpace(account.GetCredential("access_token"))
		expiresAt = account.GetCredentialAsTime("expires_at")
	}
	if p.tokenCache != nil {
		ttl := 30 * time.Minute
		if expiresAt != nil {
			remaining := time.Until(*expiresAt)
			if remaining > cursorTokenRefreshSkew {
				ttl = remaining - cursorTokenRefreshSkew
			} else if remaining > 0 {
				ttl = remaining
			}
		}
		_ = p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
	}
	return accessToken, nil
}

func (p *CursorTokenProvider) ForceRefresh(ctx context.Context, account *Account) (string, error) {
	if p == nil || p.refreshAPI == nil || p.executor == nil {
		return "", errors.New("cursor OAuth refresh is not configured")
	}
	refreshCtx, cancel := context.WithTimeout(ctx, cursorRequestRefreshTimeout)
	defer cancel()
	result, err := p.refreshAPI.RefreshIfNeeded(refreshCtx, account, p.executor, 100*365*24*time.Hour)
	if err != nil {
		return "", err
	}
	if result != nil && result.LockHeld {
		return "", errors.New("cursor token refresh is already in progress")
	}
	if result != nil && result.Account != nil {
		account = result.Account
	}
	token := strings.TrimSpace(account.GetCredential("access_token"))
	if result != nil && result.NewCredentials != nil {
		if refreshed, _ := result.NewCredentials["access_token"].(string); strings.TrimSpace(refreshed) != "" {
			token = refreshed
		}
	}
	if token == "" {
		return "", errors.New("cursor access token is missing after refresh")
	}
	if p.tokenCache != nil {
		_ = p.tokenCache.SetAccessToken(ctx, CursorTokenCacheKey(account), token, 30*time.Minute)
	}
	return token, nil
}
