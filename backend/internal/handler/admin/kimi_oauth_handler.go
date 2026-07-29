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

type KimiOAuthHandler struct {
	kimiOAuthService *service.KimiOAuthService
	adminService     service.AdminService
	tokenInvalidator service.TokenCacheInvalidator
}

func NewKimiOAuthHandler(kimiOAuthService *service.KimiOAuthService, adminService service.AdminService, tokenInvalidator service.TokenCacheInvalidator) *KimiOAuthHandler {
	return &KimiOAuthHandler{kimiOAuthService: kimiOAuthService, adminService: adminService, tokenInvalidator: tokenInvalidator}
}

func (h *KimiOAuthHandler) StartDeviceAuthorization(c *gin.Context) {
	var req struct {
		ProxyID *int64 `json:"proxy_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.kimiOAuthService.StartDeviceAuthorization(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *KimiOAuthHandler) DeviceAuthorizationStatus(c *gin.Context) {
	result, err := h.kimiOAuthService.GetDeviceAuthorizationStatus(c.Request.Context(), c.Query("session_id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *KimiOAuthHandler) CancelDeviceAuthorization(c *gin.Context) {
	if err := h.kimiOAuthService.CancelDeviceAuthorization(c.Request.Context(), c.Query("session_id")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *KimiOAuthHandler) CreateAccountFromDevice(c *gin.Context) {
	var req struct {
		SessionID   string  `json:"session_id" binding:"required"`
		Name        string  `json:"name"`
		Notes       *string `json:"notes"`
		ProxyID     *int64  `json:"proxy_id"`
		Concurrency int     `json:"concurrency"`
		Priority    int     `json:"priority"`
		GroupIDs    []int64 `json:"group_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	token, err := h.kimiOAuthService.ConsumeAuthorizedSession(c.Request.Context(), req.SessionID)
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
		name = "Kimi OAuth Account"
	}
	account, err := h.adminService.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
		Name: name, Notes: req.Notes, Platform: service.PlatformKimi, Type: service.AccountTypeOAuth,
		Credentials: h.kimiOAuthService.BuildAccountCredentials(token), ProxyID: token.ProxyID,
		Concurrency: req.Concurrency, Priority: req.Priority, GroupIDs: req.GroupIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

func (h *KimiOAuthHandler) ReauthorizeAccountFromDevice(c *gin.Context) {
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
	if account.Platform != service.PlatformKimi || account.Type != service.AccountTypeOAuth {
		response.BadRequest(c, "Account is not a Kimi OAuth account")
		return
	}
	token, err := h.kimiOAuthService.ConsumeAuthorizedSession(c.Request.Context(), req.SessionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !sameOptionalInt64(account.ProxyID, token.ProxyID) {
		response.BadRequest(c, "proxy_id must match the proxy used for device authorization")
		return
	}
	credentials := service.MergeCredentials(account.Credentials, h.kimiOAuthService.BuildAccountCredentials(token))
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

func (h *KimiOAuthHandler) RefreshAccountToken(c *gin.Context) {
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
	if account.Platform != service.PlatformKimi || account.Type != service.AccountTypeOAuth {
		response.BadRequest(c, "Account is not a Kimi OAuth account")
		return
	}
	token, err := h.kimiOAuthService.RefreshAccountToken(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	credentials := service.MergeCredentials(account.Credentials, h.kimiOAuthService.BuildAccountCredentials(token))
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
