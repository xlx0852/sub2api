\set ON_ERROR_STOP on

-- Reconcile requests that were already in flight on the old instance when the
-- denomination transaction committed. Run only after traffic has switched to
-- a freshly restarted instance and the old instance has drained.

BEGIN;
SELECT pg_advisory_xact_lock(202607141012::bigint);

DO $guard$
BEGIN
    IF EXISTS (
        SELECT 1 FROM settings
        WHERE key = 'BALANCE_DENOMINATION_TAIL_RECONCILIATION_20260714'
    ) THEN
        RAISE EXCEPTION 'tail reconciliation already applied';
    END IF;
END
$guard$;

CREATE TEMP TABLE affected_tail ON COMMIT DROP AS
WITH marker AS (
    SELECT (value::jsonb->>'last_scaled_usage_id')::bigint last_id
    FROM settings WHERE key = 'BALANCE_DENOMINATION_MIGRATION_20260714'
)
SELECT ul.id, ul.user_id, ul.api_key_id, ul.actual_cost, ul.subscription_id
FROM usage_logs ul
CROSS JOIN marker m
JOIN groups g ON g.id = ul.group_id
WHERE ul.id > m.last_id
  AND g.rate_multiplier > 0
  AND ul.rate_multiplier / g.rate_multiplier > 5;

DO $guard$
BEGIN
    IF EXISTS (SELECT 1 FROM affected_tail WHERE subscription_id IS NOT NULL) THEN
        RAISE EXCEPTION 'tail contains subscription billing';
    END IF;
    IF EXISTS (
        SELECT 1 FROM affected_tail a JOIN api_keys k ON k.id = a.api_key_id
        WHERE k.quota <> 0 OR k.rate_limit_5h <> 0
           OR k.rate_limit_1d <> 0 OR k.rate_limit_7d <> 0
    ) THEN
        RAISE EXCEPTION 'tail contains API-key quota effects requiring explicit reconciliation';
    END IF;
END
$guard$;

WITH refunds AS (
    SELECT user_id, sum(actual_cost * 0.9) amount
    FROM affected_tail GROUP BY user_id
)
UPDATE users u
SET balance = u.balance + r.amount, updated_at = now()
FROM refunds r
WHERE u.id = r.user_id;

UPDATE usage_logs ul
SET actual_cost = ul.actual_cost / 10,
    rate_multiplier = ul.rate_multiplier / 10
FROM affected_tail a
WHERE ul.id = a.id;

INSERT INTO settings(key, value, updated_at)
SELECT 'BALANCE_DENOMINATION_TAIL_RECONCILIATION_20260714',
       jsonb_build_object(
           'completed_at', now(),
           'rows', count(*),
           'first_usage_id', min(id),
           'last_usage_id', max(id),
           'balance_refund', coalesce(sum(actual_cost * 0.9), 0)
       )::text,
       now()
FROM affected_tail;

COMMIT;
