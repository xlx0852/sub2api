package service

import (
	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/config"
)

func accountWaitPlanFromScheduling(plan scheduling.WaitPlan) *AccountWaitPlan {
	return &AccountWaitPlan{
		AccountID:      plan.AccountID,
		MaxConcurrency: plan.MaxConcurrency,
		Timeout:        plan.Timeout,
		MaxWaiting:     plan.MaxWaiting,
	}
}

func newStickyWaitPlan(accountID int64, maxConcurrency int, cfg config.GatewaySchedulingConfig) *AccountWaitPlan {
	return accountWaitPlanFromScheduling(scheduling.StickyWaitPlan(accountID, maxConcurrency, cfg))
}

func newFallbackWaitPlan(accountID int64, maxConcurrency int, cfg config.GatewaySchedulingConfig) *AccountWaitPlan {
	return accountWaitPlanFromScheduling(scheduling.FallbackWaitPlan(accountID, maxConcurrency, cfg))
}
