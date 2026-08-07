-- P4: expose sell-price policy identity on usage_logs without rewriting history.
-- channel_id remains the physical dual-use column (legacy name + policy id).
-- pricing_policy_id is a generated alias for new code/report paths.

ALTER TABLE usage_logs
  ADD COLUMN IF NOT EXISTS pricing_policy_id BIGINT
  GENERATED ALWAYS AS (channel_id) STORED;

COMMENT ON COLUMN usage_logs.pricing_policy_id IS
  'Alias of channel_id for sell-price policy reporting (P4). Physical writes still use channel_id.';

CREATE INDEX IF NOT EXISTS idx_usage_logs_pricing_policy_id
  ON usage_logs (pricing_policy_id)
  WHERE pricing_policy_id IS NOT NULL;
