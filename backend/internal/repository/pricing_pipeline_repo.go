package repository

import (
	"context"
	"fmt"
	"strings"

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

// ListPriceCosts 读取全部成本基（后台成本基列表页用）。
func (r *channelRepository) ListPriceCosts(ctx context.Context) ([]domain.PriceCost, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, vendor, model_pattern, currency, input_per_mtok,
		       cached_per_mtok, output_per_mtok, cache_write_per_mtok, discount_pct, observed_at, source_url, notes
		FROM price_costs ORDER BY vendor, model_pattern`)
	if err != nil {
		return nil, fmt.Errorf("list price costs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []domain.PriceCost
	for rows.Next() {
		var cost domain.PriceCost
		var cached, output, cacheWrite *float64
		if err := rows.Scan(&cost.ID, &cost.Vendor, &cost.ModelPattern, &cost.Currency, &cost.InputPerMTok,
			&cached, &output, &cacheWrite, &cost.DiscountPct, &cost.ObservedAt, &cost.SourceURL, &cost.Notes); err != nil {
			return nil, fmt.Errorf("scan price cost: %w", err)
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
		result = append(result, cost)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate price costs: %w", err)
	}
	return result, nil
}

// UpsertPriceCost 按 (vendor, model_pattern) 幂等写入成本基。
func (r *channelRepository) UpsertPriceCost(ctx context.Context, cost *domain.PriceCost) (*domain.PriceCost, error) {
	if cost == nil {
		return nil, fmt.Errorf("price cost is nil")
	}
	vendor := strings.TrimSpace(cost.Vendor)
	pattern := strings.TrimSpace(cost.ModelPattern)
	if vendor == "" || pattern == "" {
		return nil, fmt.Errorf("vendor and model_pattern are required")
	}
	if cost.Currency == "" {
		cost.Currency = domain.PricingCurrencyUSD
	}
	var out domain.PriceCost
	var cached, output, cacheWrite *float64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO price_costs (vendor, model_pattern, currency, input_per_mtok, cached_per_mtok,
		                         output_per_mtok, cache_write_per_mtok, discount_pct, observed_at, source_url, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, $10)
		ON CONFLICT (vendor, model_pattern) DO UPDATE
		SET currency = EXCLUDED.currency,
		    input_per_mtok = EXCLUDED.input_per_mtok,
		    cached_per_mtok = EXCLUDED.cached_per_mtok,
		    output_per_mtok = EXCLUDED.output_per_mtok,
		    cache_write_per_mtok = EXCLUDED.cache_write_per_mtok,
		    discount_pct = EXCLUDED.discount_pct,
		    observed_at = NOW(),
		    source_url = EXCLUDED.source_url,
		    notes = EXCLUDED.notes,
		    updated_at = NOW()
		RETURNING id, vendor, model_pattern, currency, input_per_mtok,
		          cached_per_mtok, output_per_mtok, cache_write_per_mtok, discount_pct, observed_at, source_url, notes`,
		vendor, pattern, string(domain.NormalizePricingCurrency(string(cost.Currency))),
		cost.InputPerMTok, nullableFloat(cost.CachedPerMTok), cost.OutputPerMTok,
		nullableFloat(cost.CacheWritePerMTok), cost.DiscountPct, cost.SourceURL, cost.Notes).Scan(
		&out.ID, &out.Vendor, &out.ModelPattern, &out.Currency, &out.InputPerMTok,
		&cached, &output, &cacheWrite, &out.DiscountPct, &out.ObservedAt, &out.SourceURL, &out.Notes)
	if err != nil {
		return nil, fmt.Errorf("upsert price cost: %w", err)
	}
	if cached != nil {
		out.CachedPerMTok = *cached
	}
	if output != nil {
		out.OutputPerMTok = *output
	}
	if cacheWrite != nil {
		out.CacheWritePerMTok = *cacheWrite
	}
	return &out, nil
}

// DeletePriceCost 删除一条成本基；被价格行引用时由外键 SET NULL 兜底。
func (r *channelRepository) DeletePriceCost(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid price cost id")
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM price_costs WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete price cost %d: %w", id, err)
	}
	return nil
}

func nullableFloat(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}
