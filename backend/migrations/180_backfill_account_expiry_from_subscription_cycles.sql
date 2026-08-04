-- 订阅账号的调度到期时间统一跟随当前成本周期。
-- 保留 OAuth token 到期在 credentials 中，不作为账号调度到期。
WITH cycle_termination AS (
    SELECT cycle_id, MIN(effective_at) AS effective_at
    FROM account_subscription_terminations
    WHERE reversed_at IS NULL
    GROUP BY cycle_id
), ranked_cycles AS (
    SELECT
        c.id,
        c.account_id,
        c.starts_at,
        c.period_days,
        t.effective_at AS termination_at,
        ROW_NUMBER() OVER (
            PARTITION BY c.account_id
            ORDER BY
                CASE
                    WHEN c.starts_at <= NOW()
                        AND NOW() < c.starts_at + make_interval(days => c.period_days)
                        AND (t.effective_at IS NULL OR t.effective_at > NOW()) THEN 0
                    WHEN c.starts_at > NOW() THEN 1
                    ELSE 2
                END,
                CASE WHEN c.starts_at > NOW() THEN c.starts_at END ASC,
                CASE WHEN c.starts_at <= NOW() THEN c.starts_at END DESC,
                c.id DESC
        ) AS rank_no
    FROM account_subscription_cycles c
    JOIN accounts a ON a.id = c.account_id
    LEFT JOIN cycle_termination t ON t.cycle_id = c.id
    WHERE a.deleted_at IS NULL
      AND a.type IN ('oauth', 'setup-token')
), chosen AS (
    SELECT
        account_id,
        LEAST(
            starts_at + make_interval(days => period_days),
            COALESCE(termination_at, starts_at + make_interval(days => period_days))
        ) AS expires_at
    FROM ranked_cycles
    WHERE rank_no = 1
)
UPDATE accounts a
SET expires_at = chosen.expires_at,
    updated_at = NOW()
FROM chosen
WHERE a.id = chosen.account_id
  AND a.expires_at IS DISTINCT FROM chosen.expires_at;
