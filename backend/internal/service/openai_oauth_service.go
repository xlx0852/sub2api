package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// OpenAIOAuthService handles OpenAI OAuth authentication flows
type OpenAIOAuthService struct {
	sessionStore         *openai.SessionStore
	deviceSessionStore   ProviderOAuthSessionStore
	proxyRepo            ProxyRepository
	oauthClient          OpenAIOAuthClient
	privacyClientFactory PrivacyClientFactory // 用于调用 chatgpt.com/backend-api（ImpersonateChrome）
	now                  func() time.Time
}

// NewOpenAIOAuthService creates a new OpenAI OAuth service
func NewOpenAIOAuthService(proxyRepo ProxyRepository, oauthClient OpenAIOAuthClient, deviceStores ...ProviderOAuthSessionStore) *OpenAIOAuthService {
	service := &OpenAIOAuthService{
		sessionStore: openai.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
		now:          time.Now,
	}
	if len(deviceStores) > 0 {
		service.deviceSessionStore = deviceStores[0]
	}
	return service
}

// SetPrivacyClientFactory 注入 ImpersonateChrome 客户端工厂，
// 用于调用 chatgpt.com/backend-api 获取账号信息（plan_type 等）。
func (s *OpenAIOAuthService) SetPrivacyClientFactory(factory PrivacyClientFactory) {
	s.privacyClientFactory = factory
}

// OpenAIAuthURLResult contains the authorization URL and session info
type OpenAIAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
}

// GenerateAuthURL generates an OpenAI OAuth authorization URL
func (s *OpenAIOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI, platform string) (*OpenAIAuthURLResult, error) {
	// Generate PKCE values
	state, err := openai.GenerateState()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_STATE_FAILED", "failed to generate state: %v", err)
	}

	codeVerifier, err := openai.GenerateCodeVerifier()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_VERIFIER_FAILED", "failed to generate code verifier: %v", err)
	}

	codeChallenge := openai.GenerateCodeChallenge(codeVerifier)

	// Generate session ID
	sessionID, err := openai.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
	}

	// Get proxy URL if specified
	var proxyURL string
	if proxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
		}
		if proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	// Use default redirect URI if not specified
	if redirectURI == "" {
		redirectURI = openai.DefaultRedirectURI
	}
	normalizedPlatform := normalizeOpenAIOAuthPlatform(platform)
	clientID, _ := openai.OAuthClientConfigByPlatform(normalizedPlatform)

	// Store session
	session := &openai.OAuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		ClientID:     clientID,
		RedirectURI:  redirectURI,
		ProxyURL:     proxyURL,
		CreatedAt:    time.Now(),
	}
	s.sessionStore.Set(sessionID, session)

	// Build authorization URL
	authURL := openai.BuildAuthorizationURLForPlatform(state, codeChallenge, redirectURI, normalizedPlatform)

	return &OpenAIAuthURLResult{
		AuthURL:   authURL,
		SessionID: sessionID,
	}, nil
}

// OpenAIExchangeCodeInput represents the input for code exchange
type OpenAIExchangeCodeInput struct {
	SessionID   string
	Code        string
	State       string
	RedirectURI string
	ProxyID     *int64
}

// OpenAITokenInfo represents the token information for OpenAI
type OpenAITokenInfo struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	IDToken               string `json:"id_token,omitempty"`
	ExpiresIn             int64  `json:"expires_in"`
	ExpiresAt             int64  `json:"expires_at"`
	ClientID              string `json:"client_id,omitempty"`
	AuthMode              string `json:"auth_mode,omitempty"`
	Email                 string `json:"email,omitempty"`
	ChatGPTAccountID      string `json:"chatgpt_account_id,omitempty"`
	ChatGPTUserID         string `json:"chatgpt_user_id,omitempty"`
	ChatGPTAccountFedRAMP bool   `json:"chatgpt_account_is_fedramp,omitempty"`
	OrganizationID        string `json:"organization_id,omitempty"`
	PlanType              string `json:"plan_type,omitempty"`
	SubscriptionExpiresAt string `json:"subscription_expires_at,omitempty"`
	PrivacyMode           string `json:"privacy_mode,omitempty"`
}

type OpenAIDeviceAuthorizationResult struct {
	SessionID       string `json:"session_id"`
	Status          string `json:"status"`
	VerificationURI string `json:"verification_uri"`
	UserCode        string `json:"user_code"`
	ExpiresIn       int64  `json:"expires_in"`
	Interval        int64  `json:"interval"`
	RetryAfter      int64  `json:"retry_after,omitempty"`
	Error           string `json:"error,omitempty"`
}

type openAIDeviceSessionPayload struct {
	DeviceAuthID    string           `json:"device_auth_id"`
	UserCode        string           `json:"user_code"`
	ProxyURL        string           `json:"proxy_url,omitempty"`
	ProxyID         *int64           `json:"proxy_id,omitempty"`
	ClientID        string           `json:"client_id"`
	IntervalSeconds int64            `json:"interval_seconds"`
	Token           *OpenAITokenInfo `json:"token,omitempty"`
}

type OpenAIDeviceAuthorizationCredential struct {
	Token   *OpenAITokenInfo
	ProxyID *int64
}

const (
	openAIDeviceDefaultInterval = 5 * time.Second
	openAIDeviceSessionTTL      = 15 * time.Minute
)

func (s *OpenAIOAuthService) StartDeviceAuthorization(ctx context.Context, proxyID *int64) (*OpenAIDeviceAuthorizationResult, error) {
	deviceClient, ok := s.oauthClient.(OpenAIDeviceOAuthClient)
	if !ok || s.deviceSessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_DEVICE_OAUTH_NOT_CONFIGURED", "OpenAI device authorization is not configured")
	}
	proxyURL, err := s.deviceProxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	clientID := openai.ClientID
	device, err := deviceClient.RequestDeviceCode(ctx, proxyURL, clientID)
	if err != nil {
		return nil, err
	}
	if device == nil || strings.TrimSpace(device.DeviceAuthID) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_INVALID_RESPONSE", "OpenAI returned an incomplete device code response")
	}
	userCode := strings.TrimSpace(device.UserCode)
	if userCode == "" {
		userCode = strings.TrimSpace(device.UserCodeAlt)
	}
	interval := parseOpenAIDevicePollInterval(device.Interval)
	sessionID, err := openai.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_DEVICE_OAUTH_SESSION_FAILED", "generate session: %v", err)
	}
	payload := &openAIDeviceSessionPayload{
		DeviceAuthID: device.DeviceAuthID, UserCode: userCode, ProxyURL: proxyURL,
		ProxyID: proxyID, ClientID: clientID, IntervalSeconds: int64(interval.Seconds()),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_DEVICE_OAUTH_SESSION_FAILED", "encode session: %v", err)
	}
	now := s.now()
	session := &ProviderOAuthSession{
		ID: sessionID, Provider: PlatformOpenAI, Flow: ProviderOAuthFlowDeviceCode,
		Status: ProviderOAuthSessionPending, NextPollAtUnixMilli: now.UnixMilli(),
		ExpiresAtUnixMilli: now.Add(openAIDeviceSessionTTL).UnixMilli(), Payload: encoded,
	}
	if err := s.deviceSessionStore.Create(ctx, session, openAIDeviceSessionTTL); err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "OPENAI_DEVICE_OAUTH_SESSION_STORE_FAILED", "store session: %v", err)
	}
	return openAIDeviceAuthorizationResult(session, payload, now), nil
}

func (s *OpenAIOAuthService) GetDeviceAuthorizationStatus(ctx context.Context, sessionID string) (*OpenAIDeviceAuthorizationResult, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_DEVICE_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.deviceSessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_DEVICE_OAUTH_NOT_CONFIGURED", "OpenAI device authorization is not configured")
	}
	deviceClient, ok := s.oauthClient.(OpenAIDeviceOAuthClient)
	if !ok {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_DEVICE_OAUTH_NOT_CONFIGURED", "OpenAI device authorization is not configured")
	}
	now := s.now()
	lease, err := s.deviceSessionStore.AcquirePollLease(ctx, sessionID, now, 30*time.Second)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "OPENAI_DEVICE_OAUTH_SESSION_STORE_FAILED", "acquire poll lease: %v", err)
	}
	if lease == nil || lease.Session == nil || lease.Session.Provider != PlatformOpenAI || lease.Session.Flow != ProviderOAuthFlowDeviceCode {
		return nil, infraerrors.New(http.StatusNotFound, "OPENAI_DEVICE_OAUTH_SESSION_NOT_FOUND", "OpenAI device authorization session not found or expired")
	}
	session := lease.Session
	if session.Status == ProviderOAuthSessionCancelled {
		return nil, infraerrors.New(http.StatusGone, "OPENAI_DEVICE_OAUTH_SESSION_CANCELLED", "OpenAI device authorization was cancelled")
	}
	payload, err := decodeOpenAIDeviceSessionPayload(session)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_SESSION_INVALID", "decode session: %v", err)
	}
	if now.UnixMilli() >= session.ExpiresAtUnixMilli {
		_, _ = s.deviceSessionStore.Cancel(ctx, session.ID, 5*time.Minute)
		return nil, infraerrors.New(http.StatusGone, "OPENAI_DEVICE_OAUTH_SESSION_EXPIRED", "OpenAI device authorization expired")
	}
	if session.Status != ProviderOAuthSessionPending {
		return openAIDeviceAuthorizationResult(session, payload, now), nil
	}
	if !lease.Held {
		if session.NextPollAtUnixMilli <= now.UnixMilli() {
			session.NextPollAtUnixMilli = now.Add(time.Second).UnixMilli()
		}
		return openAIDeviceAuthorizationResult(session, payload, now), nil
	}

	authorization, err := deviceClient.PollDeviceAuthorization(ctx, payload.DeviceAuthID, payload.UserCode, payload.ProxyURL)
	if err != nil {
		session.NextPollAtUnixMilli = now.Add(time.Duration(payload.IntervalSeconds) * time.Second).UnixMilli()
		_, _ = s.deviceSessionStore.CommitPoll(ctx, lease, session)
		return nil, err
	}
	if authorization == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_INVALID_RESPONSE", "OpenAI returned an empty device authorization response")
	}
	session.NextPollAtUnixMilli = now.Add(time.Duration(payload.IntervalSeconds) * time.Second).UnixMilli()
	if authorization.Pending {
		committed, commitErr := s.deviceSessionStore.CommitPoll(ctx, lease, session)
		if commitErr != nil {
			return nil, infraerrors.Newf(http.StatusServiceUnavailable, "OPENAI_DEVICE_OAUTH_SESSION_STORE_FAILED", "update session: %v", commitErr)
		}
		if !committed {
			return nil, infraerrors.New(http.StatusGone, "OPENAI_DEVICE_OAUTH_SESSION_CHANGED", "OpenAI device authorization was cancelled or superseded")
		}
		return openAIDeviceAuthorizationResult(session, payload, now), nil
	}

	tokenResponse, err := s.oauthClient.ExchangeCode(ctx, authorization.AuthorizationCode, authorization.CodeVerifier, openai.DeviceExchangeRedirect, payload.ProxyURL, payload.ClientID)
	if err != nil {
		_, _ = s.deviceSessionStore.CommitPoll(ctx, lease, session)
		return nil, err
	}
	payload.Token = s.tokenInfoFromResponse(ctx, tokenResponse, payload.ClientID, payload.ProxyURL)
	if payload.Token == nil || strings.TrimSpace(payload.Token.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_INVALID_TOKEN", "OpenAI returned an empty access token")
	}
	session.Status = ProviderOAuthSessionAuthorized
	session.Payload, err = json.Marshal(payload)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_DEVICE_OAUTH_SESSION_FAILED", "encode session: %v", err)
	}
	committed, err := s.deviceSessionStore.CommitPoll(ctx, lease, session)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "OPENAI_DEVICE_OAUTH_SESSION_STORE_FAILED", "update session: %v", err)
	}
	if !committed {
		return nil, infraerrors.New(http.StatusGone, "OPENAI_DEVICE_OAUTH_SESSION_CHANGED", "OpenAI device authorization was cancelled or superseded")
	}
	return openAIDeviceAuthorizationResult(session, payload, now), nil
}

func (s *OpenAIOAuthService) CancelDeviceAuthorization(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return infraerrors.New(http.StatusBadRequest, "OPENAI_DEVICE_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.deviceSessionStore == nil {
		return infraerrors.New(http.StatusServiceUnavailable, "OPENAI_DEVICE_OAUTH_NOT_CONFIGURED", "OpenAI device authorization is not configured")
	}
	if _, err := s.deviceSessionStore.Cancel(ctx, sessionID, 5*time.Minute); err != nil {
		return infraerrors.Newf(http.StatusServiceUnavailable, "OPENAI_DEVICE_OAUTH_SESSION_STORE_FAILED", "cancel device session: %v", err)
	}
	return nil
}

func (s *OpenAIOAuthService) ConsumeDeviceAuthorization(ctx context.Context, sessionID string) (*OpenAIDeviceAuthorizationCredential, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_DEVICE_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.deviceSessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_DEVICE_OAUTH_NOT_CONFIGURED", "OpenAI device authorization is not configured")
	}
	session, err := s.deviceSessionStore.ConsumeAuthorized(ctx, sessionID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "OPENAI_DEVICE_OAUTH_SESSION_STORE_FAILED", "consume device session: %v", err)
	}
	if session == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_DEVICE_OAUTH_SESSION_NOT_FOUND", "OpenAI authorization session not found, incomplete, expired, or already consumed")
	}
	payload, err := decodeOpenAIDeviceSessionPayload(session)
	if err != nil || payload.Token == nil || strings.TrimSpace(payload.Token.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusConflict, "OPENAI_DEVICE_OAUTH_NOT_AUTHORIZED", "OpenAI device authorization is not complete")
	}
	return &OpenAIDeviceAuthorizationCredential{Token: payload.Token, ProxyID: payload.ProxyID}, nil
}

// ExchangeCode exchanges authorization code for tokens
func (s *OpenAIOAuthService) ExchangeCode(ctx context.Context, input *OpenAIExchangeCodeInput) (*OpenAITokenInfo, error) {
	// Get session
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}
	if input.State == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_STATE_REQUIRED", "oauth state is required")
	}
	if subtle.ConstantTimeCompare([]byte(input.State), []byte(session.State)) != 1 {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_STATE", "invalid oauth state")
	}

	// Get proxy URL: prefer input.ProxyID, fallback to session.ProxyURL
	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
		}
		if proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	// Use redirect URI from session or input
	redirectURI := session.RedirectURI
	if input.RedirectURI != "" {
		redirectURI = input.RedirectURI
	}
	clientID := strings.TrimSpace(session.ClientID)
	if clientID == "" {
		clientID = openai.ClientID
	}

	// Exchange code for token
	tokenResp, err := s.oauthClient.ExchangeCode(ctx, input.Code, session.CodeVerifier, redirectURI, proxyURL, clientID)
	if err != nil {
		return nil, err
	}

	// Delete session after successful exchange
	s.sessionStore.Delete(input.SessionID)

	tokenInfo := s.tokenInfoFromResponse(ctx, tokenResp, clientID, proxyURL)
	if tokenInfo == nil || strings.TrimSpace(tokenInfo.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_OAUTH_INVALID_TOKEN_RESPONSE", "OpenAI returned an empty access token")
	}
	return tokenInfo, nil
}

// RefreshToken refreshes an OpenAI OAuth token
func (s *OpenAIOAuthService) RefreshToken(ctx context.Context, refreshToken string, proxyURL string) (*OpenAITokenInfo, error) {
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, "")
}

// RefreshTokenWithClientID refreshes an OpenAI OAuth token with optional client_id.
func (s *OpenAIOAuthService) RefreshTokenWithClientID(ctx context.Context, refreshToken string, proxyURL string, clientID string) (*OpenAITokenInfo, error) {
	tokenResp, err := s.oauthClient.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
	if err != nil {
		return nil, err
	}

	tokenInfo := s.tokenInfoFromResponse(ctx, tokenResp, clientID, proxyURL)
	if tokenInfo == nil || strings.TrimSpace(tokenInfo.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_OAUTH_INVALID_TOKEN_RESPONSE", "OpenAI returned an empty access token")
	}
	return tokenInfo, nil
}

func (s *OpenAIOAuthService) tokenInfoFromResponse(ctx context.Context, tokenResp *openai.TokenResponse, clientID, proxyURL string) *OpenAITokenInfo {
	if tokenResp == nil {
		return nil
	}
	var userInfo *openai.UserInfo
	if tokenResp.IDToken != "" {
		claims, parseErr := openai.ParseIDToken(tokenResp.IDToken)
		if parseErr != nil {
			slog.Warn("openai_oauth_id_token_parse_failed", "error", parseErr)
		} else {
			userInfo = claims.GetUserInfo()
		}
	}
	now := s.now()
	tokenInfo := &OpenAITokenInfo{
		AccessToken: tokenResp.AccessToken, RefreshToken: tokenResp.RefreshToken,
		IDToken: tokenResp.IDToken, ExpiresIn: tokenResp.ExpiresIn,
		ExpiresAt: now.Unix() + tokenResp.ExpiresIn, ClientID: strings.TrimSpace(clientID),
	}
	if userInfo != nil {
		tokenInfo.Email = userInfo.Email
		tokenInfo.ChatGPTAccountID = userInfo.ChatGPTAccountID
		tokenInfo.ChatGPTUserID = userInfo.ChatGPTUserID
		tokenInfo.OrganizationID = userInfo.OrganizationID
		tokenInfo.PlanType = userInfo.PlanType
	}
	s.enrichTokenInfo(ctx, tokenInfo, proxyURL)
	return tokenInfo
}

// enrichTokenInfo 通过 ChatGPT backend-api 补全 tokenInfo 并设置隐私（best-effort）。
// 从 accounts/check 获取最新 plan_type、subscription_expires_at、email，
// 然后尝试关闭训练数据共享。适用于所有获取/刷新 token 的路径。
func (s *OpenAIOAuthService) enrichTokenInfo(ctx context.Context, tokenInfo *OpenAITokenInfo, proxyURL string) {
	if tokenInfo.AccessToken == "" || s.privacyClientFactory == nil {
		return
	}

	// 从 access_token JWT 中提取 orgID（poid），用于匹配正确的账号
	orgID := tokenInfo.OrganizationID
	if orgID == "" {
		if atClaims, err := openai.DecodeIDToken(tokenInfo.AccessToken); err == nil && atClaims.OpenAIAuth != nil {
			orgID = atClaims.OpenAIAuth.POID
		}
	}
	if info := fetchChatGPTAccountInfo(ctx, s.privacyClientFactory, tokenInfo.AccessToken, proxyURL, orgID); info != nil {
		// chatgpt_plan_type from the ID token is the canonical personal-plan value.
		// accounts/check is a multi-account/workspace endpoint; inactive team or
		// business workspaces can otherwise overwrite Pro/Free with internal
		// workspace billing plan names such as self_serve_business_usage_based.
		if shouldApplyChatGPTAccountInfoPlanType(tokenInfo.PlanType, info.PlanType) {
			tokenInfo.PlanType = info.PlanType
		}
		if info.SubscriptionExpiresAt != "" {
			tokenInfo.SubscriptionExpiresAt = info.SubscriptionExpiresAt
		}
		if tokenInfo.Email == "" && info.Email != "" {
			tokenInfo.Email = info.Email
		}
	}
	if strings.TrimSpace(tokenInfo.SubscriptionExpiresAt) == "" {
		if expiresAt := fetchChatGPTSubscriptionExpiresAt(ctx, s.privacyClientFactory, tokenInfo.AccessToken, proxyURL, resolveChatGPTSubscriptionAccountID(tokenInfo, orgID)); expiresAt != "" {
			tokenInfo.SubscriptionExpiresAt = expiresAt
		}
	}

	// 尝试设置隐私（关闭训练数据共享），best-effort
	tokenInfo.PrivacyMode = disableOpenAITraining(ctx, s.privacyClientFactory, tokenInfo.AccessToken, proxyURL)
}

func shouldApplyChatGPTAccountInfoPlanType(current, candidate string) bool {
	return strings.TrimSpace(candidate) != "" && strings.TrimSpace(current) == ""
}

func resolveChatGPTSubscriptionAccountID(tokenInfo *OpenAITokenInfo, orgID string) string {
	for _, candidate := range []string{
		tokenInfo.ChatGPTAccountID,
		tokenInfo.OrganizationID,
		orgID,
	} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// RefreshAccountToken refreshes token for an OpenAI OAuth account
func (s *OpenAIOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*OpenAITokenInfo, error) {
	if account.Platform != PlatformOpenAI {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_ACCOUNT", "account is not an OpenAI account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_ACCOUNT_TYPE", "account is not an OAuth account")
	}

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	accessToken := account.GetCredential("access_token")
	if account.IsOpenAIPersonalAccessToken() {
		if accessToken == "" {
			return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_CODEX_PAT_REQUIRED", "access token is required")
		}
		return s.ValidateCodexPersonalAccessToken(ctx, accessToken, proxyURL)
	}

	refreshToken := account.GetCredential("refresh_token")
	if refreshToken == "" {
		if accessToken != "" {
			tokenInfo := &OpenAITokenInfo{
				AccessToken:           accessToken,
				RefreshToken:          "",
				IDToken:               account.GetCredential("id_token"),
				ClientID:              account.GetCredential("client_id"),
				Email:                 account.GetCredential("email"),
				ChatGPTAccountID:      account.GetCredential("chatgpt_account_id"),
				ChatGPTUserID:         account.GetCredential("chatgpt_user_id"),
				OrganizationID:        account.GetCredential("organization_id"),
				PlanType:              account.GetCredential("plan_type"),
				SubscriptionExpiresAt: account.GetCredential("subscription_expires_at"),
			}
			if expiresAt := account.GetCredentialAsTime("expires_at"); expiresAt != nil {
				tokenInfo.ExpiresAt = expiresAt.Unix()
				tokenInfo.ExpiresIn = int64(time.Until(*expiresAt).Seconds())
			}
			s.enrichTokenInfo(ctx, tokenInfo, proxyURL)
			return tokenInfo, nil
		}
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
	}

	clientID := account.GetCredential("client_id")
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
}

// BuildAccountCredentials builds credentials map from token info
func (s *OpenAIOAuthService) BuildAccountCredentials(tokenInfo *OpenAITokenInfo) map[string]any {
	if tokenInfo == nil {
		return nil
	}
	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
	}
	if tokenInfo.ExpiresAt > 0 {
		creds["expires_at"] = time.Unix(tokenInfo.ExpiresAt, 0).Format(time.RFC3339)
	}
	// 仅在刷新响应返回了新的 refresh_token 时才更新，防止用空值覆盖已有令牌
	if strings.TrimSpace(tokenInfo.RefreshToken) != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
	}

	if tokenInfo.IDToken != "" {
		creds["id_token"] = tokenInfo.IDToken
	}
	if tokenInfo.Email != "" {
		creds["email"] = tokenInfo.Email
	}
	if tokenInfo.ChatGPTAccountID != "" {
		creds["chatgpt_account_id"] = tokenInfo.ChatGPTAccountID
	}
	if tokenInfo.ChatGPTUserID != "" {
		creds["chatgpt_user_id"] = tokenInfo.ChatGPTUserID
	}
	if tokenInfo.OrganizationID != "" {
		creds["organization_id"] = tokenInfo.OrganizationID
	}
	if tokenInfo.PlanType != "" {
		creds["plan_type"] = tokenInfo.PlanType
	}
	if tokenInfo.SubscriptionExpiresAt != "" {
		creds["subscription_expires_at"] = tokenInfo.SubscriptionExpiresAt
	}
	if strings.TrimSpace(tokenInfo.ClientID) != "" {
		creds["client_id"] = strings.TrimSpace(tokenInfo.ClientID)
	}
	if tokenInfo.AuthMode == OpenAIAuthModePersonalAccessToken {
		creds[openAIAuthModeCredentialKey] = OpenAIAuthModePersonalAccessToken
		creds[openAIAuthModeLegacyCredentialKey] = "personal_access_token"
		creds["token_type"] = "Bearer"
		creds["chatgpt_account_is_fedramp"] = tokenInfo.ChatGPTAccountFedRAMP
	} else if tokenInfo.ChatGPTAccountFedRAMP {
		creds["chatgpt_account_is_fedramp"] = true
	}

	return NormalizeOpenAIPersonalAccessTokenCredentials(nil, tokenInfo, creds)
}

// Stop stops the session store cleanup goroutine
func (s *OpenAIOAuthService) Stop() {
	s.sessionStore.Stop()
}

func (s *OpenAIOAuthService) deviceProxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_AVAILABLE", "proxy repository is not available")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found")
	}
	return proxy.URL(), nil
}

func parseOpenAIDevicePollInterval(raw json.RawMessage) time.Duration {
	if len(raw) == 0 {
		return openAIDeviceDefaultInterval
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if seconds, parseErr := strconv.ParseInt(strings.TrimSpace(text), 10, 64); parseErr == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	var seconds int64
	if err := json.Unmarshal(raw, &seconds); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return openAIDeviceDefaultInterval
}

func decodeOpenAIDeviceSessionPayload(session *ProviderOAuthSession) (*openAIDeviceSessionPayload, error) {
	if session == nil || len(session.Payload) == 0 {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_SESSION_INVALID", "OpenAI device session payload is missing")
	}
	var payload openAIDeviceSessionPayload
	if err := json.Unmarshal(session.Payload, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func openAIDeviceAuthorizationResult(session *ProviderOAuthSession, payload *openAIDeviceSessionPayload, now time.Time) *OpenAIDeviceAuthorizationResult {
	if session == nil || payload == nil {
		return nil
	}
	result := &OpenAIDeviceAuthorizationResult{
		SessionID: session.ID, Status: session.Status, VerificationURI: openai.DeviceVerificationURL,
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

func normalizeOpenAIOAuthPlatform(platform string) string {
	return openai.OAuthPlatformOpenAI
}
