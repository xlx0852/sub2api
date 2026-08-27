package repository

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

var retrospectiveRunColumns = []string{
	"id", "run_key", "scheduled_day", "status", "manual", "policy_version",
	"claimed_count", "reviewed_count", "flagged_count", "failed_count", "input_tokens",
	"cost_units", "error", "started_at", "heartbeat_at", "finished_at",
}

func TestSetRiskProfileClearIsAudited(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery("SELECT level FROM content_moderation_user_risk_profiles").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"level"}).AddRow(domain.RiskLevelHigh))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO content_moderation_user_risk_profiles").WithArgs(int64(42), domain.RiskLevelNormal, "cleared", nil).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO content_moderation_risk_events").WithArgs(sqlmock.AnyArg(), int64(42), domain.RiskLevelHigh, domain.RiskLevelNormal, int64(9), "false positive confirmed").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO content_moderation_operator_audits").WithArgs(int64(9), int64(42), "false positive confirmed").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT p.user_id").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{
		"user_id", "email", "username", "level", "score", "confidence", "reason_codes", "provenance", "review_status", "expires_at", "last_event_at", "created_at", "updated_at",
	}).AddRow(42, "user@example.com", "user", domain.RiskLevelNormal, 0.0, 0.0, []byte(`[]`), "admin", "cleared", nil, now, now, now))
	repo := &retrospectiveRiskRepository{db: db}
	adminID := int64(9)
	profile, err := repo.SetRiskProfile(context.Background(), 42, domain.RiskLevelNormal, "cleared", "false positive confirmed", &adminID, 7*24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, domain.RiskLevelNormal, profile.Level)
	require.Equal(t, "cleared", profile.ReviewStatus)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateEvidenceAccessGrantUsesConsistentEvidenceIDType(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	expiresAt := time.Now().Add(5 * time.Minute)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO content_moderation_evidence_access_grants").
		WithArgs("evidence-1", int64(9), "token-hash", "incident review", expiresAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`SELECT \$2,'evidence_authorize',\$1::varchar\(36\),user_id,\$3,'granted'`).
		WithArgs("evidence-1", int64(9), "incident review").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := &retrospectiveRiskRepository{db: db}
	require.NoError(t, repo.CreateEvidenceAccessGrant(context.Background(), "evidence-1", 9, "token-hash", "incident review", expiresAt))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateReviewRunResumesInterruptedRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery("INSERT INTO content_moderation_review_runs").WillReturnRows(sqlmock.NewRows(retrospectiveRunColumns))
	mock.ExpectQuery("UPDATE content_moderation_review_runs").WithArgs("scheduled:2026-08-22:v1").WillReturnRows(
		sqlmock.NewRows(retrospectiveRunColumns).AddRow(7, "scheduled:2026-08-22:v1", now, "running", false, "v1", 3, 2, 1, 0, 120, 0.0, "", now, now, nil),
	)
	repo := &retrospectiveRiskRepository{db: db}
	run, active, err := repo.CreateReviewRun(context.Background(), "scheduled:2026-08-22:v1", now, false, "v1", []byte(`{}`))
	require.NoError(t, err)
	require.True(t, active)
	require.Equal(t, int64(7), run.ID)
	require.Equal(t, int64(2), run.ReviewedCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateReviewRunDoesNotRepeatCompletedRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery("INSERT INTO content_moderation_review_runs").WillReturnRows(sqlmock.NewRows(retrospectiveRunColumns))
	mock.ExpectQuery("UPDATE content_moderation_review_runs").WithArgs("scheduled:2026-08-22:v1").WillReturnRows(sqlmock.NewRows(retrospectiveRunColumns))
	mock.ExpectQuery("SELECT id, run_key, scheduled_day").WithArgs("scheduled:2026-08-22:v1").WillReturnRows(
		sqlmock.NewRows(retrospectiveRunColumns).AddRow(8, "scheduled:2026-08-22:v1", now, "completed", false, "v1", 4, 4, 0, 0, 200, 0.0, "", now, now, now),
	)
	repo := &retrospectiveRiskRepository{db: db}
	run, active, err := repo.CreateReviewRun(context.Background(), "scheduled:2026-08-22:v1", now, false, "v1", []byte(`{}`))
	require.NoError(t, err)
	require.False(t, active)
	require.Equal(t, "completed", run.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}
