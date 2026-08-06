package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	subscriptionAutoRenewInterval = 15 * time.Minute
	subscriptionAutoRenewTimeout  = 45 * time.Second
)

// SubscriptionAutoRenewService periodically extends subscription cost cycles when
// auto_renew is enabled, using the previous cycle's fee and period days.
type SubscriptionAutoRenewService struct {
	profit *ProfitService

	interval  time.Duration
	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

func NewSubscriptionAutoRenewService(profit *ProfitService) *SubscriptionAutoRenewService {
	return &SubscriptionAutoRenewService{
		profit:   profit,
		interval: subscriptionAutoRenewInterval,
		stopCh:   make(chan struct{}),
	}
}

func (s *SubscriptionAutoRenewService) Start() {
	if s == nil || s.profit == nil {
		return
	}
	s.startOnce.Do(func() {
		logger.LegacyPrintf("service.subscription_auto_renew", "[SubscriptionAutoRenew] started interval=%s", s.interval)
		s.wg.Add(1)
		go s.runLoop()
	})
}

func (s *SubscriptionAutoRenewService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.wg.Wait()
		logger.LegacyPrintf("service.subscription_auto_renew", "[SubscriptionAutoRenew] stopped")
	})
}

func (s *SubscriptionAutoRenewService) runLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.runOnce()
	for {
		select {
		case <-ticker.C:
			s.runOnce()
		case <-s.stopCh:
			return
		}
	}
}

func (s *SubscriptionAutoRenewService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), subscriptionAutoRenewTimeout)
	defer cancel()
	created, err := s.profit.AutoRenewDueSubscriptionCycles(ctx, time.Now())
	if err != nil {
		logger.LegacyPrintf("service.subscription_auto_renew", "[SubscriptionAutoRenew] run failed err=%v", err)
		return
	}
	if created > 0 {
		logger.LegacyPrintf("service.subscription_auto_renew", "[SubscriptionAutoRenew] created cycles count=%d", created)
	}
}

// AutoRenewDueSubscriptionCycles creates the next cycle for accounts with
// auto_renew when the latest non-terminated cycle has ended.
//
// Rules:
//   - only subscription oauth/setup-token accounts
//   - copy period_fee / period_days / currency from the previous cycle
//   - new starts_at = previous starts_at + period_days (no gap, no overlap)
//   - skip if a cycle already starts at that timestamp
//   - skip if previous cycle is ban-settled (active termination)
//   - may create at most one next cycle per account per run (catch-up of long
//     outages is done by subsequent runs)
func (s *ProfitService) AutoRenewDueSubscriptionCycles(ctx context.Context, now time.Time) (int, error) {
	if s == nil || s.profitRepo == nil || s.accountRepo == nil {
		return 0, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	configs, err := s.profitRepo.ListAutoRenewSubscriptionAccounts(ctx)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, cfg := range configs {
		if cfg == nil || !cfg.AutoRenew || cfg.AccountID <= 0 {
			continue
		}
		account, err := s.accountRepo.GetByID(ctx, cfg.AccountID)
		if err != nil {
			return created, err
		}
		if account == nil || !isSubscriptionAccountType(account.Type) {
			continue
		}
		// Soft-deleted / banned accounts should not auto-renew cost ledger.
		if account.DeletedAt != nil || account.SubscriptionBanned {
			continue
		}
		cycles, err := s.profitRepo.ListSubscriptionCycles(ctx, cfg.AccountID)
		if err != nil {
			return created, err
		}
		prev := latestSubscriptionCycle(cycles)
		if prev == nil || prev.PeriodDays <= 0 {
			continue
		}
		// Do not extend a ban-settled cycle.
		if term := activeCycleTermination(prev); term != nil {
			continue
		}
		nextStart := prev.StartsAt.UTC().AddDate(0, 0, prev.PeriodDays)
		// Not due yet.
		if nextStart.After(now.UTC()) {
			continue
		}
		exists, err := s.profitRepo.HasSubscriptionCycleStartingAt(ctx, cfg.AccountID, nextStart)
		if err != nil {
			return created, err
		}
		if exists {
			continue
		}
		fee := prev.PeriodFee
		days := prev.PeriodDays
		currency := prev.Currency
		if currency == "" {
			currency = "USD"
		}
		// Prefer cost-config template fee/days when set (>0), else previous cycle.
		if cfg.PeriodFee > 0 {
			fee = cfg.PeriodFee
		}
		if cfg.PeriodDays > 0 {
			days = cfg.PeriodDays
		}
		cycle := &AccountSubscriptionCycle{
			AccountID:  cfg.AccountID,
			StartsAt:   nextStart,
			PeriodFee:  fee,
			PeriodDays: days,
			Currency:   currency,
			Notes:      fmt.Sprintf("auto-renewed from cycle #%d", prev.ID),
		}
		if _, err := s.CreateSubscriptionCycle(ctx, cycle); err != nil {
			logger.LegacyPrintf("service.subscription_auto_renew",
				"[SubscriptionAutoRenew] create failed account=%d prev=%d err=%v", cfg.AccountID, prev.ID, err)
			continue
		}
		created++
	}
	return created, nil
}

// latestSubscriptionCycle picks the cycle with the greatest starts_at (then id).
func latestSubscriptionCycle(cycles []*AccountSubscriptionCycle) *AccountSubscriptionCycle {
	var best *AccountSubscriptionCycle
	for _, c := range cycles {
		if c == nil {
			continue
		}
		if best == nil || c.StartsAt.After(best.StartsAt) || (c.StartsAt.Equal(best.StartsAt) && c.ID > best.ID) {
			best = c
		}
	}
	return best
}
