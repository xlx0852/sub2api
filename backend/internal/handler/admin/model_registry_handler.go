package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// 模型注册表后台接口（model-registry 能力）。
// 价格写入的三条护栏在 service 层：未登记模型禁止配价、空价行拒绝、别名冲突拒绝。

type modelRegistryResponse struct {
	ID               string          `json:"id"`
	Platform         string          `json:"platform"`
	DisplayName      string          `json:"display_name"`
	Family           string          `json:"family"`
	OwnedBy          string          `json:"owned_by"`
	ContextWindow    *int            `json:"context_window"`
	MaxContextWindow *int            `json:"max_context_window"`
	Media            map[string]bool `json:"media"`
	IsReasoning      bool            `json:"is_reasoning"`
	Enabled          bool            `json:"enabled"`
	Retired          bool            `json:"retired"`
	IsDefaultChat    bool            `json:"is_default_chat"`
	IsDefaultTest    bool            `json:"is_default_test"`
	UpstreamID       string          `json:"upstream_id"`
	Version          int64           `json:"version"`
	Notes            string          `json:"notes"`
	PriceRowCount    int             `json:"price_row_count"`
}

// ListModels 返回模型清单。
// GET /api/v1/admin/models?platform=&search=
func (h *ChannelHandler) ListModels(c *gin.Context) {
	models, err := h.channelService.ListModelRegistry(c.Request.Context(),
		c.Query("platform"), c.Query("search"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]modelRegistryResponse, 0, len(models))
	for _, m := range models {
		out = append(out, modelRegistryResponse{
			ID: m.ID, Platform: m.Platform, DisplayName: m.DisplayName, Family: m.Family,
			OwnedBy: m.OwnedBy, ContextWindow: m.ContextWindow, MaxContextWindow: m.MaxContextWindow,
			Media: m.Media, IsReasoning: m.IsReasoning, Enabled: m.Enabled, Retired: m.Retired,
			IsDefaultChat: m.IsDefaultChat, IsDefaultTest: m.IsDefaultTest,
			UpstreamID: m.UpstreamID, Version: m.Version, Notes: m.Notes,
			PriceRowCount: m.PriceRowCount,
		})
	}
	response.Success(c, gin.H{"items": out})
}

type upsertModelRequest struct {
	ID               string          `json:"id" binding:"required,max=200"`
	Platform         string          `json:"platform" binding:"required,max=32"`
	DisplayName      string          `json:"display_name"`
	Family           string          `json:"family"`
	OwnedBy          string          `json:"owned_by"`
	ContextWindow    *int            `json:"context_window"`
	MaxContextWindow *int            `json:"max_context_window"`
	Media            map[string]bool `json:"media"`
	IsReasoning      bool            `json:"is_reasoning"`
	Enabled          *bool           `json:"enabled"`
	Retired          bool            `json:"retired"`
	IsDefaultChat    bool            `json:"is_default_chat"`
	IsDefaultTest    bool            `json:"is_default_test"`
	UpstreamID       string          `json:"upstream_id"`
	Notes            string          `json:"notes"`
	Version          int64           `json:"version"`
}

// UpsertModel 新增/更新模型。
// PUT /api/v1/admin/models
func (h *ChannelHandler) UpsertModel(c *gin.Context) {
	var req upsertModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_MODEL", err.Error()))
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	entry := &domain.ModelRegistryEntry{
		ID: req.ID, Platform: req.Platform, DisplayName: req.DisplayName, Family: req.Family,
		OwnedBy: req.OwnedBy, ContextWindow: req.ContextWindow, MaxContextWindow: req.MaxContextWindow,
		Media: req.Media, IsReasoning: req.IsReasoning, Enabled: enabled, Retired: req.Retired,
		IsDefaultChat: req.IsDefaultChat, IsDefaultTest: req.IsDefaultTest,
		UpstreamID: req.UpstreamID, Notes: req.Notes,
	}
	if err := h.channelService.UpsertModelRegistry(c.Request.Context(), entry, req.Version); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": entry.ID})
}

type modelAliasRequest struct {
	Aliases []struct {
		Alias    string `json:"alias" binding:"required,max=200"`
		Platform string `json:"platform" binding:"max=32"`
		Kind     string `json:"kind" binding:"omitempty,oneof=alias id_override id_reverse retired"`
	} `json:"aliases"`
}

// ListModelAliases 返回某模型的别名。
// GET /api/v1/admin/models/:id/aliases
func (h *ChannelHandler) ListModelAliases(c *gin.Context) {
	aliases, err := h.channelService.ListModelAliasesFor(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": aliases})
}

// ReplaceModelAliases 覆盖某模型的别名集合。
// PUT /api/v1/admin/models/:id/aliases
func (h *ChannelHandler) ReplaceModelAliases(c *gin.Context) {
	var req modelAliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_ALIAS", err.Error()))
		return
	}
	modelID := c.Param("id")
	aliases := make([]domain.ModelAliasEntry, 0, len(req.Aliases))
	for _, a := range req.Aliases {
		aliases = append(aliases, domain.ModelAliasEntry{
			Alias: a.Alias, Platform: a.Platform, ModelID: modelID, Kind: a.Kind,
		})
	}
	if err := h.channelService.ReplaceModelAliases(c.Request.Context(), modelID, aliases); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": modelID})
}

type upsertModelPriceRequest struct {
	// GroupID 为空表示"平台默认价"（兜底）。
	GroupID     *int64   `json:"group_id"`
	BillingMode string   `json:"billing_mode" binding:"omitempty,oneof=token per_request image"`
	Currency    string   `json:"currency" binding:"omitempty,oneof=USD CNY"`
	FxMode      string   `json:"fx_mode" binding:"omitempty,oneof=manual cny_native fx_auto"`
	CostID      *int64   `json:"cost_id"`
	FxRate      *float64 `json:"fx_rate"`
	MarkupPct   float64  `json:"markup_pct"`

	InputPrice          *float64 `json:"input_price"`
	OutputPrice         *float64 `json:"output_price"`
	CacheWritePrice     *float64 `json:"cache_write_price"`
	CacheReadPrice      *float64 `json:"cache_read_price"`
	InputPricePriority  *float64 `json:"input_price_priority"`
	OutputPricePriority *float64 `json:"output_price_priority"`
	ImageOutputPrice    *float64 `json:"image_output_price"`
	PerRequestPrice     *float64 `json:"per_request_price"`
}

// UpsertModelPrice 写入某模型的一条价格行。
// PUT /api/v1/admin/models/:id/price
func (h *ChannelHandler) UpsertModelPrice(c *gin.Context) {
	var req upsertModelPriceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_MODEL_PRICE", err.Error()))
		return
	}
	row := &domain.ModelPriceRow{
		ModelID:             strings.TrimSpace(c.Param("id")),
		GroupID:             req.GroupID,
		BillingMode:         req.BillingMode,
		Currency:            req.Currency,
		FxMode:              req.FxMode,
		CostID:              req.CostID,
		FxRate:              req.FxRate,
		MarkupPct:           req.MarkupPct,
		InputPrice:          req.InputPrice,
		OutputPrice:         req.OutputPrice,
		CacheWritePrice:     req.CacheWritePrice,
		CacheReadPrice:      req.CacheReadPrice,
		InputPricePriority:  req.InputPricePriority,
		OutputPricePriority: req.OutputPricePriority,
		ImageOutputPrice:    req.ImageOutputPrice,
		PerRequestPrice:     req.PerRequestPrice,
		Enabled:             true,
	}
	if err := h.channelService.UpsertModelPrice(c.Request.Context(), row); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"model_id": row.ModelID})
}

// DeleteModelPrice 删除某模型的一条价格行（?group_id= 为空表示默认价）。
// DELETE /api/v1/admin/models/:id/price
func (h *ChannelHandler) DeleteModelPrice(c *gin.Context) {
	var groupID *int64
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_ID", "invalid group_id"))
			return
		}
		groupID = &v
	}
	if err := h.channelService.DeleteModelPrice(c.Request.Context(), c.Param("id"), groupID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"model_id": c.Param("id")})
}

// modelPriceRowResponse 是价格行的后台视图（价格单位：每 token，前端乘 1e6 显示）。
type modelPriceRowResponse struct {
	ID                  int64    `json:"id"`
	ModelID             string   `json:"model_id"`
	GroupID             *int64   `json:"group_id"`
	BillingMode         string   `json:"billing_mode"`
	Currency            string   `json:"currency"`
	FxMode              string   `json:"fx_mode"`
	CostID              *int64   `json:"cost_id"`
	FxRate              *float64 `json:"fx_rate"`
	MarkupPct           float64  `json:"markup_pct"`
	InputPrice          *float64 `json:"input_price"`
	OutputPrice         *float64 `json:"output_price"`
	CacheWritePrice     *float64 `json:"cache_write_price"`
	CacheReadPrice      *float64 `json:"cache_read_price"`
	InputPricePriority  *float64 `json:"input_price_priority"`
	OutputPricePriority *float64 `json:"output_price_priority"`
	ImageOutputPrice    *float64 `json:"image_output_price"`
	PerRequestPrice     *float64 `json:"per_request_price"`
	Enabled             bool     `json:"enabled"`
	Version             int64    `json:"version"`
}

// ListModelPrices 返回某模型的全部价格行。
// GET /api/v1/admin/models/:id/prices
func (h *ChannelHandler) ListModelPrices(c *gin.Context) {
	rows, err := h.channelService.ListModelPricesFor(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]modelPriceRowResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, modelPriceRowResponse{
			ID: r.ID, ModelID: r.ModelID, GroupID: r.GroupID, BillingMode: r.BillingMode,
			Currency: r.Currency, FxMode: r.FxMode, CostID: r.CostID, FxRate: r.FxRate,
			MarkupPct: r.MarkupPct, InputPrice: r.InputPrice, OutputPrice: r.OutputPrice,
			CacheWritePrice: r.CacheWritePrice, CacheReadPrice: r.CacheReadPrice,
			InputPricePriority: r.InputPricePriority, OutputPricePriority: r.OutputPricePriority,
			ImageOutputPrice: r.ImageOutputPrice, PerRequestPrice: r.PerRequestPrice,
			Enabled: r.Enabled, Version: r.Version,
		})
	}
	response.Success(c, gin.H{"items": out})
}
