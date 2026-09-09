package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *OpenAIGatewayHandler) CursorModels(c *gin.Context) {
	v, ok := c.Get(string(middleware.ContextKeyAPIKey))
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}
	key, ok := v.(*service.APIKey)
	if !ok || key.GroupID == nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	models, err := h.gatewayService.CursorModels(c.Request.Context(), *key.GroupID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"type": "upstream_error", "message": "Unable to retrieve Cursor models"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": models})
}
