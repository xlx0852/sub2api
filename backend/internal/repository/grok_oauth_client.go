package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/imroc/req/v3"
)

type grokOAuthClient struct {
	tokenURL string
}

func NewGrokOAuthClient() service.GrokOAuthClient {
	return &grokOAuthClient{tokenURL: xai.EffectiveTokenURL()}
}

func (c *grokOAuthClient) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*xai.TokenResponse, error) {
	client, err := createGrokReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = xai.EffectiveClientID()
	}

	formData := url.Values{}
	formData.Set("grant_type", "authorization_code")
	formData.Set("client_id", clientID)
	formData.Set("code", code)
	formData.Set("redirect_uri", xai.EffectiveRedirectURI(redirectURI))
	formData.Set("code_verifier", codeVerifier)

	var tokenResp xai.TokenResponse
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("User-Agent", "sub2api-grok-oauth/1.0").
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(c.tokenURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, grokOAuthStatusError("GROK_OAUTH_TOKEN_EXCHANGE_FAILED", "token exchange failed", resp)
	}
	return &tokenResp, nil
}

func (c *grokOAuthClient) RefreshToken(ctx context.Context, refreshToken, proxyURL, clientID string) (*xai.TokenResponse, error) {
	return c.RefreshTokenAtEndpoint(ctx, refreshToken, c.tokenURL, proxyURL, clientID)
}

func (c *grokOAuthClient) RefreshTokenAtEndpoint(ctx context.Context, refreshToken, tokenEndpoint, proxyURL, clientID string) (*xai.TokenResponse, error) {
	client, err := createGrokReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}

	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = xai.EffectiveClientID()
	}

	formData := url.Values{}
	formData.Set("grant_type", "refresh_token")
	formData.Set("client_id", clientID)
	formData.Set("refresh_token", refreshToken)

	var tokenResp xai.TokenResponse
	tokenEndpoint, err = xai.ValidateOAuthEndpointURL(tokenEndpoint)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_INVALID_TOKEN_ENDPOINT", "invalid token endpoint: %v", err)
	}

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("User-Agent", "sub2api-grok-oauth/1.0").
		SetFormDataFromValues(formData).
		SetSuccessResult(&tokenResp).
		Post(tokenEndpoint)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_REQUEST_FAILED", "request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, grokOAuthStatusError("GROK_OAUTH_TOKEN_REFRESH_FAILED", "token refresh failed", resp)
	}
	return &tokenResp, nil
}

func (c *grokOAuthClient) StartDeviceFlow(ctx context.Context, proxyURL, clientID, scope string) (*xai.DeviceCodeResponse, error) {
	discovery, err := c.discover(ctx, proxyURL)
	if err != nil {
		return nil, err
	}
	client, err := createGrokReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = xai.EffectiveClientID()
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = xai.EffectiveScope()
	}
	formData := url.Values{"client_id": {clientID}, "scope": {scope}}
	var device xai.DeviceCodeResponse
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("User-Agent", "sub2api-grok-oauth/1.0").
		SetFormDataFromValues(formData).
		SetSuccessResult(&device).
		Post(discovery.DeviceAuthorizationEndpoint)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_DEVICE_REQUEST_FAILED", "request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, grokOAuthStatusError("GROK_OAUTH_DEVICE_REQUEST_FAILED", "device authorization failed", resp)
	}
	if strings.TrimSpace(device.DeviceCode) == "" || strings.TrimSpace(device.UserCode) == "" ||
		(strings.TrimSpace(device.VerificationURI) == "" && strings.TrimSpace(device.VerificationURIComplete) == "") {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_INVALID_DEVICE_RESPONSE", "xAI returned an incomplete device authorization response")
	}
	device.TokenEndpoint = discovery.TokenEndpoint
	return &device, nil
}

func (c *grokOAuthClient) PollDeviceToken(ctx context.Context, deviceCode, tokenEndpoint, proxyURL, clientID string) (*xai.DeviceTokenResponse, error) {
	client, err := createGrokReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	tokenEndpoint, err = xai.ValidateOAuthEndpointURL(tokenEndpoint)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_INVALID_TOKEN_ENDPOINT", "invalid token endpoint: %v", err)
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		clientID = xai.EffectiveClientID()
	}
	formData := url.Values{
		"grant_type":  {xai.DeviceCodeGrantType},
		"device_code": {strings.TrimSpace(deviceCode)},
		"client_id":   {clientID},
	}
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("User-Agent", "sub2api-grok-oauth/1.0").
		SetFormDataFromValues(formData).
		Post(tokenEndpoint)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_DEVICE_POLL_FAILED", "request failed: %v", err)
	}
	var token xai.DeviceTokenResponse
	if err := json.Unmarshal(resp.Bytes(), &token); err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_INVALID_DEVICE_TOKEN", "decode device token response: %v", err)
	}
	if token.Error != "" {
		return &token, nil
	}
	if !resp.IsSuccessState() {
		return nil, grokOAuthStatusError("GROK_OAUTH_DEVICE_POLL_FAILED", "device token polling failed", resp)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_INVALID_DEVICE_TOKEN", "xAI returned an empty access token")
	}
	return &token, nil
}

func (c *grokOAuthClient) discover(ctx context.Context, proxyURL string) (*xai.OIDCDiscoveryDocument, error) {
	client, err := createGrokReqClient(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	discoveryURL, err := xai.ValidatedDiscoveryURL()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_INVALID_DISCOVERY_URL", "invalid discovery URL: %v", err)
	}
	var discovery xai.OIDCDiscoveryDocument
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("User-Agent", "sub2api-grok-oauth/1.0").
		SetSuccessResult(&discovery).
		Get(discoveryURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_DISCOVERY_FAILED", "request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, grokOAuthStatusError("GROK_OAUTH_DISCOVERY_FAILED", "OIDC discovery failed", resp)
	}
	discovery.DeviceAuthorizationEndpoint, err = xai.ValidateOAuthEndpointURL(discovery.DeviceAuthorizationEndpoint)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_INVALID_DEVICE_ENDPOINT", "invalid device endpoint: %v", err)
	}
	discovery.TokenEndpoint, err = xai.ValidateOAuthEndpointURL(discovery.TokenEndpoint)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_OAUTH_INVALID_TOKEN_ENDPOINT", "invalid token endpoint: %v", err)
	}
	if discovery.DeviceAuthorizationEndpoint == discovery.TokenEndpoint {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_OAUTH_INVALID_DISCOVERY", fmt.Sprintf("xAI discovery returned duplicate OAuth endpoints: %s", discovery.TokenEndpoint))
	}
	return &discovery, nil
}

func createGrokReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  60 * time.Second,
	})
}

func grokOAuthStatusError(code, message string, resp *req.Response) error {
	statusCode := http.StatusBadGateway
	errorCode := code
	upstreamStatus := 0
	if resp != nil && resp.StatusCode == http.StatusForbidden {
		statusCode = http.StatusForbidden
		errorCode = "GROK_OAUTH_ENTITLEMENT_DENIED"
	}
	body := ""
	if resp != nil {
		upstreamStatus = resp.StatusCode
		body = logredact.RedactText(resp.String())
	}
	return infraerrors.Newf(statusCode, errorCode, "%s: status %d, body: %s", message, upstreamStatus, body)
}
