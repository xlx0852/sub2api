-- P4.1a: non-destructive product aliases for sell-price policies.
-- Physical tables remain channels / channel_groups; views expose policy naming
-- for ops/SQL and future cutover. Application still reads base tables.

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
  c.apply_pricing_to_account_stats,
  c.created_at,
  c.updated_at
FROM channels c;

COMMENT ON VIEW sell_price_policies IS
  'P4.1a alias of channels (sell-price policy entity). Do not write via this view in app code yet.';

CREATE OR REPLACE VIEW sell_price_policy_groups AS
SELECT
  cg.channel_id AS policy_id,
  cg.group_id
FROM channel_groups cg;

COMMENT ON VIEW sell_price_policy_groups IS
  'P4.1a alias of channel_groups. Prefer groups.sell_price_policy_id as source of truth.';

-- Convenience: groups with resolved policy (column first, reverse bind fallback).
CREATE OR REPLACE VIEW group_sell_price_bindings AS
SELECT
  g.id AS group_id,
  g.name AS group_name,
  g.platform,
  g.rate_multiplier,
  g.status AS group_status,
  COALESCE(g.sell_price_policy_id, cg.channel_id) AS policy_id,
  g.sell_price_policy_id AS policy_id_explicit,
  cg.channel_id AS policy_id_from_channel_groups
FROM groups g
LEFT JOIN channel_groups cg ON cg.group_id = g.id
WHERE g.deleted_at IS NULL;

COMMENT ON VIEW group_sell_price_bindings IS
  'P4.1a diagnostic view: explicit group.sell_price_policy_id vs legacy channel_groups.';
