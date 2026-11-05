package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// L1 成本基管理（挂在 ChannelHandler 上，复用已有的 channelService 依赖）。
// 成本基不直接参与扣费：只用于 fx_auto 派生与毛利显示，删错也不影响账单。

type pricingCostResponse struct {
	ID                int64   `json:"id"`
	Vendor            string  `json:"vendor"`
	ModelPattern      string  `json:"model_pattern"`
	Currency          string  `json:"currency"`
	InputPerMTok      float64 `json:"input_per_mtok"`
	CachedPerMTok     float64 `json:"cached_per_mtok"`
	OutputPerMTok     float64 `json:"output_per_mtok"`
	CacheWritePerMTok float64 `json:"cache_write_per_mtok"`
	DiscountPct       float64 `json:"discount_pct"`
	ObservedAt        string  `json:"observed_at"`
	SourceURL         string  `json:"source_url"`
	Notes             string  `json:"notes"`
}

// ListPriceCosts 返回全部成本基。
// GET /api/v1/admin/settings/price-costs
func (h *ChannelHandler) ListPriceCosts(c *gin.Context) {
	costs, err := h.channelService.ListPriceCosts(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]pricingCostResponse, 0, len(costs))
	for _, cost := range costs {
		out = append(out, pricingCostResponse{
			ID:                cost.ID,
			Vendor:            cost.Vendor,
			ModelPattern:      cost.ModelPattern,
			Currency:          string(cost.Currency),
			InputPerMTok:      cost.InputPerMTok,
			CachedPerMTok:     cost.CachedPerMTok,
			OutputPerMTok:     cost.OutputPerMTok,
			CacheWritePerMTok: cost.CacheWritePerMTok,
			DiscountPct:       cost.DiscountPct,
			ObservedAt:        cost.ObservedAt.Format("2006-01-02T15:04:05Z"),
			SourceURL:         cost.SourceURL,
			Notes:             cost.Notes,
		})
	}
	response.Success(c, gin.H{"items": out})
}

type upsertPricingCostRequest struct {
	Vendor            string   `json:"vendor" binding:"required,max=64"`
	ModelPattern      string   `json:"model_pattern" binding:"required,max=200"`
	Currency          string   `json:"currency" binding:"omitempty,oneof=USD CNY"`
	InputPerMTok      float64  `json:"input_per_mtok" binding:"gte=0"`
	CachedPerMTok     float64  `json:"cached_per_mtok" binding:"gte=0"`
	OutputPerMTok     float64  `json:"output_per_mtok" binding:"gte=0"`
	CacheWritePerMTok *float64 `json:"cache_write_per_mtok"`
	DiscountPct       float64  `json:"discount_pct" binding:"gte=0,lte=100"`
	SourceURL         string   `json:"source_url"`
	Notes             string   `json:"notes"`
}

// UpsertPriceCost 幂等写入成本基（vendor + model_pattern 唯一）。
// POST /api/v1/admin/settings/price-costs
func (h *ChannelHandler) UpsertPriceCost(c *gin.Context) {
	var req upsertPricingCostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_PRICE_COST", err.Error()))
		return
	}
	currency := domain.PricingCurrencyUSD
	if req.Currency == string(domain.PricingCurrencyCNY) {
		currency = domain.PricingCurrencyCNY
	}
	cost := &domain.PriceCost{
		Vendor:            req.Vendor,
		ModelPattern:      req.ModelPattern,
		Currency:          currency,
		InputPerMTok:      req.InputPerMTok,
		CachedPerMTok:     req.CachedPerMTok,
		OutputPerMTok:     req.OutputPerMTok,
		CacheWritePerMTok: req.InputPerMTok,
		DiscountPct:       req.DiscountPct,
		SourceURL:         req.SourceURL,
		Notes:             req.Notes,
	}
	if req.CacheWritePerMTok != nil {
		cost.CacheWritePerMTok = *req.CacheWritePerMTok
	}
	saved, err := h.channelService.UpsertPriceCost(c.Request.Context(), cost)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": saved.ID})
}

// DeletePriceCost 删除一条成本基。
// DELETE /api/v1/admin/settings/price-costs/:id
func (h *ChannelHandler) DeletePriceCost(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_PRICE_COST_ID", "invalid id"))
		return
	}
	if err := h.channelService.DeletePriceCost(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}
