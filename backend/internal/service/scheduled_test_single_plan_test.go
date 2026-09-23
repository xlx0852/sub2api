package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
)

// planRepoStub covers the plan-side rules (single plan per account).
type planRepoStub struct {
	plans   []*domain.ScheduledTestPlan
	created []*domain.ScheduledTestPlan
}

func (r *planRepoStub) Create(_ context.Context, plan *domain.ScheduledTestPlan) (*domain.ScheduledTestPlan, error) {
	plan.ID = int64(len(r.created) + 1)
	r.created = append(r.created, plan)
	r.plans = append(r.plans, plan)
	return plan, nil
}

func (r *planRepoStub) GetByID(_ context.Context, _ int64) (*domain.ScheduledTestPlan, error) {
	return nil, nil
}

func (r *planRepoStub) ListByAccountID(_ context.Context, accountID int64) ([]*domain.ScheduledTestPlan, error) {
	var out []*domain.ScheduledTestPlan
	for _, plan := range r.plans {
		if plan.AccountID == accountID {
			out = append(out, plan)
		}
	}
	return out, nil
}

func (r *planRepoStub) ClaimDue(_ context.Context, _ time.Time, _ int, _ time.Duration) ([]*domain.ScheduledTestPlan, error) {
	return nil, nil
}

func (r *planRepoStub) Reschedule(_ context.Context, _ int64, _ time.Time) error { return nil }

func (r *planRepoStub) Update(_ context.Context, plan *domain.ScheduledTestPlan) (*domain.ScheduledTestPlan, error) {
	return plan, nil
}

func (r *planRepoStub) Delete(_ context.Context, _ int64) error { return nil }

func (r *planRepoStub) UpdateAfterRun(_ context.Context, _ int64, _ time.Time, _ time.Time) error {
	return nil
}

func (r *planRepoStub) ListDiagnosticsStatusByAccountIDs(_ context.Context, _ []int64) (map[int64]domain.ScheduledDiagnosticsStatus, error) {
	return map[int64]domain.ScheduledDiagnosticsStatus{}, nil
}

func TestCreatePlanAllowsOnlyOnePlanPerAccount(t *testing.T) {
	repo := &planRepoStub{}
	svc := &ScheduledTestService{planRepo: repo}

	first, err := svc.CreatePlan(context.Background(), &domain.ScheduledTestPlan{
		AccountID: 126, ModelID: "swe-2", CronExpression: "1-59/5 * * * *", Enabled: true,
	})
	if err != nil {
		t.Fatalf("first plan must be accepted: %v", err)
	}
	if first.ID == 0 || first.NextRunAt == nil {
		t.Fatalf("first plan was not persisted properly: %+v", first)
	}

	_, err = svc.CreatePlan(context.Background(), &domain.ScheduledTestPlan{
		AccountID: 126, ModelID: "gpt-5.5", CronExpression: "0 * * * *", Enabled: true,
	})
	if !errors.Is(err, ErrScheduledPlanExists) {
		t.Fatalf("second plan for the same account must be rejected, got %v", err)
	}

	// A different account is unaffected.
	if _, err := svc.CreatePlan(context.Background(), &domain.ScheduledTestPlan{
		AccountID: 127, ModelID: "gpt-5.5", CronExpression: "0 * * * *", Enabled: true,
	}); err != nil {
		t.Fatalf("another account must still be able to create its plan: %v", err)
	}
}

var _ ports.ScheduledTestPlanRepository = (*planRepoStub)(nil)
