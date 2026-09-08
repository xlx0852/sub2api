package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

const cursorTokenRefreshSkew = 24 * time.Hour

type CursorOAuthTokenService interface {
	RefreshAccountToken(ctx context.Context, account *Account) (*CursorOAuthTokenInfo, error)
	BuildAccountCredentials(tokenInfo *CursorOAuthTokenInfo) map[string]any
}

type CursorTokenRefresher struct {
	cursorOAuthService CursorOAuthTokenService
}

func NewCursorTokenRefresher(cursorOAuthService CursorOAuthTokenService) *CursorTokenRefresher {
	return &CursorTokenRefresher{cursorOAuthService: cursorOAuthService}
}

func (r *CursorTokenRefresher) CacheKey(account *Account) string {
	return CursorTokenCacheKey(account)
}

func (r *CursorTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformCursor && account.Type == AccountTypeOAuth
}

func (r *CursorTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if account == nil {
		return false
	}
	hasRefreshToken := strings.TrimSpace(account.GetCredential("refresh_token")) != ""
	hasAccessToken := strings.TrimSpace(account.GetCredential("access_token")) != ""
	if !hasRefreshToken && !hasAccessToken {
		return false
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return true
	}
	if refreshWindow < cursorTokenRefreshSkew {
		refreshWindow = cursorTokenRefreshSkew
	}
	return time.Until(*expiresAt) < refreshWindow
}

func (r *CursorTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r == nil || r.cursorOAuthService == nil {
		return nil, errors.New("cursor oauth service is not configured")
	}
	tokenInfo, err := r.cursorOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}
	newCredentials := r.cursorOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)
	return newCredentials, nil
}

func CursorTokenCacheKey(account *Account) string {
	if account == nil {
		return "cursor:account:0"
	}
	return "cursor:account:" + strconv.FormatInt(account.ID, 10)
}
