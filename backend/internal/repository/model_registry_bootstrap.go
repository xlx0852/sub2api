package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/modelcatalog"
)

// 模型注册表来源开关：
//
//	file —— 只用编译进二进制的 catalog 文件（默认，行为与今天一致）
//	dual —— 同时读取注册表并比对差异，只打日志，不覆盖
//	db   —— 用注册表重建 catalog 的 models / aliases / 默认价（ui_presets 仍来自文件）
const (
	modelRegistrySourceFile = "file"
	modelRegistrySourceDual = "dual"
	modelRegistrySourceDB   = "db"

	settingKeyModelRegistrySource = "model_registry_source"
)

func normalizeModelRegistrySource(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case modelRegistrySourceDual:
		return modelRegistrySourceDual
	case modelRegistrySourceDB, "database":
		return modelRegistrySourceDB
	default:
		return modelRegistrySourceFile
	}
}

// ApplyModelRegistrySource 在迁移完成后、路由注册前执行一次：
// 读取运维开关，按需用数据库注册表覆盖内存中的 catalog。
//
// 注册表自带"文件兜底"语义：读表失败或开关为 file 时什么都不做，
// 服务继续用编译进来的 catalog，绝不会因为注册表问题启动失败。
func ApplyModelRegistrySource(ctx context.Context, db *sql.DB) {
	if db == nil {
		return
	}
	source := modelRegistrySourceFile
	var raw sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = $1`, settingKeyModelRegistrySource).Scan(&raw); err == nil && raw.Valid {
		source = normalizeModelRegistrySource(raw.String)
	}

	switch source {
	case modelRegistrySourceFile:
		slog.Info("model_registry.source", "source", source, "action", "file catalog in use")
		return
	case modelRegistrySourceDual:
		snap, err := loadRegistrySnapshot(ctx, db)
		if err != nil {
			slog.Warn("model_registry.dual_load_failed", "error", err)
			return
		}
		diffs, err := modelcatalog.DiffRegistry(snap)
		if err != nil {
			slog.Warn("model_registry.dual_diff_failed", "error", err)
			return
		}
		for _, d := range diffs {
			slog.Warn("model_registry.mismatch", "diff", d)
		}
		slog.Info("model_registry.source", "source", source, "diffs", len(diffs))
	case modelRegistrySourceDB:
		snap, err := loadRegistrySnapshot(ctx, db)
		if err != nil {
			slog.Warn("model_registry.db_load_failed_falling_back_to_file", "error", err)
			return
		}
		diffs, err := modelcatalog.ApplyRegistry(snap)
		if err != nil {
			slog.Warn("model_registry.apply_failed_falling_back_to_file", "error", err)
			return
		}
		slog.Info("model_registry.source", "source", source,
			"models", len(snap.Models), "aliases", len(snap.Aliases),
			"default_prices", len(snap.DefaultPrices), "diffs_from_file", len(diffs))
	}
}

func loadRegistrySnapshot(ctx context.Context, db *sql.DB) (*modelcatalog.RegistrySnapshot, error) {
	snap := &modelcatalog.RegistrySnapshot{DefaultPrices: map[string]modelcatalog.PriceEntry{}}

	rows, err := db.QueryContext(ctx, `
		SELECT id, platform, display_name, family, owned_by, context_window, max_context_window,
		       media, is_reasoning, enabled, retired, is_default_chat, is_default_test, upstream_id
		FROM models ORDER BY platform, id`)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	for rows.Next() {
		var m modelcatalog.RegistryModel
		var media []byte
		var ctxWindow, maxCtx sql.NullInt64
		if err := rows.Scan(&m.ID, &m.Platform, &m.DisplayName, &m.Family, &m.OwnedBy,
			&ctxWindow, &maxCtx, &media, &m.IsReasoning, &m.Enabled, &m.Retired,
			&m.IsDefaultChat, &m.IsDefaultTest, &m.UpstreamID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan model: %w", err)
		}
		if ctxWindow.Valid {
			m.ContextWindow = int(ctxWindow.Int64)
		}
		if maxCtx.Valid {
			m.MaxContextWindow = int(maxCtx.Int64)
		}
		if len(media) > 0 {
			m.Media = modelcatalog.DecodeMediaFlags(media)
		}
		snap.Models = append(snap.Models, m)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate models: %w", err)
	}
	_ = rows.Close()

	aliasRows, err := db.QueryContext(ctx, `SELECT alias, platform, model_id, kind FROM model_aliases ORDER BY platform, alias`)
	if err != nil {
		return nil, fmt.Errorf("list aliases: %w", err)
	}
	for aliasRows.Next() {
		var a modelcatalog.RegistryAlias
		if err := aliasRows.Scan(&a.Alias, &a.Platform, &a.ModelID, &a.Kind); err != nil {
			_ = aliasRows.Close()
			return nil, fmt.Errorf("scan alias: %w", err)
		}
		snap.Aliases = append(snap.Aliases, a)
	}
	if err := aliasRows.Err(); err != nil {
		_ = aliasRows.Close()
		return nil, fmt.Errorf("iterate aliases: %w", err)
	}
	_ = aliasRows.Close()

	priceRows, err := db.QueryContext(ctx, `
		SELECT model_id, currency, input_price, output_price, cache_write_price, cache_read_price,
		       input_price_priority, output_price_priority, image_output_price
		FROM model_prices WHERE group_id IS NULL AND enabled`)
	if err != nil {
		return nil, fmt.Errorf("list default prices: %w", err)
	}
	defer func() { _ = priceRows.Close() }()
	for priceRows.Next() {
		var modelID, currency string
		var in, out, cw, cr, inP, outP, img *float64
		if err := priceRows.Scan(&modelID, &currency, &in, &out, &cw, &cr, &inP, &outP, &img); err != nil {
			return nil, fmt.Errorf("scan default price: %w", err)
		}
		entry := modelcatalog.PriceEntry{Currency: currency}
		if in != nil {
			entry.InputCostPerToken = *in
		}
		if out != nil {
			entry.OutputCostPerToken = *out
		}
		if cw != nil {
			entry.CacheCreationInputTokenCost = *cw
		}
		if cr != nil {
			entry.CacheReadInputTokenCost = *cr
		}
		if inP != nil {
			entry.InputCostPerTokenPriority = *inP
		}
		if outP != nil {
			entry.OutputCostPerTokenPriority = *outP
		}
		if img != nil {
			entry.OutputCostPerImage = *img
		}
		snap.DefaultPrices[modelID] = entry
	}
	if err := priceRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate default prices: %w", err)
	}
	return snap, nil
}
