package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type profitRepository struct {
	db *sql.DB
}

// NewProfitRepository 创建利润分析仓储。
func NewProfitRepository(db *sql.DB) service.ProfitRepository {
	return &profitRepository{db: db}
}

func (r *profitRepository) UpsertCostConfig(ctx context.Context, cfg *service.AccountCostConfig) (*service.AccountCostConfig, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO account_cost_configs (account_id, cost_type, period_fee, period_days, currency, window_baseline_revenue, auto_renew, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		ON CONFLICT (account_id) DO UPDATE SET
			cost_type = EXCLUDED.cost_type,
			period_fee = EXCLUDED.period_fee,
			period_days = EXCLUDED.period_days,
			currency = EXCLUDED.currency,
			window_baseline_revenue = EXCLUDED.window_baseline_revenue,
			auto_renew = EXCLUDED.auto_renew,
			notes = EXCLUDED.notes,
			updated_at = NOW()
		RETURNING id, account_id, cost_type, period_fee, period_days, currency, window_baseline_revenue, auto_renew, notes, created_at, updated_at
	`, cfg.AccountID, cfg.CostType, cfg.PeriodFee, cfg.PeriodDays, cfg.Currency, cfg.WindowBaselineRevenue, cfg.AutoRenew, cfg.Notes)
	return scanCostConfig(row)
}

func (r *profitRepository) GetCostConfig(ctx context.Context, accountID int64) (*service.AccountCostConfig, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, account_id, cost_type, period_fee, period_days, currency, window_baseline_revenue, auto_renew, notes, created_at, updated_at
		FROM account_cost_configs WHERE account_id = $1
	`, accountID)
	cfg, err := scanCostConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return cfg, err
}

func (r *profitRepository) ListCostConfigs(ctx context.Context) ([]*service.AccountCostConfig, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, cost_type, period_fee, period_days, currency, window_baseline_revenue, auto_renew, notes, created_at, updated_at
		FROM account_cost_configs ORDER BY account_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []*service.AccountCostConfig
	for rows.Next() {
		cfg, err := scanCostConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

func (r *profitRepository) InsertCostConfigsIfAbsent(ctx context.Context, configs []*service.AccountCostConfig) ([]int64, error) {
	if len(configs) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(configs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		INSERT INTO account_cost_configs (account_id, cost_type, period_fee, period_days, currency, window_baseline_revenue, auto_renew, notes, created_at, updated_at)
		SELECT account_id, cost_type, period_fee, period_days, currency, window_baseline_revenue, COALESCE(auto_renew, false), notes, NOW(), NOW()
		FROM jsonb_to_recordset($1::jsonb) AS x(
			account_id bigint,
			cost_type varchar,
			period_fee numeric,
			period_days integer,
			currency varchar,
			window_baseline_revenue numeric,
			auto_renew boolean,
			notes text
		)
		ON CONFLICT (account_id) DO NOTHING
		RETURNING account_id
	`, payload)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	inserted := make([]int64, 0, len(configs))
	for rows.Next() {
		var accountID int64
		if err := rows.Scan(&accountID); err != nil {
			return nil, err
		}
		inserted = append(inserted, accountID)
	}
	return inserted, rows.Err()
}

func (r *profitRepository) DeleteCostConfig(ctx context.Context, accountID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM account_cost_configs WHERE account_id = $1`, accountID)
	return err
}

func (r *profitRepository) ListSubscriptionCycles(ctx context.Context, accountID int64) ([]*service.AccountSubscriptionCycle, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, starts_at, period_fee, period_days, currency, notes, created_at, updated_at
		FROM account_subscription_cycles
		WHERE account_id = $1
		ORDER BY starts_at DESC, id DESC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var cycles []*service.AccountSubscriptionCycle
	for rows.Next() {
		cycle, err := scanSubscriptionCycle(rows)
		if err != nil {
			return nil, err
		}
		cycles = append(cycles, cycle)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close()
	return r.enrichSubscriptionCycles(ctx, cycles)
}

func (r *profitRepository) ListSubscriptionCyclesBatch(ctx context.Context, accountIDs []int64) ([]*service.AccountSubscriptionCycle, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, starts_at, period_fee, period_days, currency, notes, created_at, updated_at
		FROM account_subscription_cycles
		WHERE account_id = ANY($1)
		ORDER BY account_id ASC, starts_at DESC, id DESC
	`, pq.Array(accountIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var cycles []*service.AccountSubscriptionCycle
	for rows.Next() {
		cycle, err := scanSubscriptionCycle(rows)
		if err != nil {
			return nil, err
		}
		cycles = append(cycles, cycle)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close()
	return r.enrichSubscriptionCycles(ctx, cycles)
}

func (r *profitRepository) GetSubscriptionCycle(ctx context.Context, id int64) (*service.AccountSubscriptionCycle, error) {
	cycle, err := scanSubscriptionCycle(r.db.QueryRowContext(ctx, `
		SELECT id, account_id, starts_at, period_fee, period_days, currency, notes, created_at, updated_at
		FROM account_subscription_cycles
		WHERE id = $1
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubscriptionCycleNotFound
	}
	if err != nil {
		return nil, err
	}
	cycles, err := r.enrichSubscriptionCycles(ctx, []*service.AccountSubscriptionCycle{cycle})
	if err != nil {
		return nil, err
	}
	return cycles[0], nil
}

func (r *profitRepository) CreateSubscriptionCycle(ctx context.Context, cycle *service.AccountSubscriptionCycle) (*service.AccountSubscriptionCycle, error) {
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO account_subscription_cycles (account_id, starts_at, period_fee, period_days, currency, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id, account_id, starts_at, period_fee, period_days, currency, notes, created_at, updated_at
	`, cycle.AccountID, cycle.StartsAt, cycle.PeriodFee, cycle.PeriodDays, cycle.Currency, cycle.Notes)
	return scanSubscriptionCycle(row)
}

func (r *profitRepository) DeleteSubscriptionCycle(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM account_subscription_cycles c
		WHERE c.id = $1
		  AND NOT EXISTS (
			SELECT 1 FROM account_subscription_terminations t WHERE t.cycle_id = c.id
		  )
	`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows > 0 {
		return nil
	}
	var settled bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM account_subscription_terminations WHERE cycle_id = $1)`, id).Scan(&settled); err != nil {
		return err
	}
	if settled {
		return service.ErrSubscriptionCycleSettled
	}
	return service.ErrSubscriptionCycleNotFound
}

func (r *profitRepository) GetAccountUsageStatsBatch(ctx context.Context, accountIDs []int64, start, end time.Time) (map[int64]*service.ProfitUsageStats, error) {
	result := make(map[int64]*service.ProfitUsageStats, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			account_id,
			COUNT(*) AS requests,
			COALESCE(SUM(actual_cost), 0) AS revenue,
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) AS metered_cost
		FROM usage_logs
		WHERE account_id = ANY($1) AND created_at >= $2 AND created_at < $3
		GROUP BY account_id
	`, pq.Array(accountIDs), start, end)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var accountID int64
		stats := &service.ProfitUsageStats{}
		if err := rows.Scan(&accountID, &stats.Requests, &stats.Revenue, &stats.MeteredCost); err != nil {
			return nil, err
		}
		result[accountID] = stats
	}
	return result, rows.Err()
}

func (r *profitRepository) GetAccountUsageStatsForRanges(ctx context.Context, ranges []service.ProfitAccountUsageRange) (map[int64]*service.ProfitUsageStats, error) {
	result := make(map[int64]*service.ProfitUsageStats, len(ranges))
	if len(ranges) == 0 {
		return result, nil
	}
	accountIDs := make([]int64, 0, len(ranges))
	starts := make([]time.Time, 0, len(ranges))
	ends := make([]time.Time, 0, len(ranges))
	for _, item := range ranges {
		accountIDs = append(accountIDs, item.AccountID)
		starts = append(starts, item.Start)
		ends = append(ends, item.End)
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH requested_ranges AS (
			SELECT * FROM UNNEST($1::bigint[], $2::timestamptz[], $3::timestamptz[])
				AS r(account_id, starts_at, ends_at)
		)
		SELECT
			r.account_id,
			COUNT(ul.id) AS requests,
			COALESCE(SUM(ul.actual_cost), 0) AS revenue,
			COALESCE(SUM(COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)), 0) AS metered_cost
		FROM requested_ranges r
		LEFT JOIN usage_logs ul ON ul.account_id = r.account_id
			AND ul.created_at >= r.starts_at AND ul.created_at < r.ends_at
		GROUP BY r.account_id
	`, pq.Array(accountIDs), pq.Array(starts), pq.Array(ends))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var accountID int64
		stats := &service.ProfitUsageStats{}
		if err := rows.Scan(&accountID, &stats.Requests, &stats.Revenue, &stats.MeteredCost); err != nil {
			return nil, err
		}
		result[accountID] = stats
	}
	return result, rows.Err()
}

func (r *profitRepository) GetAccountDailyUsageStats(ctx context.Context, accountIDs []int64, start, end time.Time, tzName string) ([]*service.ProfitAccountDailyUsagePoint, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			ul.account_id,
			TO_CHAR(date_trunc('day', ul.created_at AT TIME ZONE $4), 'YYYY-MM-DD') AS day,
			COUNT(*) AS requests,
			COALESCE(SUM(ul.actual_cost), 0) AS revenue,
			COALESCE(SUM(
				CASE WHEN a.type = 'apikey'
					THEN COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)
					ELSE 0
				END
			), 0) AS metered_cost
		FROM usage_logs ul
		LEFT JOIN accounts a ON a.id = ul.account_id
		WHERE ul.account_id = ANY($1) AND ul.created_at >= $2 AND ul.created_at < $3
		GROUP BY ul.account_id, 2
		ORDER BY 2, ul.account_id
	`, pq.Array(accountIDs), start, end, tzName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*service.ProfitAccountDailyUsagePoint
	for rows.Next() {
		point := &service.ProfitAccountDailyUsagePoint{}
		if err := rows.Scan(&point.AccountID, &point.Date, &point.Requests, &point.Revenue, &point.MeteredCost); err != nil {
			return nil, err
		}
		out = append(out, point)
	}
	return out, rows.Err()
}

func (r *profitRepository) GetDailyUsageStats(ctx context.Context, accountID *int64, start, end time.Time, tzName string) ([]*service.ProfitDailyUsagePoint, error) {
	query := `
		SELECT
			TO_CHAR(date_trunc('day', ul.created_at AT TIME ZONE $4), 'YYYY-MM-DD') AS day,
			COALESCE(SUM(ul.actual_cost), 0) AS revenue,
			COALESCE(SUM(
				CASE WHEN a.type = 'apikey'
					THEN COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)
					ELSE 0
				END
			), 0) AS metered_cost
		FROM usage_logs ul
		LEFT JOIN accounts a ON a.id = ul.account_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2
	`
	args := []any{start, end}
	if accountID != nil {
		query += ` AND ul.account_id = $3`
		args = append(args, *accountID)
	} else {
		query += ` AND $3::bigint IS NULL`
		args = append(args, nil)
	}
	query += ` GROUP BY 1 ORDER BY 1`
	args = append(args, tzName)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []*service.ProfitDailyUsagePoint
	for rows.Next() {
		p := &service.ProfitDailyUsagePoint{}
		if err := rows.Scan(&p.Date, &p.Revenue, &p.MeteredCost); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *profitRepository) GetBestWindowRevenue(ctx context.Context, accountID int64, since time.Time, windowSeconds int64) (float64, error) {
	if windowSeconds <= 0 {
		windowSeconds = 5 * 3600
	}
	var best float64
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(window_revenue), 0)
		FROM (
			SELECT SUM(actual_cost) OVER (
				ORDER BY created_at
				RANGE BETWEEN CURRENT ROW AND ($3 * INTERVAL '1 second') FOLLOWING
			) AS window_revenue
			FROM usage_logs
			WHERE account_id = $1 AND created_at >= $2 AND actual_cost > 0
		) w
	`, accountID, since, windowSeconds).Scan(&best)
	return best, err
}

func (r *profitRepository) GetBestWindowRevenueBatch(ctx context.Context, accountIDs []int64, since time.Time, windowSeconds int64) (map[int64]float64, error) {
	result := make(map[int64]float64, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
	}
	if windowSeconds <= 0 {
		windowSeconds = 5 * 3600
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT account_id, COALESCE(MAX(window_revenue), 0)
		FROM (
			SELECT account_id, SUM(actual_cost) OVER (
				PARTITION BY account_id
				ORDER BY created_at
				RANGE BETWEEN CURRENT ROW AND ($3 * INTERVAL '1 second') FOLLOWING
			) AS window_revenue
			FROM usage_logs
			WHERE account_id = ANY($1) AND created_at >= $2 AND actual_cost > 0
		) w
		GROUP BY account_id
	`, pq.Array(accountIDs), since, windowSeconds)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var accountID int64
		var best float64
		if err := rows.Scan(&accountID, &best); err != nil {
			return nil, err
		}
		result[accountID] = best
	}
	return result, rows.Err()
}

type costConfigScanner interface {
	Scan(dest ...any) error
}

func scanCostConfig(row costConfigScanner) (*service.AccountCostConfig, error) {
	cfg := &service.AccountCostConfig{}
	err := row.Scan(
		&cfg.ID, &cfg.AccountID, &cfg.CostType, &cfg.PeriodFee,
		&cfg.PeriodDays, &cfg.Currency,
		&cfg.WindowBaselineRevenue, &cfg.AutoRenew, &cfg.Notes, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func scanSubscriptionCycle(row costConfigScanner) (*service.AccountSubscriptionCycle, error) {
	cycle := &service.AccountSubscriptionCycle{}
	err := row.Scan(&cycle.ID, &cycle.AccountID, &cycle.StartsAt, &cycle.PeriodFee, &cycle.PeriodDays, &cycle.Currency, &cycle.Notes, &cycle.CreatedAt, &cycle.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return cycle, nil
}

// ListAutoRenewSubscriptionAccounts returns subscription cost configs with auto_renew enabled.
func (r *profitRepository) ListAutoRenewSubscriptionAccounts(ctx context.Context) ([]*service.AccountCostConfig, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, cost_type, period_fee, period_days, currency, window_baseline_revenue, auto_renew, notes, created_at, updated_at
		FROM account_cost_configs
		WHERE auto_renew = TRUE AND cost_type = 'subscription'
		ORDER BY account_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*service.AccountCostConfig
	for rows.Next() {
		cfg, err := scanCostConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

// HasSubscriptionCycleStartingAt reports whether account already has a cycle at starts_at (UTC day).
func (r *profitRepository) HasSubscriptionCycleStartingAt(ctx context.Context, accountID int64, startsAt time.Time) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM account_subscription_cycles
			WHERE account_id = $1 AND starts_at = $2
		)
	`, accountID, startsAt.UTC()).Scan(&exists)
	return exists, err
}

