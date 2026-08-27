package repository

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestRetrospectiveRiskMigrationIntegration(t *testing.T) {
	dsn := os.Getenv("SUB2API_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set SUB2API_MIGRATION_TEST_DSN")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	require.NoError(t, ApplyMigrations(ctx, db))
	for _, table := range []string{
		"content_moderation_evidence", "content_moderation_review_runs",
		"content_moderation_review_items", "content_moderation_user_risk_profiles",
		"content_moderation_risk_events",
		"content_moderation_evidence_access_grants", "content_moderation_operator_audits",
	} {
		var name string
		require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass($1)::text`, table).Scan(&name))
		require.Equal(t, table, name)
	}
}

func TestRetrospectiveMigrationStoresMetadataNotPlaintext(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/200_retrospective_risk_review.sql")
	require.NoError(t, err)
	sqlText := strings.ToLower(string(raw))
	require.Contains(t, sqlText, "content_hash")
	require.Contains(t, sqlText, "redacted_excerpt")
	require.NotContains(t, sqlText, "original_content")
	require.NotContains(t, sqlText, "plaintext_content")
}
