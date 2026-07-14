\set ON_ERROR_STOP on

-- Post-migration correction applied on 2026-07-15.
-- Historical recharge credits for the two accounts below used a 1:5 scale.
-- Usage charges, default grants, API-key quotas and platform quotas remain 1:10.

BEGIN;
SELECT pg_advisory_xact_lock(202607150105::bigint);

DO $guard$
DECLARE
    migration_at timestamptz;
BEGIN
    IF EXISTS (
        SELECT 1 FROM settings
        WHERE key = 'BALANCE_DENOMINATION_SPECIAL_ACCOUNT_COMPENSATION_20260715'
    ) THEN
        RAISE EXCEPTION 'special-account compensation already applied';
    END IF;

    IF (SELECT count(*) FROM users WHERE lower(email) = 's928215036@gmail.com') <> 1
       OR (SELECT count(*) FROM users WHERE lower(email) = 'shizeying@joyme.sg') <> 1 THEN
        RAISE EXCEPTION 'special-account identity guard failed';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM users
        WHERE id = 37 AND lower(email) = 's928215036@gmail.com'
          AND balance = 51.69508291 AND frozen_balance = 0 AND total_recharged = 500
    ) THEN
        RAISE EXCEPTION 's928 pre-compensation values changed';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM users
        WHERE id = 45 AND lower(email) = 'shizeying@joyme.sg'
          AND balance = -1.81081407 AND frozen_balance = 0 AND total_recharged = 200
    ) THEN
        RAISE EXCEPTION 'shizeying pre-compensation values changed';
    END IF;

    SELECT (value::jsonb->>'completed_at')::timestamptz INTO migration_at
    FROM settings WHERE key = 'BALANCE_DENOMINATION_MIGRATION_20260714';

    IF EXISTS (
        SELECT 1 FROM usage_logs
        WHERE user_id IN (37, 45) AND created_at >= migration_at
    ) OR EXISTS (
        SELECT 1 FROM payment_orders
        WHERE user_id IN (37, 45) AND created_at >= migration_at
    ) THEN
        RAISE EXCEPTION 'post-migration activity exists for a special account';
    END IF;
END
$guard$;

CREATE TEMP TABLE compensation_before ON COMMIT DROP AS
SELECT jsonb_build_object(
    'users', (
        SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id)
        FROM (
            SELECT id, email, balance, frozen_balance, total_recharged
            FROM users WHERE id IN (37, 45)
        ) x
    ),
    'orders', (
        SELECT jsonb_agg(to_jsonb(x) ORDER BY x.user_id, x.id)
        FROM (
            SELECT id, user_id, status, order_type, amount, pay_amount, refund_amount
            FROM payment_orders WHERE user_id IN (37, 45)
        ) x
    ),
    'redeems', (
        SELECT jsonb_agg(to_jsonb(x) ORDER BY x.used_by, x.id)
        FROM (
            SELECT id, used_by, type, value, status
            FROM redeem_codes WHERE used_by IN (37, 45)
        ) x
    )
) snapshot;

-- The first account's order/total_recharged fields were already converted at
-- 1:5, but its balance was incorrectly divided wholesale by five. Rebuild it
-- from the normal 1:10 baseline plus the extra half of the corrected recharge.
UPDATE users
SET balance = round(
        (SELECT (value::jsonb->'preflight'->>'special_user_balance')::numeric / 10
         FROM settings WHERE key = 'BALANCE_DENOMINATION_MIGRATION_20260714')
        + total_recharged / 2,
        8
    ),
    updated_at = now()
WHERE id = 37;

UPDATE redeem_codes
SET value = value * 2
WHERE used_by = 37 AND type = 'balance';

-- The second account was initially treated as a normal 1:10 account. Add the
-- missing half of its completed recharge credits and convert recharge history.
UPDATE users
SET balance = balance + total_recharged,
    total_recharged = total_recharged * 2,
    updated_at = now()
WHERE id = 45;

UPDATE payment_orders
SET amount = amount * 2,
    refund_amount = refund_amount * 2,
    updated_at = now()
WHERE user_id = 45 AND lower(order_type) = 'balance';

UPDATE redeem_codes
SET value = value * 2
WHERE used_by = 45 AND type = 'balance';

INSERT INTO settings(key, value, updated_at)
SELECT
    'BALANCE_DENOMINATION_SPECIAL_ACCOUNT_COMPENSATION_20260715',
    jsonb_build_object(
        'status', 'completed',
        'completed_at', now(),
        'reason', 'historical 1:5 recharge assets must be separated from globally 1:10 usage charges',
        'accounts', jsonb_build_array(
            jsonb_build_object(
                'user_id', 37,
                'email', 's928215036@gmail.com',
                'before_balance', 51.69508291,
                'after_balance', 275.84754146,
                'total_recharged', 500
            ),
            jsonb_build_object(
                'user_id', 45,
                'email', 'shizeying@joyme.sg',
                'before_balance', -1.81081407,
                'after_balance', 198.18918593,
                'before_total_recharged', 200,
                'after_total_recharged', 400
            )
        ),
        'before', (SELECT snapshot FROM compensation_before)
    )::text,
    now();

DO $verify$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM users
        WHERE id = 37 AND balance = 275.84754146 AND total_recharged = 500
    ) THEN
        RAISE EXCEPTION 's928 verification failed';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM users
        WHERE id = 45 AND balance = 198.18918593 AND total_recharged = 400
    ) THEN
        RAISE EXCEPTION 'shizeying verification failed';
    END IF;
    IF (SELECT sum(amount) FROM payment_orders
        WHERE user_id = 45 AND status = 'COMPLETED' AND lower(order_type) = 'balance') <> 400 THEN
        RAISE EXCEPTION 'shizeying completed order total verification failed';
    END IF;
    IF (SELECT sum(value) FROM redeem_codes
        WHERE used_by = 37 AND type = 'balance' AND status = 'used') <> 500
       OR (SELECT sum(value) FROM redeem_codes
           WHERE used_by = 45 AND type = 'balance' AND status = 'used') <> 400 THEN
        RAISE EXCEPTION 'redeem value verification failed';
    END IF;
END
$verify$;

COMMIT;
