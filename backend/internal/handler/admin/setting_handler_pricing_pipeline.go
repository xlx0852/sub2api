package admin

import (
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PricingPipelineSettings 是定价开关的后台视图（管线模式 + 价格来源）。
type PricingPipelineSettings struct {
	// Mode 是当前生效的模式：legacy / shadow / unified。
	Mode string `json:"mode"`
	// Configured 是设置项里显式配置的值（空串表示跟随环境变量默认值）。
	Configured string `json:"configured"`
	// Modes 列出可选值，供前端渲染选择器。
	Modes []string `json:"modes"`
	// PricingSource 是价格来源：legacy（现有链路）/ dual（双算打点）/ model_prices（注册表）。
	PricingSource string `json:"pricing_source"`
	// PricingSources 列出可选值。
	PricingSources []string `json:"pricing_sources"`
	// RegistrySource 是模型注册表来源：file（编译进二进制的 catalog）/ dual / db。
	RegistrySource  string   `json:"registry_source"`
	RegistrySources []string `json:"registry_sources"`
}

// GetPricingPipelineSettings 读取定价管线开关。
// GET /api/v1/admin/settings/pricing-pipeline
func (h *SettingHandler) GetPricingPipelineSettings(c *gin.Context) {
	ctx := c.Request.Context()
	raw, err := h.settingService.GetRawValue(ctx, service.SettingKeyPricingPipeline)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	mode := service.NormalizePricingPipelineMode(raw)
	if strings.TrimSpace(raw) == "" {
		// 未配置时回落到环境变量默认值（legacy）。
		mode = service.NormalizePricingPipelineMode("")
	}
	pricingSource, err := h.settingService.GetRawValue(ctx, service.SettingKeyModelPricingSource)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	registrySource, err := h.settingService.GetRawValue(ctx, service.SettingKeyModelRegistrySource)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, PricingPipelineSettings{
		Mode:            mode,
		Configured:      strings.TrimSpace(raw),
		Modes:           []string{service.PricingPipelineLegacy, service.PricingPipelineShadow, service.PricingPipelineUnified},
		PricingSource:   service.NormalizeModelPricingSourceForAPI(pricingSource),
		PricingSources:  []string{service.PricingModelSourceLegacy, service.PricingModelSourceDual, service.PricingModelSourceRegistry},
		RegistrySource:  service.NormalizeModelRegistrySourceForAPI(registrySource),
		RegistrySources: []string{service.ModelRegistrySourceFile, service.ModelRegistrySourceDual, service.ModelRegistrySourceDB},
	})
}

// updatePricingPipelineSettingsRequest 更新定价管线开关请求。
type updatePricingPipelineSettingsRequest struct {
	// 三个字段至少给一个；分别独立更新，互不影响。
	Mode           string `json:"mode" binding:"omitempty,oneof=legacy shadow unified"`
	PricingSource  string `json:"pricing_source" binding:"omitempty,oneof=legacy dual model_prices"`
	RegistrySource string `json:"registry_source" binding:"omitempty,oneof=file dual db"`
}

// UpdatePricingPipelineSettings 切换定价管线：
// legacy 保持现有 11 层路径，shadow 双算打点但按 legacy 扣费，unified 全量走统一管线。
// PUT /api/v1/admin/settings/pricing-pipeline
func (h *SettingHandler) UpdatePricingPipelineSettings(c *gin.Context) {
	var req updatePricingPipelineSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("INVALID_PRICING_PIPELINE_MODE",
			"mode must be one of legacy, shadow, unified"))
		return
	}
	if strings.TrimSpace(req.Mode) == "" && strings.TrimSpace(req.PricingSource) == "" && strings.TrimSpace(req.RegistrySource) == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("EMPTY_SETTINGS",
			"provide mode and/or pricing_source"))
		return
	}
	ctx := c.Request.Context()
	if strings.TrimSpace(req.Mode) != "" {
		mode := service.NormalizePricingPipelineMode(req.Mode)
		if err := h.settingService.SetRawValue(ctx, service.SettingKeyPricingPipeline, mode); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	if strings.TrimSpace(req.PricingSource) != "" {
		src := service.NormalizeModelPricingSourceForAPI(req.PricingSource)
		if err := h.settingService.SetRawValue(ctx, service.SettingKeyModelPricingSource, src); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	if strings.TrimSpace(req.RegistrySource) != "" {
		src := service.NormalizeModelRegistrySourceForAPI(req.RegistrySource)
		if err := h.settingService.SetRawValue(ctx, service.SettingKeyModelRegistrySource, src); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	h.GetPricingPipelineSettings(c)
}
