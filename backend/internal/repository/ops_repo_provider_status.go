package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) InsertProviderStatusSnapshot(ctx context.Context, input *service.ProviderStatusSnapshotRecord) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("nil ops repository")
	}
	if input == nil || input.Provider == "" || input.ContentHash == "" {
		return false, fmt.Errorf("invalid provider status snapshot")
	}
	result, err := r.db.ExecContext(ctx, `
INSERT INTO provider_status_snapshots (
  provider, source_url, overall_indicator, overall_description,
  components, incidents, source_updated_at, fetched_at, content_hash
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (provider, content_hash) DO NOTHING`,
		input.Provider,
		input.SourceURL,
		input.OverallIndicator,
		input.OverallDescription,
		input.ComponentsJSON,
		input.IncidentsJSON,
		opsNullTime(input.SourceUpdatedAt),
		input.FetchedAt,
		input.ContentHash,
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (r *opsRepository) GetLatestProviderStatusSnapshot(ctx context.Context, provider string) (*service.ProviderStatusSnapshotRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT id, provider, source_url, overall_indicator, overall_description,
       components, incidents, source_updated_at, fetched_at, content_hash
FROM provider_status_snapshots
WHERE provider = $1
ORDER BY fetched_at DESC, id DESC
LIMIT 1`, provider)
	item, err := scanProviderStatusSnapshot(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return item, nil
}

func (r *opsRepository) ListProviderStatusSnapshots(ctx context.Context, filter *service.ProviderStatusHistoryFilter) ([]*service.ProviderStatusSnapshotRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		filter = &service.ProviderStatusHistoryFilter{Provider: service.ProviderStatusOpenAI}
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	start := time.Unix(0, 0).UTC()
	if filter.StartTime != nil {
		start = filter.StartTime.UTC()
	}
	end := time.Now().UTC().Add(time.Minute)
	if filter.EndTime != nil {
		end = filter.EndTime.UTC()
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, provider, source_url, overall_indicator, overall_description,
       components, incidents, source_updated_at, fetched_at, content_hash
FROM provider_status_snapshots
WHERE provider = $1 AND fetched_at >= $2 AND fetched_at < $3
ORDER BY fetched_at DESC, id DESC
LIMIT $4`, filter.Provider, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]*service.ProviderStatusSnapshotRecord, 0, limit)
	for rows.Next() {
		item, err := scanProviderStatusSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type providerStatusScanner interface {
	Scan(dest ...any) error
}

func scanProviderStatusSnapshot(scanner providerStatusScanner) (*service.ProviderStatusSnapshotRecord, error) {
	var item service.ProviderStatusSnapshotRecord
	var sourceUpdatedAt sql.NullTime
	if err := scanner.Scan(
		&item.ID,
		&item.Provider,
		&item.SourceURL,
		&item.OverallIndicator,
		&item.OverallDescription,
		&item.ComponentsJSON,
		&item.IncidentsJSON,
		&sourceUpdatedAt,
		&item.FetchedAt,
		&item.ContentHash,
	); err != nil {
		return nil, err
	}
	if sourceUpdatedAt.Valid {
		t := sourceUpdatedAt.Time
		item.SourceUpdatedAt = &t
	}
	return &item, nil
}
