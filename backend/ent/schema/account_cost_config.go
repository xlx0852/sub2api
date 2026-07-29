package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AccountCostConfig 定义账号成本配置实体的 schema。
//
// 绑定账号的成本模型，用于利润分析：
// - subscription: 订阅制（固定周期费用，按周期摊销，如 ChatGPT Pro $200/月）
// - metered: 按量付费（成本直接取 usage_logs.account_stats_cost 汇总）
type AccountCostConfig struct {
	ent.Schema
}

func (AccountCostConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "account_cost_configs"},
	}
}

func (AccountCostConfig) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (AccountCostConfig) Fields() []ent.Field {
	return []ent.Field{
		// account_id: 绑定的账号 ID（唯一）
		field.Int64("account_id").
			Unique(),

		// cost_type: 成本模型类型 subscription / metered
		field.String("cost_type").
			MaxLen(20).
			Default("metered"),

		// period_fee: 订阅周期费用（仅 subscription 使用）
		field.Float("period_fee").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),

		// period_days: 计费周期长度（天）
		field.Int("period_days").
			Default(30),

		// currency: 货币（默认 USD）
		field.String("currency").
			MaxLen(10).
			Default("USD"),

		// window_baseline_revenue: 5h 窗口满载理论产出基准（美元）。
		// nil 表示按历史最佳窗口收入自动学习。
		field.Float("window_baseline_revenue").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Optional().
			Nillable(),

		// notes: 管理员备注
		field.String("notes").
			Optional().
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (AccountCostConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("cost_type"),
	}
}
