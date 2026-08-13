package domain

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/Wei-Shaw/sub2api/internal/pkg/oauth"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

// P1 过渡别名：ports 只允许依赖 domain，OAuth/TLS 运行时类型仍住在 pkg。
// 类型别名保持与 pkg 完全同一类型，repository 返回值无需改签名。

type OpenAITokenResponse = openai.TokenResponse
type OpenAIDeviceCodeResponse = openai.DeviceCodeResponse
type OpenAIDeviceAuthorizationResponse = openai.DeviceAuthorizationResponse

type GrokTokenResponse = xai.TokenResponse
type GrokDeviceCodeResponse = xai.DeviceCodeResponse
type GrokDeviceTokenResponse = xai.DeviceTokenResponse

type ClaudeOAuthTokenResponse = oauth.TokenResponse

type KimiDeviceCodeResponse = kimi.DeviceCodeResponse
type KimiOAuthClientTokenResponse = kimi.TokenResponse
type KimiDeviceHeaders = kimi.DeviceHeaders

type TLSFingerprintRuntimeProfile = tlsfingerprint.Profile
