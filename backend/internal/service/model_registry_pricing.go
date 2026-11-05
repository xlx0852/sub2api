package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// 定价来源开关（注册表价格 vs 现有链路）。
const (
	PricingModelSourceLegacy   = "legacy"
	PricingModelSourceDual     = "dual"
	PricingModelSourceRegistry = "model_prices"

	// SettingKeyModelPricingSource 控制价格来源：legacy（现有链路）/ dual（双算打点）/
	// model_prices（只用 model_prices 表）。
	SettingKeyModelPricingSource = "model_pricing_source"
)

func normalizeModelPricingSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PricingModelSourceDual:
		return PricingModelSourceDual
	case PricingModelSourceRegistry, "registry":
		return PricingModelSourceRegistry
	default:
		return PricingModelSourceLegacy
	}
}

// modelPricingSourceCache 缓存设置项 10 秒，避免每请求查库。
type modelPricingSourceCache struct {
	mu       sync.RWMutex
	value    string
	loadedAt time.Time
}

func (s *ChannelService) modelPricingSource(ctx context.Context) string {
	// 注入的设置读取器由 SetSettingsReader 提供；未注入时永远 legacy。
	if s == nil || s.settingsReader == nil || ctx == nil {
		return PricingModelSourceLegacy
	}
	s.pricingSourceCache.mu.RLock()
	cached, loadedAt := s.pricingSourceCache.value, s.pricingSourceCache.loadedAt
	s.pricingSourceCache.mu.RUnlock()
	if !loadedAt.IsZero() && time.Since(loadedAt) < pipelineSettingTTL {
		return normalizeModelPricingSource(cached)
	}
	raw, err := s.settingsReader.GetRawValue(ctx, SettingKeyModelPricingSource)
	if err != nil {
		return normalizeModelPricingSource(cached)
	}
	s.pricingSourceCache.mu.Lock()
	s.pricingSourceCache.value = raw
	s.pricingSourceCache.loadedAt = time.Now()
	s.pricingSourceCache.mu.Unlock()
	return normalizeModelPricingSource(raw)
}

// SetSettingsReader 让渠道服务也能读运维开关（与 BillingService 共用同一个读取器）。
func (s *ChannelService) SetSettingsReader(reader billingSettingsReader) {
	if s == nil {
		return
	}
	s.settingsReader = reader
}

// GetModelPrice 返回模型在某分组下的注册表价格（精确分组行 → 默认行 → nil）。
func (s *ChannelService) GetModelPrice(ctx context.Context, modelID string, groupID *int64) *domain.ModelPriceRow {
	if s == nil || s.repo == nil || strings.TrimSpace(modelID) == "" {
		return nil
	}
	row, err := s.repo.GetModelPrice(ctx, modelID, groupID)
	if err != nil {
		slog.Warn("model registry price lookup failed", "model", modelID, "error", err)
		return nil
	}
	return row
}

// modelPriceRowToPricing 把注册表价格行转成计费用的 ModelPricing。
// fx_auto 行在这里完成派生（成本基 × 汇率 × (1+加价)）。
func (s *ChannelService) modelPriceRowToPricing(ctx context.Context, row *domain.ModelPriceRow) (*ModelPricing, error) {
	if row == nil {
		return nil, ErrPriceCostMissing
	}
	currency := domain.NormalizePricingCurrency(row.Currency)
	pricing := &ModelPricing{Currency: currency}

	if row.InputPrice != nil {
		pricing.InputPricePerToken = *row.InputPrice
	}
	if row.OutputPrice != nil {
		pricing.OutputPricePerToken = *row.OutputPrice
	}
	if row.CacheReadPrice != nil {
		pricing.CacheReadPricePerToken = *row.CacheReadPrice
	}
	if row.CacheWritePrice != nil {
		pricing.CacheCreationPricePerToken = *row.CacheWritePrice
		pricing.CacheCreationPriceExplicit = true
	}
	if row.InputPricePriority != nil {
		pricing.InputPricePerTokenPriority = *row.InputPricePriority
	} else {
		pricing.InputPricePerTokenPriority = pricing.InputPricePerToken
	}
	if row.OutputPricePriority != nil {
		pricing.OutputPricePerTokenPriority = *row.OutputPricePriority
	} else {
		pricing.OutputPricePerTokenPriority = pricing.OutputPricePerToken
	}
	if row.ImageOutputPrice != nil {
		pricing.ImageOutputPricePerToken = *row.ImageOutputPrice
		pricing.ImageOutputPriceExplicit = true
	}

	if NormalizePricingFxMode(row.FxMode) == FxModeAuto {
		if row.CostID == nil || row.FxRate == nil {
			return nil, ErrPriceCostMissing
		}
		cost := s.GetPriceCost(ctx, *row.CostID)
		if cost == nil {
			return nil, ErrPriceCostMissing
		}
		derived, err := DeriveSellPrice(*cost, row.MarkupPct, *row.FxRate)
		if err != nil {
			return nil, err
		}
		pricing = ApplyDerivedSellPrice(pricing, derived)
	}

	return pricing, nil
}

// NormalizeModelPricingSourceForAPI 供 handler 使用（导出受支持的三态字符串）。
func NormalizeModelPricingSourceForAPI(value string) string {
	return normalizeModelPricingSource(value)
}
