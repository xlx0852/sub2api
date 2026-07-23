package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/robfig/cron/v3"
)

const (
	scheduledTestDefaultMaxWorkers   = 10
	scheduledTestDefaultBatchSize    = 20
	scheduledTestDefaultBatchTimeout = 4 * time.Minute
	scheduledTestDefaultClaimLease   = 12 * time.Minute
	scheduledTestDefaultPlanTimeout  = 90 * time.Second
	scheduledTestTickDelay           = 5 * time.Second
)

// ScheduledTestRunnerService periodically scans due test plans and executes them.
type ScheduledTestRunnerService struct {
	planRepo       ScheduledTestPlanRepository
	scheduledSvc   *ScheduledTestService
	accountTestSvc *AccountTestService
	rateLimitSvc   *RateLimitService
	cfg            *config.Config

	cron      *cron.Cron
	startOnce sync.Once
	stopOnce  sync.Once
	running   atomic.Bool
}

// NewScheduledTestRunnerService creates a new runner.
func NewScheduledTestRunnerService(
	planRepo ScheduledTestPlanRepository,
	scheduledSvc *ScheduledTestService,
	accountTestSvc *AccountTestService,
	rateLimitSvc *RateLimitService,
	cfg *config.Config,
) *ScheduledTestRunnerService {
	return &ScheduledTestRunnerService{
		planRepo:       planRepo,
		scheduledSvc:   scheduledSvc,
		accountTestSvc: accountTestSvc,
		rateLimitSvc:   rateLimitSvc,
		cfg:            cfg,
	}
}

// Start begins the cron ticker (every minute).
func (s *ScheduledTestRunnerService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		loc := time.Local
		if s.cfg != nil {
			if parsed, err := time.LoadLocation(s.cfg.Timezone); err == nil && parsed != nil {
				loc = parsed
			}
		}

		c := cron.New(cron.WithParser(scheduledTestCronParser), cron.WithLocation(loc))
		_, err := c.AddFunc("* * * * *", func() { s.runScheduled() })
		if err != nil {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] not started (invalid schedule): %v", err)
			return
		}
		s.cron = c
		s.cron.Start()
		logger.LegacyPrintf(
			"service.scheduled_test_runner",
			"[ScheduledTestRunner] started (tick=every minute, workers=%d, batch=%d)",
			s.maxWorkers(),
			s.batchSize(),
		)
	})
}

// Stop gracefully shuts down the cron scheduler.
func (s *ScheduledTestRunnerService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cron != nil {
			ctx := s.cron.Stop()
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
				logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] cron stop timed out")
			}
		}
	})
}

func (s *ScheduledTestRunnerService) maxWorkers() int {
	return scheduledTestDefaultMaxWorkers
}

func (s *ScheduledTestRunnerService) batchSize() int {
	return scheduledTestDefaultBatchSize
}

func (s *ScheduledTestRunnerService) batchTimeout() time.Duration {
	return scheduledTestDefaultBatchTimeout
}

func (s *ScheduledTestRunnerService) claimLease() time.Duration {
	return scheduledTestDefaultClaimLease
}

func (s *ScheduledTestRunnerService) planTimeout() time.Duration {
	return scheduledTestDefaultPlanTimeout
}

func (s *ScheduledTestRunnerService) runScheduled() {
	// Skip overlapping ticks so a slow batch does not pile up goroutines.
	if !s.running.CompareAndSwap(false, true) {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] previous batch still running, skip tick")
		return
	}
	defer s.running.Store(false)

	// Small delay avoids clashing with other top-of-minute jobs.
	time.Sleep(scheduledTestTickDelay)

	ctx, cancel := context.WithTimeout(context.Background(), s.batchTimeout())
	defer cancel()

	now := time.Now()
	plans, err := s.planRepo.ClaimDue(ctx, now, s.batchSize(), s.claimLease())
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] ClaimDue error: %v", err)
		return
	}
	if len(plans) == 0 {
		return
	}

	logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] claimed %d due plans", len(plans))

	workers := s.maxWorkers()
	if workers > len(plans) {
		workers = len(plans)
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for _, plan := range plans {
		if ctx.Err() != nil {
			// Batch timed out before we started this plan: release soon so next tick can retry.
			s.releaseClaimedPlan(context.Background(), plan.ID)
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(p *ScheduledTestPlan) {
			defer wg.Done()
			defer func() { <-sem }()
			s.runOnePlan(ctx, p)
		}(plan)
	}

	wg.Wait()
}

func (s *ScheduledTestRunnerService) releaseClaimedPlan(ctx context.Context, planID int64) {
	// Push only 1 minute ahead so the next tick can reclaim without waiting for the full lease.
	next := time.Now().Add(1 * time.Minute)
	if err := s.planRepo.Reschedule(ctx, planID, next); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d release claim error: %v", planID, err)
	}
}

func (s *ScheduledTestRunnerService) runOnePlan(parent context.Context, plan *ScheduledTestPlan) {
	ctx, cancel := context.WithTimeout(parent, s.planTimeout())
	defer cancel()

	result, err := s.accountTestSvc.RunTestBackground(ctx, plan.AccountID, plan.ModelID)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d RunTestBackground error: %v", plan.ID, err)
		// Still advance schedule so a stuck plan does not thrash every tick after lease expiry.
		s.finishPlanSchedule(context.Background(), plan)
		return
	}

	if err := s.scheduledSvc.SaveResult(context.Background(), plan.ID, plan.MaxResults, result); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d SaveResult error: %v", plan.ID, err)
	}

	// Auto-recover account if test succeeded and auto_recover is enabled.
	if result.Status == "success" && plan.AutoRecover {
		s.tryRecoverAccount(context.Background(), plan.AccountID, plan.ID)
	}

	s.finishPlanSchedule(context.Background(), plan)
}

func (s *ScheduledTestRunnerService) finishPlanSchedule(ctx context.Context, plan *ScheduledTestPlan) {
	nextRun, err := computeNextRun(plan.CronExpression, time.Now())
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d computeNextRun error: %v", plan.ID, err)
		// Fall back to lease-like deferral so the plan stays schedulable.
		if resErr := s.planRepo.Reschedule(ctx, plan.ID, time.Now().Add(s.claimLease())); resErr != nil {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d fallback reschedule error: %v", plan.ID, resErr)
		}
		return
	}

	if err := s.planRepo.UpdateAfterRun(ctx, plan.ID, time.Now(), nextRun); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d UpdateAfterRun error: %v", plan.ID, err)
	}
}

// tryRecoverAccount attempts to recover an account from recoverable runtime state.
func (s *ScheduledTestRunnerService) tryRecoverAccount(ctx context.Context, accountID int64, planID int64) {
	if s.rateLimitSvc == nil {
		return
	}

	recovery, err := s.rateLimitSvc.RecoverAccountAfterSuccessfulTest(ctx, accountID)
	if err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover failed: %v", planID, err)
		return
	}
	if recovery == nil {
		return
	}

	if recovery.ClearedError {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: account=%d recovered from error status", planID, accountID)
	}
	if recovery.ClearedRateLimit {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: account=%d cleared rate-limit/runtime state", planID, accountID)
	}
}
