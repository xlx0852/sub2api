// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// GeminiOAuthClient is retained as a no-op port for wire/DI.
type GeminiOAuthClient interface {
	ExchangeCode(ctx context.Context, oauthType, code, codeVerifier, redirectURI, proxyURL string) (*domain.GeminiTokenResponse, error)
	RefreshToken(ctx context.Context, oauthType, refreshToken, proxyURL string) (*domain.GeminiTokenResponse, error)
}
