package service

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

var errGeminiRetired = fmt.Errorf("gemini platform/OAuth has been retired (Gemini CLI discontinued)")

// GeminiOAuthService is a retired stub kept for DI/wire compatibility.
type GeminiOAuthService struct{}

type GeminiOAuthCapabilities struct {
	AIStudioOAuthEnabled bool     `json:"ai_studio_oauth_enabled"`
	RequiredRedirectURIs []string `json:"required_redirect_uris"`
}

type GeminiAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
}

type GeminiExchangeCodeInput struct {
	ProxyID      *int64
	Code         string
	State        string
	SessionID    string
	RedirectURI  string
	OAuthType    string
	ProjectID    string
	TierID       string
	CodeVerifier string
}

type GeminiTokenInfo struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	TokenType    string
	Scope        string
	Email        string
	ProjectID    string
	TierID       string
	OAuthType    string
}

func NewGeminiOAuthService(
	_ ProxyRepository,
	_ GeminiOAuthClient,
	_ GeminiCliCodeAssistClient,
	_ any,
	_ *config.Config,
) *GeminiOAuthService {
	return &GeminiOAuthService{}
}

func (s *GeminiOAuthService) Stop() {}

func (s *GeminiOAuthService) GetOAuthConfig() *GeminiOAuthCapabilities {
	return &GeminiOAuthCapabilities{AIStudioOAuthEnabled: false}
}

func (s *GeminiOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI, projectID, oauthType, tierID string) (*GeminiAuthURLResult, error) {
	return nil, errGeminiRetired
}

func (s *GeminiOAuthService) ExchangeCode(ctx context.Context, input *GeminiExchangeCodeInput) (*GeminiTokenInfo, error) {
	return nil, errGeminiRetired
}

func (s *GeminiOAuthService) RefreshToken(ctx context.Context, oauthType, refreshToken, proxyURL string) (*GeminiTokenInfo, error) {
	return nil, errGeminiRetired
}

func (s *GeminiOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*GeminiTokenInfo, error) {
	return nil, errGeminiRetired
}

func (s *GeminiOAuthService) BuildAccountCredentials(tokenInfo *GeminiTokenInfo) map[string]any {
	if tokenInfo == nil {
		return map[string]any{}
	}
	return map[string]any{
		"access_token":  tokenInfo.AccessToken,
		"refresh_token": tokenInfo.RefreshToken,
		"expires_in":    tokenInfo.ExpiresIn,
		"token_type":    tokenInfo.TokenType,
		"scope":         tokenInfo.Scope,
		"email":         tokenInfo.Email,
		"project_id":    tokenInfo.ProjectID,
		"tier_id":       tokenInfo.TierID,
		"oauth_type":    tokenInfo.OAuthType,
	}
}

func (s *GeminiOAuthService) FetchGoogleOneTier(ctx context.Context, accessToken, proxyURL string) (string, any, error) {
	return "", nil, errGeminiRetired
}

func (s *GeminiOAuthService) RefreshAccountGoogleOneTier(ctx context.Context, account *Account) error {
	return errGeminiRetired
}
