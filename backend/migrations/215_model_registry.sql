-- 215: 模型注册表三张表（models / model_aliases / model_prices）。
--
-- 目标：把"模型是否存在、有什么能力"和"某分组卖多少钱"从两个来源收敛成
--   注册表（1:1）+ 别名（1:N）+ 价格（1:N，group_id IS NULL = 平台默认价）。
-- 本迁移纯增量：不改动 channel_model_pricing / groups / usage_logs，默认开关
-- 仍走 file + legacy，因此部署后扣费行为不变。

CREATE TABLE IF NOT EXISTS models (
    id                 TEXT PRIMARY KEY,
    platform           VARCHAR(32) NOT NULL DEFAULT '',   -- 价格专用兼容 id 可能没有平台
    display_name       TEXT NOT NULL DEFAULT '',
    family             TEXT NOT NULL DEFAULT '',
    owned_by           TEXT NOT NULL DEFAULT '',
    context_window     BIGINT,
    max_context_window BIGINT,
    media              JSONB NOT NULL DEFAULT '{}'::JSONB,
    is_reasoning       BOOLEAN NOT NULL DEFAULT FALSE,
    enabled            BOOLEAN NOT NULL DEFAULT TRUE,
    retired            BOOLEAN NOT NULL DEFAULT FALSE,
    is_default_chat    BOOLEAN NOT NULL DEFAULT FALSE,
    is_default_test    BOOLEAN NOT NULL DEFAULT FALSE,
    -- 上游真实 id（catalog 的 default_mapping；空 = 与 id 相同）
    upstream_id        TEXT NOT NULL DEFAULT '',
    version            BIGINT NOT NULL DEFAULT 1,
    notes              TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS models_one_default_chat_per_platform
    ON models (platform) WHERE is_default_chat;
CREATE UNIQUE INDEX IF NOT EXISTS models_one_default_test_per_platform
    ON models (platform) WHERE is_default_test;
CREATE INDEX IF NOT EXISTS idx_models_platform ON models (platform) WHERE enabled;

CREATE TABLE IF NOT EXISTS model_aliases (
    alias    TEXT NOT NULL,
    platform VARCHAR(32) NOT NULL DEFAULT '',
    model_id TEXT NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    kind     VARCHAR(16) NOT NULL DEFAULT 'alias',  -- alias | id_override | id_reverse | retired
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (alias, platform)
);

CREATE TABLE IF NOT EXISTS model_prices (
    id                    BIGSERIAL PRIMARY KEY,
    model_id              TEXT NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    -- NULL = 平台默认价（取代 catalog 的 fallback_pricing）
    group_id              BIGINT REFERENCES groups (id) ON DELETE CASCADE,
    billing_mode          VARCHAR(20) NOT NULL DEFAULT 'token',
    currency              VARCHAR(3) NOT NULL DEFAULT 'USD',  -- 仅展示，不参与扣费
    fx_mode               VARCHAR(16) NOT NULL DEFAULT 'manual',
    cost_id               BIGINT REFERENCES price_costs (id) ON DELETE SET NULL,
    fx_rate               NUMERIC(12, 6),
    markup_pct            NUMERIC(6, 3) NOT NULL DEFAULT 0,
    input_price           NUMERIC(20, 12),
    output_price          NUMERIC(20, 12),
    cache_write_price     NUMERIC(20, 12),
    cache_read_price      NUMERIC(20, 12),
    input_price_priority  NUMERIC(20, 12),
    output_price_priority NUMERIC(20, 12),
    image_output_price    NUMERIC(20, 8),
    per_request_price     NUMERIC(20, 10),
    context_tiers         JSONB NOT NULL DEFAULT '[]'::JSONB,
    image_tiers           JSONB NOT NULL DEFAULT '[]'::JSONB,
    enabled               BOOLEAN NOT NULL DEFAULT TRUE,
    version               BIGINT NOT NULL DEFAULT 1,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- group_id 为 NULL 时 UNIQUE(model_id, group_id) 不会去重（NULL 互不相等），
-- 因此用两条部分唯一索引分别管"默认价"和"分组价"。
CREATE UNIQUE INDEX IF NOT EXISTS model_prices_one_default_per_model
    ON model_prices (model_id) WHERE group_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS model_prices_one_per_model_group
    ON model_prices (model_id, group_id) WHERE group_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_model_prices_group ON model_prices (group_id) WHERE enabled;

DO $$
BEGIN
    ALTER TABLE models
        ADD CONSTRAINT models_retired_not_enabled CHECK (NOT (retired AND enabled)) NOT VALID;
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE model_prices
        ADD CONSTRAINT model_prices_fx_mode_check
        CHECK (fx_mode IN ('manual', 'cny_native', 'fx_auto')) NOT VALID;
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;
