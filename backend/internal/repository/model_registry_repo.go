package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// --- 模型注册表后台管理（models / model_aliases / model_prices） ---

// ListModelRegistry 返回模型清单（可按平台/关键字过滤），并带上已配价格行数。
func (r *channelRepository) ListModelRegistry(ctx context.Context, platform, search string) ([]domain.ModelRegistryEntry, error) {
	args := []any{}
	where := []string{"1=1"}
	if p := strings.TrimSpace(platform); p != "" {
		where = append(where, fmt.Sprintf("m.platform = $%d", len(args)+1))
		args = append(args, p)
	}
	if q := strings.TrimSpace(search); q != "" {
		where = append(where, fmt.Sprintf("(m.id ILIKE $%d OR m.display_name ILIKE $%d)", len(args)+1, len(args)+1))
		args = append(args, "%"+q+"%")
	}

	query := fmt.Sprintf(`
		SELECT m.id, m.platform, m.display_name, m.family, m.owned_by,
		       m.context_window, m.max_context_window, m.media, m.is_reasoning,
		       m.enabled, m.retired, m.is_default_chat, m.is_default_test,
		       m.upstream_id, m.version, m.notes,
		       (SELECT count(*) FROM model_prices p WHERE p.model_id = m.id) AS price_rows
		FROM models m
		WHERE %s
		ORDER BY m.platform, m.id
		LIMIT 2000`, strings.Join(where, " AND "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.ModelRegistryEntry
	for rows.Next() {
		var m domain.ModelRegistryEntry
		var ctxWin, maxCtx sql.NullInt64
		var media []byte
		if err := rows.Scan(&m.ID, &m.Platform, &m.DisplayName, &m.Family, &m.OwnedBy,
			&ctxWin, &maxCtx, &media, &m.IsReasoning, &m.Enabled, &m.Retired,
			&m.IsDefaultChat, &m.IsDefaultTest, &m.UpstreamID, &m.Version, &m.Notes,
			&m.PriceRowCount); err != nil {
			return nil, fmt.Errorf("scan model: %w", err)
		}
		if ctxWin.Valid {
			v := int(ctxWin.Int64)
			m.ContextWindow = &v
		}
		if maxCtx.Valid {
			v := int(maxCtx.Int64)
			m.MaxContextWindow = &v
		}
		if len(media) > 0 {
			_ = json.Unmarshal(media, &m.Media)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate models: %w", err)
	}
	return out, nil
}

// UpsertModelRegistry（写入语义见 service 层）；created=false 表示更新。
func (r *channelRepository) UpsertModelRegistry(ctx context.Context, m *domain.ModelRegistryEntry, expectedVersion int64) (int64, error) {
	if m == nil || strings.TrimSpace(m.ID) == "" {
		return 0, fmt.Errorf("model id is required")
	}
	media, err := json.Marshal(m.Media)
	if err != nil {
		return 0, fmt.Errorf("marshal media: %w", err)
	}
	var ctxWin, maxCtx any
	if m.ContextWindow != nil {
		ctxWin = *m.ContextWindow
	}
	if m.MaxContextWindow != nil {
		maxCtx = *m.MaxContextWindow
	}

	var version int64
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO models (id, platform, display_name, family, owned_by, context_window,
		                    max_context_window, media, is_reasoning, enabled, retired,
		                    is_default_chat, is_default_test, upstream_id, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (id) DO UPDATE SET
		    platform = EXCLUDED.platform,
		    display_name = EXCLUDED.display_name,
		    family = EXCLUDED.family,
		    owned_by = EXCLUDED.owned_by,
		    context_window = EXCLUDED.context_window,
		    max_context_window = EXCLUDED.max_context_window,
		    media = EXCLUDED.media,
		    is_reasoning = EXCLUDED.is_reasoning,
		    enabled = EXCLUDED.enabled,
		    retired = EXCLUDED.retired,
		    is_default_chat = EXCLUDED.is_default_chat,
		    is_default_test = EXCLUDED.is_default_test,
		    upstream_id = EXCLUDED.upstream_id,
		    notes = EXCLUDED.notes,
		    version = models.version + 1,
		    updated_at = NOW()
		WHERE $16 = 0 OR models.version = $16
		RETURNING version`,
		m.ID, m.Platform, m.DisplayName, m.Family, m.OwnedBy, ctxWin, maxCtx, media,
		m.IsReasoning, m.Enabled, m.Retired, m.IsDefaultChat, m.IsDefaultTest,
		m.UpstreamID, m.Notes, expectedVersion).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errVersionConflict
	}
	if err != nil {
		return 0, fmt.Errorf("upsert model: %w", err)
	}
	return version, nil
}

// ListModelAliases 返回某模型的别名。
func (r *channelRepository) ListModelAliases(ctx context.Context, modelID string) ([]domain.ModelAliasEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT alias, platform, model_id, kind FROM model_aliases
		WHERE model_id = $1 ORDER BY kind, alias`, modelID)
	if err != nil {
		return nil, fmt.Errorf("list aliases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.ModelAliasEntry
	for rows.Next() {
		var a domain.ModelAliasEntry
		if err := rows.Scan(&a.Alias, &a.Platform, &a.ModelID, &a.Kind); err != nil {
			return nil, fmt.Errorf("scan alias: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AliasConflict 报告该 alias 是否已指向别的模型。
func (r *channelRepository) AliasConflict(ctx context.Context, alias, platform, modelID string) (string, error) {
	var existing string
	err := r.db.QueryRowContext(ctx, `
		SELECT model_id FROM model_aliases WHERE alias = $1 AND platform = $2`,
		strings.TrimSpace(alias), strings.TrimSpace(platform)).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("check alias: %w", err)
	}
	if existing != modelID {
		return existing, nil
	}
	return "", nil
}

// ReplaceModelAliases 用给定集合替换该模型的别名（只动本模型的别名）。
func (r *channelRepository) ReplaceModelAliases(ctx context.Context, modelID string, aliases []domain.ModelAliasEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM model_aliases WHERE model_id = $1`, modelID); err != nil {
		return fmt.Errorf("clear aliases: %w", err)
	}
	for _, a := range aliases {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO model_aliases (alias, platform, model_id, kind)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (alias, platform) DO UPDATE SET model_id = EXCLUDED.model_id, kind = EXCLUDED.kind`,
			strings.TrimSpace(a.Alias), strings.TrimSpace(a.Platform), modelID, a.Kind); err != nil {
			return fmt.Errorf("insert alias: %w", err)
		}
	}
	return tx.Commit()
}

// UpsertModelPrice 写入一条价格行（group_id 为空 = 平台默认价）。
func (r *channelRepository) UpsertModelPrice(ctx context.Context, row *domain.ModelPriceRow) error {
	if row == nil || strings.TrimSpace(row.ModelID) == "" {
		return fmt.Errorf("model id is required")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO model_prices (model_id, group_id, billing_mode, currency, fx_mode, cost_id,
		                          fx_rate, markup_pct, input_price, output_price, cache_write_price,
		                          cache_read_price, input_price_priority, output_price_priority,
		                          image_output_price, per_request_price, context_tiers, image_tiers, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::jsonb,$18::jsonb,$19)
		ON CONFLICT (model_id, group_id) WHERE group_id IS NOT NULL DO UPDATE SET
		    billing_mode = EXCLUDED.billing_mode, currency = EXCLUDED.currency,
		    fx_mode = EXCLUDED.fx_mode, cost_id = EXCLUDED.cost_id, fx_rate = EXCLUDED.fx_rate,
		    markup_pct = EXCLUDED.markup_pct, input_price = EXCLUDED.input_price,
		    output_price = EXCLUDED.output_price, cache_write_price = EXCLUDED.cache_write_price,
		    cache_read_price = EXCLUDED.cache_read_price,
		    input_price_priority = EXCLUDED.input_price_priority,
		    output_price_priority = EXCLUDED.output_price_priority,
		    image_output_price = EXCLUDED.image_output_price,
		    per_request_price = EXCLUDED.per_request_price,
		    context_tiers = EXCLUDED.context_tiers, image_tiers = EXCLUDED.image_tiers,
		    enabled = EXCLUDED.enabled, version = model_prices.version + 1, updated_at = NOW()`,
		row.ModelID, row.GroupID, row.BillingMode, row.Currency, row.FxMode, row.CostID,
		row.FxRate, row.MarkupPct, row.InputPrice, row.OutputPrice, row.CacheWritePrice,
		row.CacheReadPrice, row.InputPricePriority, row.OutputPricePriority,
		row.ImageOutputPrice, row.PerRequestPrice, defaultJSON(row.ContextTiers), defaultJSON(row.ImageTiers),
		row.Enabled)
	if err != nil {
		// 默认价（group_id 为空）走部分唯一索引，单独处理
		if strings.Contains(err.Error(), "model_prices_one_default_per_model") {
			_, err = r.db.ExecContext(ctx, `
				UPDATE model_prices SET billing_mode = $2, currency = $3, fx_mode = $4, cost_id = $5,
				    fx_rate = $6, markup_pct = $7, input_price = $8, output_price = $9,
				    cache_write_price = $10, cache_read_price = $11, input_price_priority = $12,
				    output_price_priority = $13, image_output_price = $14, per_request_price = $15,
				    context_tiers = $16::jsonb, image_tiers = $17::jsonb, enabled = $18,
				    version = version + 1, updated_at = NOW()
				WHERE model_id = $1 AND group_id IS NULL`,
				row.ModelID, row.BillingMode, row.Currency, row.FxMode, row.CostID, row.FxRate,
				row.MarkupPct, row.InputPrice, row.OutputPrice, row.CacheWritePrice, row.CacheReadPrice,
				row.InputPricePriority, row.OutputPricePriority, row.ImageOutputPrice,
				row.PerRequestPrice, defaultJSON(row.ContextTiers), defaultJSON(row.ImageTiers), row.Enabled)
		}
		if err != nil {
			return fmt.Errorf("upsert model price: %w", err)
		}
	}
	return nil
}

// DeleteModelPrice 删除一条价格行（groupID 为空表示默认价）。
func (r *channelRepository) DeleteModelPrice(ctx context.Context, modelID string, groupID *int64) error {
	if groupID == nil {
		_, err := r.db.ExecContext(ctx, `DELETE FROM model_prices WHERE model_id = $1 AND group_id IS NULL`, modelID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM model_prices WHERE model_id = $1 AND group_id = $2`, modelID, *groupID)
	return err
}

var errVersionConflict = errors.New("version conflict")

// ErrRegistryVersionConflict 暴露给 service 层判断并发冲突。
func ErrRegistryVersionConflict() error { return errVersionConflict }

func defaultJSON(b []byte) []byte {
	if len(b) == 0 {
		return []byte("[]")
	}
	return b
}

// ListModelPrices 返回某模型的全部价格行（默认价 + 各组价）。
func (r *channelRepository) ListModelPrices(ctx context.Context, modelID string) ([]domain.ModelPriceRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, model_id, group_id, billing_mode, currency, fx_mode, cost_id, fx_rate, markup_pct,
		       input_price, output_price, cache_write_price, cache_read_price,
		       input_price_priority, output_price_priority, image_output_price, per_request_price,
		       context_tiers, image_tiers, enabled, version
		FROM model_prices WHERE model_id = $1
		ORDER BY group_id NULLS FIRST`, modelID)
	if err != nil {
		return nil, fmt.Errorf("list model prices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.ModelPriceRow
	for rows.Next() {
		var row domain.ModelPriceRow
		var group sql.NullInt64
		if err := rows.Scan(&row.ID, &row.ModelID, &group, &row.BillingMode, &row.Currency, &row.FxMode,
			&row.CostID, &row.FxRate, &row.MarkupPct, &row.InputPrice, &row.OutputPrice,
			&row.CacheWritePrice, &row.CacheReadPrice, &row.InputPricePriority, &row.OutputPricePriority,
			&row.ImageOutputPrice, &row.PerRequestPrice, &row.ContextTiers, &row.ImageTiers,
			&row.Enabled, &row.Version); err != nil {
			return nil, fmt.Errorf("scan model price: %w", err)
		}
		if group.Valid {
			v := group.Int64
			row.GroupID = &v
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
