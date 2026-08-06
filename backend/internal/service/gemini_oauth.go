package service

import "context"

// GeminiTokenResponse is a minimal token payload retained for DI compatibility.
// Gemini CLI OAuth is retired; methods return errors.
type GeminiTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// GeminiOAuthClient is retained as a no-op port for wire/DI.
type GeminiOAuthClient interface {
	ExchangeCode(ctx context.Context, oauthType, code, codeVerifier, redirectURI, proxyURL string) (*GeminiTokenResponse, error)
	RefreshToken(ctx context.Context, oauthType, refreshToken, proxyURL string) (*GeminiTokenResponse, error)
}
