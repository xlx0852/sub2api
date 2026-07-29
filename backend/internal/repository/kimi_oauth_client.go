package repository

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/imroc/req/v3"
)

type kimiOAuthClient struct{}

func NewKimiOAuthClient() service.KimiOAuthClient { return &kimiOAuthClient{} }

func (c *kimiOAuthClient) StartDeviceFlow(ctx context.Context, proxyURL string, headers kimi.DeviceHeaders) (*kimi.DeviceCodeResponse, error) {
	client, err := createKimiReqClient(proxyURL)
	if err != nil {
		return nil, kimiClientInitError(err)
	}
	form := url.Values{}
	form.Set("client_id", kimi.ClientID)
	var result kimi.DeviceCodeResponse
	resp, err := client.R().SetContext(ctx).SetHeaders(kimiRequestHeaders(headers)).SetFormDataFromValues(form).SetSuccessResult(&result).Post(kimi.DeviceAuthorizationURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_REQUEST_FAILED", "device authorization request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, kimiOAuthStatusError("KIMI_OAUTH_DEVICE_AUTHORIZATION_FAILED", "device authorization failed", resp)
	}
	return &result, nil
}

func (c *kimiOAuthClient) PollDeviceToken(ctx context.Context, deviceCode, proxyURL string, headers kimi.DeviceHeaders) (*kimi.TokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", kimi.ClientID)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", kimi.DeviceCodeGrantType)
	return c.requestToken(ctx, form, proxyURL, headers, "KIMI_OAUTH_DEVICE_TOKEN_FAILED")
}

func (c *kimiOAuthClient) RefreshToken(ctx context.Context, refreshToken, proxyURL string, headers kimi.DeviceHeaders) (*kimi.TokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", kimi.ClientID)
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")
	return c.requestToken(ctx, form, proxyURL, headers, "KIMI_OAUTH_TOKEN_REFRESH_FAILED")
}

func (c *kimiOAuthClient) requestToken(ctx context.Context, form url.Values, proxyURL string, headers kimi.DeviceHeaders, code string) (*kimi.TokenResponse, error) {
	client, err := createKimiReqClient(proxyURL)
	if err != nil {
		return nil, kimiClientInitError(err)
	}
	var success kimi.TokenResponse
	var failure kimi.TokenResponse
	resp, err := client.R().SetContext(ctx).SetHeaders(kimiRequestHeaders(headers)).SetFormDataFromValues(form).
		SetSuccessResult(&success).SetErrorResult(&failure).Post(kimi.TokenURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_REQUEST_FAILED", "token request failed: %v", err)
	}
	if resp.IsSuccessState() {
		return &success, nil
	}
	if strings.TrimSpace(failure.Error) != "" {
		return &failure, nil
	}
	return nil, kimiOAuthStatusError(code, "token request failed", resp)
}

func createKimiReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{ProxyURL: proxyURL, Timeout: 60 * time.Second})
}

func kimiRequestHeaders(headers kimi.DeviceHeaders) map[string]string {
	return map[string]string{
		"Accept": "application/json", "User-Agent": kimi.ClientUserAgent,
		"X-Msh-Platform": kimi.ClientPlatform, "X-Msh-Version": kimi.ClientVersion,
		"X-Msh-Device-Name": headers.DeviceName, "X-Msh-Device-Model": headers.DeviceModel, "X-Msh-Device-Id": headers.DeviceID,
	}
}

func kimiClientInitError(err error) error {
	return infraerrors.Newf(http.StatusBadGateway, "KIMI_OAUTH_CLIENT_INIT_FAILED", "create Kimi OAuth client: %v", err)
}

func kimiOAuthStatusError(code, message string, resp *req.Response) error {
	status := 0
	body := ""
	if resp != nil {
		status = resp.StatusCode
		body = logredact.RedactText(resp.String())
	}
	return infraerrors.Newf(http.StatusBadGateway, code, "%s: status %d, body: %s", message, status, body)
}
