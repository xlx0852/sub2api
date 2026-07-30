package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestProfitRepositoryListSubscriptionCyclesEnrichesBanAndRefund(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	bannedAt := start.AddDate(0, 0, 10)
	now := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("FROM account_subscription_cycles") + `.*` + regexp.QuoteMeta("WHERE account_id = $1")).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "starts_at", "period_fee", "period_days", "currency", "notes", "created_at", "updated_at"}).
			AddRow(int64(8), int64(91), start, 865.0, 30, "USD", "", now, now))
	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("FROM account_subscription_terminations") + `.*` + regexp.QuoteMeta("reversed_at IS NULL")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "cycle_id", "account_id", "effective_at", "reason", "notes", "reversed_at", "reversal_reason", "created_at", "updated_at"}).
			AddRow(int64(19), int64(8), int64(91), bannedAt, "upstream_banned", "policy", nil, "", now, now))
	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("FROM account_subscription_refunds r") + `.*` + regexp.QuoteMeta("JOIN account_subscription_terminations t")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "termination_id", "cycle_id", "account_id", "amount", "currency", "received_at", "notes", "voided_at", "void_reason", "created_at", "updated_at"}).
			AddRow(int64(27), int64(19), int64(8), int64(91), 200.0, "USD", bannedAt, "partial", nil, "", now, now))

	repo := &profitRepository{db: db}
	cycles, err := repo.ListSubscriptionCycles(context.Background(), 91)
	if err != nil {
		t.Fatal(err)
	}
	if len(cycles) != 1 || cycles[0].Termination == nil || cycles[0].Termination.ID != 19 {
		t.Fatalf("termination not enriched: %+v", cycles)
	}
	if len(cycles[0].Refunds) != 1 || cycles[0].Refunds[0].Amount != 200 {
		t.Fatalf("refund not enriched: %+v", cycles[0].Refunds)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfitRepositoryGetSubscriptionCycleRevenueBatchUsesOneQuery(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("FROM jsonb_to_recordset($1::jsonb)") + `.*` + regexp.QuoteMeta("GROUP BY x.cycle_id")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"cycle_id", "revenue"}).AddRow(int64(8), 300.0))

	repo := &profitRepository{db: db}
	revenues, err := repo.GetSubscriptionCycleRevenueBatch(context.Background(), []service.SubscriptionCycleUsageRange{{
		CycleID: 8, AccountID: 91, Start: start, End: start.AddDate(0, 0, 10),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if revenues[8] != 300 {
		t.Fatalf("revenue = %v, want 300", revenues[8])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfitRepositoryDeleteSettledCycleReturnsConflict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectExec(`(?s)` + regexp.QuoteMeta("DELETE FROM account_subscription_cycles c")).
		WithArgs(int64(8)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM account_subscription_terminations WHERE cycle_id = $1)")).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	repo := &profitRepository{db: db}
	err = repo.DeleteSubscriptionCycle(context.Background(), 8)
	if err != service.ErrSubscriptionCycleSettled {
		t.Fatalf("error = %v, want settled conflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
