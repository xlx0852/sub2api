package repository

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/imroc/req/v3"
)

// NewOpenAIOAuthClient creates a new OpenAI OAuth client
func NewOpenAIOAuthClient() service.OpenAIOAuthClient {
	return &openaiOAuthService{
		tokenURL:               openai.TokenURL,
		deviceUserCodeURL:      openai.DeviceUserCodeURL,
		deviceAuthorizationURL: openai.DeviceAuthorizationURL,
	}
}

type openaiOAuthService struct {
	tokenURL               string
	deviceUserCodeURL      string
	deviceAuthorizationURL string
}

func (s *openaiOAuthService) RequestDeviceCode(ctx context.Context, proxyURL, clientID string) (*openai.DeviceCodeResponse, error) {
	client, err := createOpenAIReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = openai.ClientID
	}
	var result openai.DeviceCodeResponse
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", "codex-cli/0.91.0").
		SetBody(map[string]string{"client_id": clientID}).
		SetSuccessResult(&result).
		Post(s.deviceUserCodeURL)
	if err != nil {
		if shouldReturnOpenAINoProxyHint(ctx, proxyURL, err) {
			return nil, newOpenAINoProxyHintError(err)
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_REQUEST_FAILED", "device code request failed: status %d, body: %s", resp.StatusCode, resp.String())
	}
	if strings.TrimSpace(result.DeviceAuthID) == "" || (strings.TrimSpace(result.UserCode) == "" && strings.TrimSpace(result.UserCodeAlt) == "") {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_INVALID_RESPONSE", "OpenAI returned an incomplete device code response")
	}
	return &result, nil
}

func (s *openaiOAuthService) PollDeviceAuthorization(ctx context.Context, deviceAuthID, userCode, proxyURL string) (*openai.DeviceAuthorizationResponse, error) {
	client, err := createOpenAIReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	requestBody := map[string]string{
		"device_auth_id": strings.TrimSpace(deviceAuthID),
		"user_code":      strings.TrimSpace(userCode),
	}
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", "codex-cli/0.91.0").
		SetBody(requestBody).
		Post(s.deviceAuthorizationURL)
	if err != nil {
		if shouldReturnOpenAINoProxyHint(ctx, proxyURL, err) {
			return nil, newOpenAINoProxyHintError(err)
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_POLL_FAILED", "request failed: %v", err)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return &openai.DeviceAuthorizationResponse{Pending: true}, nil
	}
	if !resp.IsSuccessState() {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_POLL_FAILED", "device authorization polling failed: status %d, body: %s", resp.StatusCode, resp.String())
	}
	var result openai.DeviceAuthorizationResponse
	if err := json.Unmarshal(resp.Bytes(), &result); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_INVALID_RESPONSE", "decode device authorization response: %v", err)
	}
	if strings.TrimSpace(result.AuthorizationCode) == "" || strings.TrimSpace(result.CodeVerifier) == "" || strings.TrimSpace(result.CodeChallenge) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_DEVICE_OAUTH_INVALID_RESPONSE", "OpenAI returned an incomplete device authorization response")
	}
	return &result, nil
}

func (s *openaiOAuthService) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*openai.TokenResponse, error) {
	client, err := createOpenAIReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	if redirectURI == "" {
		redirectURI = openai.DefaultRedirectURI
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = openai.ClientID
	}

	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("client_id", clientID)
	formData.Set("code", code)
	formData.Set("redirect_uri", redirectURI)
	formData.Set("code_verifier", codeVerifier)

	var tokenResp openai.TokenResponse

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("User-Agent", "codex-cli/0.91.0").
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(s.tokenURL)

	if err != nil {
		if shouldReturnOpenAINoProxyHint(ctx, proxyURL, err) {
			return nil, newOpenAINoProxyHintError(err)
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}

	if !resp.IsSuccessState() {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_TOKEN_EXCHANGE_FAILED", "token exchange failed: status %d, body: %s", resp.StatusCode, resp.String())
	}

	return &tokenResp, nil
}

func (s *openaiOAuthService) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, "")
}

func (s *openaiOAuthService) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	// 调用方应始终传入正确的 client_id；为兼容旧数据，未指定时默认使用 OpenAI ClientID
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = openai.ClientID
	}
	return s.refreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
}

func (s *openaiOAuthService) refreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL, clientID string) (*openai.TokenResponse, error) {
	client, err := createOpenAIReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	formData := url.Values{}
	formData.Set("grant_type", "refresh_token")
	formData.Set("refresh_token", refreshToken)
	formData.Set("client_id", clientID)
	formData.Set("scope", openai.RefreshScopes)

	var tokenResp openai.TokenResponse

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("User-Agent", "codex-cli/0.91.0").
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(s.tokenURL)

	if err != nil {
		if shouldReturnOpenAINoProxyHint(ctx, proxyURL, err) {
			return nil, newOpenAINoProxyHintError(err)
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}

	if !resp.IsSuccessState() {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed: status %d, body: %s", resp.StatusCode, resp.String())
	}

	return &tokenResp, nil
}

func createOpenAIReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  120 * time.Second,
	})
}

func shouldReturnOpenAINoProxyHint(ctx context.Context, proxyURL string, err error) bool {
	if strings.TrimSpace(proxyURL) != "" || err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return !errors.Is(err, context.Canceled)
}

func newOpenAINoProxyHintError(cause error) error {
	return infraerrors.New(
		http.StatusBadGateway,
		"OPENAI_OAUTH_PROXY_REQUIRED",
		"OpenAI OAuth request failed: no proxy is configured and this server could not reach OpenAI directly. Select a proxy that can access OpenAI, then retry; if the authorization code has expired, regenerate the authorization URL.",
	).WithCause(cause)
}
