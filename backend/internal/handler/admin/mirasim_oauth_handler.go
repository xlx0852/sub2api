package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// MirasimOAuthHandler 暴露 mirasim 授权登录（取 refresh token）的管理端接口。
//
// 关键约束（改动前请先读）：
//   - 这些接口只生产凭据，不落库、不创建账号；凭据由前端回填账号表单，最终走
//     既有的 POST /admin/accounts 路径。
//   - 绝不把 refresh token 写日志，也不在错误信息里回显它。响应体里会带上
//     refresh_token（管理员自己发起、自己接收，与手工粘贴等价）。
//   - 登录环节不做任何 refresh / 轮换调用（见 mirasim_oauth_service.go 头部约束）。
type MirasimOAuthHandler struct {
	mirasimOAuthService *service.MirasimOAuthService
}

func NewMirasimOAuthHandler(mirasimOAuthService *service.MirasimOAuthService) *MirasimOAuthHandler {
	return &MirasimOAuthHandler{mirasimOAuthService: mirasimOAuthService}
}

type mirasimOAuthProviderRequest struct {
	Provider string `json:"provider"`
	AuthURL  string `json:"auth_url"`
	ProxyID  *int64 `json:"proxy_id"`
}

// ListProviders 返回上游支持的登录方式（github / google）。
func (h *MirasimOAuthHandler) ListProviders(c *gin.Context) {
	var proxyID *int64
	if raw := strings.TrimSpace(c.Query("proxy_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid proxy_id")
			return
		}
		proxyID = &parsed
	}
	result, err := h.mirasimOAuthService.ListProviders(c.Request.Context(), c.Query("auth_url"), proxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"providers": result})
}

// GenerateAuthURL 生成浏览器授权链接（redirect_uri 固定为 loopback 占位符）。
func (h *MirasimOAuthHandler) GenerateAuthURL(c *gin.Context) {
	var req mirasimOAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.mirasimOAuthService.GenerateAuthURL(c.Request.Context(), req.Provider, req.AuthURL, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type mirasimCompleteRequest struct {
	// CallbackURL 是管理员从浏览器地址栏复制的完整回调链接（内含 refresh token）。
	CallbackURL string `json:"callback_url"`
	// RefreshToken 允许直接粘贴 token 原文（回调链接拿不到时的兜底）。
	RefreshToken string `json:"refresh_token"`
	Provider     string `json:"provider"`
}

// Complete 从回调链接或裸 token 里取出 refresh token 并做本地校验。
func (h *MirasimOAuthHandler) Complete(c *gin.Context) {
	var req mirasimCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	callbackURL := strings.TrimSpace(req.CallbackURL)
	refreshToken := strings.TrimSpace(req.RefreshToken)
	switch {
	case callbackURL != "":
		result, err := h.mirasimOAuthService.CompleteWithCallbackURL(callbackURL, req.Provider)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, result)
	case refreshToken != "":
		result, err := h.mirasimOAuthService.CompleteWithRefreshToken(refreshToken, req.Provider)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, result)
	default:
		response.BadRequest(c, "callback_url 与 refresh_token 至少需要一个")
	}
}

type mirasimEmailStartRequest struct {
	Email   string `json:"email"`
	AuthURL string `json:"auth_url"`
	ProxyID *int64 `json:"proxy_id"`
}

// StartEmailCode 触发上游邮箱验证码。
func (h *MirasimOAuthHandler) StartEmailCode(c *gin.Context) {
	var req mirasimEmailStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.mirasimOAuthService.StartEmailCode(c.Request.Context(), req.Email, req.AuthURL, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type mirasimEmailVerifyRequest struct {
	Email   string `json:"email"`
	Code    string `json:"code"`
	AuthURL string `json:"auth_url"`
	ProxyID *int64 `json:"proxy_id"`
}

// CompleteEmailCode 用邮箱 + 验证码换 refresh token。
func (h *MirasimOAuthHandler) CompleteEmailCode(c *gin.Context) {
	var req mirasimEmailVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.mirasimOAuthService.CompleteEmailCode(c.Request.Context(), req.Email, req.Code, req.AuthURL, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
