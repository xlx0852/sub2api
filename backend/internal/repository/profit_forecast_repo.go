package repository

import (
	"context"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *profitRepository) GetStoredValueSnapshot(ctx context.Context) (*service.StoredValueSnapshot, error) {
	snapshot := &service.StoredValueSnapshot{}
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(GREATEST(balance, 0)), 0)::double precision AS spendable_balance,
			COALESCE(SUM(GREATEST(frozen_balance, 0)), 0)::double precision AS frozen_balance,
			COUNT(*)::bigint AS eligible_users
		FROM users
		WHERE deleted_at IS NULL
			AND status = 'active'
			AND role <> 'admin'
	`).Scan(&snapshot.SpendableBalance, &snapshot.FrozenBalance, &snapshot.EligibleUsers)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (r *profitRepository) GetSupplyForecastUsageSamples(ctx context.Context, start, end time.Time, tzName string) ([]*service.SupplyForecastUsageSample, error) {
	if tzName == "" {
		tzName = "UTC"
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			TO_CHAR(ul.created_at AT TIME ZONE $3, 'YYYY-MM-DD') AS usage_day,
			a.platform,
			ul.account_id,
			a.type,
			COALESCE(SUM(ul.actual_cost), 0)::double precision AS revenue,
			COALESCE(SUM(
				CASE WHEN a.type NOT IN ('oauth', 'setup-token')
					THEN COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)
					ELSE 0
				END
			), 0)::double precision AS metered_cost
		FROM usage_logs ul
		JOIN accounts a ON a.id = ul.account_id
		WHERE ul.created_at >= $1
			AND ul.created_at < $2
			AND ul.subscription_id IS NULL
			AND ul.actual_cost > 0
			AND a.deleted_at IS NULL
		GROUP BY usage_day, a.platform, ul.account_id, a.type
		ORDER BY usage_day ASC, a.platform ASC, ul.account_id ASC
	`, start, end, tzName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	samples := make([]*service.SupplyForecastUsageSample, 0)
	for rows.Next() {
		sample := &service.SupplyForecastUsageSample{}
		if err := rows.Scan(&sample.Date, &sample.Platform, &sample.AccountID, &sample.AccountType, &sample.Revenue, &sample.MeteredCost); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (r *profitRepository) GetSchedulableSubscriptionSupply(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT platform, COUNT(DISTINCT id)::integer
		FROM accounts
		WHERE deleted_at IS NULL
			AND type IN ('oauth', 'setup-token')
			AND status = 'active'
			AND schedulable = TRUE
			AND (expires_at IS NULL OR expires_at > NOW() OR auto_pause_on_expired = FALSE)
			AND (rate_limit_reset_at IS NULL OR rate_limit_reset_at <= NOW())
			AND (overload_until IS NULL OR overload_until <= NOW())
			AND (temp_unschedulable_until IS NULL OR temp_unschedulable_until <= NOW())
		GROUP BY platform
		ORDER BY platform ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int)
	for rows.Next() {
		var platform string
		var count int
		if err := rows.Scan(&platform, &count); err != nil {
			return nil, err
		}
		result[platform] = count
	}
	return result, rows.Err()
}

// GetSubscriptionQuotaSnapshots 返回可调度订阅号的额度快照（账号自己的额度
// 视角），用于供给预测的产能折算。只返回有额度数据的号。
//
// 额度口径（账号额度，非按量消耗）：
//   - openai/codex：codex_7d_used_percent（7d 窗口已用 %），reset_at 过期视为 100%
//   - grok：grok_usage_snapshot.tokens（remaining/limit）
//   - kimi：kimi_quota_7d_utilization
//   - 其他/无数据：跳过（HasValidData=false 会被 service 层过滤）
func (r *profitRepository) GetSubscriptionQuotaSnapshots(ctx context.Context) ([]*service.SubscriptionQuotaSnapshot, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			a.id,
			a.platform,
			COALESCE(a.extra->>'codex_7d_used_percent', '') AS codex_7d_used_pct,
			COALESCE(a.extra->>'codex_7d_reset_at', '') AS codex_7d_reset_at,
			COALESCE(a.extra->'grok_usage_snapshot'->'tokens'->>'limit', '') AS grok_limit,
			COALESCE(a.extra->'grok_usage_snapshot'->'tokens'->>'remaining', '') AS grok_remaining,
			COALESCE(a.extra->'grok_usage_snapshot'->>'updated_at', '') AS grok_updated_at,
			COALESCE(a.extra->>'kimi_quota_7d_utilization', '') AS kimi_7d_util,
			COALESCE(a.extra->>'codex_usage_updated_at', '') AS codex_updated_at
		FROM accounts a
		WHERE a.deleted_at IS NULL
			AND a.status = 'active'
			AND a.type IN ('oauth', 'setup-token')
			AND a.schedulable = TRUE
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*service.SubscriptionQuotaSnapshot, 0)
	for rows.Next() {
		var (
			snap                                     service.SubscriptionQuotaSnapshot
			codex7dUsed, codex7dReset                string
			grokLimit, grokRemaining, grokUpdatedAt  string
			kimi7dUtil, codexUpdatedAt               string
		)
		if err := rows.Scan(&snap.AccountID, &snap.Platform,
			&codex7dUsed, &codex7dReset,
			&grokLimit, &grokRemaining, &grokUpdatedAt,
			&kimi7dUtil, &codexUpdatedAt); err != nil {
			return nil, err
		}
		snap.WindowDays = 7.0
		snap.HasValidData = false

		switch snap.Platform {
		case "openai":
			// codex_7d_used_percent + reset_at（reset 过期视为额度已耗尽）
			if pct, err := strconv.ParseFloat(codex7dUsed, 64); err == nil {
				snap.RemainingPct = 100 - pct
				snap.HasValidData = true
				if ts, err := time.Parse(time.RFC3339, codex7dReset); err == nil && time.Now().After(ts) {
					snap.RemainingPct = 0 // reset 已过期 → 额度视为已耗尽
				}
				if ts, err := time.Parse(time.RFC3339, codexUpdatedAt); err == nil {
					snap.UpdatedAt = ts
				}
			}
		case "grok":
			// grok_usage_snapshot.tokens: remaining / limit（24h 滚动窗口）
			if limit, err := strconv.ParseFloat(grokLimit, 64); err == nil && limit > 0 {
				if remaining, err := strconv.ParseFloat(grokRemaining, 64); err == nil {
					snap.RemainingPct = remaining / limit * 100
					snap.WindowDays = 1.0 // grok 是 24h 滚动
					snap.HasValidData = true
					if ts, err := time.Parse(time.RFC3339, grokUpdatedAt); err == nil {
						snap.UpdatedAt = ts
					}
				}
			}
		case "kimi":
			if pct, err := strconv.ParseFloat(kimi7dUtil, 64); err == nil {
				snap.RemainingPct = 100 - pct
				snap.HasValidData = true
			}
		}
		out = append(out, &snap)
	}
	return out, rows.Err()
}
