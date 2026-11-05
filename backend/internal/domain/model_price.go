package domain

// ModelPriceRow 是 model_prices 的一行（注册表价格）。
//
// group_id 为空表示"平台默认价"，即某模型在没有分组覆盖时的兜底价格；这取代了
// catalog.json 里 fallback_pricing 的角色。
type ModelPriceRow struct {
	ID          int64
	ModelID     string
	GroupID     *int64
	BillingMode string
	Currency    string
	FxMode      string
	CostID      *int64
	FxRate      *float64
	MarkupPct   float64

	InputPrice          *float64
	OutputPrice         *float64
	CacheWritePrice     *float64
	CacheReadPrice      *float64
	InputPricePriority  *float64
	OutputPricePriority *float64
	ImageOutputPrice    *float64
	PerRequestPrice     *float64

	ContextTiers []byte
	ImageTiers   []byte
	Enabled      bool
	Version      int64
}

// IsEmpty 报告该行是否没有任何价格（历史上这种行导致了 0 元计费，保存时应被拒绝）。
func (r *ModelPriceRow) IsEmpty() bool {
	if r == nil {
		return true
	}
	return r.InputPrice == nil && r.OutputPrice == nil &&
		r.CacheReadPrice == nil && r.CacheWritePrice == nil &&
		r.PerRequestPrice == nil && r.ImageOutputPrice == nil
}
