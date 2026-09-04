package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type byIDAccountLookup struct {
	getByID func(context.Context, int64) (*Account, error)
}

func (l byIDAccountLookup) GetAccount(ctx context.Context, accountID int64) (*domain.Account, error) {
	return l.getByID(ctx, accountID)
}

func optionalSnapshotLookup(snapshot *SchedulerSnapshotService) scheduling.AccountLookup {
	if snapshot == nil {
		return nil
	}
	return snapshot
}

func repoAccountLookup(repo AccountRepository) scheduling.AccountLookup {
	if repo == nil {
		return nil
	}
	return byIDAccountLookup{getByID: repo.GetByID}
}

func schedulingConfigFrom(cfg *config.Config) config.GatewaySchedulingConfig {
	if cfg == nil {
		return scheduling.ConfigOrDefault(nil)
	}
	return scheduling.ConfigOrDefault(&cfg.Gateway.Scheduling)
}

func waitPlanToScheduling(plan *AccountWaitPlan) *scheduling.WaitPlan {
	if plan == nil {
		return nil
	}
	out := scheduling.WaitPlan{
		AccountID:      plan.AccountID,
		MaxConcurrency: plan.MaxConcurrency,
		Timeout:        plan.Timeout,
		MaxWaiting:     plan.MaxWaiting,
	}
	return &out
}

func hydrateSelectedAccount(ctx context.Context, snapshot *SchedulerSnapshotService, account *Account, kind string) (*Account, error) {
	return scheduling.HydrateSelected(ctx, optionalSnapshotLookup(snapshot), account, kind)
}

func lookupSchedulableAccount(ctx context.Context, snapshot *SchedulerSnapshotService, repo AccountRepository, accountID int64) (*Account, error) {
	return scheduling.LookupSchedulable(ctx, optionalSnapshotLookup(snapshot), repoAccountLookup(repo), accountID)
}

func newHydratedSelectionResult(ctx context.Context, snapshot *SchedulerSnapshotService, account *Account, kind string, acquired bool, release func(), waitPlan *AccountWaitPlan) (*AccountSelectionResult, error) {
	hydrated, err := hydrateSelectedAccount(ctx, snapshot, account, kind)
	if err != nil {
		if acquired && release != nil {
			release()
		}
		return nil, err
	}
	return AccountSelectionFromScheduling(scheduling.NewResult(hydrated, acquired, release, waitPlanToScheduling(waitPlan)), nil)
}

func newHydratedModelLimitedResult(ctx context.Context, snapshot *SchedulerSnapshotService, account *Account, kind string) (*AccountSelectionResult, error) {
	hydrated, err := hydrateSelectedAccount(ctx, snapshot, account, kind)
	if err != nil {
		return nil, err
	}
	return AccountSelectionFromScheduling(scheduling.NewModelLimitedResult(hydrated), nil)
}

func newHydratedAcquiredResult(ctx context.Context, snapshot *SchedulerSnapshotService, account *Account, kind string, release func()) (*AccountSelectionResult, error) {
	return newHydratedSelectionResult(ctx, snapshot, account, kind, true, release, nil)
}
