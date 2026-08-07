-- Migration: 190_drop_channel_account_stats_pricing
-- 售价策略不再承载「账号成本」能力。apply_pricing_to_account_stats 已无写入/读取消费者
-- （applyAccountStatsCost 已删、Create/Update 恒 false）。删除该列并重挂 187 视图。

DROP VIEW IF EXISTS sell_price_policies;
ALTER TABLE channels DROP COLUMN IF EXISTS apply_pricing_to_account_stats;

CREATE OR REPLACE VIEW sell_price_policies AS
SELECT
  c.id,
  c.name,
  c.description,
  c.status,
  c.model_mapping,
  c.billing_model_source,
  c.restrict_models,
  c.features,
  c.features_config,
  c.created_at,
  c.updated_at
FROM channels c;
