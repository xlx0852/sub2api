-- 账号成本配置表：绑定账号成本模型，用于利润分析。
-- cost_type = subscription: 订阅制（固定周期费用，按周期摊销）
-- cost_type = metered: 按量付费（成本直接取 usage_logs.account_stats_cost 汇总，无需配置费用）
CREATE TABLE IF NOT EXISTS account_cost_configs (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL UNIQUE,
    cost_type VARCHAR(20) NOT NULL DEFAULT 'metered',
    period_fee NUMERIC(20,8) NOT NULL DEFAULT 0,
    billing_cycle_day INTEGER NOT NULL DEFAULT 1,
    period_days INTEGER NOT NULL DEFAULT 30,
    currency VARCHAR(10) NOT NULL DEFAULT 'USD',
    window_baseline_revenue NUMERIC(20,8),
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS account_cost_configs_cost_type_idx ON account_cost_configs (cost_type);
