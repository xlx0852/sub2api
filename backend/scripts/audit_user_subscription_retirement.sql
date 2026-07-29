-- Read-only pre/post retirement audit for the user subscription/quota mode.
-- Run with: psql "$DATABASE_URL" -X -v ON_ERROR_STOP=1 -f backend/scripts/audit_user_subscription_retirement.sql

SELECT 'subscription_groups' AS metric, COUNT(*)::BIGINT AS value
FROM groups
WHERE subscription_type = 'subscription'
UNION ALL
SELECT 'active_or_suspended_user_subscriptions', COUNT(*)::BIGINT
FROM user_subscriptions
WHERE deleted_at IS NULL AND status IN ('active', 'suspended')
UNION ALL
SELECT 'active_subscription_users_with_non_positive_balance', COUNT(DISTINCT u.id)::BIGINT
FROM users u
JOIN user_subscriptions us ON us.user_id = u.id
WHERE u.deleted_at IS NULL
  AND u.status = 'active'
  AND u.balance <= 0
  AND us.deleted_at IS NULL
  AND us.status IN ('active', 'suspended')
UNION ALL
SELECT 'subscription_plans_for_sale', COUNT(*)::BIGINT
FROM subscription_plans
WHERE for_sale = TRUE
UNION ALL
SELECT 'pending_subscription_orders', COUNT(*)::BIGINT
FROM payment_orders
WHERE order_type = 'subscription' AND status = 'PENDING'
UNION ALL
SELECT 'paid_or_fulfilling_subscription_orders', COUNT(*)::BIGINT
FROM payment_orders
WHERE order_type = 'subscription' AND status IN ('PAID', 'RECHARGING')
UNION ALL
SELECT 'unused_subscription_redeem_codes', COUNT(*)::BIGINT
FROM redeem_codes
WHERE type = 'subscription' AND status = 'unused'
ORDER BY metric;

-- Historical evidence must stay readable after retirement.
SELECT
    COUNT(*) AS historical_subscription_rows,
    COUNT(*) FILTER (WHERE status = 'expired') AS expired_rows,
    COUNT(*) FILTER (WHERE deleted_at IS NOT NULL) AS soft_deleted_rows
FROM user_subscriptions;

SELECT
    COUNT(*) FILTER (WHERE subscription_id IS NOT NULL) AS historical_usage_rows_with_subscription,
    MIN(created_at) FILTER (WHERE subscription_id IS NOT NULL) AS first_subscription_usage_at,
    MAX(created_at) FILTER (WHERE subscription_id IS NOT NULL) AS last_subscription_usage_at
FROM usage_logs;
