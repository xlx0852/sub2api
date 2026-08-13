// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ScheduledTestPlanRepository defines the data access interface for test plans.
type ScheduledTestPlanRepository interface {
	Create(ctx context.Context, plan *domain.ScheduledTestPlan) (*domain.ScheduledTestPlan, error)
	GetByID(ctx context.Context, id int64) (*domain.ScheduledTestPlan, error)
	ListByAccountID(ctx context.Context, accountID int64) ([]*domain.ScheduledTestPlan, error)
	// ClaimDue atomically claims up to limit due plans for execution.
	// Claimed rows have next_run_at pushed forward by lease so other runners skip them.
	ClaimDue(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]*domain.ScheduledTestPlan, error)
	// Reschedule updates only next_run_at (used to release/defer a claimed plan).
	Reschedule(ctx context.Context, id int64, nextRunAt time.Time) error
	Update(ctx context.Context, plan *domain.ScheduledTestPlan) (*domain.ScheduledTestPlan, error)
	Delete(ctx context.Context, id int64) error
	UpdateAfterRun(ctx context.Context, id int64, lastRunAt time.Time, nextRunAt time.Time) error
	ListDiagnosticsStatusByAccountIDs(ctx context.Context, accountIDs []int64) (map[int64]domain.ScheduledDiagnosticsStatus, error)
}

// ScheduledTestResultRepository defines the data access interface for test results.
type ScheduledTestResultRepository interface {
	Create(ctx context.Context, result *domain.ScheduledTestResult) (*domain.ScheduledTestResult, error)
	ListByPlanID(ctx context.Context, planID int64, limit int) ([]*domain.ScheduledTestResult, error)
	PruneOldResults(ctx context.Context, planID int64, keepCount int) error
}
