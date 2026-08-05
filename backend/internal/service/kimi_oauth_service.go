package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/google/uuid"
)

const (
	kimiDeviceSessionPrefix = "kmi_"
	kimiTokenRefreshSkew    = 5 * time.Minute
	// The Kimi HTTP client allows requests to run for up to 60 seconds. Keep
	// the poll lease longer than that so a slow token request cannot outlive
	// its lease and be duplicated by another status request.
	kimiDevicePollLeaseTTL = 90 * time.Second
)

type KimiOAuthClient interface {
	StartDeviceFlow(ctx context.Context, proxyURL string, headers kimi.DeviceHeaders) (*kimi.DeviceCodeResponse, error)
	PollDeviceToken(ctx context.Context, deviceCode, proxyURL string, headers kimi.DeviceHeaders) (*kimi.TokenResponse, error)
	RefreshToken(ctx context.Context, refreshToken, proxyURL string, headers kimi.DeviceHeaders) (*kimi.TokenResponse, error)
}

type KimiDeviceSessionStore interface {
	Create(ctx context.Context, session *KimiDeviceSession, ttl time.Duration) error
	Get(ctx context.Context, sessionID string) (*KimiDeviceSession, error)
	AcquirePollLease(ctx context.Context, sessionID string, now time.Time, leaseTTL time.Duration) (*KimiDevicePollLease, error)
	CommitPoll(ctx context.Context, lease *KimiDevicePollLease, updated *KimiDeviceSession) (bool, error)
	ConsumeAuthorized(ctx context.Context, sessionID string) (*KimiDeviceSession, error)
	Cancel(ctx context.Context, sessionID string, tombstoneTTL time.Duration) (bool, error)
	Delete(ctx context.Context, sessionID string) error
}

type KimiDevicePollLease struct {
	Session       *KimiDeviceSession
	ProviderLease *ProviderOAuthSessionLease
	Held          bool
}

type KimiDeviceSession struct {
	ID                      string              `json:"id"`
	Status                  string              `json:"status"`
	DeviceCode              string              `json:"device_code"`
	UserCode                string              `json:"user_code"`
	VerificationURI         string              `json:"verification_uri"`
	VerificationURIComplete string              `json:"verification_uri_complete"`
	ProxyURL                string              `json:"proxy_url,omitempty"`
	ProxyID                 *int64              `json:"proxy_id,omitempty"`
	DeviceID                string              `json:"device_id"`
	DeviceName              string              `json:"device_name"`
	DeviceModel             string              `json:"device_model"`
	IntervalSeconds         int64               `json:"interval_seconds"`
	NextPollAt              time.Time           `json:"next_poll_at"`
	ExpiresAt               time.Time           `json:"expires_at"`
	Token                   *KimiOAuthTokenInfo `json:"token,omitempty"`
	Error                   string              `json:"error,omitempty"`
}

type KimiOAuthTokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	Scope        string `json:"scope,omitempty"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at"`
	DeviceID     string `json:"device_id"`
	DeviceName   string `json:"device_name"`
	DeviceModel  string `json:"device_model"`
	// UserID is the durable Kimi subject (JWT user_id/sub). Used as the OAuth
	// merge identity because Kimi device tokens do not include an email claim.
	UserID  string `json:"user_id,omitempty"`
	ProxyID *int64 `json:"proxy_id,omitempty"`
}

type KimiDeviceAuthorizationResult struct {
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

type KimiOAuthService struct {
	proxyRepo    ProxyRepository
	oauthClient  KimiOAuthClient
	sessionStore KimiDeviceSessionStore
	now          func() time.Time
}

func NewKimiOAuthService(proxyRepo ProxyRepository, oauthClient KimiOAuthClient, sessionStore KimiDeviceSessionStore) *KimiOAuthService {
	return &KimiOAuthService{proxyRepo: proxyRepo, oauthClient: oauthClient, sessionStore: sessionStore, now: time.Now}
}

func (s *KimiOAuthService) StartDeviceAuthorization(ctx context.Context, proxyID *int64) (*KimiDeviceAuthorizationResult, error) {
	if s == nil || s.oauthClient == nil || s.sessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "KIMI_OAUTH_NOT_CONFIGURED", "Kimi OAuth is not configured")
	}
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	sessionID, err := randomKimiID(kimiDeviceSessionPrefix, 24)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "KIMI_OAUTH_SESSION_FAILED", "generate session: %v", err)
	}
	deviceID, err := randomKimiDeviceID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "KIMI_OAUTH_DEVICE_FAILED", "generate device identity: %v", err)
	}
	headers := defaultKimiDeviceHeaders(deviceID)
	device, err := s.oauthClient.StartDeviceFlow(ctx, proxyURL, headers)
	if err != nil {
		return nil, err
	}
	if device == nil || strings.TrimSpace(device.DeviceCode) == "" || strings.TrimSpace(device.UserCode) == "" || strings.TrimSpace(device.VerificationURI) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "KIMI_OAUTH_INVALID_DEVICE_RESPONSE", "Kimi returned an invalid device authorization response")
	}
	interval := device.Interval
	if interval < kimi.DefaultDevicePollInterval {
		interval = kimi.DefaultDevicePollInterval
	}
	expiresIn := device.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = kimi.DefaultDeviceCodeExpiresIn
	}
	now := s.now()
	session := &KimiDeviceSession{
		ID: sessionID, Status: "pending", DeviceCode: device.DeviceCode, UserCode: device.UserCode,
		VerificationURI: device.VerificationURI, VerificationURIComplete: device.VerificationURIComplete,
		ProxyURL: proxyURL, ProxyID: proxyID, DeviceID: deviceID, DeviceName: headers.DeviceName, DeviceModel: headers.DeviceModel,
		IntervalSeconds: interval,
		// Enforce Kimi's minimum polling interval server-side as well as in the
		// frontend. An immediate status request must not hit the token endpoint.
		NextPollAt: now.Add(time.Duration(interval) * time.Second),
		ExpiresAt:  now.Add(time.Duration(expiresIn) * time.Second),
	}
	if err := s.sessionStore.Create(ctx, session, time.Duration(expiresIn)*time.Second); err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "KIMI_OAUTH_SESSION_STORE_FAILED", "store device session: %v", err)
	}
	return kimiDeviceResult(session, now), nil
}

func (s *KimiOAuthService) GetDeviceAuthorizationStatus(ctx context.Context, sessionID string) (*KimiDeviceAuthorizationResult, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.sessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "KIMI_OAUTH_NOT_CONFIGURED", "Kimi OAuth is not configured")
	}
	now := s.now()
	lease, err := s.sessionStore.AcquirePollLease(ctx, sessionID, now, kimiDevicePollLeaseTTL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "KIMI_OAUTH_SESSION_STORE_FAILED", "acquire poll lease: %v", err)
	}
	if lease == nil || lease.Session == nil {
		return nil, infraerrors.New(http.StatusNotFound, "KIMI_OAUTH_SESSION_NOT_FOUND", "Kimi authorization session not found or expired")
	}
	session := lease.Session
	if session.Status == ProviderOAuthSessionCancelled {
		return nil, infraerrors.New(http.StatusGone, "KIMI_OAUTH_SESSION_CANCELLED", "Kimi device authorization was cancelled")
	}
	if !now.Before(session.ExpiresAt) {
		_, _ = s.sessionStore.Cancel(ctx, session.ID, 5*time.Minute)
		return nil, infraerrors.New(http.StatusGone, "KIMI_OAUTH_SESSION_EXPIRED", "Kimi device authorization expired")
	}
	if session.Status != "pending" {
		return kimiDeviceResult(session, now), nil
	}
	if !lease.Held {
		if !now.Before(session.NextPollAt) {
			session.NextPollAt = now.Add(time.Second)
		}
		return kimiDeviceResult(session, now), nil
	}

	headers := kimi.DeviceHeaders{DeviceID: session.DeviceID, DeviceName: session.DeviceName, DeviceModel: session.DeviceModel}
	token, err := s.oauthClient.PollDeviceToken(ctx, session.DeviceCode, session.ProxyURL, headers)
	if err != nil {
		session.NextPollAt = now.Add(time.Duration(session.IntervalSeconds) * time.Second)
		_, _ = s.sessionStore.CommitPoll(ctx, lease, session)
		return nil, err
	}
	if token == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "KIMI_OAUTH_INVALID_TOKEN_RESPONSE", "Kimi returned an empty token response")
	}
	session.NextPollAt = now.Add(time.Duration(session.IntervalSeconds) * time.Second)
	switch strings.TrimSpace(token.Error) {
	case "authorization_pending":
	case "slow_down":
		session.IntervalSeconds += 5
		session.NextPollAt = now.Add(time.Duration(session.IntervalSeconds) * time.Second)
	case "expired_token":
		_, _ = s.sessionStore.Cancel(ctx, session.ID, 5*time.Minute)
		return nil, infraerrors.New(http.StatusGone, "KIMI_OAUTH_DEVICE_CODE_EXPIRED", "Kimi device code expired")
	case "access_denied":
		session.Status = "denied"
		session.Error = "authorization denied"
	case "":
		if strings.TrimSpace(token.AccessToken) == "" {
			return nil, infraerrors.New(http.StatusBadGateway, "KIMI_OAUTH_INVALID_TOKEN_RESPONSE", "Kimi returned an empty access token")
		}
		session.Status = "authorized"
		session.Token = s.tokenInfo(token, headers)
		session.Token.ProxyID = session.ProxyID
	default:
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_UPSTREAM_ERROR", "Kimi authorization failed: %s", token.Error)
	}
	committed, err := s.sessionStore.CommitPoll(ctx, lease, session)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "KIMI_OAUTH_SESSION_STORE_FAILED", "update device session: %v", err)
	}
	if !committed {
		return nil, infraerrors.New(http.StatusGone, "KIMI_OAUTH_SESSION_CHANGED", "Kimi device authorization was cancelled or superseded")
	}
	return kimiDeviceResult(session, now), nil
}

func (s *KimiOAuthService) ConsumeAuthorizedSession(ctx context.Context, sessionID string) (*KimiOAuthTokenInfo, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.sessionStore == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "KIMI_OAUTH_NOT_CONFIGURED", "Kimi OAuth is not configured")
	}
	session, err := s.sessionStore.ConsumeAuthorized(ctx, sessionID)
	if err != nil {
		// Do not turn a Redis/storage outage into a misleading "not found"
		// response. The message contains no session/token data.
		return nil, infraerrors.Newf(http.StatusServiceUnavailable, "KIMI_OAUTH_SESSION_STORE_FAILED", "consume device session: %v", err)
	}
	if session == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_SESSION_NOT_FOUND", "Kimi authorization session not found, expired, or already consumed")
	}
	if session.Status != "authorized" || session.Token == nil || strings.TrimSpace(session.Token.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusConflict, "KIMI_OAUTH_NOT_AUTHORIZED", "Kimi authorization is not complete")
	}
	return session.Token, nil
}

func (s *KimiOAuthService) CancelDeviceAuthorization(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	if s == nil || s.sessionStore == nil {
		return infraerrors.New(http.StatusServiceUnavailable, "KIMI_OAUTH_NOT_CONFIGURED", "Kimi OAuth is not configured")
	}
	if _, err := s.sessionStore.Cancel(ctx, sessionID, 5*time.Minute); err != nil {
		return infraerrors.Newf(http.StatusServiceUnavailable, "KIMI_OAUTH_SESSION_STORE_FAILED", "cancel device session: %v", err)
	}
	return nil
}

func (s *KimiOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*KimiOAuthTokenInfo, error) {
	if account == nil || account.Platform != PlatformKimi || account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_INVALID_ACCOUNT", "account is not a Kimi OAuth account")
	}
	refreshToken := strings.TrimSpace(account.GetCredential("refresh_token"))
	if refreshToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_NO_REFRESH_TOKEN", "Kimi refresh token is missing")
	}
	proxyURL, err := s.proxyURL(ctx, account.ProxyID)
	if err != nil {
		return nil, err
	}
	headers := kimi.DeviceHeaders{
		DeviceID: account.GetCredential("device_id"), DeviceName: account.GetCredential("device_name"), DeviceModel: account.GetCredential("device_model"),
	}
	if strings.TrimSpace(headers.DeviceID) == "" {
		// CPA generates an in-memory device identity when older credentials do
		// not contain one. Keep refresh compatible with those credentials and
		// let BuildAccountCredentials persist the generated ID for subsequent
		// requests, while preserving any existing device name/model below.
		deviceID, errID := randomKimiDeviceID()
		if errID != nil {
			return nil, infraerrors.Newf(http.StatusInternalServerError, "KIMI_OAUTH_DEVICE_FAILED", "generate device identity: %v", errID)
		}
		headers.DeviceID = deviceID
	}
	defaults := defaultKimiDeviceHeaders(headers.DeviceID)
	if headers.DeviceName == "" {
		headers.DeviceName = defaults.DeviceName
	}
	if headers.DeviceModel == "" {
		headers.DeviceModel = defaults.DeviceModel
	}
	token, err := s.oauthClient.RefreshToken(ctx, refreshToken, proxyURL, headers)
	if err != nil {
		return nil, err
	}
	if token == nil || strings.TrimSpace(token.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "KIMI_OAUTH_INVALID_TOKEN_RESPONSE", "Kimi returned an empty access token")
	}
	info := s.tokenInfo(token, headers)
	if info.RefreshToken == "" {
		info.RefreshToken = refreshToken
	}
	return info, nil
}

func (s *KimiOAuthService) BuildAccountCredentials(token *KimiOAuthTokenInfo) map[string]any {
	if token == nil {
		return nil
	}
	userID := strings.TrimSpace(token.UserID)
	if userID == "" {
		userID = kimiUserIDFromAccessToken(token.AccessToken)
	}
	credentials := map[string]any{
		"access_token": token.AccessToken, "token_type": token.TokenType,
		"expires_at": time.Unix(token.ExpiresAt, 0).UTC().Format(time.RFC3339),
		"device_id":  token.DeviceID, "device_name": token.DeviceName, "device_model": token.DeviceModel,
		"client_id": kimi.ClientID, "base_url": kimi.CodingBaseURL,
	}
	if userID != "" {
		// Stable account identity for CreateAccount merge. Kimi device tokens do
		// not expose a real email; user_id/sub from the access JWT is the only
		// durable subject. Store it as both user_id and email so the existing
		// platform+email OAuth merge path can collapse re-logins into one row.
		credentials["user_id"] = userID
		credentials["email"] = userID
	}
	if token.RefreshToken != "" {
		credentials["refresh_token"] = token.RefreshToken
	}
	if token.Scope != "" {
		credentials["scope"] = token.Scope
	}
	return credentials
}

func (s *KimiOAuthService) getDeviceSession(ctx context.Context, sessionID string) (*KimiDeviceSession, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_SESSION_REQUIRED", "session_id is required")
	}
	session, err := s.sessionStore.Get(ctx, sessionID)
	if err != nil || session == nil {
		return nil, infraerrors.New(http.StatusNotFound, "KIMI_OAUTH_SESSION_NOT_FOUND", "Kimi authorization session not found or expired")
	}
	return session, nil
}

func (s *KimiOAuthService) tokenInfo(token *kimi.TokenResponse, headers kimi.DeviceHeaders) *KimiOAuthTokenInfo {
	expiresIn := int64(token.ExpiresIn)
	if expiresIn <= 0 {
		expiresIn = kimi.DefaultAccessTokenExpiresIn
	}
	tokenType := strings.TrimSpace(token.TokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}
	return &KimiOAuthTokenInfo{
		AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenType: tokenType, Scope: token.Scope,
		ExpiresIn: expiresIn, ExpiresAt: s.now().Add(time.Duration(expiresIn) * time.Second).Unix(),
		DeviceID: headers.DeviceID, DeviceName: headers.DeviceName, DeviceModel: headers.DeviceModel,
		UserID: kimiUserIDFromAccessToken(token.AccessToken),
	}
}

// kimiUserIDFromAccessToken extracts the durable Kimi subject from a JWT access
// token without verifying the signature (identity is used only as a local merge
// key after a successful OAuth exchange). Prefer user_id, then sub.
func kimiUserIDFromAccessToken(accessToken string) string {
	accessToken = strings.TrimSpace(accessToken)
	parts := strings.Split(accessToken, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}
	var claims struct {
		UserID string `json:"user_id"`
		Sub    string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	if id := strings.TrimSpace(claims.UserID); id != "" {
		return id
	}
	return strings.TrimSpace(claims.Sub)
}

func (s *KimiOAuthService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_PROXY_NOT_AVAILABLE", "proxy repository is not available")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "KIMI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", infraerrors.New(http.StatusBadRequest, "KIMI_OAUTH_PROXY_NOT_FOUND", "proxy not found")
	}
	return proxy.URL(), nil
}

func defaultKimiDeviceHeaders(deviceID string) kimi.DeviceHeaders {
	hostname, _ := os.Hostname()
	if strings.TrimSpace(hostname) == "" {
		hostname = "sub2api"
	}
	osName := runtime.GOOS
	switch osName {
	case "darwin":
		osName = "macOS"
	case "windows":
		osName = "Windows"
	case "linux":
		osName = "Linux"
	}
	return kimi.DeviceHeaders{DeviceID: deviceID, DeviceName: hostname, DeviceModel: osName + " " + runtime.GOARCH}
}

func randomKimiID(prefix string, size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}

func randomKimiDeviceID() (string, error) {
	deviceID, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return deviceID.String(), nil
}

func kimiDeviceResult(session *KimiDeviceSession, now time.Time) *KimiDeviceAuthorizationResult {
	result := &KimiDeviceAuthorizationResult{SessionID: session.ID, Status: session.Status, VerificationURI: session.VerificationURI,
		VerificationURIComplete: session.VerificationURIComplete, UserCode: session.UserCode, Interval: session.IntervalSeconds, Error: session.Error}
	if remaining := int64(session.ExpiresAt.Sub(now).Seconds()); remaining > 0 {
		result.ExpiresIn = remaining
	}
	if now.Before(session.NextPollAt) {
		result.RetryAfter = int64(session.NextPollAt.Sub(now).Seconds()) + 1
	}
	return result
}
