package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// GetPricingOverview 返回售价表总览（独立售价表页面用）。
// GET /api/v1/admin/sell-price-policies/overview
func (h *ChannelHandler) GetPricingOverview(c *gin.Context) {
	overview, err := h.channelService.BuildPricingOverview(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}
