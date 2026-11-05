package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ListPriceModifiers 读取所有启用的修饰因子（量极小：种子只有 3 行）。
func (r *channelRepository) ListPriceModifiers(ctx context.Context) ([]domain.PriceModifier, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, scope, target, kind, when_mode, window_start, window_end, tz,
		       weekdays, tier_value, multiplier, enabled, priority
		FROM price_modifiers
		WHERE enabled
		ORDER BY scope, priority, id`)
	if err != nil {
		return nil, fmt.Errorf("list price modifiers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []domain.PriceModifier
	for rows.Next() {
		var m domain.PriceModifier
		var weekdays []int64
		if err := rows.Scan(
			&m.ID, &m.Scope, &m.Target, &m.Kind, &m.WhenMode, &m.WindowStart, &m.WindowEnd, &m.TZ,
			&weekdays, &m.TierValue, &m.Multiplier, &m.Enabled, &m.Priority,
		); err != nil {
			return nil, fmt.Errorf("scan price modifier: %w", err)
		}
		if len(weekdays) > 0 {
			m.Weekdays = make([]int, 0, len(weekdays))
			for _, d := range weekdays {
				m.Weekdays = append(m.Weekdays, int(d))
			}
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate price modifiers: %w", err)
	}
	return result, nil
}

// GetPriceCost 读取一条成本基。
func (r *channelRepository) GetPriceCost(ctx context.Context, id int64) (*domain.PriceCost, error) {
	if id <= 0 {
		return nil, nil
	}
	var cost domain.PriceCost
	var cached, output, cacheWrite *float64
	err := r.db.QueryRowContext(ctx, `
		SELECT id, vendor, model_pattern, currency, input_per_mtok,
		       cached_per_mtok, output_per_mtok, cache_write_per_mtok, discount_pct, observed_at
		FROM price_costs WHERE id = $1`, id).Scan(
		&cost.ID, &cost.Vendor, &cost.ModelPattern, &cost.Currency, &cost.InputPerMTok,
		&cached, &output, &cacheWrite, &cost.DiscountPct, &cost.ObservedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get price cost %d: %w", id, err)
	}
	if cached != nil {
		cost.CachedPerMTok = *cached
	}
	if output != nil {
		cost.OutputPerMTok = *output
	}
	if cacheWrite != nil {
		cost.CacheWritePerMTok = *cacheWrite
	}
	return &cost, nil
}
