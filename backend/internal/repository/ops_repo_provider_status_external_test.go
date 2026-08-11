package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// This opt-in test verifies the live public status payload and PostgreSQL
// change-only persistence together without making the regular unit suite
// depend on network or Docker availability.
func TestProviderStatusLiveFetchAndPersistence(t *testing.T) {
	dsn := os.Getenv("PROVIDER_STATUS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PROVIDER_STATUS_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))
	migration, err := migrations.FS.ReadFile("194_provider_status_snapshots.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "TRUNCATE provider_status_snapshots RESTART IDENTITY")
	require.NoError(t, err)

	record, err := service.NewOpenAIStatusClient(5*time.Second, 512*1024).Fetch(ctx, time.Now().UTC())
	require.NoError(t, err)
	repo := NewOpsRepository(db)
	inserted, err := repo.InsertProviderStatusSnapshot(ctx, record)
	require.NoError(t, err)
	require.True(t, inserted)
	inserted, err = repo.InsertProviderStatusSnapshot(ctx, record)
	require.NoError(t, err)
	require.False(t, inserted)
	latest, err := repo.GetLatestProviderStatusSnapshot(ctx, service.ProviderStatusOpenAI)
	require.NoError(t, err)
	require.Equal(t, record.ContentHash, latest.ContentHash)
}
