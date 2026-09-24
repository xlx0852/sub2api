package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/ports"
)

// 上游 OAuth 客户端端口的 service 层别名。
//
// 注：Claude（Anthropic）的 OAuth 客户端端口已随 Claude OAuth 登录能力下线一并删除
// （见 openspec/changes/remove-claude-oauth-login）。
type OpenAIOAuthClient = ports.OpenAIOAuthClient
type OpenAIDeviceOAuthClient = ports.OpenAIDeviceOAuthClient
type GrokOAuthClient = ports.GrokOAuthClient

// GrokOAuthTokenService is the narrow refresh port used by Grok token providers.
type GrokOAuthTokenService interface {
	RefreshAccountToken(ctx context.Context, account *Account) (*GrokTokenInfo, error)
	BuildAccountCredentials(tokenInfo *GrokTokenInfo) map[string]any
}
