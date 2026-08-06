package repository

import (
	"context"
	"fmt"

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
