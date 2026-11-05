package service

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// 统一定价管线（L1 成本 → L2 售价 → L3 修饰因子）。
//
// 设计约束（见 openspec/changes/unify-pricing-pipeline/design.md）：
//   - 扣费只认 L2 的售价；L1 只提供成本与毛利口径；
//   - L2 每行声明一种派生方式：cny_native（人民币原生，永不换汇）、fx_auto（由成本派生）、
//     manual（人工填写，永不换汇）；
//   - Currency 仅是标签，永远不参与扣费计算；
//   - 修饰因子按 vendor_time → request_tier → group_time 的固定顺序连乘，并记录生效的
//     modifier id，便于账单事后复盘。
const (
	// PricingPipelineLegacy 保留现有 11 层解析路径（默认值，行为不变）。
	PricingPipelineLegacy = "legacy"
	// PricingPipelineShadow 双算：按 legacy 扣费，同时计算 unified 并记录差异。
	PricingPipelineShadow = "shadow"
	// PricingPipelineUnified 全量走统一管线。
	PricingPipelineUnified = "unified"
)

// PricingFxMode 是 L2 价格行的派生方式。
type PricingFxMode string

const (
	// FxModeManual 人工填写的售价，永不换汇。
	FxModeManual PricingFxMode = "manual"
	// FxModeCnyNative 以人民币报价的售价，永不换汇。
	FxModeCnyNative PricingFxMode = "cny_native"
	// FxModeAuto 由 L1 成本按 汇率 × (1+markup) 派生售价。
	FxModeAuto PricingFxMode = "fx_auto"
)

// NormalizePricingPipelineMode 把外部配置值收敛为受支持的模式，未知值降级为 legacy（安全默认）。
func NormalizePricingPipelineMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PricingPipelineShadow:
		return PricingPipelineShadow
	case PricingPipelineUnified:
		return PricingPipelineUnified
	default:
		return PricingPipelineLegacy
	}
}

// NormalizePricingFxMode 收敛派生方式，未知值降级为 manual（不换汇，最保守）。
func NormalizePricingFxMode(value string) PricingFxMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(FxModeAuto):
		return FxModeAuto
	case string(FxModeCnyNative):
		return FxModeCnyNative
	default:
		return FxModeManual
	}
}

// PriceCost 是 L1 成本基（每百万 token，供应商币种口径）。
type PriceCost struct {
	Vendor            string
	ModelPattern      string
	Currency          domain.PricingCurrency
	InputPerMTok      float64
	CachedPerMTok     float64
	OutputPerMTok     float64
	CacheWritePerMTok float64
	DiscountPct       float64
}

// DerivedSellPrice 是 L2 派生结果，单位：每个 token。
type DerivedSellPrice struct {
	InputPerToken      float64
	CachedPerToken     float64
	OutputPerToken     float64
	CacheWritePerToken float64
	EffectiveRate      float64 // 折算成一个 USD→CNY 的实际乘数，便于审计
}

// ErrPriceCostMissing 表示 fx_auto 行缺少成本基，属于配置错误，必须 fail-closed。
var ErrPriceCostMissing = fmt.Errorf("fx_auto price row has no cost basis")

// DeriveSellPrice 由成本基派生售价：cost × (1 − discount) × fxRate × (1 + markup)。
//
// 仅接受 USD 成本基换汇；CNY 成本基无需换汇，调用方应把行标为 cny_native，这里返回错误
// 以避免"人民币价被再乘一次汇率"这类事故。
func DeriveSellPrice(cost PriceCost, markupPct, fxRate float64) (DerivedSellPrice, error) {
	if cost.Currency != domain.PricingCurrencyUSD {
		return DerivedSellPrice{}, fmt.Errorf("fx_auto requires a USD cost basis, got %q", cost.Currency)
	}
	if fxRate <= 0 || math.IsNaN(fxRate) || math.IsInf(fxRate, 0) {
		return DerivedSellPrice{}, fmt.Errorf("invalid fx rate %v", fxRate)
	}
	if cost.InputPerMTok < 0 || cost.OutputPerMTok < 0 || cost.CachedPerMTok < 0 || cost.CacheWritePerMTok < 0 {
		return DerivedSellPrice{}, fmt.Errorf("negative cost basis for %s/%s", cost.Vendor, cost.ModelPattern)
	}
	if markupPct < -100 || math.IsNaN(markupPct) {
		return DerivedSellPrice{}, fmt.Errorf("invalid markup %v", markupPct)
	}
	if cost.DiscountPct < 0 || cost.DiscountPct > 100 {
		return DerivedSellPrice{}, fmt.Errorf("invalid discount %v", cost.DiscountPct)
	}

	// 每百万 token 的美元成本 → 折后 → 汇率 → 加价 → 人民币每百万 token → 每个 token。
	factor := (1 - cost.DiscountPct/100) * fxRate * (1 + markupPct/100)
	perToken := func(usdPerMTok float64) float64 {
		return usdPerMTok * factor / 1_000_000
	}

	return DerivedSellPrice{
		InputPerToken:      perToken(cost.InputPerMTok),
		CachedPerToken:     perToken(cost.CachedPerMTok),
		OutputPerToken:     perToken(cost.OutputPerMTok),
		CacheWritePerToken: perToken(cost.CacheWritePerMTok),
		EffectiveRate:      factor,
	}, nil
}

// ApplyDerivedSellPrice 把派生结果写入价格表（保留未提供的缓存价）。
func ApplyDerivedSellPrice(pricing *ModelPricing, derived DerivedSellPrice) *ModelPricing {
	if pricing == nil {
		return nil
	}
	cloned := *pricing
	cloned.InputPricePerToken = derived.InputPerToken
	cloned.InputPricePerTokenPriority = derived.InputPerToken
	cloned.OutputPricePerToken = derived.OutputPerToken
	cloned.OutputPricePerTokenPriority = derived.OutputPerToken
	if derived.CachedPerToken > 0 {
		cloned.CacheReadPricePerToken = derived.CachedPerToken
		cloned.CacheReadPricePerTokenPriority = derived.CachedPerToken
	}
	if derived.CacheWritePerToken > 0 {
		cloned.CacheCreationPricePerToken = derived.CacheWritePerToken
		cloned.CacheCreationPricePerTokenPriority = derived.CacheWritePerToken
		cloned.CacheCreationPriceExplicit = true
	}
	return &cloned
}

// PriceModifierScope 是修饰因子作用域。
type PriceModifierScope string

const (
	// ModifierScopeVendorTime 供应商时间窗（如 DeepSeek 峰谷）。
	ModifierScopeVendorTime PriceModifierScope = "vendor_time"
	// ModifierScopeRequestTier 请求档位（flex / priority）。
	ModifierScopeRequestTier PriceModifierScope = "request_tier"
	// ModifierScopeGroupTime 分组时间窗（分组峰时加价）。
	ModifierScopeGroupTime PriceModifierScope = "group_time"
)

// PriceModifier 是一条可配置的扣费修饰因子。
type PriceModifier struct {
	ID          int64
	Scope       PriceModifierScope
	Target      string // 模型 pattern（vendor/request）；分组 id 字符串（group）
	Kind        string // window | tier
	WhenMode    string // inside | outside
	WindowStart string // HH:MM
	WindowEnd   string // HH:MM
	TZ          string // IANA 时区，默认 UTC
	Weekdays    []int  // 0=Sunday..6=Saturday；nil = 每天
	TierValue   string // request_tier 用
	Multiplier  float64
	Enabled     bool
	Priority    int
}

// ModifierContext 是判定所需的请求上下文。
type ModifierContext struct {
	Model       string
	GroupID     int64
	ServiceTier string
	Now         time.Time
	// GroupPeakMultiplier 由分组配置（groups.peak_*）提供。保留在分组表里，
	// 但通过同一引擎应用，保证扣费路径只有一处。
	GroupPeakMultiplier float64
}

// AppliedModifier 记录实际生效的因子。
type AppliedModifier struct {
	ID    int64
	Scope PriceModifierScope
	Value float64
}

// ApplyPriceModifiers 按固定顺序（vendor_time → request_tier → group_time）连乘修饰因子。
//
// 返回的价格是副本；未命中的因子不参与计算。空 modifiers 且无分组峰时因子时，价格原样返回。
func ApplyPriceModifiers(pricing *ModelPricing, modifiers []PriceModifier, ctx ModifierContext) (*ModelPricing, []AppliedModifier) {
	if pricing == nil {
		return nil, nil
	}
	applied := make([]AppliedModifier, 0, len(modifiers)+1)
	result := pricing

	multiply := func(factor float64) {
		if factor == 1 {
			return
		}
		if result == pricing {
			cloned := *pricing
			result = &cloned
		}
		result.InputPricePerToken *= factor
		result.InputPricePerTokenPriority *= factor
		result.OutputPricePerToken *= factor
		result.OutputPricePerTokenPriority *= factor
		result.CacheReadPricePerToken *= factor
		result.CacheReadPricePerTokenPriority *= factor
		result.CacheCreationPricePerToken *= factor
		result.CacheCreationPricePerTokenPriority *= factor
		result.CacheCreation5mPrice *= factor
		result.CacheCreation1hPrice *= factor
		result.ImageOutputPricePerToken *= factor
	}

	for _, scope := range []PriceModifierScope{
		ModifierScopeVendorTime,
		ModifierScopeRequestTier,
		ModifierScopeGroupTime,
	} {
		for _, m := range modifiers {
			if !m.Enabled || m.Scope != scope {
				continue
			}
			if !modifierMatches(m, ctx) {
				continue
			}
			multiply(m.Multiplier)
			applied = append(applied, AppliedModifier{ID: m.ID, Scope: m.Scope, Value: m.Multiplier})
		}
	}

	// 分组峰时因子最后应用：它来自分组配置而不是 price_modifiers 表。
	if ctx.GroupPeakMultiplier > 0 && ctx.GroupPeakMultiplier != 1 {
		multiply(ctx.GroupPeakMultiplier)
		applied = append(applied, AppliedModifier{ID: 0, Scope: ModifierScopeGroupTime, Value: ctx.GroupPeakMultiplier})
	}

	return result, applied
}

func modifierMatches(m PriceModifier, ctx ModifierContext) bool {
	switch m.Scope {
	case ModifierScopeVendorTime:
		if !domain.MatchModelPattern(strings.ToLower(strings.TrimSpace(m.Target)), strings.ToLower(strings.TrimSpace(ctx.Model))) {
			return false
		}
		return modifierWindowMatches(m, ctx.Now)
	case ModifierScopeRequestTier:
		if !domain.MatchModelPattern(strings.ToLower(strings.TrimSpace(m.Target)), strings.ToLower(strings.TrimSpace(ctx.Model))) {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(m.TierValue), strings.TrimSpace(ctx.ServiceTier))
	case ModifierScopeGroupTime:
		if strings.TrimSpace(m.Target) == "" {
			return false
		}
		if fmt.Sprintf("%d", ctx.GroupID) != strings.TrimSpace(m.Target) {
			return false
		}
		return modifierWindowMatches(m, ctx.Now)
	default:
		return false
	}
}

// modifierWindowMatches 判定时间窗：when_mode=inside 命中区间时生效；outside 在区间外生效。
func modifierWindowMatches(m PriceModifier, now time.Time) bool {
	if strings.TrimSpace(m.WindowStart) == "" || strings.TrimSpace(m.WindowEnd) == "" {
		return m.WhenMode != "outside"
	}
	loc, err := time.LoadLocation(strings.TrimSpace(m.TZ))
	if err != nil || loc == nil {
		loc = time.UTC
	}
	t := now.In(loc)
	if len(m.Weekdays) > 0 {
		matched := false
		for _, d := range m.Weekdays {
			if int(t.Weekday()) == d {
				matched = true
				break
			}
		}
		if !matched {
			// 不在允许的星期内：window 视为不命中；outside 语义下"不在窗口"成立。
			return strings.EqualFold(m.WhenMode, "outside")
		}
	}
	start, okStart := domain.ParseMinutes(strings.TrimSpace(m.WindowStart))
	end, okEnd := domain.ParseMinutes(strings.TrimSpace(m.WindowEnd))
	if !okStart || !okEnd {
		return false
	}
	cur := t.Hour()*60 + t.Minute()
	var inside bool
	if start <= end {
		inside = cur >= start && cur < end
	} else {
		// 跨天窗口（如 22:00–02:00）
		inside = cur >= start || cur < end
	}
	if strings.EqualFold(m.WhenMode, "outside") {
		return !inside
	}
	return inside
}

func parseClockMinutes(value string) (int, bool) {
	parts := strings.SplitN(strings.TrimSpace(value), ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	hour := 0
	minute := 0
	for _, c := range strings.TrimSpace(parts[0]) {
		if c < '0' || c > '9' {
			return 0, false
		}
		hour = hour*10 + int(c-'0')
	}
	for _, c := range strings.TrimSpace(parts[1]) {
		if c < '0' || c > '9' {
			return 0, false
		}
		minute = minute*10 + int(c-'0')
	}
	if hour > 23 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

// PricingShadowDiff 描述 legacy 与 unified 两条管线的差异，用于灰度期打点。
type PricingShadowDiff struct {
	Model      string
	GroupID    int64
	LegacyIn   float64
	UnifiedIn  float64
	LegacyOut  float64
	UnifiedOut float64
}

// priceTuplesDiffer 判断两条管线结果是否超出四舍五入容差。
func priceTuplesDiffer(legacy, unified *ModelPricing) bool {
	if legacy == nil || unified == nil {
		return legacy != unified
	}
	const epsilon = 1e-12
	return math.Abs(legacy.InputPricePerToken-unified.InputPricePerToken) > epsilon ||
		math.Abs(legacy.OutputPricePerToken-unified.OutputPricePerToken) > epsilon ||
		math.Abs(legacy.CacheReadPricePerToken-unified.CacheReadPricePerToken) > epsilon ||
		math.Abs(legacy.CacheCreationPricePerToken-unified.CacheCreationPricePerToken) > epsilon
}
