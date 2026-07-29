-- Retire the user-facing subscription/quota billing mode.
--
-- This migration deliberately preserves user_subscriptions, subscription_plans,
-- payment_orders, redeem_codes and usage_logs.subscription_id as historical
-- audit data. Account procurement subscription cycles used by profit analysis
-- are unrelated and are not touched here.

-- Snapshot every row whose live behavior is changed so the retirement remains
-- traceable without rewriting historical business records.
CREATE TABLE IF NOT EXISTS user_subscription_retirement_audit (
    entity_type VARCHAR(40) NOT NULL,
    entity_id BIGINT NOT NULL,
    snapshot JSONB NOT NULL,
    retired_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (entity_type, entity_id)
);

INSERT INTO user_subscription_retirement_audit (entity_type, entity_id, snapshot)
SELECT 'group', g.id, to_jsonb(g)
FROM groups g
WHERE g.subscription_type = 'subscription'
ON CONFLICT (entity_type, entity_id) DO NOTHING;

INSERT INTO user_subscription_retirement_audit (entity_type, entity_id, snapshot)
SELECT 'user_subscription', us.id, to_jsonb(us)
FROM user_subscriptions us
WHERE us.deleted_at IS NULL
  AND us.status IN ('active', 'suspended')
ON CONFLICT (entity_type, entity_id) DO NOTHING;

INSERT INTO user_subscription_retirement_audit (entity_type, entity_id, snapshot)
SELECT 'subscription_plan', sp.id, to_jsonb(sp)
FROM subscription_plans sp
WHERE sp.for_sale = TRUE
ON CONFLICT (entity_type, entity_id) DO NOTHING;

INSERT INTO user_subscription_retirement_audit (entity_type, entity_id, snapshot)
SELECT 'subscription_redeem_code', rc.id, to_jsonb(rc)
FROM redeem_codes rc
WHERE rc.type = 'subscription'
  AND rc.status = 'unused'
ON CONFLICT (entity_type, entity_id) DO NOTHING;

INSERT INTO user_subscription_retirement_audit (entity_type, entity_id, snapshot)
SELECT 'pending_subscription_order', po.id, to_jsonb(po)
FROM payment_orders po
WHERE po.order_type = 'subscription'
  AND po.status = 'PENDING'
ON CONFLICT (entity_type, entity_id) DO NOTHING;

-- All API-key traffic now uses the normal balance-billing mode.
UPDATE groups
SET subscription_type = 'standard',
    daily_limit_usd = NULL,
    weekly_limit_usd = NULL,
    monthly_limit_usd = NULL,
    updated_at = NOW()
WHERE subscription_type = 'subscription';

-- Stop active/suspended grants from authorizing traffic while keeping their
-- periods and counters available for historical inspection.
UPDATE user_subscriptions
SET status = 'expired',
    updated_at = NOW()
WHERE deleted_at IS NULL
  AND status IN ('active', 'suspended');

-- Disable every source that could create another user subscription.
UPDATE subscription_plans
SET for_sale = FALSE,
    updated_at = NOW()
WHERE for_sale = TRUE;

UPDATE redeem_codes
SET status = 'disabled'
WHERE type = 'subscription'
  AND status = 'unused';

UPDATE payment_orders
SET status = 'CANCELLED',
    failed_reason = CASE
        WHEN COALESCE(failed_reason, '') = '' THEN 'user subscription mode retired'
        ELSE failed_reason
    END,
    updated_at = NOW()
WHERE order_type = 'subscription'
  AND status = 'PENDING';

UPDATE settings
SET value = '[]',
    updated_at = NOW()
WHERE key = 'default_subscriptions'
   OR key LIKE 'auth_source_default_%_subscriptions';

UPDATE settings
SET value = CASE key
        WHEN 'subscription_expiry_notify_enabled' THEN 'false'
        WHEN 'payment_subscription_usd_to_cny_rate' THEN '0'
        ELSE value
    END,
    updated_at = NOW()
WHERE key IN (
    'subscription_expiry_notify_enabled',
    'payment_subscription_usd_to_cny_rate'
);

COMMENT ON TABLE user_subscription_retirement_audit IS
    'Read-only snapshots captured when the user subscription/quota billing mode was retired';
