package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

const kimiRequestRefreshTimeout = 8 * time.Second

type KimiTokenProvider struct {
	accountRepo AccountRepository
	tokenCache  GeminiTokenCache
	refreshAPI  *OAuthRefreshAPI
	executor    OAuthRefreshExecutor
}

func NewKimiTokenProvider(accountRepo AccountRepository, tokenCache GeminiTokenCache) *KimiTokenProvider {
	return &KimiTokenProvider{accountRepo: accountRepo, tokenCache: tokenCache}
}

func (p *KimiTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI, p.executor = api, executor
}

func (p *KimiTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil || account.Platform != PlatformKimi || account.Type != AccountTypeOAuth {
		return "", errors.New("not a Kimi OAuth account")
	}
	cacheKey := KimiTokenCacheKey(account)
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
		}
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= kimiTokenRefreshSkew
	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		refreshCtx, cancel := context.WithTimeout(ctx, kimiRequestRefreshTimeout)
		defer cancel()
		result, err := p.refreshAPI.RefreshIfNeeded(refreshCtx, account, p.executor, kimiTokenRefreshSkew)
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
		return "", errors.New("Kimi access token is missing")
	}
	if expiresAt != nil && !time.Now().Before(*expiresAt) {
		return "", errors.New("Kimi access token is expired")
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
			if remaining > kimiTokenRefreshSkew {
				ttl = remaining - kimiTokenRefreshSkew
			} else if remaining > 0 {
				ttl = remaining
			}
		}
		_ = p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
	}
	return accessToken, nil
}

func (p *KimiTokenProvider) ForceRefresh(ctx context.Context, account *Account) (string, error) {
	if p == nil || p.refreshAPI == nil || p.executor == nil {
		return "", errors.New("Kimi OAuth refresh is not configured")
	}
	refreshCtx, cancel := context.WithTimeout(ctx, kimiRequestRefreshTimeout)
	defer cancel()
	result, err := p.refreshAPI.RefreshIfNeeded(refreshCtx, account, p.executor, 100*365*24*time.Hour)
	if err != nil {
		return "", err
	}
	if result != nil && result.LockHeld {
		return "", errors.New("Kimi token refresh is already in progress")
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
		return "", errors.New("Kimi access token is missing after refresh")
	}
	if p.tokenCache != nil {
		_ = p.tokenCache.SetAccessToken(ctx, KimiTokenCacheKey(account), token, 30*time.Minute)
	}
	return token, nil
}
