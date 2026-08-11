package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// QuotaWindowSweepService periodically probes subscription accounts so the quota
// window ledger keeps tracking official resets even when accounts see no traffic.
type QuotaWindowSweepService struct {
	accountRepo AccountRepository
	openAI      *OpenAIQuotaService
	kimi        KimiQuotaQuerier
	grok        GrokQuotaProber
	settingRepo SettingRepository

	interval  time.Duration
	batchSize int
	perAcct   time.Duration

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	cursor  int
}

func NewQuotaWindowSweepService(
	accountRepo AccountRepository,
	openAI *OpenAIQuotaService,
	kimi KimiQuotaQuerier,
	grok GrokQuotaProber,
	settingRepo SettingRepository,
) *QuotaWindowSweepService {
	return &QuotaWindowSweepService{
		accountRepo: accountRepo,
		openAI:      openAI,
		kimi:        kimi,
		grok:        grok,
		settingRepo: settingRepo,
		interval:    30 * time.Minute,
		batchSize:   24,
		perAcct:     45 * time.Second,
	}
}

// Start launches the background sweeper (idempotent).
func (s *QuotaWindowSweepService) Start() {
	if s == nil || s.accountRepo == nil {
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()
	go s.loop()
}

// Stop halts the sweeper.
func (s *QuotaWindowSweepService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()
}

func (s *QuotaWindowSweepService) loop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.sweepOnce()
		}
	}
}

func (s *QuotaWindowSweepService) enabled(ctx context.Context) bool {
	if s.settingRepo == nil {
		return true
	}
	v, err := s.settingRepo.GetValue(ctx, "quota_window_sweep_enabled")
	if err != nil {
		return true
	}
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return true
	}
	return v != "false" && v != "0" && v != "off"
}

func (s *QuotaWindowSweepService) sweepOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), s.interval-time.Minute)
	defer cancel()
	if !s.enabled(ctx) {
		return
	}
	accounts, _, err := s.accountRepo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 500}, "", "", "", "", 0, "")
	if err != nil {
		slog.Warn("quota_window_sweep_list_failed", "error", err)
		return
	}
	eligible := make([]*Account, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || acc.DeletedAt != nil || !isSubscriptionAccountType(acc.Type) {
			continue
		}
		eligible = append(eligible, acc)
	}
	if len(eligible) == 0 {
		return
	}
	s.mu.Lock()
	start := s.cursor % len(eligible)
	limit := s.batchSize
	if limit > len(eligible) {
		limit = len(eligible)
	}
	s.cursor = (start + limit) % len(eligible)
	s.mu.Unlock()
	for i := 0; i < limit; i++ {
		acc := eligible[(start+i)%len(eligible)]
		aCtx, aCancel := context.WithTimeout(ctx, s.perAcct)
		s.probeOne(aCtx, acc)
		aCancel()
	}
	slog.Info("quota_window_sweep_done", "accounts", limit)
}

func (s *QuotaWindowSweepService) probeOne(ctx context.Context, acc *Account) {
	if acc == nil {
		return
	}
	switch acc.Platform {
	case PlatformOpenAI:
		if s.openAI != nil {
			usage, err := s.openAI.QueryUsage(ctx, acc.ID)
			if err != nil {
				slog.Debug("quota_window_sweep_openai_failed", "account_id", acc.ID, "error", err)
			} else if usage != nil {
				observeOpenAIQuotaUsage(ctx, s.openAI.quotaWindowLedger, acc.ID, usage, time.Now())
			}
		}
	case PlatformKimi:
		if s.kimi != nil {
			if _, err := s.kimi.QueryUsage(ctx, acc.ID); err != nil {
				slog.Debug("quota_window_sweep_kimi_failed", "account_id", acc.ID, "error", err)
			}
		}
	case PlatformGrok:
		if s.grok != nil {
			if _, err := s.grok.ProbeUsage(ctx, acc.ID); err != nil {
				slog.Debug("quota_window_sweep_grok_failed", "account_id", acc.ID, "error", err)
			}
		}
	default:
		// Anthropic/Claude session windows are driven by request headers; skip.
	}
}
