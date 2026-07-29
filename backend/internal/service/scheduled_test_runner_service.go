package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
	// Keep demotion shorter than the 5-minute probe cadence can recover from.
	scheduledDiagnosticsDemoteDuration = 12 * time.Minute
)

// ScheduledTestRunnerService periodically scans due test plans and executes them.
type ScheduledTestRunnerService struct {
	planRepo       ScheduledTestPlanRepository
	accountRepo    AccountRepository
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
	accountRepo AccountRepository,
	scheduledSvc *ScheduledTestService,
	accountTestSvc *AccountTestService,
	rateLimitSvc *RateLimitService,
	cfg *config.Config,
) *ScheduledTestRunnerService {
	return &ScheduledTestRunnerService{
		planRepo:       planRepo,
		accountRepo:    accountRepo,
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
	// Whole multi-model chain shares one parent budget, with per-model timeouts.
	ctx, cancel := context.WithTimeout(parent, s.planTimeout()*time.Duration(scheduledDiagnosticsMaxModels))
	if d, ok := parent.Deadline(); ok {
		// Never exceed parent batch deadline.
		if time.Until(d) < s.planTimeout() {
			cancel()
			ctx, cancel = context.WithTimeout(parent, time.Until(d))
		}
	}
	defer cancel()

	result := s.runDiagnosticsChain(ctx, plan)

	if err := s.scheduledSvc.SaveResult(context.Background(), plan.ID, plan.MaxResults, result); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d SaveResult error: %v", plan.ID, err)
	}

	switch result.Status {
	case "success":
		if plan.AutoRecover {
			s.tryRecoverAccount(context.Background(), plan.AccountID, plan.ID)
		}
	case "failed":
		s.tryDemoteAccount(context.Background(), plan.AccountID, plan.ID, result.ErrorMessage)
	}

	s.finishPlanSchedule(context.Background(), plan)
}

func (s *ScheduledTestRunnerService) runDiagnosticsChain(ctx context.Context, plan *ScheduledTestPlan) *ScheduledTestResult {
	startedAt := time.Now()
	models := []string{strings.TrimSpace(plan.ModelID)}
	if s.accountRepo != nil {
		if account, err := s.accountRepo.GetByID(ctx, plan.AccountID); err == nil && account != nil {
			models = ResolveScheduledDiagnosticsModels(account, plan.ModelID)
		}
	}
	if len(models) == 0 {
		models = []string{strings.TrimSpace(plan.ModelID)}
	}

	var (
		attempts     []string
		lastResult   *ScheduledTestResult
		lastErrText  string
		firstStarted = startedAt
	)

	for i, modelID := range models {
		if ctx.Err() != nil {
			lastErrText = ctx.Err().Error()
			break
		}
		modelCtx, modelCancel := context.WithTimeout(ctx, s.planTimeout())
		result, err := s.accountTestSvc.RunTestBackground(modelCtx, plan.AccountID, modelID)
		modelCancel()
		if err != nil {
			msg := err.Error()
			attempts = append(attempts, fmt.Sprintf("%s: error(%s)", modelID, truncateDiagText(msg, 160)))
			lastErrText = msg
			lastResult = &ScheduledTestResult{
				Status:       "failed",
				ErrorMessage: msg,
				LatencyMs:    time.Since(startedAt).Milliseconds(),
				StartedAt:    firstStarted,
				FinishedAt:   time.Now(),
			}
			// Transport/setup errors: still try next model.
			continue
		}
		if result == nil {
			attempts = append(attempts, fmt.Sprintf("%s: empty_result", modelID))
			continue
		}
		if firstStarted.IsZero() {
			firstStarted = result.StartedAt
		}
		if result.Status == "success" {
			prefix := ""
			if i > 0 {
				prefix = fmt.Sprintf("[diagnostics upgraded model=%s after %d failed attempt(s)]\n", modelID, i)
				if len(attempts) > 0 {
					prefix += "failed_models:\n- " + strings.Join(attempts, "\n- ") + "\n"
				}
			} else if modelID != "" && modelID != strings.TrimSpace(plan.ModelID) {
				prefix = fmt.Sprintf("[diagnostics model=%s]\n", modelID)
			}
			result.ResponseText = prefix + result.ResponseText
			if result.StartedAt.IsZero() {
				result.StartedAt = firstStarted
			}
			return result
		}

		errText := strings.TrimSpace(result.ErrorMessage)
		if errText == "" {
			errText = "failed"
		}
		attempts = append(attempts, fmt.Sprintf("%s: %s", modelID, truncateDiagText(errText, 160)))
		lastErrText = errText
		lastResult = result
	}

	finishedAt := time.Now()
	if lastResult == nil {
		lastResult = &ScheduledTestResult{
			Status:     "failed",
			StartedAt:  firstStarted,
			FinishedAt: finishedAt,
			LatencyMs:  finishedAt.Sub(firstStarted).Milliseconds(),
		}
	}
	lastResult.Status = "failed"
	if lastResult.StartedAt.IsZero() {
		lastResult.StartedAt = firstStarted
	}
	lastResult.FinishedAt = finishedAt
	lastResult.LatencyMs = finishedAt.Sub(firstStarted).Milliseconds()

	var b strings.Builder
	if len(attempts) > 1 {
		b.WriteString("all diagnostic models failed\n")
		for _, item := range attempts {
			b.WriteString("- ")
			b.WriteString(item)
			b.WriteByte('\n')
		}
	} else if len(attempts) == 1 {
		// Keep a stable, parseable single-attempt format for the UI.
		b.WriteString(attempts[0])
	} else if lastErrText != "" {
		b.WriteString(lastErrText)
	} else {
		b.WriteString("all diagnostic models failed")
	}
	lastResult.ErrorMessage = strings.TrimSpace(b.String())
	return lastResult
}

func truncateDiagText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
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

	recovery, err := s.rateLimitSvc.RecoverAccountAfterScheduledDiagnostics(ctx, accountID)
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
	if recovery.ClearedSchedulable {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-recover: account=%d restored schedulable=true", planID, accountID)
	}
}

func (s *ScheduledTestRunnerService) tryDemoteAccount(ctx context.Context, accountID int64, planID int64, errMsg string) {
	if s.rateLimitSvc == nil {
		return
	}
	// When the diagnostics response carries an explicit quota reset timestamp, set a
	// proper rate-limit state instead of a generic temp_unschedulable.
	if resetAt, ok := parseUsageLimitResetAt(errMsg); ok {
		reason := fmt.Sprintf("scheduled diagnostics: usage limit reached (plan=%d)", planID)
		if err := s.rateLimitSvc.RateLimitAccountAfterDiagnostics(ctx, accountID, resetAt, reason); err != nil {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d rate-limit after diagnostics failed: %v", planID, err)
		} else {
			logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d rate-limited account=%d until %s", planID, accountID, resetAt.Format(time.RFC3339))
		}
		return
	}
	if !shouldDemoteAfterDiagnosticsFailure(errMsg) {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d skip demote for soft failure: %s", planID, truncateDiagText(errMsg, 160))
		return
	}
	reason := fmt.Sprintf("scheduled diagnostics failed (plan=%d): %s", planID, truncateDiagText(errMsg, 240))
	if err := s.rateLimitSvc.DemoteAccountAfterFailedDiagnostics(ctx, accountID, scheduledDiagnosticsDemoteDuration, reason); err != nil {
		logger.LegacyPrintf("service.scheduled_test_runner", "[ScheduledTestRunner] plan=%d auto-demote failed: %v", planID, err)
		return
	}
	logger.LegacyPrintf(
		"service.scheduled_test_runner",
		"[ScheduledTestRunner] plan=%d auto-demote: account=%d temp_unschedulable for %s",
		planID,
		accountID,
		scheduledDiagnosticsDemoteDuration,
	)
}

// parseUsageLimitResetAt extracts the resets_at unix timestamp from a usage_limit_reached
// error message. Returns the reset time and true when a valid future timestamp is found.
func parseUsageLimitResetAt(errMsg string) (time.Time, bool) {
	if !strings.Contains(errMsg, "usage_limit_reached") {
		return time.Time{}, false
	}
	idx := strings.Index(errMsg, `"resets_at":`)
	if idx < 0 {
		return time.Time{}, false
	}
	rest := errMsg[idx+len(`"resets_at":`):]
	end := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
	if end == 0 {
		return time.Time{}, false
	}
	if end < 0 {
		end = len(rest)
	}
	ts, err := strconv.ParseInt(rest[:end], 10, 64)
	if err != nil || ts <= 0 {
		return time.Time{}, false
	}
	t := time.Unix(ts, 0)
	if t.Before(time.Now()) {
		return time.Time{}, false
	}
	return t, true
}

func shouldDemoteAfterDiagnosticsFailure(errMsg string) bool {
	msg := strings.ToLower(strings.TrimSpace(errMsg))
	if msg == "" {
		return false
	}
	allModelsFailed := strings.HasPrefix(msg, "all diagnostic models failed")

	// Hard account/auth signals always demote.
	hardMarkers := []string{
		"unauthorized",
		"forbidden",
		"invalid_api_key",
		"invalid api key",
		"authentication",
		"permission",
		"account deactivated",
		"account disabled",
		"spending-limit",
		"personal-team-blocked",
		"401",
		"403",
	}
	for _, m := range hardMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}

	if !allModelsFailed {
		// Single-model failure: skip demote for any transient/soft error.
		softMarkers := []string{
			"context deadline exceeded",
			"context canceled",
			"timeout",
			"i/o timeout",
			"connection reset",
			"connection refused",
			"temporary failure",
			"tls handshake",
			"model_not_found",
			"model not found",
			"invalid model",
			"unknown model",
			"does not exist",
			"not available",
			"overloaded",
			"rate limit",
			"too many requests",
			"usage_limit_reached",
			"concurrency limit exceeded",
			"429",
			"502",
			"503",
			"504",
		}
		for _, m := range softMarkers {
			if strings.Contains(msg, m) {
				return false
			}
		}
		return false
	}

	// All candidate models failed. Only spare the account for quota/rate-limit errors —
	// those already have dedicated state management via real-request handling.
	quotaMarkers := []string{
		"rate limit",
		"too many requests",
		"usage_limit_reached",
		"concurrency limit exceeded",
		"429",
	}
	for _, m := range quotaMarkers {
		if strings.Contains(msg, m) {
			return false
		}
	}
	// Every other all-models-failed scenario (timeouts, 404/model_not_found, 502/503, etc.)
	// indicates the account is currently unusable; apply a short temp_unschedulable cooldown.
	return true
}
