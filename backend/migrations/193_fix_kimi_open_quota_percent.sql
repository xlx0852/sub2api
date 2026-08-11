-- Kimi persists utilization as percentage points (0-100), not a 0-1 ratio.
-- Repair currently open rows that were previously multiplied by 100.

WITH kimi_open_values AS (
    SELECT
        q.id,
        CASE q.kind
            WHEN '5h' THEN a.extra->>'kimi_quota_5h_utilization'
            WHEN '7d' THEN a.extra->>'kimi_quota_7d_utilization'
        END AS raw_used
    FROM account_quota_windows q
    JOIN accounts a ON a.id = q.account_id
    WHERE q.platform = 'kimi'
      AND q.is_open = TRUE
      AND q.kind IN ('5h', '7d')
)
UPDATE account_quota_windows q
SET used_percent_open = LEAST(100, GREATEST(0, v.raw_used::DOUBLE PRECISION)),
    updated_at = NOW()
FROM kimi_open_values v
WHERE q.id = v.id
  AND v.raw_used ~ '^[+-]?([0-9]+([.][0-9]*)?|[.][0-9]+)$';
