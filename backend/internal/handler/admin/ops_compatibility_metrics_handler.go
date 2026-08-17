package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// GetOpenAICompatibilityFallbackMetrics returns process-local compatibility
// counters used to decide whether legacy sticky reads and writes can retire.
// GET /api/v1/admin/ops/runtime/openai-compatibility-fallbacks
func (h *OpsHandler) GetOpenAICompatibilityFallbackMetrics(c *gin.Context) {
	if h == nil || h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.opsService.GetOpenAICompatibilityFallbackMetrics())
}
