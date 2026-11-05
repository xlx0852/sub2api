package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// UnifiedPriceRequest 是一次统一管线计算所需的上下文。
type UnifiedPriceRequest struct {
	Model               string
	GroupID             int64
	ServiceTier         string
	Now                 time.Time
	GroupPeakMultiplier float64
	// Base 是 legacy 管线已经算出的单价（渠道价或默认价），统一管线在它之上做派生与因子。
	Base *ModelPricing
}

// UnifiedTokenPrice 按统一管线计算 token 模式单价。
//
// 返回 ok=false 表示当前数据不足以走统一管线（例如 fx_auto 行缺少成本基或汇率），
// 调用方必须保持 legacy 结果 —— 灰度期间宁可少算一层，也不要静默改价。
func (r *ModelPricingResolver) UnifiedTokenPrice(ctx context.Context, resolved *ResolvedPricing, req UnifiedPriceRequest) (*ModelPricing, []AppliedModifier, bool) {
	if r == nil || resolved == nil || req.Base == nil {
		return nil, nil, false
	}

	pricing := req.Base
	if chPricing := resolved.channelPricing; chPricing != nil {
		mode := NormalizePricingFxMode(chPricing.FxMode)
		if mode == FxModeAuto {
			if chPricing.CostID == nil || chPricing.FxRate == nil {
				return nil, nil, false
			}
			cost := r.priceCost(ctx, *chPricing.CostID)
			if cost == nil {
				return nil, nil, false
			}
			derived, err := DeriveSellPrice(*cost, chPricing.MarkupPct, *chPricing.FxRate)
			if err != nil {
				return nil, nil, false
			}
			pricing = ApplyDerivedSellPrice(pricing, derived)
		}
	}

	modifiers := r.priceModifiers(ctx)
	adjusted, applied := ApplyPriceModifiers(pricing, modifiers, ModifierContext{
		Model:               req.Model,
		GroupID:             req.GroupID,
		ServiceTier:         req.ServiceTier,
		Now:                 req.Now,
		GroupPeakMultiplier: req.GroupPeakMultiplier,
	})
	if adjusted == nil {
		return nil, nil, false
	}
	return adjusted, applied, true
}

func (r *ModelPricingResolver) priceCost(ctx context.Context, id int64) *domain.PriceCost {
	if r.channelService == nil {
		return nil
	}
	return r.channelService.GetPriceCost(ctx, id)
}

func (r *ModelPricingResolver) priceModifiers(ctx context.Context) []domain.PriceModifier {
	if r.channelService == nil {
		return nil
	}
	return r.channelService.ListPriceModifiers(ctx)
}

// pipelineSkipsLegacyTimeOfDay 判断统一管线是否已经接管峰谷，避免 legacy 谷时折扣二次叠加。
func pipelineSkipsLegacyTimeOfDay(mode string) bool {
	return NormalizePricingPipelineMode(mode) == PricingPipelineUnified
}

// hasVendorTimeModifier 报告是否存在针对该模型的 vendor_time 因子（统一管线接管峰谷的信号）。
func hasVendorTimeModifier(modifiers []domain.PriceModifier, model string) bool {
	for _, m := range modifiers {
		if !m.Enabled || m.Scope != domain.ModifierScopeVendorTime {
			continue
		}
		if domain.MatchModelPattern(strings.ToLower(strings.TrimSpace(m.Target)), strings.ToLower(strings.TrimSpace(model))) {
			return true
		}
	}
	return false
}
