package admin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type GeminiOAuthHandler struct {
	geminiOAuthService *service.GeminiOAuthService
}

func NewGeminiOAuthHandler(geminiOAuthService *service.GeminiOAuthService) *GeminiOAuthHandler {
	return &GeminiOAuthHandler{geminiOAuthService: geminiOAuthService}
}

// GetCapabilities returns the Gemini OAuth configuration capabilities.
// GET /api/v1/admin/gemini/oauth/capabilities
func (h *GeminiOAuthHandler) GetCapabilities(c *gin.Context) {
	response.Error(c, http.StatusGone, "Gemini platform/OAuth has been retired")
	return
}

type GeminiGenerateAuthURLRequest struct {
	ProxyID   *int64 `json:"proxy_id"`
	ProjectID string `json:"project_id"`
	// OAuth 类型: "code_assist" (需要 project_id) 或 "ai_studio" (不需要 project_id)
	// 默认为 "code_assist" 以保持向后兼容
	OAuthType string `json:"oauth_type"`
	// TierID is a user-selected tier to be used when auto detection is unavailable or fails.
	TierID string `json:"tier_id"`
}

// GenerateAuthURL generates Google OAuth authorization URL for Gemini.
// POST /api/v1/admin/gemini/oauth/auth-url
func (h *GeminiOAuthHandler) GenerateAuthURL(c *gin.Context) {
	response.Error(c, http.StatusGone, "Gemini platform/OAuth has been retired")
	return
}

type GeminiExchangeCodeRequest struct {
	SessionID string `json:"session_id" binding:"required"`
	State     string `json:"state" binding:"required"`
	Code      string `json:"code" binding:"required"`
	ProxyID   *int64 `json:"proxy_id"`
	// OAuth 类型: "code_assist" 或 "ai_studio"，需要与 GenerateAuthURL 时的类型一致
	OAuthType string `json:"oauth_type"`
	// TierID is a user-selected tier to be used when auto detection is unavailable or fails.
	// This field is optional; when omitted, the server uses the tier stored in the OAuth session.
	TierID string `json:"tier_id"`
}

// ExchangeCode exchanges authorization code for tokens.
// POST /api/v1/admin/gemini/oauth/exchange-code
func (h *GeminiOAuthHandler) ExchangeCode(c *gin.Context) {
	response.Error(c, http.StatusGone, "Gemini platform/OAuth has been retired")
	return
}

func deriveGeminiRedirectURI(c *gin.Context) string {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin != "" {
		return strings.TrimRight(origin, "/") + "/auth/callback"
	}

	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if xfProto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); xfProto != "" {
		scheme = strings.TrimSpace(strings.Split(xfProto, ",")[0])
	}

	host := strings.TrimSpace(c.Request.Host)
	if xfHost := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); xfHost != "" {
		host = strings.TrimSpace(strings.Split(xfHost, ",")[0])
	}

	return fmt.Sprintf("%s://%s/auth/callback", scheme, host)
}
