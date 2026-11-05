-- 211: 统一定价管线（L1 成本 / L2 售价派生 / L3 修饰因子）。
--
-- 全部为增量变更，默认值保证现有行为一字不变：
--   * fx_mode 默认 manual —— 存量价格行继续按人工填写的售价扣费；
--   * context_tiers / image_tiers 默认空数组 —— 表示"无分层"，行为同之前；
--   * price_modifiers 新建空表 —— 引擎读不到记录时等价于全部因子为 1。
--
-- 注意：usage_logs 已 140 万行 / 3.5 GB，本迁移刻意不对其做任何 ALTER 或回填，
-- 计费可观测列的添加放在后续独立迁移里（必须 NULL 默认、禁止 UPDATE 回填）。

CREATE TABLE IF NOT EXISTS price_costs (
    id                 BIGSERIAL PRIMARY KEY,
    vendor             VARCHAR(64)    NOT NULL,
    model_pattern      VARCHAR(200)   NOT NULL,
    currency           VARCHAR(3)     NOT NULL DEFAULT 'USD',
    input_per_mtok     NUMERIC(20, 10) NOT NULL,
    cached_per_mtok    NUMERIC(20, 10),
    output_per_mtok    NUMERIC(20, 10),
    cache_write_per_mtok NUMERIC(20, 10),
    discount_pct       NUMERIC(6, 3)  NOT NULL DEFAULT 0,
    observed_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    source_url         TEXT           NOT NULL DEFAULT '',
    notes              TEXT           NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    UNIQUE (vendor, model_pattern)
);

DO $$
BEGIN
    ALTER TABLE price_costs
        ADD CONSTRAINT price_costs_currency_check CHECK (currency IN ('USD', 'CNY'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE price_costs
        ADD CONSTRAINT price_costs_discount_check CHECK (discount_pct >= 0 AND discount_pct <= 100);
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS fx_mode VARCHAR(16) NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS cost_id BIGINT,
    ADD COLUMN IF NOT EXISTS markup_pct NUMERIC(6, 3) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS fx_rate NUMERIC(12, 6),
    ADD COLUMN IF NOT EXISTS context_tiers JSONB NOT NULL DEFAULT '[]'::JSONB,
    ADD COLUMN IF NOT EXISTS image_tiers JSONB NOT NULL DEFAULT '[]'::JSONB;

DO $$
BEGIN
    ALTER TABLE channel_model_pricing
        ADD CONSTRAINT channel_model_pricing_fx_mode_check
        CHECK (fx_mode IN ('cny_native', 'fx_auto', 'manual'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE channel_model_pricing
        ADD CONSTRAINT channel_model_pricing_markup_check CHECK (markup_pct >= -100);
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE channel_model_pricing
        ADD CONSTRAINT channel_model_pricing_cost_fk
        FOREIGN KEY (cost_id) REFERENCES price_costs (id) ON DELETE SET NULL;
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS price_modifiers (
    id           BIGSERIAL PRIMARY KEY,
    scope        VARCHAR(20)   NOT NULL,
    target       VARCHAR(200)  NOT NULL DEFAULT '*',
    kind         VARCHAR(16)   NOT NULL DEFAULT 'window',
    when_mode    VARCHAR(16)   NOT NULL DEFAULT 'inside',
    window_start VARCHAR(5)    NOT NULL DEFAULT '',
    window_end   VARCHAR(5)    NOT NULL DEFAULT '',
    tz           VARCHAR(64)   NOT NULL DEFAULT 'UTC',
    weekdays     SMALLINT[],
    tier_value   VARCHAR(32)   NOT NULL DEFAULT '',
    multiplier   NUMERIC(10, 6) NOT NULL DEFAULT 1,
    enabled      BOOLEAN       NOT NULL DEFAULT TRUE,
    priority     INTEGER       NOT NULL DEFAULT 100,
    notes        TEXT          NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

DO $$
BEGIN
    ALTER TABLE price_modifiers
        ADD CONSTRAINT price_modifiers_scope_check
        CHECK (scope IN ('vendor_time', 'request_tier', 'group_time'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE price_modifiers
        ADD CONSTRAINT price_modifiers_kind_check CHECK (kind IN ('window', 'tier'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE price_modifiers
        ADD CONSTRAINT price_modifiers_when_mode_check CHECK (when_mode IN ('inside', 'outside'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE price_modifiers
        ADD CONSTRAINT price_modifiers_multiplier_check CHECK (multiplier >= 0);
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_price_modifiers_scope_target
    ON price_modifiers (scope, target)
    WHERE enabled;

CREATE INDEX IF NOT EXISTS idx_channel_model_pricing_fx_mode
    ON channel_model_pricing (fx_mode)
    WHERE fx_mode <> 'manual';
