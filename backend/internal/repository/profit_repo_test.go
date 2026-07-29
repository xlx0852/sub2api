package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestProfitRepositoryGetDailyUsageStatsOnlyIncludesAPIKeyMeteredCost(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	start := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	queryPattern := `(?s)` +
		`CASE\s+WHEN a\.type = 'apikey'.*` +
		regexp.QuoteMeta("LEFT JOIN accounts a ON a.id = ul.account_id") + `.*` +
		regexp.QuoteMeta("WHERE ul.created_at >= $1 AND ul.created_at < $2")
	mock.ExpectQuery(queryPattern).
		WithArgs(start, end, nil, "UTC").
		WillReturnRows(sqlmock.NewRows([]string{"day", "revenue", "metered_cost"}).
			AddRow("2026-07-28", 538.90, 122.24))

	repo := &profitRepository{db: db}
	points, err := repo.GetDailyUsageStats(context.Background(), nil, start, end, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].Revenue != 538.90 || points[0].MeteredCost != 122.24 {
		t.Fatalf("unexpected points: %+v", points)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfitRepositoryGetAccountDailyUsageStatsAggregatesSummaryAndTrendBase(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	start := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT")+`.*`+
		regexp.QuoteMeta("ul.account_id")+`.*`+
		regexp.QuoteMeta("CASE WHEN a.type = 'apikey'")+`.*`+
		regexp.QuoteMeta("GROUP BY ul.account_id, 2")).
		WithArgs(sqlmock.AnyArg(), start, end, "UTC").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "day", "requests", "revenue", "metered_cost"}).
			AddRow(int64(69), "2026-07-28", int64(100), 538.90, 122.24))

	repo := &profitRepository{db: db}
	points, err := repo.GetAccountDailyUsageStats(context.Background(), []int64{69, 72}, start, end, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].AccountID != 69 || points[0].Requests != 100 || points[0].MeteredCost != 122.24 {
		t.Fatalf("unexpected points: %+v", points)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfitRepositoryInsertCostConfigsIfAbsentUsesSingleStatement(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("INSERT INTO account_cost_configs") + `.*` +
		regexp.QuoteMeta("FROM jsonb_to_recordset($1::jsonb)") + `.*` +
		regexp.QuoteMeta("ON CONFLICT (account_id) DO NOTHING") + `.*` +
		regexp.QuoteMeta("RETURNING account_id")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(int64(1)).AddRow(int64(2)))

	repo := &profitRepository{db: db}
	inserted, err := repo.InsertCostConfigsIfAbsent(context.Background(), []*service.AccountCostConfig{
		{AccountID: 1, CostType: "subscription", PeriodFee: 200, PeriodDays: 30, Currency: "USD"},
		{AccountID: 2, CostType: "subscription", PeriodFee: 200, PeriodDays: 30, Currency: "USD"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inserted) != 2 || inserted[0] != 1 || inserted[1] != 2 {
		t.Fatalf("inserted = %v, want [1 2]", inserted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfitRepositoryGetAccountUsageStatsForRangesBatchesIndependentWindows(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("WITH requested_ranges AS")+`.*`+
		regexp.QuoteMeta("UNNEST($1::bigint[], $2::timestamptz[], $3::timestamptz[])")+`.*`+
		regexp.QuoteMeta("GROUP BY r.account_id")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "requests", "revenue", "metered_cost"}).
			AddRow(int64(1), int64(10), 500.0, 100.0).
			AddRow(int64(2), int64(5), 200.0, 40.0))

	repo := &profitRepository{db: db}
	stats, err := repo.GetAccountUsageStatsForRanges(context.Background(), []service.ProfitAccountUsageRange{
		{AccountID: 1, Start: start, End: start.AddDate(0, 0, 30)},
		{AccountID: 2, Start: start.AddDate(0, 0, 5), End: start.AddDate(0, 0, 20)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats[1] == nil || stats[1].Revenue != 500 || stats[2] == nil || stats[2].Revenue != 200 {
		t.Fatalf("unexpected range stats: %+v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfitRepositoryGetStoredValueSnapshotUsesCurrentPositiveUserBalance(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("SUM(GREATEST(balance, 0))") + `.*` + regexp.QuoteMeta("role <> 'admin'")).
		WillReturnRows(sqlmock.NewRows([]string{"spendable_balance", "frozen_balance", "eligible_users"}).AddRow(500.0, 20.0, int64(4)))

	repo := &profitRepository{db: db}
	snapshot, err := repo.GetStoredValueSnapshot(context.Background())
	if err != nil || snapshot.SpendableBalance != 500 || snapshot.FrozenBalance != 20 || snapshot.EligibleUsers != 4 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfitRepositoryGetSupplyForecastUsageSamplesUsesBalanceBilledAccountDays(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 30)
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("a.type NOT IN ('oauth', 'setup-token')")+`.*`+
		regexp.QuoteMeta("ul.subscription_id IS NULL")+`.*`+
		regexp.QuoteMeta("GROUP BY usage_day, a.platform, ul.account_id, a.type")).
		WithArgs(start, end, "UTC").
		WillReturnRows(sqlmock.NewRows([]string{"usage_day", "platform", "account_id", "type", "revenue", "metered_cost"}).
			AddRow("2026-07-28", "openai", int64(1), "apikey", 100.0, 20.0))

	repo := &profitRepository{db: db}
	samples, err := repo.GetSupplyForecastUsageSamples(context.Background(), start, end, "UTC")
	if err != nil || len(samples) != 1 || samples[0].MeteredCost != 20 {
		t.Fatalf("samples=%+v err=%v", samples, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfitRepositoryGetSchedulableSubscriptionSupplyDeduplicatesByAccount(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(`(?s)` + regexp.QuoteMeta("COUNT(DISTINCT id)") + `.*` +
		regexp.QuoteMeta("type IN ('oauth', 'setup-token')") + `.*` +
		regexp.QuoteMeta("schedulable = TRUE") + `.*` +
		regexp.QuoteMeta("GROUP BY platform")).
		WillReturnRows(sqlmock.NewRows([]string{"platform", "count"}).AddRow("openai", 5).AddRow("grok", 2))

	repo := &profitRepository{db: db}
	supply, err := repo.GetSchedulableSubscriptionSupply(context.Background())
	if err != nil || supply["openai"] != 5 || supply["grok"] != 2 {
		t.Fatalf("supply=%+v err=%v", supply, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
