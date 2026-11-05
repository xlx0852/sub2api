package service

import (
	"context"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// 售价表总览（独立页面的数据源）：把"哪张表、定了哪些模型、被哪些分组绑、实付多少"
// 一次性返回，前端据此渲染「售价表列表」与「按模型的价目矩阵」。
// 数据量极小（≤10 张表、≤50 个模型名），因此不做分页与缓存。

// PricingOverviewRow 是售价表里的一行（token 模式价格已折算成每百万 token）。
type PricingOverviewRow struct {
	Models      []string `json:"models"`
	BillingMode string   `json:"billing_mode"`
	Currency    string   `json:"currency"`
	// 以下均为"每百万 token"的售价；nil 表示该行没定价（只是模型清单容器）
	InputPerMTok      *float64 `json:"input_per_mtok,omitempty"`
	OutputPerMTok     *float64 `json:"output_per_mtok,omitempty"`
	CacheReadPerMTok  *float64 `json:"cache_read_per_mtok,omitempty"`
	CacheWritePerMTok *float64 `json:"cache_write_per_mtok,omitempty"`
	PerRequestPrice   *float64 `json:"per_request_price,omitempty"`
	FxMode            string   `json:"fx_mode"`
	CostID            *int64   `json:"cost_id,omitempty"`
	FxRate            *float64 `json:"fx_rate,omitempty"`
	MarkupPct         float64  `json:"markup_pct"`
}

// PricingOverviewPolicy 是一张售价表。
type PricingOverviewPolicy struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	Source     string   `json:"source"`
	Platforms  []string `json:"platforms"`
	ModelCount int      `json:"model_count"`
	// PricedRowCount 为 0 说明这张表只是模型清单容器，没有任何真实价格。
	PricedRowCount int                    `json:"priced_row_count"`
	Rows           []PricingOverviewRow   `json:"rows"`
	BoundGroups    []PricingOverviewGroup `json:"bound_groups"`
	// 示例：第一条定价行 × 第一个绑定分组的折扣 = 用户实付
	ExampleModel          string   `json:"example_model,omitempty"`
	ExampleSellInput      *float64 `json:"example_sell_input,omitempty"`
	ExampleEffectiveInput *float64 `json:"example_effective_input,omitempty"`
	ExampleCurrency       string   `json:"example_currency,omitempty"`
	ExampleFxMode         string   `json:"example_fx_mode,omitempty"`
}

// PricingOverviewGroup 是绑定了售价表的分组。
type PricingOverviewGroup struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	RateMultiplier float64 `json:"rate_multiplier"`
}

// PricingOverview 是整页数据。
type PricingOverview struct {
	Policies []PricingOverviewPolicy `json:"policies"`
}

// BuildPricingOverview 汇总所有售价表（含未绑定分组的表），用于独立售价表页面。
func (s *ChannelService) BuildPricingOverview(ctx context.Context) (*PricingOverview, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	bindings, err := s.repo.ListGroupRefsByPolicy(ctx)
	if err != nil {
		// 绑定关系读失败不应该让整页 500：价格本身仍然可见。
		bindings = map[int64][]domain.GroupPolicyBinding{}
	}

	out := &PricingOverview{Policies: make([]PricingOverviewPolicy, 0, len(channels))}
	for i := range channels {
		ch := &channels[i]
		policy := PricingOverviewPolicy{
			ID:          ch.ID,
			Name:        ch.Name,
			Status:      ch.Status,
			Source:      SellPriceSourcePolicy,
			Rows:        make([]PricingOverviewRow, 0, len(ch.ModelPricing)),
			BoundGroups: make([]PricingOverviewGroup, 0, len(bindings[ch.ID])),
		}

		platforms := make(map[string]struct{})
		for _, ref := range bindings[ch.ID] {
			policy.BoundGroups = append(policy.BoundGroups, PricingOverviewGroup{
				ID: ref.GroupID, Name: ref.Name, RateMultiplier: ref.RateMultiplier,
			})
		}

		for _, row := range ch.ModelPricing {
			platforms[row.Platform] = struct{}{}
			models := make([]string, 0, len(row.Models))
			for _, m := range row.Models {
				if trimmed := strings.TrimSpace(m); trimmed != "" {
					models = append(models, trimmed)
				}
			}
			policy.ModelCount += len(models)

			converted := PricingOverviewRow{
				Models:      models,
				BillingMode: string(row.BillingMode),
				Currency:    string(domain.NormalizePricingCurrency(string(row.Currency))),
				FxMode:      string(NormalizePricingFxMode(row.FxMode)),
				CostID:      row.CostID,
				FxRate:      row.FxRate,
				MarkupPct:   row.MarkupPct,
			}
			if converted.BillingMode == "" {
				converted.BillingMode = string(BillingModeToken)
			}
			if row.InputPrice != nil {
				converted.InputPerMTok = perMillion(row.InputPrice)
			}
			if row.OutputPrice != nil {
				converted.OutputPerMTok = perMillion(row.OutputPrice)
			}
			if row.CacheReadPrice != nil {
				converted.CacheReadPerMTok = perMillion(row.CacheReadPrice)
			}
			if row.CacheWritePrice != nil {
				converted.CacheWritePerMTok = perMillion(row.CacheWritePrice)
			}
			if row.PerRequestPrice != nil {
				v := *row.PerRequestPrice
				converted.PerRequestPrice = &v
			}
			if converted.InputPerMTok != nil || converted.OutputPerMTok != nil || converted.PerRequestPrice != nil {
				policy.PricedRowCount++
			}
			policy.Rows = append(policy.Rows, converted)

			// 示例：拿第一条有输入价的行 + 第一个绑定分组的折扣
			if policy.ExampleModel == "" && converted.InputPerMTok != nil && len(models) > 0 {
				policy.ExampleModel = models[0]
				policy.ExampleSellInput = converted.InputPerMTok
				policy.ExampleCurrency = converted.Currency
				policy.ExampleFxMode = converted.FxMode
				if len(policy.BoundGroups) > 0 {
					rate := policy.BoundGroups[0].RateMultiplier
					if rate <= 0 {
						rate = 1
					}
					effective := *converted.InputPerMTok * rate
					policy.ExampleEffectiveInput = &effective
				}
			}
		}

		policy.Platforms = make([]string, 0, len(platforms))
		for p := range platforms {
			if p != "" {
				policy.Platforms = append(policy.Platforms, p)
			}
		}
		sort.Strings(policy.Platforms)
		out.Policies = append(out.Policies, policy)
	}

	// 定价中且被绑定的表排在前面，方便一眼看到"真正在生效"的
	sort.SliceStable(out.Policies, func(i, j int) bool {
		a, b := out.Policies[i], out.Policies[j]
		if (len(a.BoundGroups) > 0) != (len(b.BoundGroups) > 0) {
			return len(a.BoundGroups) > 0
		}
		if (a.PricedRowCount > 0) != (b.PricedRowCount > 0) {
			return a.PricedRowCount > 0
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return out, nil
}

func perMillion(perToken *float64) *float64 {
	if perToken == nil {
		return nil
	}
	v := *perToken * 1_000_000
	return &v
}
