package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type OpenAIOAuthClient interface {
	ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*domain.OpenAITokenResponse, error)
	RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*domain.OpenAITokenResponse, error)
	RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*domain.OpenAITokenResponse, error)
}

type OpenAIDeviceOAuthClient interface {
	RequestDeviceCode(ctx context.Context, proxyURL, clientID string) (*domain.OpenAIDeviceCodeResponse, error)
	PollDeviceAuthorization(ctx context.Context, deviceAuthID, userCode, proxyURL string) (*domain.OpenAIDeviceAuthorizationResponse, error)
}

type GrokOAuthClient interface {
	ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*domain.GrokTokenResponse, error)
	RefreshToken(ctx context.Context, refreshToken, proxyURL, clientID string) (*domain.GrokTokenResponse, error)
	RefreshTokenAtEndpoint(ctx context.Context, refreshToken, tokenEndpoint, proxyURL, clientID string) (*domain.GrokTokenResponse, error)
	StartDeviceFlow(ctx context.Context, proxyURL, clientID, scope string) (*domain.GrokDeviceCodeResponse, error)
	PollDeviceToken(ctx context.Context, deviceCode, tokenEndpoint, proxyURL, clientID string) (*domain.GrokDeviceTokenResponse, error)
}

type KimiOAuthClient interface {
	StartDeviceFlow(ctx context.Context, proxyURL string, headers domain.KimiDeviceHeaders) (*domain.KimiDeviceCodeResponse, error)
	PollDeviceToken(ctx context.Context, deviceCode, proxyURL string, headers domain.KimiDeviceHeaders) (*domain.KimiOAuthClientTokenResponse, error)
	RefreshToken(ctx context.Context, refreshToken, proxyURL string, headers domain.KimiDeviceHeaders) (*domain.KimiOAuthClientTokenResponse, error)
}
