package domain

import "time"

// 统一定价管线（L1 成本 → L2 售价 → L3 修饰因子）的领域类型。
//
// 之所以放在 domain：仓储层按这些类型读写，服务层只负责算法，避免 repository → service 的
// 反向依赖（参见 architecture-governance）。

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

// PriceCost 是 L1 成本基（供应商币种，按每百万 token 记账）。
type PriceCost struct {
	ID                int64
	Vendor            string
	ModelPattern      string
	Currency          PricingCurrency
	InputPerMTok      float64
	CachedPerMTok     float64
	OutputPerMTok     float64
	CacheWritePerMTok float64
	DiscountPct       float64
	ObservedAt        time.Time
}
