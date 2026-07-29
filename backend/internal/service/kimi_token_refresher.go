package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

type KimiTokenRefresher struct{ oauthService *KimiOAuthService }

func NewKimiTokenRefresher(oauthService *KimiOAuthService) *KimiTokenRefresher {
	return &KimiTokenRefresher{oauthService: oauthService}
}

func (r *KimiTokenRefresher) CacheKey(account *Account) string { return KimiTokenCacheKey(account) }

func (r *KimiTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformKimi && account.Type == AccountTypeOAuth
}

func (r *KimiTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if !r.CanRefresh(account) || strings.TrimSpace(account.GetCredential("refresh_token")) == "" {
		return false
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return true
	}
	if refreshWindow < kimiTokenRefreshSkew {
		refreshWindow = kimiTokenRefreshSkew
	}
	return time.Until(*expiresAt) <= refreshWindow
}

func (r *KimiTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r == nil || r.oauthService == nil {
		return nil, errors.New("Kimi OAuth service is not configured")
	}
	token, err := r.oauthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}
	return MergeCredentials(account.Credentials, r.oauthService.BuildAccountCredentials(token)), nil
}

func KimiTokenCacheKey(account *Account) string {
	if account == nil {
		return "kimi:account:0"
	}
	return "kimi:account:" + strconv.FormatInt(account.ID, 10)
}
