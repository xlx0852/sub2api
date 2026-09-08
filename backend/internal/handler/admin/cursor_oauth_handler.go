package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CursorOAuthHandler struct {
	cursorOAuthService *service.CursorOAuthService
	adminService       service.AdminService
	tokenInvalidator   service.TokenCacheInvalidator
}

func NewCursorOAuthHandler(
	cursorOAuthService *service.CursorOAuthService,
	adminService service.AdminService,
	tokenInvalidator service.TokenCacheInvalidator,
) *CursorOAuthHandler {
	return &CursorOAuthHandler{
		cursorOAuthService: cursorOAuthService,
		adminService:       adminService,
		tokenInvalidator:   tokenInvalidator,
	}
}

func (h *CursorOAuthHandler) StartDeviceAuthorization(c *gin.Context) {
	var req struct {
		ProxyID *int64 `json:"proxy_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.cursorOAuthService.StartAuthorization(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *CursorOAuthHandler) DeviceAuthorizationStatus(c *gin.Context) {
	result, err := h.cursorOAuthService.GetAuthorizationStatus(c.Request.Context(), c.Query("session_id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *CursorOAuthHandler) CancelDeviceAuthorization(c *gin.Context) {
	if err := h.cursorOAuthService.CancelAuthorization(c.Request.Context(), c.Query("session_id")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *CursorOAuthHandler) CreateAccountFromDevice(c *gin.Context) {
	var req struct {
		SessionID    string         `json:"session_id" binding:"required"`
		Name         string         `json:"name"`
		Notes        *string        `json:"notes"`
		ProxyID      *int64         `json:"proxy_id"`
		Concurrency  int            `json:"concurrency"`
		Priority     int            `json:"priority"`
		GroupIDs     []int64        `json:"group_ids"`
		ModelMapping map[string]any `json:"model_mapping"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	token, err := h.cursorOAuthService.ConsumeAuthorization(c.Request.Context(), req.SessionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !sameOptionalInt64(req.ProxyID, token.ProxyID) {
		response.BadRequest(c, "proxy_id must match the proxy used for device authorization")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		if token.Email != "" {
			name = "Cursor (" + token.Email + ")"
		} else {
			name = "Cursor OAuth Account"
		}
	}
	credentials := h.cursorOAuthService.BuildAccountCredentials(token)
	if len(req.ModelMapping) > 0 {
		credentials["model_mapping"] = req.ModelMapping
	}
	account, err := h.adminService.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
		Name:        name,
		Notes:       req.Notes,
		Platform:    service.PlatformCursor,
		Type:        service.AccountTypeOAuth,
		Credentials: credentials,
		ProxyID:     token.ProxyID,
		Concurrency: req.Concurrency,
		Priority:    req.Priority,
		GroupIDs:    req.GroupIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

func (h *CursorOAuthHandler) ReauthorizeAccountFromDevice(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var req struct {
		SessionID string `json:"session_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if account.Platform != service.PlatformCursor || account.Type != service.AccountTypeOAuth {
		response.BadRequest(c, "Account is not a Cursor OAuth account")
		return
	}
	token, err := h.cursorOAuthService.ConsumeAuthorization(c.Request.Context(), req.SessionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !sameOptionalInt64(account.ProxyID, token.ProxyID) {
		response.BadRequest(c, "proxy_id must match the proxy used for device authorization")
		return
	}
	credentials := service.MergeCredentials(account.Credentials, h.cursorOAuthService.BuildAccountCredentials(token))
	updated, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Type:        service.AccountTypeOAuth,
		Credentials: credentials,
		ProxyID:     token.ProxyID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.tokenInvalidator != nil {
		_ = h.tokenInvalidator.InvalidateToken(c.Request.Context(), updated)
	}
	updated, err = h.adminService.ClearAccountError(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(updated))
}

func (h *CursorOAuthHandler) RefreshAccountToken(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if account.Platform != service.PlatformCursor || account.Type != service.AccountTypeOAuth {
		response.BadRequest(c, "Account is not a Cursor OAuth account")
		return
	}
	token, err := h.cursorOAuthService.RefreshAccountToken(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	credentials := service.MergeCredentials(account.Credentials, h.cursorOAuthService.BuildAccountCredentials(token))
	updated, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{Credentials: credentials})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.tokenInvalidator != nil {
		_ = h.tokenInvalidator.InvalidateToken(c.Request.Context(), updated)
	}
	if cleared, clearErr := h.adminService.ClearAccountError(c.Request.Context(), accountID); clearErr == nil {
		updated = cleared
	}
	response.Success(c, dto.AccountFromService(updated))
}

func (h *CursorOAuthHandler) ExchangeUserAPIKey(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var req struct {
		APIKey string `json:"api_key"`
	}
	_ = c.ShouldBindJSON(&req)

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if account.Platform != service.PlatformCursor {
		response.BadRequest(c, "Account is not a Cursor account")
		return
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(account.GetCredential("api_key"))
	}
	if apiKey == "" {
		response.BadRequest(c, "api_key is required")
		return
	}

	tokenInfo, err := h.cursorOAuthService.ExchangeUserAPIKey(c.Request.Context(), apiKey, account.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	credentials := service.MergeCredentials(account.Credentials, h.cursorOAuthService.BuildAccountCredentials(tokenInfo))
	credentials["api_key"] = apiKey
	updated, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{Credentials: credentials})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.tokenInvalidator != nil {
		_ = h.tokenInvalidator.InvalidateToken(c.Request.Context(), updated)
	}
	response.Success(c, dto.AccountFromService(updated))
}
