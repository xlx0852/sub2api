package service

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

var scheduledTestCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// ScheduledTestService provides CRUD operations for scheduled test plans and results.
type ScheduledTestService struct {
	planRepo    ScheduledTestPlanRepository
	resultRepo  ScheduledTestResultRepository
	accountRepo AccountRepository
}

// NewScheduledTestService creates a new ScheduledTestService.
func NewScheduledTestService(
	planRepo ScheduledTestPlanRepository,
	resultRepo ScheduledTestResultRepository,
	accountRepo AccountRepository,
) *ScheduledTestService {
	return &ScheduledTestService{
		planRepo:    planRepo,
		resultRepo:  resultRepo,
		accountRepo: accountRepo,
	}
}

// CreatePlan validates the cron expression, computes next_run_at, and persists the plan.
func (s *ScheduledTestService) CreatePlan(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun

	if plan.MaxResults <= 0 {
		plan.MaxResults = 50
	}

	return s.planRepo.Create(ctx, plan)
}

// GetPlan retrieves a plan by ID.
func (s *ScheduledTestService) GetPlan(ctx context.Context, id int64) (*ScheduledTestPlan, error) {
	return s.planRepo.GetByID(ctx, id)
}

// ListPlansByAccount returns all plans for a given account.
func (s *ScheduledTestService) ListPlansByAccount(ctx context.Context, accountID int64) ([]*ScheduledTestPlan, error) {
	return s.planRepo.ListByAccountID(ctx, accountID)
}

// UpdatePlan validates cron and updates the plan.
func (s *ScheduledTestService) UpdatePlan(ctx context.Context, plan *ScheduledTestPlan) (*ScheduledTestPlan, error) {
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	plan.NextRunAt = &nextRun

	return s.planRepo.Update(ctx, plan)
}

// DeletePlan removes a plan and its results (via CASCADE).
func (s *ScheduledTestService) DeletePlan(ctx context.Context, id int64) error {
	return s.planRepo.Delete(ctx, id)
}

// ListResults returns the most recent results for a plan.
func (s *ScheduledTestService) ListResults(ctx context.Context, planID int64, limit int) ([]*ScheduledTestResult, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.resultRepo.ListByPlanID(ctx, planID, limit)
}

// SaveResult inserts a result and prunes old entries beyond maxResults.
func (s *ScheduledTestService) SaveResult(ctx context.Context, planID int64, maxResults int, result *ScheduledTestResult) error {
	result.PlanID = planID
	if _, err := s.resultRepo.Create(ctx, result); err != nil {
		return err
	}
	return s.resultRepo.PruneOldResults(ctx, planID, maxResults)
}

// EnsureDefaultDiagnosticsPlan creates a default enabled plan when the account has none.
// If plans already exist, it returns the first one without modification.
func (s *ScheduledTestService) EnsureDefaultDiagnosticsPlan(ctx context.Context, accountID int64) (*ScheduledTestPlan, bool, error) {
	if accountID <= 0 {
		return nil, false, fmt.Errorf("invalid account id")
	}
	existing, err := s.planRepo.ListByAccountID(ctx, accountID)
	if err != nil {
		return nil, false, err
	}
	if len(existing) > 0 {
		return existing[0], false, nil
	}

	var account *Account
	if s.accountRepo != nil {
		account, err = s.accountRepo.GetByID(ctx, accountID)
		if err != nil {
			return nil, false, err
		}
	}

	plan := &ScheduledTestPlan{
		AccountID:      accountID,
		ModelID:        DefaultScheduledDiagnosticsModel(account),
		CronExpression: DefaultScheduledDiagnosticsCron(accountID),
		Enabled:        true,
		MaxResults:     50,
		AutoRecover:    true,
	}
	created, err := s.CreatePlan(ctx, plan)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

// EnsureDefaultDiagnosticsPlansForAllActiveAccounts enables diagnostics for accounts missing plans.
func (s *ScheduledTestService) EnsureDefaultDiagnosticsPlansForAllActiveAccounts(ctx context.Context) (created int, err error) {
	if s.accountRepo == nil {
		return 0, fmt.Errorf("account repository unavailable")
	}
	accounts, err := s.accountRepo.ListActive(ctx)
	if err != nil {
		return 0, err
	}
	for i := range accounts {
		account := accounts[i]
		if account.ID <= 0 {
			continue
		}
		if _, made, ensureErr := s.EnsureDefaultDiagnosticsPlan(ctx, account.ID); ensureErr != nil {
			// Continue other accounts; return last error summary via count only.
			continue
		} else if made {
			created++
		}
	}
	return created, nil
}

func (s *ScheduledTestService) ListDiagnosticsStatusByAccountIDs(ctx context.Context, accountIDs []int64) (map[int64]ScheduledDiagnosticsStatus, error) {
	if s.planRepo == nil {
		return map[int64]ScheduledDiagnosticsStatus{}, nil
	}
	return s.planRepo.ListDiagnosticsStatusByAccountIDs(ctx, accountIDs)
}

func computeNextRun(cronExpr string, from time.Time) (time.Time, error) {
	sched, err := scheduledTestCronParser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(from), nil
}
