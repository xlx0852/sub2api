package repository

import (
	"os"
	"strings"
	"testing"
)

// TestScheduledTestPlanQueriesSelectEveryScannedColumn guards the diagnostics runner from
// silently losing plans: every query that feeds scanPlan must return the full column set.
//
// A column added to the table but missed in one list (exactly what happened to the
// p.-prefixed RETURNING in ClaimDue when purity settings were added) makes the row scan
// fail at runtime, and the runner then claims nothing at all.
func TestScheduledTestPlanQueriesSelectEveryScannedColumn(t *testing.T) {
	body, err := os.ReadFile("scheduled_test_repo.go")
	if err != nil {
		t.Fatalf("read repository source: %v", err)
	}
	// The columns scanPlan reads, in order.
	required := []string{
		"id", "account_id", "model_id", "cron_expression", "enabled", "max_results",
		"auto_recover",
		"last_run_at", "next_run_at", "created_at", "updated_at",
	}

	checked := 0
	for _, statement := range backtickStatements(string(body)) {
		if !strings.Contains(statement, "scheduled_test_plans") {
			continue
		}
		// Only full plan-row projections must carry every column; aggregate summaries
		// (counts per account) legitimately select a different shape.
		if !strings.Contains(statement, "model_id") {
			continue
		}
		if !strings.Contains(statement, "SELECT") && !strings.Contains(statement, "RETURNING") {
			continue
		}
		checked++
		for _, column := range required {
			if !containsColumn(statement, column) {
				t.Errorf("plan query is missing column %q:\n%s", column, statement)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no plan row queries were inspected; the guard would silently pass")
	}
}

// backtickStatements extracts the SQL held in Go raw string literals.
func backtickStatements(source string) []string {
	parts := strings.Split(source, "`")
	statements := make([]string, 0, len(parts)/2)
	for index := 1; index < len(parts); index += 2 {
		statements = append(statements, parts[index])
	}
	return statements
}

// containsColumn matches a column name as a whole token, ignoring a table alias prefix.
func containsColumn(statement string, column string) bool {
	for _, token := range strings.FieldsFunc(statement, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == '(' || r == ')' || r == ';'
	}) {
		trimmed := strings.TrimSpace(token)
		if trimmed == column || strings.HasSuffix(trimmed, "."+column) {
			return true
		}
	}
	return false
}
