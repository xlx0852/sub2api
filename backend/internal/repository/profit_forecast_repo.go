package repository

import (
	"context"
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
