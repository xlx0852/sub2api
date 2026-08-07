-- P2: explicit group → sell-price policy pointer (dual-write with channel_groups).
-- Storage for policies remains channels; this column makes the group the source of truth
-- for "which sell policy am I on" without requiring the reverse bind to be rediscovered.

ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS sell_price_policy_id BIGINT NULL;

COMMENT ON COLUMN groups.sell_price_policy_id IS
  'Sell-price policy id (channels.id). NULL = follow official base pricing. Dual-written with channel_groups.';

-- Backfill from existing channel_groups (group_id is UNIQUE → at most one policy).
UPDATE groups g
SET sell_price_policy_id = cg.channel_id
FROM channel_groups cg
WHERE cg.group_id = g.id
  AND g.deleted_at IS NULL
  AND (g.sell_price_policy_id IS DISTINCT FROM cg.channel_id);

CREATE INDEX IF NOT EXISTS idx_groups_sell_price_policy_id
  ON groups (sell_price_policy_id)
  WHERE sell_price_policy_id IS NOT NULL AND deleted_at IS NULL;
