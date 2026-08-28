package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration203AddsConcurrentStartedReconciliationIndex(t *testing.T) {
	content, err := FS.ReadFile("203_usage_settlement_started_reconciliation_index_notx.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_settlement_attempts_started_reconciliation")
	require.Contains(t, sql, "ON usage_settlement_attempts (reconciliation_deadline, id)")
	require.Contains(t, sql, "WHERE state = 'started' AND reconciliation_deadline IS NOT NULL")
	require.NotContains(t, strings.ToUpper(sql), "BEGIN")
	require.NotContains(t, strings.ToUpper(sql), "COMMIT")
}
