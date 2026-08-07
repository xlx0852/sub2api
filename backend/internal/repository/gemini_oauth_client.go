package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/imroc/req/v3"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type geminiOAuthClient struct{}

func NewGeminiOAuthClient(_ *config.Config) service.GeminiOAuthClient {
	return &geminiOAuthClient{}
}

func (c *geminiOAuthClient) ExchangeCode(ctx context.Context, oauthType, code, codeVerifier, redirectURI, proxyURL string) (*service.GeminiTokenResponse, error) {
	return nil, fmt.Errorf("gemini oauth retired")
}

func (c *geminiOAuthClient) RefreshToken(ctx context.Context, oauthType, refreshToken, proxyURL string) (*service.GeminiTokenResponse, error) {
	return nil, fmt.Errorf("gemini oauth retired")
}

// createGeminiReqClient builds the shared HTTP client used by Gemini outbound calls.
// Kept minimal here; tests assert HTTP/2 forcing stays disabled for Gemini.
func createGeminiReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  60 * time.Second,
	})
}
