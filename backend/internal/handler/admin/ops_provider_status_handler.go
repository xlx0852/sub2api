package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpsHandler) GetProviderStatus(c *gin.Context) {
	if h == nil || h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	provider := strings.TrimSpace(c.Param("provider"))
	result, err := h.opsService.GetProviderStatusCurrent(c.Request.Context(), provider)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *OpsHandler) ListProviderStatusHistory(c *gin.Context) {
	if h == nil || h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	filter := &service.ProviderStatusHistoryFilter{Provider: strings.TrimSpace(c.Param("provider")), Limit: 200}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 && value <= 500 {
			filter.Limit = value
		}
	}
	if raw := strings.TrimSpace(c.Query("start_time")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid start_time")
			return
		}
		filter.StartTime = &value
	}
	if raw := strings.TrimSpace(c.Query("end_time")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid end_time")
			return
		}
		filter.EndTime = &value
	}
	items, err := h.opsService.ListProviderStatusHistory(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}
