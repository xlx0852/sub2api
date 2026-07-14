\set ON_ERROR_STOP on

-- Production cutover for customer balance denomination 1:10 -> 1:1.
-- Preconditions:
--   1. the application remains online; the script drains billing writes with a
--      short users-table lock only after the long historical update finishes;
--   2. BALANCE_PAYMENT_DISABLED=true;
--   3. a verified PostgreSQL backup exists;
--   4. exactly one special historical 1:5 recharge account exists.
-- This script is intentionally not embedded in application startup migrations.
-- Historical note: this is the exact 2026-07-14 cutover script. Do not reuse it
-- without also applying compensate-special-1-to-5-accounts.sql; that follow-up
-- separates 1:5 recharge credits from globally 1:10 usage charges and adds the
-- second operator-confirmed special account.

BEGIN;
SET LOCAL lock_timeout = '10s';
SET LOCAL statement_timeout = '0';
SELECT pg_advisory_xact_lock(202607141011::bigint);

DO $guard$
DECLARE
    special_count bigint;
    recharge_multiplier numeric;
    recharge_disabled boolean;
BEGIN
    IF EXISTS (
        SELECT 1 FROM settings
        WHERE key = 'BALANCE_DENOMINATION_MIGRATION_20260714'
    ) THEN
        RAISE EXCEPTION 'balance denomination migration marker already exists';
    END IF;

    SELECT count(*) INTO special_count
    FROM users
    WHERE lower(email) = 's928215036@gmail.com';
    IF special_count <> 1 THEN
        RAISE EXCEPTION 'expected exactly one special account, found %', special_count;
    END IF;

    SELECT value::numeric INTO recharge_multiplier
    FROM settings WHERE key = 'BALANCE_RECHARGE_MULTIPLIER';
    IF recharge_multiplier IS DISTINCT FROM 10 THEN
        RAISE EXCEPTION 'expected recharge multiplier 10, found %', recharge_multiplier;
    END IF;

    SELECT value::boolean INTO recharge_disabled
    FROM settings WHERE key = 'BALANCE_PAYMENT_DISABLED';
    IF recharge_disabled IS DISTINCT FROM true THEN
        RAISE EXCEPTION 'balance recharge must be disabled before migration';
    END IF;

    IF EXISTS (
        SELECT 1 FROM payment_orders
        WHERE status NOT IN ('COMPLETED', 'CANCELLED', 'EXPIRED', 'FAILED', 'REFUNDED')
    ) THEN
        RAISE EXCEPTION 'in-flight payment order exists';
    END IF;

    IF EXISTS (
        SELECT 1 FROM batch_image_jobs
        WHERE status NOT IN ('completed', 'failed', 'cancelled') OR settled_at IS NULL
    ) THEN
        RAISE EXCEPTION 'running or unsettled batch image job exists';
    END IF;
END
$guard$;

CREATE TEMP TABLE denomination_preflight (
    metric text PRIMARY KEY,
    value numeric NOT NULL
) ON COMMIT DROP;

CREATE TEMP TABLE denomination_cutoffs (
    initial_usage_id bigint NOT NULL,
    last_scaled_usage_id bigint
) ON COMMIT DROP;

INSERT INTO denomination_cutoffs(initial_usage_id)
SELECT coalesce(max(id), 0) FROM usage_logs;

INSERT INTO denomination_preflight(metric, value)
SELECT 'usage_actual_cost', coalesce(sum(actual_cost), 0) FROM usage_logs WHERE id <= (SELECT initial_usage_id FROM denomination_cutoffs)
UNION ALL SELECT 'usage_raw_total_cost', coalesce(sum(total_cost), 0) FROM usage_logs WHERE id <= (SELECT initial_usage_id FROM denomination_cutoffs)
UNION ALL SELECT 'usage_account_cost', coalesce(sum(account_stats_cost), 0) FROM usage_logs WHERE id <= (SELECT initial_usage_id FROM denomination_cutoffs)
UNION ALL SELECT 'billing_delta', coalesce(sum(delta_usd), 0) FROM billing_usage_entries
UNION ALL SELECT 'payment_gateway_amount', coalesce(sum(pay_amount), 0) FROM payment_orders
UNION ALL SELECT 'group_rate', coalesce(sum(rate_multiplier), 0) FROM groups;

UPDATE usage_logs
SET actual_cost = actual_cost / 10,
    rate_multiplier = rate_multiplier / 10
WHERE id <= (SELECT initial_usage_id FROM denomination_cutoffs);

UPDATE billing_usage_entries SET delta_usd = delta_usd / 10;
UPDATE usage_dashboard_hourly SET actual_cost = actual_cost / 10;
UPDATE usage_dashboard_daily SET actual_cost = actual_cost / 10;

UPDATE payment_orders po
SET amount = po.amount / CASE
        WHEN po.user_id = (SELECT id FROM users WHERE lower(email) = 's928215036@gmail.com') THEN 5
        ELSE 10
    END,
    refund_amount = po.refund_amount / CASE
        WHEN po.user_id = (SELECT id FROM users WHERE lower(email) = 's928215036@gmail.com') THEN 5
        ELSE 10
    END,
    updated_at = now()
WHERE lower(po.order_type) = 'balance';

UPDATE redeem_codes
SET value = value / 10
WHERE lower(type) IN ('balance', 'admin_balance');

UPDATE promo_codes SET bonus_amount = bonus_amount / 10, updated_at = now();
UPDATE promo_code_usages SET bonus_amount = bonus_amount / 10;

UPDATE user_affiliates
SET aff_quota = aff_quota / 10,
    aff_history_quota = aff_history_quota / 10,
    aff_frozen_quota = aff_frozen_quota / 10,
    updated_at = now();

UPDATE user_affiliate_ledger
SET amount = amount / 10,
    balance_after = balance_after / 10,
    aff_quota_after = aff_quota_after / 10,
    aff_frozen_quota_after = aff_frozen_quota_after / 10,
    aff_history_quota_after = aff_history_quota_after / 10,
    updated_at = now();

UPDATE batch_image_jobs
SET estimated_cost = estimated_cost / 10,
    hold_amount = hold_amount / 10,
    actual_cost = actual_cost / 10,
    group_rate_multiplier = group_rate_multiplier / 10,
    billable_unit_price = billable_unit_price / 10,
    hold_unit_price = hold_unit_price / 10,
    updated_at = now();
UPDATE batch_image_items SET billed_amount = billed_amount / 10;

-- Drain existing billing transactions only after the long history update. New
-- billing transactions wait here for a few seconds while the hot fields switch.
LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE;
SELECT pg_sleep(2);

UPDATE denomination_cutoffs
SET last_scaled_usage_id = (SELECT coalesce(max(id), 0) FROM usage_logs);

UPDATE denomination_preflight
SET value = value + CASE metric
    WHEN 'usage_actual_cost' THEN (SELECT coalesce(sum(actual_cost), 0) FROM usage_logs WHERE id > (SELECT initial_usage_id FROM denomination_cutoffs) AND id <= (SELECT last_scaled_usage_id FROM denomination_cutoffs))
    WHEN 'usage_raw_total_cost' THEN (SELECT coalesce(sum(total_cost), 0) FROM usage_logs WHERE id > (SELECT initial_usage_id FROM denomination_cutoffs) AND id <= (SELECT last_scaled_usage_id FROM denomination_cutoffs))
    WHEN 'usage_account_cost' THEN (SELECT coalesce(sum(account_stats_cost), 0) FROM usage_logs WHERE id > (SELECT initial_usage_id FROM denomination_cutoffs) AND id <= (SELECT last_scaled_usage_id FROM denomination_cutoffs))
    ELSE 0
END
WHERE metric IN ('usage_actual_cost', 'usage_raw_total_cost', 'usage_account_cost');

UPDATE usage_logs
SET actual_cost = actual_cost / 10,
    rate_multiplier = rate_multiplier / 10
WHERE id > (SELECT initial_usage_id FROM denomination_cutoffs)
  AND id <= (SELECT last_scaled_usage_id FROM denomination_cutoffs);

INSERT INTO denomination_preflight(metric, value)
SELECT 'normal_user_balance', coalesce(sum(balance), 0) FROM users WHERE lower(email) <> 's928215036@gmail.com'
UNION ALL SELECT 'special_user_balance', coalesce(sum(balance), 0) FROM users WHERE lower(email) = 's928215036@gmail.com'
UNION ALL SELECT 'normal_user_frozen', coalesce(sum(frozen_balance), 0) FROM users WHERE lower(email) <> 's928215036@gmail.com'
UNION ALL SELECT 'special_user_frozen', coalesce(sum(frozen_balance), 0) FROM users WHERE lower(email) = 's928215036@gmail.com'
UNION ALL SELECT 'normal_user_recharged', coalesce(sum(total_recharged), 0) FROM users WHERE lower(email) <> 's928215036@gmail.com'
UNION ALL SELECT 'special_user_recharged', coalesce(sum(total_recharged), 0) FROM users WHERE lower(email) = 's928215036@gmail.com'
UNION ALL SELECT 'api_key_quota', coalesce(sum(quota), 0) FROM api_keys
UNION ALL SELECT 'api_key_quota_used', coalesce(sum(quota_used), 0) FROM api_keys;

-- The special account's recharge-backed assets use its historical 1:5 scale.
-- Usage charges and every other customer-denominated value still use 1:10.
UPDATE users u
SET balance = u.balance / CASE WHEN lower(u.email) = 's928215036@gmail.com' THEN 5 ELSE 10 END,
    frozen_balance = u.frozen_balance / CASE WHEN lower(u.email) = 's928215036@gmail.com' THEN 5 ELSE 10 END,
    total_recharged = u.total_recharged / CASE WHEN lower(u.email) = 's928215036@gmail.com' THEN 5 ELSE 10 END,
    balance_notify_threshold = CASE
        WHEN u.balance_notify_threshold_type = 'fixed' AND u.balance_notify_threshold IS NOT NULL
        THEN u.balance_notify_threshold / CASE WHEN lower(u.email) = 's928215036@gmail.com' THEN 5 ELSE 10 END
        ELSE u.balance_notify_threshold
    END,
    updated_at = now();

UPDATE api_keys
SET quota = quota / 10,
    quota_used = quota_used / 10,
    rate_limit_5h = rate_limit_5h / 10,
    rate_limit_1d = rate_limit_1d / 10,
    rate_limit_7d = rate_limit_7d / 10,
    usage_5h = usage_5h / 10,
    usage_1d = usage_1d / 10,
    usage_7d = usage_7d / 10,
    updated_at = now();

UPDATE groups
SET rate_multiplier = rate_multiplier / 10,
    daily_limit_usd = daily_limit_usd / 10,
    weekly_limit_usd = weekly_limit_usd / 10,
    monthly_limit_usd = monthly_limit_usd / 10,
    image_rate_multiplier = CASE WHEN image_rate_independent THEN image_rate_multiplier / 10 ELSE image_rate_multiplier END,
    video_rate_multiplier = CASE WHEN video_rate_independent THEN video_rate_multiplier / 10 ELSE video_rate_multiplier END,
    updated_at = now();

UPDATE user_group_rate_multipliers
SET rate_multiplier = rate_multiplier / 10,
    updated_at = now();

UPDATE user_platform_quotas
SET daily_limit_usd = daily_limit_usd / 10,
    weekly_limit_usd = weekly_limit_usd / 10,
    monthly_limit_usd = monthly_limit_usd / 10,
    daily_usage_usd = daily_usage_usd / 10,
    weekly_usage_usd = weekly_usage_usd / 10,
    monthly_usage_usd = monthly_usage_usd / 10,
    updated_at = now();

UPDATE user_subscriptions
SET daily_usage_usd = daily_usage_usd / 10,
    weekly_usage_usd = weekly_usage_usd / 10,
    monthly_usage_usd = monthly_usage_usd / 10,
    updated_at = now();

UPDATE settings
SET value = (value::numeric / 10)::text,
    updated_at = now()
WHERE key = 'default_balance'
   OR key = 'balance_low_notify_threshold'
   OR key LIKE 'auth_source_default_%_balance';

WITH scaled AS (
    SELECT s.id,
           jsonb_object_agg(
               platform.key,
               (SELECT jsonb_object_agg(
                    win.key,
                    CASE WHEN jsonb_typeof(win.value) = 'number'
                         THEN to_jsonb((win.value #>> '{}')::numeric / 10)
                         ELSE win.value END
                ) FROM jsonb_each(platform.value) AS win)
           ) AS value
    FROM settings s
    CROSS JOIN LATERAL jsonb_each(s.value::jsonb) AS platform
    WHERE s.key = 'default_platform_quotas'
       OR s.key LIKE 'auth_source_default_%_platform_quotas'
    GROUP BY s.id
)
UPDATE settings s
SET value = scaled.value::text,
    updated_at = now()
FROM scaled
WHERE s.id = scaled.id;

UPDATE settings
SET value = '1.00', updated_at = now()
WHERE key = 'BALANCE_RECHARGE_MULTIPLIER';

INSERT INTO settings(key, value, updated_at)
SELECT 'BALANCE_DENOMINATION_MIGRATION_20260714',
       jsonb_build_object(
           'status', 'completed',
           'completed_at', now(),
           'normal_scale', 10,
           'special_email', 's928215036@gmail.com',
           'special_user_id', (SELECT id FROM users WHERE lower(email) = 's928215036@gmail.com'),
           'special_asset_scale', 5,
           'initial_usage_id', (SELECT initial_usage_id FROM denomination_cutoffs),
           'last_scaled_usage_id', (SELECT last_scaled_usage_id FROM denomination_cutoffs),
           'preflight', (SELECT jsonb_object_agg(metric, value) FROM denomination_preflight)
       )::text,
       now();

-- Transaction-local reconciliation guards. Any mismatch rolls back everything.
DO $reconcile$
DECLARE
    pre jsonb;
    expected numeric;
    actual numeric;
BEGIN
    SELECT (value::jsonb)->'preflight' INTO pre
    FROM settings WHERE key = 'BALANCE_DENOMINATION_MIGRATION_20260714';

    expected := (pre->>'normal_user_balance')::numeric / 10
              + (pre->>'special_user_balance')::numeric / 5;
    SELECT coalesce(sum(balance), 0) INTO actual FROM users;
    IF abs(actual - expected) > (SELECT count(*) FROM users) * 0.000000005 THEN
        RAISE EXCEPTION 'user balance reconciliation failed: actual %, expected %', actual, expected;
    END IF;

    expected := (pre->>'normal_user_frozen')::numeric / 10
              + (pre->>'special_user_frozen')::numeric / 5;
    SELECT coalesce(sum(frozen_balance), 0) INTO actual FROM users;
    IF abs(actual - expected) > (SELECT count(*) FROM users) * 0.000000005 THEN
        RAISE EXCEPTION 'user frozen_balance reconciliation failed: actual %, expected %', actual, expected;
    END IF;

    expected := (pre->>'normal_user_recharged')::numeric / 10
              + (pre->>'special_user_recharged')::numeric / 5;
    SELECT coalesce(sum(total_recharged), 0) INTO actual FROM users;
    IF abs(actual - expected) > (SELECT count(*) FROM users) * 0.000000005 THEN
        RAISE EXCEPTION 'user total_recharged reconciliation failed: actual %, expected %', actual, expected;
    END IF;

    SELECT coalesce(sum(actual_cost), 0) INTO actual FROM usage_logs
    WHERE id <= (SELECT last_scaled_usage_id FROM denomination_cutoffs);
    expected := (pre->>'usage_actual_cost')::numeric / 10;
    IF abs(actual - expected) > (SELECT count(*) FROM usage_logs WHERE id <= (SELECT last_scaled_usage_id FROM denomination_cutoffs)) * 0.00000000005 THEN
        RAISE EXCEPTION 'usage actual_cost reconciliation failed: actual %, expected %', actual, expected;
    END IF;

    SELECT coalesce(sum(total_cost), 0) INTO actual FROM usage_logs
    WHERE id <= (SELECT last_scaled_usage_id FROM denomination_cutoffs);
    IF actual IS DISTINCT FROM (pre->>'usage_raw_total_cost')::numeric THEN
        RAISE EXCEPTION 'raw usage total_cost changed';
    END IF;

    SELECT coalesce(sum(account_stats_cost), 0) INTO actual FROM usage_logs
    WHERE id <= (SELECT last_scaled_usage_id FROM denomination_cutoffs);
    IF actual IS DISTINCT FROM (pre->>'usage_account_cost')::numeric THEN
        RAISE EXCEPTION 'account_stats_cost changed';
    END IF;

    SELECT coalesce(sum(pay_amount), 0) INTO actual FROM payment_orders;
    IF actual IS DISTINCT FROM (pre->>'payment_gateway_amount')::numeric THEN
        RAISE EXCEPTION 'gateway pay_amount changed';
    END IF;

    IF (SELECT value FROM settings WHERE key = 'BALANCE_RECHARGE_MULTIPLIER')::numeric <> 1 THEN
        RAISE EXCEPTION 'recharge multiplier was not converted to 1';
    END IF;
END
$reconcile$;

COMMIT;
