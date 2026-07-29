package service

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const grokDefaultAccessTokenTTL = 6 * time.Hour

type GrokOAuthService struct {
	sessionStore       *xai.SessionStore
	deviceSessionStore ProviderOAuthSessionStore
	proxyRepo          ProxyRepository
	oauthClient        GrokOAuthClient
	now                func() time.Time
}

func NewGrokOAuthService(proxyRepo ProxyRepository, oauthClient GrokOAuthClient, deviceStores ...ProviderOAuthSessionStore) *GrokOAuthService {
	service := &GrokOAuthService{
		sessionStore: xai.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
		now:          time.Now,
	}
	if len(deviceStores) > 0 {
		service.deviceSessionStore = deviceStores[0]
	}
	return service
}

func ProvideGrokOAuthService(proxyRepo ProxyRepository, oauthClient GrokOAuthClient, deviceStore ProviderOAuthSessionStore) *GrokOAuthService {
	return NewGrokOAuthService(proxyRepo, oauthClient, deviceStore)
}

type GrokAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

func (s *GrokOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI string) (*GrokAuthURLResult, error) {
	state, err := xai.GenerateState()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_OAUTH_STATE_FAILED", "failed to generate state: %v", err)
	}
	nonce, err := xai.GenerateNonce()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_OAUTH_NONCE_FAILED", "failed to generate nonce: %v", err)
	}
	codeVerifier, err := xai.GenerateCodeVerifier()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_OAUTH_VERIFIER_FAILED", "failed to generate code verifier: %v", err)
	}
	sessionID, err := xai.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
	}

	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	redirectURI = xai.EffectiveRedirectURI(redirectURI)
	codeChallenge := xai.GenerateCodeChallenge(codeVerifier)

	authURL, err := xai.BuildAuthorizationURL(state, codeChallenge, redirectURI, nonce)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadRequest, "GROK_OAUTH_INVALID_AUTHORIZE_URL", "%v", err)
	}

	s.sessionStore.Set(sessionID, &xai.OAuthSession{
		State:         state,
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
		ClientID:      xai.EffectiveClientID(),
		Scope:         xai.EffectiveScope(),
		ProxyURL:      proxyURL,
		RedirectURI:   redirectURI,
		CreatedAt:     time.Now(),
	})

	return &GrokAuthURLResult{
		AuthURL:   authURL,
		SessionID: sessionID,
		State:     state,
	}, nil
}

type GrokExchangeCodeInput struct {
	SessionID   string
	Code        string
	State       string
	RedirectURI string
	ProxyID     *int64
}

type GrokTokenInfo struct {
	AccessToken       string `json:"access_token"`
	RefreshToken      string `json:"refresh_token,omitempty"`
	IDToken           string `json:"id_token,omitempty"`
	TokenType         string `json:"token_type,omitempty"`
	ExpiresIn         int64  `json:"expires_in"`
	ExpiresAt         int64  `json:"expires_at"`
	ClientID          string `json:"client_id,omitempty"`
	Scope             string `json:"scope,omitempty"`
	Email             string `json:"email,omitempty"`
	SubscriptionTier  string `json:"subscription_tier,omitempty"`
	EntitlementStatus string `json:"entitlement_status,omitempty"`
	TokenEndpoint     string `json:"token_endpoint,omitempty"`
}

type GrokDeviceAuthorizationResult struct {
	SessionID               string `json:"session_id"`
	Status                  string `json:"status"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	UserCode                string `json:"user_code"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval"`
	RetryAfter              int64  `json:"retry_after,omitempty"`
	Error                   string `json:"error,omitempty"`
}

type grokDeviceSessionPayload struct {
	DeviceCode              string         `json:"device_code"`
	UserCode                string         `json:"user_code"`
	VerificationURI         string         `json:"verification_uri"`
	VerificationURIComplete string         `json:"verification_uri_complete"`
	ProxyURL                string         `json:"proxy_url,omitempty"`
	ProxyID                 *int64         `json:"proxy_id,omitempty"`
	ClientID                string         `json:"client_id"`
	Scope                   string         `json:"scope"`
	TokenEndpoint           string         `json:"token_endpoint"`
	IntervalSeconds         int64          `json:"interval_seconds"`
	Token                   *GrokTokenInfo `json:"token,omitempty"`
}

type GrokDeviceAuthorizationCredential struct {
	Token   *GrokTokenInfo
	ProxyID *int64
}

const (
	grokDeviceDefaultInterval = 5 * time.Second
	grokDeviceDefaultExpiry   = 30 * time.Minute
)

func (s *GrokOAuthService) StartDeviceAuthorization(ctx context.Context, proxyID *int64) (*GrokDeviceAuthorizationResult, error) {
	if s == nil || s.oauthClient == nil || s.deviceSessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "GROK_DEVICE_OAUTH_NOT_CONFIGURED", "Grok device authorization is not configured")
	}
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	clientID := xai.EffectiveClientID()
	scope := xai.EffectiveScope()
	device, err := s.oauthClient.StartDeviceFlow(ctx, proxyURL, clientID, scope)
	if err != nil {
		return nil, err
	}
	if device == nil || strings.TrimSpace(device.DeviceCode) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_DEVICE_OAUTH_INVALID_DEVICE", "xAI returned an incomplete device authorization response")
	}
	interval := time.Duration(device.Interval) * time.Second
	if interval < grokDeviceDefaultInterval {
		interval = grokDeviceDefaultInterval
	}
	expiresIn := time.Duration(device.ExpiresIn) * time.Second
	if expiresIn <= 0 || expiresIn > grokDeviceDefaultExpiry {
		expiresIn = grokDeviceDefaultExpiry
	}
	sessionID, err := xai.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_DEVICE_OAUTH_SESSION_FAILED", "generate session: %v", err)
	}
	payload := &grokDeviceSessionPayload{
		DeviceCode: device.DeviceCode, UserCode: device.UserCode,
		VerificationURI: device.VerificationURI, VerificationURIComplete: device.VerificationURIComplete,
		ProxyURL: proxyURL, ProxyID: proxyID, ClientID: clientID, Scope: scope,
		TokenEndpoint: device.TokenEndpoint, IntervalSeconds: int64(interval.Seconds()),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_DEVICE_OAUTH_SESSION_FAILED", "encode session: %v", err)
	}
	now := s.now()
	session := &ProviderOAuthSession{
		ID: sessionID, Provider: PlatformGrok, Flow: ProviderOAuthFlowDeviceCode,
		Status: ProviderOAuthSessionPending, NextPollAtUnixMilli: now.UnixMilli(),
		ExpiresAtUnixMilli: now.Add(expiresIn).UnixMilli(), Payload: encoded,
	}
	if err := s.deviceSessionStore.Create(ctx, session, expiresIn); err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "GROK_DEVICE_OAUTH_SESSION_STORE_FAILED", "store session: %v", err)
	}
	return grokDeviceAuthorizationResult(session, payload, now), nil
}

func (s *GrokOAuthService) GetDeviceAuthorizationStatus(ctx context.Context, sessionID string) (*GrokDeviceAuthorizationResult, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_DEVICE_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.deviceSessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "GROK_DEVICE_OAUTH_NOT_CONFIGURED", "Grok device authorization is not configured")
	}
	now := s.now()
	lease, err := s.deviceSessionStore.AcquirePollLease(ctx, sessionID, now, 30*time.Second)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "GROK_DEVICE_OAUTH_SESSION_STORE_FAILED", "acquire poll lease: %v", err)
	}
	if lease == nil || lease.Session == nil || lease.Session.Provider != PlatformGrok || lease.Session.Flow != ProviderOAuthFlowDeviceCode {
		return nil, infraerrors.New(http.StatusNotFound, "GROK_DEVICE_OAUTH_SESSION_NOT_FOUND", "Grok device authorization session not found or expired")
	}
	session := lease.Session
	if session.Status == ProviderOAuthSessionCancelled {
		return nil, infraerrors.New(http.StatusGone, "GROK_DEVICE_OAUTH_SESSION_CANCELLED", "Grok device authorization was cancelled")
	}
	payload, err := decodeGrokDeviceSessionPayload(session)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_DEVICE_OAUTH_SESSION_INVALID", "decode session: %v", err)
	}
	if now.UnixMilli() >= session.ExpiresAtUnixMilli {
		_, _ = s.deviceSessionStore.Cancel(ctx, session.ID, 5*time.Minute)
		return nil, infraerrors.New(http.StatusGone, "GROK_DEVICE_OAUTH_SESSION_EXPIRED", "Grok device authorization expired")
	}
	if session.Status != ProviderOAuthSessionPending {
		return grokDeviceAuthorizationResult(session, payload, now), nil
	}
	if !lease.Held {
		if session.NextPollAtUnixMilli <= now.UnixMilli() {
			session.NextPollAtUnixMilli = now.Add(time.Second).UnixMilli()
		}
		return grokDeviceAuthorizationResult(session, payload, now), nil
	}

	token, err := s.oauthClient.PollDeviceToken(ctx, payload.DeviceCode, payload.TokenEndpoint, payload.ProxyURL, payload.ClientID)
	if err != nil {
		session.NextPollAtUnixMilli = now.Add(time.Duration(payload.IntervalSeconds) * time.Second).UnixMilli()
		_, _ = s.deviceSessionStore.CommitPoll(ctx, lease, session)
		return nil, err
	}
	if token == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_DEVICE_OAUTH_INVALID_TOKEN", "xAI returned an empty device token response")
	}
	session.NextPollAtUnixMilli = now.Add(time.Duration(payload.IntervalSeconds) * time.Second).UnixMilli()
	switch strings.TrimSpace(token.Error) {
	case "authorization_pending":
	case "slow_down":
		payload.IntervalSeconds += int64(grokDeviceDefaultInterval.Seconds())
		session.NextPollAtUnixMilli = now.Add(time.Duration(payload.IntervalSeconds) * time.Second).UnixMilli()
	case "expired_token":
		_, _ = s.deviceSessionStore.Cancel(ctx, session.ID, 5*time.Minute)
		return nil, infraerrors.New(http.StatusGone, "GROK_DEVICE_OAUTH_DEVICE_EXPIRED", "Grok device code expired")
	case "access_denied":
		session.Status = ProviderOAuthSessionDenied
		session.Error = "authorization denied"
	case "":
		info := s.tokenInfoFromResponse(&token.TokenResponse, payload.ClientID, nil)
		if info == nil || strings.TrimSpace(info.AccessToken) == "" {
			return nil, infraerrors.New(http.StatusBadGateway, "GROK_DEVICE_OAUTH_INVALID_TOKEN", "xAI returned an empty access token")
		}
		info.TokenEndpoint = payload.TokenEndpoint
		payload.Token = info
		session.Status = ProviderOAuthSessionAuthorized
	default:
		session.NextPollAtUnixMilli = now.Add(time.Duration(payload.IntervalSeconds) * time.Second).UnixMilli()
		_, _ = s.deviceSessionStore.CommitPoll(ctx, lease, session)
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_DEVICE_OAUTH_UPSTREAM_ERROR", "Grok authorization failed: %s", token.Error)
	}
	session.Payload, err = json.Marshal(payload)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_DEVICE_OAUTH_SESSION_FAILED", "encode session: %v", err)
	}
	committed, err := s.deviceSessionStore.CommitPoll(ctx, lease, session)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "GROK_DEVICE_OAUTH_SESSION_STORE_FAILED", "update session: %v", err)
	}
	if !committed {
		return nil, infraerrors.New(http.StatusGone, "GROK_DEVICE_OAUTH_SESSION_CHANGED", "Grok device authorization was cancelled or superseded")
	}
	return grokDeviceAuthorizationResult(session, payload, now), nil
}

func (s *GrokOAuthService) CancelDeviceAuthorization(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return infraerrors.New(http.StatusBadRequest, "GROK_DEVICE_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.deviceSessionStore == nil {
		return infraerrors.New(http.StatusServiceUnavailable, "GROK_DEVICE_OAUTH_NOT_CONFIGURED", "Grok device authorization is not configured")
	}
	if _, err := s.deviceSessionStore.Cancel(ctx, sessionID, 5*time.Minute); err != nil {
		return infraerrors.Newf(http.StatusServiceUnavailable, "GROK_DEVICE_OAUTH_SESSION_STORE_FAILED", "cancel device session: %v", err)
	}
	return nil
}

func (s *GrokOAuthService) ConsumeDeviceAuthorization(ctx context.Context, sessionID string) (*GrokDeviceAuthorizationCredential, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_DEVICE_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.deviceSessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "GROK_DEVICE_OAUTH_NOT_CONFIGURED", "Grok device authorization is not configured")
	}
	session, err := s.deviceSessionStore.ConsumeAuthorized(ctx, sessionID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "GROK_DEVICE_OAUTH_SESSION_STORE_FAILED", "consume device session: %v", err)
	}
	if session == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_DEVICE_OAUTH_SESSION_NOT_FOUND", "Grok authorization session not found, incomplete, expired, or already consumed")
	}
	payload, err := decodeGrokDeviceSessionPayload(session)
	if err != nil || payload.Token == nil || strings.TrimSpace(payload.Token.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusConflict, "GROK_DEVICE_OAUTH_NOT_AUTHORIZED", "Grok device authorization is not complete")
	}
	return &GrokDeviceAuthorizationCredential{Token: payload.Token, ProxyID: payload.ProxyID}, nil
}

func (s *GrokOAuthService) ExchangeCode(ctx context.Context, input *GrokExchangeCodeInput) (*GrokTokenInfo, error) {
	if input == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_INVALID_INPUT", "input is required")
	}
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}
	defer s.sessionStore.Delete(input.SessionID)

	parsed := xai.ParseAuthorizationInput(input.Code)
	code := strings.TrimSpace(parsed.Code)
	if code == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_CODE_REQUIRED", "authorization code is required")
	}
	state := strings.TrimSpace(input.State)
	if state == "" {
		state = strings.TrimSpace(parsed.State)
	}
	if parsed.RequiresState && state == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_STATE_REQUIRED", "oauth state is required for callback URLs")
	}
	if state != "" && subtle.ConstantTimeCompare([]byte(state), []byte(session.State)) != 1 {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_INVALID_STATE", "invalid oauth state")
	}

	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		var err error
		proxyURL, err = s.proxyURL(ctx, input.ProxyID)
		if err != nil {
			return nil, err
		}
	}
	redirectURI := session.RedirectURI
	if strings.TrimSpace(input.RedirectURI) != "" {
		redirectURI = input.RedirectURI
	}

	tokenResp, err := s.oauthClient.ExchangeCode(ctx, code, session.CodeVerifier, redirectURI, proxyURL, session.ClientID)
	if err != nil {
		return nil, err
	}
	tokenInfo := s.tokenInfoFromResponse(tokenResp, session.ClientID, nil)
	if tokenInfo == nil || strings.TrimSpace(tokenInfo.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_INVALID_TOKEN_RESPONSE", "xAI returned an empty access token")
	}
	return tokenInfo, nil
}

func (s *GrokOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL, clientID string, tokenEndpoints ...string) (*GrokTokenInfo, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_NO_REFRESH_TOKEN", "refresh_token is required")
	}
	tokenEndpoint := ""
	if len(tokenEndpoints) > 0 {
		tokenEndpoint = strings.TrimSpace(tokenEndpoints[0])
	}
	var tokenResp *xai.TokenResponse
	var err error
	if tokenEndpoint != "" {
		tokenResp, err = s.oauthClient.RefreshTokenAtEndpoint(ctx, refreshToken, tokenEndpoint, proxyURL, clientID)
	} else {
		tokenResp, err = s.oauthClient.RefreshToken(ctx, refreshToken, proxyURL, clientID)
	}
	if err != nil {
		return nil, err
	}
	tokenInfo := s.tokenInfoFromResponse(tokenResp, clientID, nil)
	if tokenInfo == nil || strings.TrimSpace(tokenInfo.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_INVALID_TOKEN_RESPONSE", "xAI returned an empty access token")
	}
	tokenInfo.TokenEndpoint = tokenEndpoint
	if tokenInfo.RefreshToken == "" {
		tokenInfo.RefreshToken = refreshToken
	}
	return tokenInfo, nil
}

func (s *GrokOAuthService) ValidateRefreshToken(ctx context.Context, refreshToken string, proxyID *int64) (*GrokTokenInfo, error) {
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	return s.RefreshToken(ctx, refreshToken, proxyURL, xai.EffectiveClientID())
}

func (s *GrokOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*GrokTokenInfo, error) {
	if account == nil || account.Platform != PlatformGrok {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_INVALID_ACCOUNT", "account is not a Grok account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_INVALID_ACCOUNT_TYPE", "account is not an OAuth account")
	}

	proxyURL, err := s.proxyURL(ctx, account.ProxyID)
	if err != nil {
		return nil, err
	}
	refreshToken := account.GetCredential("refresh_token")
	if strings.TrimSpace(refreshToken) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
	}

	clientID := account.GetCredential("client_id")
	tokenInfo, err := s.RefreshToken(ctx, refreshToken, proxyURL, clientID, account.GetCredential("token_endpoint"))
	if err != nil {
		return nil, err
	}
	tokenInfo.SubscriptionTier = account.GetCredential("subscription_tier")
	tokenInfo.EntitlementStatus = account.GetCredential("entitlement_status")
	return tokenInfo, nil
}

func (s *GrokOAuthService) BuildAccountCredentials(tokenInfo *GrokTokenInfo) map[string]any {
	if tokenInfo == nil {
		return nil
	}
	expiresAt := time.Unix(tokenInfo.ExpiresAt, 0).UTC().Format(time.RFC3339)
	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
		"expires_at":   expiresAt,
	}
	if tokenInfo.RefreshToken != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
	}
	if tokenInfo.TokenType != "" {
		creds["token_type"] = tokenInfo.TokenType
	}
	if tokenInfo.IDToken != "" {
		creds["id_token"] = tokenInfo.IDToken
	}
	if tokenInfo.ClientID != "" {
		creds["client_id"] = tokenInfo.ClientID
	}
	if tokenInfo.Scope != "" {
		creds["scope"] = tokenInfo.Scope
	}
	if tokenInfo.Email != "" {
		creds["email"] = tokenInfo.Email
	}
	if tokenInfo.SubscriptionTier != "" {
		creds["subscription_tier"] = tokenInfo.SubscriptionTier
	}
	if tokenInfo.EntitlementStatus != "" {
		creds["entitlement_status"] = tokenInfo.EntitlementStatus
	}
	if tokenInfo.TokenEndpoint != "" {
		creds["token_endpoint"] = tokenInfo.TokenEndpoint
	}
	creds["base_url"] = xai.DefaultBaseURL
	return creds
}

func (s *GrokOAuthService) Stop() {
	s.sessionStore.Stop()
}

func (s *GrokOAuthService) tokenInfoFromResponse(tokenResp *xai.TokenResponse, clientID string, existing map[string]any) *GrokTokenInfo {
	if tokenResp == nil {
		return nil
	}
	now := time.Now()
	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int64(grokDefaultAccessTokenTTL.Seconds())
	}
	info := &GrokTokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    expiresIn,
		ExpiresAt:    now.Add(time.Duration(expiresIn) * time.Second).Unix(),
		ClientID:     strings.TrimSpace(clientID),
		Scope:        tokenResp.Scope,
	}
	if info.ClientID == "" {
		info.ClientID = xai.EffectiveClientID()
	}
	if info.TokenType == "" {
		info.TokenType = "Bearer"
	}
	if email := parseJWTEmailClaim(tokenResp.IDToken); email != "" {
		info.Email = email
	}
	if info.Email == "" && existing != nil {
		if email, _ := existing["email"].(string); email != "" {
			info.Email = email
		}
	}
	return info
}

func (s *GrokOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "GROK_OAUTH_PROXY_NOT_AVAILABLE", "proxy repository is not available")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "GROK_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}

func parseJWTEmailClaim(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return strings.TrimSpace(claims.Email)
}

func decodeGrokDeviceSessionPayload(session *ProviderOAuthSession) (*grokDeviceSessionPayload, error) {
	if session == nil || len(session.Payload) == 0 {
		return nil, errors.New("Grok device session payload is missing")
	}
	var payload grokDeviceSessionPayload
	if err := json.Unmarshal(session.Payload, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func grokDeviceAuthorizationResult(session *ProviderOAuthSession, payload *grokDeviceSessionPayload, now time.Time) *GrokDeviceAuthorizationResult {
	if session == nil || payload == nil {
		return nil
	}
	result := &GrokDeviceAuthorizationResult{
		SessionID: session.ID, Status: session.Status,
		VerificationURI: payload.VerificationURI, VerificationURIComplete: payload.VerificationURIComplete,
		UserCode: payload.UserCode, Interval: payload.IntervalSeconds, Error: session.Error,
	}
	if remaining := session.ExpiresAtUnixMilli - now.UnixMilli(); remaining > 0 {
		result.ExpiresIn = (remaining + int64(time.Second) - 1) / int64(time.Second)
	}
	if retry := session.NextPollAtUnixMilli - now.UnixMilli(); retry > 0 && session.Status == ProviderOAuthSessionPending {
		result.RetryAfter = (retry + int64(time.Second) - 1) / int64(time.Second)
	}
	return result
}
