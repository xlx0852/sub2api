package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"github.com/google/uuid"
)

const (
	cursorSessionPrefix   = "cur_"
	cursorDefaultExpiry   = 10 * time.Minute
	cursorDefaultInterval = 2 * time.Second
	cursorPollLeaseTTL    = 30 * time.Second
)

type CursorAuthorizationResult struct {
	SessionID  string `json:"session_id"`
	Status     string `json:"status"`
	LoginURL   string `json:"login_url,omitempty"`
	ExpiresIn  int64  `json:"expires_in"`
	Interval   int64  `json:"interval"`
	RetryAfter int64  `json:"retry_after,omitempty"`
	Error      string `json:"error,omitempty"`
}

type CursorOAuthTokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	MachineID    string `json:"machine_id"`
	Email        string `json:"email,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	ExpiresAt    int64  `json:"expires_at"`
	ProxyID      *int64 `json:"proxy_id,omitempty"`
}

type cursorSessionPayload struct {
	UUID            string                `json:"uuid"`
	Verifier        string                `json:"verifier"`
	Challenge       string                `json:"challenge"`
	LoginURL        string                `json:"login_url"`
	MachineID       string                `json:"machine_id"`
	ProxyID         *int64                `json:"proxy_id,omitempty"`
	ProxyURL        string                `json:"proxy_url,omitempty"`
	IntervalSeconds int64                 `json:"interval_seconds"`
	Token           *CursorOAuthTokenInfo `json:"token,omitempty"`
}

type CursorOAuthService struct {
	deviceSessionStore ProviderOAuthSessionStore
	proxyRepo          ProxyRepository
	backendURL         string
	websiteURL         string
	now                func() time.Time
}

func NewCursorOAuthService(proxyRepo ProxyRepository, deviceStore ProviderOAuthSessionStore) *CursorOAuthService {
	return &CursorOAuthService{
		proxyRepo:          proxyRepo,
		deviceSessionStore: deviceStore,
		backendURL:         cursor.DefaultBackendURL,
		websiteURL:         cursor.DefaultWebsiteURL,
		now:                time.Now,
	}
}

func (s *CursorOAuthService) createHTTPClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if proxyURL != "" {
		parsedURL, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("parse proxy url: %w", err)
		}
		if err := proxyutil.ConfigureTransportProxy(transport, parsedURL); err != nil {
			return nil, fmt.Errorf("configure proxy: %w", err)
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

func (s *CursorOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_PROXY_NOT_AVAILABLE", "proxy repository is not available")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "CURSOR_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}

func (s *CursorOAuthService) StartAuthorization(ctx context.Context, proxyID *int64) (*CursorAuthorizationResult, error) {
	if s == nil || s.deviceSessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "CURSOR_OAUTH_NOT_CONFIGURED", "Cursor OAuth is not configured")
	}

	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}

	verifier, challenge, err := cursor.GeneratePKCE()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CURSOR_OAUTH_PKCE_FAILED", "generate pkce: %v", err)
	}

	sessionUUID := uuid.NewString()
	machineID := uuid.NewString()
	sessionID := cursorSessionPrefix + strings.ReplaceAll(uuid.NewString(), "-", "")
	loginURL := cursor.BuildLoginURL(s.websiteURL, challenge, sessionUUID, "sand")

	payload := &cursorSessionPayload{
		UUID:            sessionUUID,
		Verifier:        verifier,
		Challenge:       challenge,
		LoginURL:        loginURL,
		MachineID:       machineID,
		ProxyID:         proxyID,
		ProxyURL:        proxyURL,
		IntervalSeconds: int64(cursorDefaultInterval.Seconds()),
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CURSOR_OAUTH_SESSION_FAILED", "encode session: %v", err)
	}

	now := s.now()
	expiresIn := cursorDefaultExpiry
	session := &ProviderOAuthSession{
		ID:                  sessionID,
		Provider:            PlatformCursor,
		Flow:                ProviderOAuthFlowAuthorizationCode,
		Status:              ProviderOAuthSessionPending,
		NextPollAtUnixMilli: now.UnixMilli(),
		ExpiresAtUnixMilli:  now.Add(expiresIn).UnixMilli(),
		Payload:             encoded,
	}

	if err := s.deviceSessionStore.Create(ctx, session, expiresIn); err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "CURSOR_OAUTH_SESSION_STORE_FAILED", "store session: %v", err)
	}

	return cursorAuthorizationResult(session, payload, now), nil
}

func (s *CursorOAuthService) GetAuthorizationStatus(ctx context.Context, sessionID string) (*CursorAuthorizationResult, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.deviceSessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "CURSOR_OAUTH_NOT_CONFIGURED", "Cursor OAuth is not configured")
	}

	now := s.now()
	lease, err := s.deviceSessionStore.AcquirePollLease(ctx, sessionID, now, cursorPollLeaseTTL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "CURSOR_OAUTH_SESSION_STORE_FAILED", "acquire poll lease: %v", err)
	}
	if lease == nil || lease.Session == nil || lease.Session.Provider != PlatformCursor {
		return nil, infraerrors.New(http.StatusNotFound, "CURSOR_OAUTH_SESSION_NOT_FOUND", "Cursor authorization session not found or expired")
	}

	session := lease.Session
	if session.Status == ProviderOAuthSessionCancelled {
		return nil, infraerrors.New(http.StatusGone, "CURSOR_OAUTH_SESSION_CANCELLED", "Cursor authorization was cancelled")
	}

	payload, err := decodeCursorSessionPayload(session)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CURSOR_OAUTH_SESSION_INVALID", "decode session: %v", err)
	}

	if now.UnixMilli() >= session.ExpiresAtUnixMilli {
		_, _ = s.deviceSessionStore.Cancel(ctx, session.ID, 5*time.Minute)
		return nil, infraerrors.New(http.StatusGone, "CURSOR_OAUTH_SESSION_EXPIRED", "Cursor authorization expired")
	}

	if session.Status != ProviderOAuthSessionPending {
		return cursorAuthorizationResult(session, payload, now), nil
	}

	if !lease.Held {
		if session.NextPollAtUnixMilli <= now.UnixMilli() {
			session.NextPollAtUnixMilli = now.Add(time.Second).UnixMilli()
		}
		return cursorAuthorizationResult(session, payload, now), nil
	}

	httpClient, err := s.createHTTPClient(payload.ProxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CURSOR_OAUTH_HTTP_CLIENT_FAILED", "create http client: %v", err)
	}

	tokenResp, ready, err := cursor.PollAuthStatus(ctx, httpClient, s.backendURL, payload.UUID, payload.Verifier)
	session.NextPollAtUnixMilli = now.Add(time.Duration(payload.IntervalSeconds) * time.Second).UnixMilli()

	if err != nil {
		_, _ = s.deviceSessionStore.CommitPoll(ctx, lease, session)
		return nil, err
	}

	if ready && tokenResp != nil {
		jwtPayload, _ := cursor.ParseJWTPayload(tokenResp.AccessToken)
		email := ""
		userID := ""
		expiresAt := now.Add(60 * 24 * time.Hour).Unix()
		if jwtPayload != nil {
			email = jwtPayload.Email
			userID = jwtPayload.Sub
			if jwtPayload.Exp > 0 {
				expiresAt = jwtPayload.Exp
			}
		}

		info := &CursorOAuthTokenInfo{
			AccessToken:  tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			MachineID:    payload.MachineID,
			Email:        email,
			UserID:       userID,
			ExpiresAt:    expiresAt,
			ProxyID:      payload.ProxyID,
		}
		payload.Token = info
		session.Status = ProviderOAuthSessionAuthorized
	}

	session.Payload, err = json.Marshal(payload)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CURSOR_OAUTH_SESSION_FAILED", "encode session: %v", err)
	}

	committed, err := s.deviceSessionStore.CommitPoll(ctx, lease, session)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "CURSOR_OAUTH_SESSION_STORE_FAILED", "update session: %v", err)
	}
	if !committed {
		return nil, infraerrors.New(http.StatusGone, "CURSOR_OAUTH_SESSION_CHANGED", "Cursor authorization was cancelled or superseded")
	}

	return cursorAuthorizationResult(session, payload, now), nil
}

func (s *CursorOAuthService) ConsumeAuthorization(ctx context.Context, sessionID string) (*CursorOAuthTokenInfo, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.deviceSessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "CURSOR_OAUTH_NOT_CONFIGURED", "Cursor OAuth is not configured")
	}

	session, err := s.deviceSessionStore.ConsumeAuthorized(ctx, sessionID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "CURSOR_OAUTH_SESSION_STORE_FAILED", "consume session: %v", err)
	}
	if session == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_SESSION_NOT_FOUND", "Cursor authorization session not found, incomplete, expired, or already consumed")
	}

	payload, err := decodeCursorSessionPayload(session)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CURSOR_OAUTH_SESSION_INVALID", "decode session: %v", err)
	}
	if session.Status != ProviderOAuthSessionAuthorized || payload.Token == nil || strings.TrimSpace(payload.Token.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusConflict, "CURSOR_OAUTH_NOT_AUTHORIZED", "Cursor authorization is not complete")
	}

	return payload.Token, nil
}

func (s *CursorOAuthService) CancelAuthorization(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.deviceSessionStore == nil {
		return infraerrors.New(http.StatusServiceUnavailable, "CURSOR_OAUTH_NOT_CONFIGURED", "Cursor OAuth is not configured")
	}
	if _, err := s.deviceSessionStore.Cancel(ctx, sessionID, 5*time.Minute); err != nil {
		return infraerrors.Newf(http.StatusServiceUnavailable, "CURSOR_OAUTH_SESSION_STORE_FAILED", "cancel session: %v", err)
	}
	return nil
}

func (s *CursorOAuthService) ExchangeUserAPIKey(ctx context.Context, apiKey string, proxyID *int64) (*CursorOAuthTokenInfo, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CURSOR_API_KEY_REQUIRED", "api_key is required")
	}

	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}

	httpClient, err := s.createHTTPClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CURSOR_OAUTH_HTTP_CLIENT_FAILED", "create http client: %v", err)
	}

	tokenResp, err := cursor.ExchangeUserAPIKey(ctx, httpClient, s.backendURL, apiKey)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadRequest, "CURSOR_API_KEY_EXCHANGE_FAILED", "%v", err)
	}

	jwtPayload, _ := cursor.ParseJWTPayload(tokenResp.AccessToken)
	email := ""
	userID := ""
	expiresAt := s.now().Add(60 * 24 * time.Hour).Unix()
	if jwtPayload != nil {
		email = jwtPayload.Email
		userID = jwtPayload.Sub
		if jwtPayload.Exp > 0 {
			expiresAt = jwtPayload.Exp
		}
	}

	return &CursorOAuthTokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		MachineID:    uuid.NewString(),
		Email:        email,
		UserID:       userID,
		ExpiresAt:    expiresAt,
		ProxyID:      proxyID,
	}, nil
}

func (s *CursorOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*CursorOAuthTokenInfo, error) {
	if account == nil || account.Platform != PlatformCursor || account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_INVALID_ACCOUNT", "account is not a Cursor OAuth account")
	}
	refreshToken := strings.TrimSpace(account.GetCredential("refresh_token"))
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(account.GetCredential("access_token"))
	}
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CURSOR_OAUTH_NO_REFRESH_TOKEN", "Cursor refresh token is missing")
	}

	proxyURL, err := s.proxyURL(ctx, account.ProxyID)
	if err != nil {
		return nil, err
	}

	httpClient, err := s.createHTTPClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CURSOR_OAUTH_HTTP_CLIENT_FAILED", "create http client: %v", err)
	}

	tokenResp, err := cursor.RefreshToken(ctx, httpClient, s.backendURL, refreshToken)
	if err != nil {
		return nil, err
	}
	if tokenResp.ShouldLogout {
		return nil, errors.New("cursor token refresh indicated account should logout")
	}

	jwtPayload, _ := cursor.ParseJWTPayload(tokenResp.AccessToken)
	email := account.GetCredential("email")
	userID := account.GetCredential("user_id")
	expiresAt := s.now().Add(60 * 24 * time.Hour).Unix()
	if jwtPayload != nil {
		if jwtPayload.Email != "" {
			email = jwtPayload.Email
		}
		if jwtPayload.Sub != "" {
			userID = jwtPayload.Sub
		}
		if jwtPayload.Exp > 0 {
			expiresAt = jwtPayload.Exp
		}
	}

	machineID := account.GetCredential("machine_id")
	if machineID == "" {
		machineID = uuid.NewString()
	}

	return &CursorOAuthTokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		MachineID:    machineID,
		Email:        email,
		UserID:       userID,
		ExpiresAt:    expiresAt,
		ProxyID:      account.ProxyID,
	}, nil
}

func (s *CursorOAuthService) BuildAccountCredentials(tokenInfo *CursorOAuthTokenInfo) map[string]any {
	if tokenInfo == nil {
		return map[string]any{}
	}
	creds := map[string]any{
		"access_token":  tokenInfo.AccessToken,
		"refresh_token": tokenInfo.RefreshToken,
		"token_type":    "Bearer",
	}
	if tokenInfo.MachineID != "" {
		creds["machine_id"] = tokenInfo.MachineID
	}
	if tokenInfo.Email != "" {
		creds["email"] = tokenInfo.Email
	}
	if tokenInfo.UserID != "" {
		creds["user_id"] = tokenInfo.UserID
	}
	if tokenInfo.ExpiresAt > 0 {
		creds["expires_at"] = time.Unix(tokenInfo.ExpiresAt, 0).Format(time.RFC3339)
	}
	return creds
}

func decodeCursorSessionPayload(session *ProviderOAuthSession) (*cursorSessionPayload, error) {
	if session == nil || len(session.Payload) == 0 {
		return nil, errors.New("empty session payload")
	}
	var payload cursorSessionPayload
	if err := json.Unmarshal(session.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal session payload: %w", err)
	}
	return &payload, nil
}

func cursorAuthorizationResult(session *ProviderOAuthSession, payload *cursorSessionPayload, now time.Time) *CursorAuthorizationResult {
	if session == nil || payload == nil {
		return nil
	}
	result := &CursorAuthorizationResult{
		SessionID: session.ID,
		Status:    session.Status,
		LoginURL:  payload.LoginURL,
		ExpiresIn: (session.ExpiresAtUnixMilli - now.UnixMilli()) / 1000,
		Interval:  payload.IntervalSeconds,
		Error:     session.Error,
	}
	if result.ExpiresIn < 0 {
		result.ExpiresIn = 0
	}
	if retry := session.NextPollAtUnixMilli - now.UnixMilli(); retry > 0 && session.Status == ProviderOAuthSessionPending {
		result.RetryAfter = (retry + 999) / 1000
	}
	return result
}
