package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
)

const (
	SettingKeySettlementDurabilityMode     = "usage_settlement_durability_mode"
	SettingKeySettlementDurabilityGroupIDs = "usage_settlement_durability_group_ids"
)

type SettlementDurabilityMode int32

const (
	SettlementDurabilityOff SettlementDurabilityMode = iota
	SettlementDurabilityShadow
	SettlementDurabilityAuthoritative
)

func ParseSettlementDurabilityMode(raw string) SettlementDurabilityMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "shadow":
		return SettlementDurabilityShadow
	case "authoritative":
		return SettlementDurabilityAuthoritative
	default:
		return SettlementDurabilityOff
	}
}

func (m SettlementDurabilityMode) String() string {
	switch m {
	case SettlementDurabilityShadow:
		return "shadow"
	case SettlementDurabilityAuthoritative:
		return "authoritative"
	default:
		return "off"
	}
}

type DurableSettlementOptions struct {
	Owner         string
	PollInterval  time.Duration
	LeaseDuration time.Duration
	BatchSize     int
	RetryBase     time.Duration
	RetryMax      time.Duration
	StatsInterval time.Duration
}

type DurableSettlement struct {
	attempts ports.UsageSettlementRepository
	billing  ports.UsageSettlementBillingRepository
	options  DurableSettlementOptions
	mode     atomic.Int32
	wake     chan struct{}

	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	observer  ports.UsageSettlementAppliedObserver
	recoverer ports.UsageSettlementTerminalRecoverer
	scope     settlementRolloutScope

	recoveredTotal         atomic.Uint64
	alreadySettledTotal    atomic.Uint64
	retryTotal             atomic.Uint64
	failedTerminalTotal    atomic.Uint64
	reconciliationTotal    atomic.Uint64
	providerRecoveredTotal atomic.Uint64
}

type settlementRolloutScope struct {
	all      bool
	groupIDs []int64
	groups   map[int64]struct{}
}

func allSettlementRolloutScope() settlementRolloutScope {
	return settlementRolloutScope{all: true}
}

func parseSettlementRolloutScope(raw string) (settlementRolloutScope, error) {
	raw = strings.TrimSpace(raw)
	if raw == "*" {
		return allSettlementRolloutScope(), nil
	}
	if raw == "" {
		return settlementRolloutScope{}, errors.New("durable settlement rollout group scope is required")
	}
	var values []int64
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return settlementRolloutScope{}, fmt.Errorf("parse durable settlement rollout group scope: %w", err)
	}
	groups := make(map[int64]struct{}, len(values))
	groupIDs := make([]int64, 0, len(values))
	for _, groupID := range values {
		if groupID <= 0 {
			return settlementRolloutScope{}, errors.New("durable settlement rollout group IDs must be positive")
		}
		if _, exists := groups[groupID]; exists {
			continue
		}
		groups[groupID] = struct{}{}
		groupIDs = append(groupIDs, groupID)
	}
	if len(groupIDs) == 0 {
		return settlementRolloutScope{}, errors.New("durable settlement rollout group scope must not be empty")
	}
	return settlementRolloutScope{groupIDs: groupIDs, groups: groups}, nil
}

func (s settlementRolloutScope) matches(groupID *int64) bool {
	if s.all {
		return true
	}
	if groupID == nil || *groupID <= 0 {
		return false
	}
	_, ok := s.groups[*groupID]
	return ok
}

func (s settlementRolloutScope) claimGroupIDs() []int64 {
	if s.all {
		return nil
	}
	return append([]int64{}, s.groupIDs...)
}

func (s settlementRolloutScope) logValue() any {
	if s.all {
		return "*"
	}
	return append([]int64(nil), s.groupIDs...)
}

func (s *DurableSettlement) SetTerminalRecoverer(recoverer ports.UsageSettlementTerminalRecoverer) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.recoverer = recoverer
	s.mu.Unlock()
}

func (s *DurableSettlement) SetAppliedObserver(observer ports.UsageSettlementAppliedObserver) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.observer = observer
	s.mu.Unlock()
}

func NewDurableSettlement(attempts ports.UsageSettlementRepository, billing ports.UsageSettlementBillingRepository, options DurableSettlementOptions) *DurableSettlement {
	if strings.TrimSpace(options.Owner) == "" {
		options.Owner = fmt.Sprintf("settlement-worker-%d", time.Now().UnixNano())
	}
	if options.PollInterval <= 0 {
		options.PollInterval = time.Second
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = 30 * time.Second
	}
	if options.BatchSize <= 0 {
		options.BatchSize = 16
	}
	if options.RetryBase <= 0 {
		options.RetryBase = time.Second
	}
	if options.RetryMax <= 0 {
		options.RetryMax = time.Minute
	}
	if options.StatsInterval <= 0 {
		options.StatsInterval = time.Minute
	}
	return &DurableSettlement{
		attempts: attempts,
		billing:  billing,
		options:  options,
		wake:     make(chan struct{}, 1),
		scope:    allSettlementRolloutScope(),
	}
}

// ProvideDurableSettlement constructs and starts the recovery worker. The mode
// is stored in the settings table so the existing YAML/env surface remains
// unchanged. A missing, unreadable, or invalid value fails closed to off.
func ProvideDurableSettlement(attempts ports.UsageSettlementRepository, billing ports.UsageSettlementBillingRepository, settings ports.SettingRepository) *DurableSettlement {
	worker := NewDurableSettlement(attempts, billing, DurableSettlementOptions{})
	raw := ""
	if settings != nil {
		value, err := settings.GetValue(context.Background(), SettingKeySettlementDurabilityMode)
		if err != nil && !errors.Is(err, domain.ErrSettingNotFound) {
			slog.Warn("failed to load durable settlement mode; keeping it off", "error", err)
		} else if err == nil {
			raw = value
		}
	}
	mode := ParseSettlementDurabilityMode(raw)
	if strings.TrimSpace(raw) != "" && mode == SettlementDurabilityOff && !strings.EqualFold(strings.TrimSpace(raw), "off") {
		slog.Warn("invalid durable settlement mode; keeping it off", "value", raw)
	}
	scope := settlementRolloutScope{}
	if mode != SettlementDurabilityOff {
		scopeRaw, scopeErr := settings.GetValue(context.Background(), SettingKeySettlementDurabilityGroupIDs)
		if scopeErr != nil {
			slog.Warn("failed to load durable settlement rollout group scope; keeping it off", "error", scopeErr)
			mode = SettlementDurabilityOff
		} else if parsedScope, parseErr := parseSettlementRolloutScope(scopeRaw); parseErr != nil {
			slog.Warn("invalid durable settlement rollout group scope; keeping it off", "error", parseErr)
			mode = SettlementDurabilityOff
		} else {
			scope = parsedScope
		}
	}
	worker.setRollout(mode, scope)
	slog.Info("durable settlement initialized", "mode", mode.String(), "group_scope", scope.logValue())
	worker.Start(context.Background())
	return worker
}

func (s *DurableSettlement) SetMode(mode SettlementDurabilityMode) {
	s.setRollout(mode, allSettlementRolloutScope())
}

func (s *DurableSettlement) setRollout(mode SettlementDurabilityMode, scope settlementRolloutScope) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.scope = scope
	s.mu.Unlock()
	s.mode.Store(int32(mode))
	if mode == SettlementDurabilityAuthoritative {
		s.Wake()
	}
}

func (s *DurableSettlement) Mode() SettlementDurabilityMode {
	if s == nil {
		return SettlementDurabilityOff
	}
	return SettlementDurabilityMode(s.mode.Load())
}

func (s *DurableSettlement) Enabled() bool {
	return s != nil && s.Mode() != SettlementDurabilityOff
}

func (s *DurableSettlement) Authoritative() bool {
	return s != nil && s.Mode() == SettlementDurabilityAuthoritative
}

func (s *DurableSettlement) EnabledFor(groupID *int64) bool {
	if !s.Enabled() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scope.matches(groupID)
}

func (s *DurableSettlement) AuthoritativeFor(groupID *int64) bool {
	return s.Authoritative() && s.EnabledFor(groupID)
}

func (s *DurableSettlement) StartAttempt(ctx context.Context, attempt *domain.UsageSettlementAttempt) (*domain.UsageSettlementAttempt, bool, error) {
	if s == nil || attempt == nil || !s.EnabledFor(attempt.GroupID) {
		return nil, false, nil
	}
	if s.attempts == nil {
		return nil, false, errors.New("durable settlement attempt repository is nil")
	}
	attempt.Authoritative = s.AuthoritativeFor(attempt.GroupID)
	return s.attempts.CreateStarted(ctx, attempt)
}

func (s *DurableSettlement) ObserveProviderRequestID(ctx context.Context, billingIdentity string, apiKeyID int64, requestFingerprint, providerRequestID string) (*domain.UsageSettlementAttempt, error) {
	if s == nil || s.Mode() == SettlementDurabilityOff {
		return nil, nil
	}
	if s.attempts == nil {
		return nil, errors.New("durable settlement attempt repository is nil")
	}
	return s.attempts.ObserveProviderRequestID(ctx, billingIdentity, apiKeyID, requestFingerprint, providerRequestID)
}

func (s *DurableSettlement) HandoffReady(ctx context.Context, billingIdentity string, apiKeyID int64, requestFingerprint string, payload *domain.UsageSettlementPayloadV1) (*domain.UsageSettlementAttempt, error) {
	if s == nil || s.Mode() == SettlementDurabilityOff {
		return nil, nil
	}
	if s.attempts == nil {
		return nil, errors.New("durable settlement attempt repository is nil")
	}
	attempt, err := s.attempts.MarkReady(ctx, billingIdentity, apiKeyID, requestFingerprint, payload)
	return attempt, err
}

func (s *DurableSettlement) Start(ctx context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	go s.run(runCtx, s.done)
}

func (s *DurableSettlement) StopAndWait(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()
	if cancel == nil || done == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		s.mu.Lock()
		if s.done == done {
			s.cancel = nil
			s.done = nil
		}
		s.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *DurableSettlement) Wake() {
	if s == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *DurableSettlement) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(s.options.PollInterval)
	defer ticker.Stop()
	statsTicker := time.NewTicker(s.options.StatsInterval)
	defer statsTicker.Stop()
	for {
		emitStats := false
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		case <-statsTicker.C:
			emitStats = true
		}
		mode := s.Mode()
		if mode == SettlementDurabilityOff {
			continue
		}
		s.recoverExpiredStarted(ctx)
		expired, expireErr := s.attempts.ExpireStarted(ctx, time.Now().UTC(), s.options.BatchSize)
		if expireErr != nil && !errors.Is(expireErr, context.Canceled) {
			slog.Warn("durable settlement reconciliation scan failed", "error", expireErr)
		} else if expired > 0 {
			s.reconciliationTotal.Add(uint64(expired))
			slog.Warn("durable settlement attempts require reconciliation", "count", expired)
		}
		if emitStats {
			s.logBacklogStats(ctx)
		}
		if mode != SettlementDurabilityAuthoritative {
			continue
		}
		if err := s.drainOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("durable settlement worker iteration failed", "error", err)
		}
	}
}

func (s *DurableSettlement) recoverExpiredStarted(ctx context.Context) {
	if s == nil || s.attempts == nil {
		return
	}
	s.mu.Lock()
	recoverer := s.recoverer
	s.mu.Unlock()
	if recoverer == nil {
		return
	}
	items, err := s.attempts.ListExpiredRecoverable(ctx, time.Now().UTC(), s.options.BatchSize)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Warn("durable settlement provider recovery scan failed", "error", err)
		}
		return
	}
	for index := range items {
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.options.LeaseDuration)
		recovered, recoverErr := recoverer.RecoverUsageSettlement(workCtx, &items[index])
		cancel()
		if recoverErr != nil {
			slog.Warn("durable settlement provider recovery failed", "attempt_id", items[index].ID, "provider", items[index].Provider, "error", recoverErr)
			continue
		}
		if recovered {
			s.providerRecoveredTotal.Add(1)
			slog.Info("durable settlement provider usage recovered", "attempt_id", items[index].ID, "provider", items[index].Provider)
		}
	}
}

func (s *DurableSettlement) drainOnce(ctx context.Context) error {
	if s.attempts == nil || s.billing == nil {
		return errors.New("durable settlement worker repositories are incomplete")
	}
	s.mu.Lock()
	groupIDs := s.scope.claimGroupIDs()
	s.mu.Unlock()
	items, err := s.attempts.Claim(ctx, ports.UsageSettlementClaimRequest{
		Owner: s.options.Owner, Limit: s.options.BatchSize, LeaseDuration: s.options.LeaseDuration,
		GroupIDs: groupIDs,
	})
	if err != nil {
		return err
	}
	for index := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		item := &items[index]
		workCtx, workCancel := context.WithTimeout(context.WithoutCancel(ctx), s.options.LeaseDuration)
		applyResult, applyErr := s.billing.ApplySettlement(workCtx, item.ID, s.options.Owner)
		if applyErr != nil {
			if isTerminalSettlementError(applyErr) {
				if markErr := s.attempts.MarkFailedTerminal(workCtx, item.ID, s.options.Owner, "SETTLEMENT_TERMINAL", applyErr.Error()); markErr != nil {
					workCancel()
					return errors.Join(applyErr, markErr)
				}
				s.failedTerminalTotal.Add(1)
				workCancel()
				continue
			}
			nextAttempt := time.Now().Add(s.retryDelay(item.RetryCount + 1))
			if markErr := s.attempts.MarkRetryable(workCtx, item.ID, s.options.Owner, nextAttempt, "SETTLEMENT_RETRYABLE", applyErr.Error()); markErr != nil {
				workCancel()
				return errors.Join(applyErr, markErr)
			}
			s.retryTotal.Add(1)
		}
		if applyErr == nil && applyResult != nil {
			if applyResult.AlreadySettled {
				s.alreadySettledTotal.Add(1)
			} else if applyResult.Billing.Applied {
				s.recoveredTotal.Add(1)
			}
			s.mu.Lock()
			observer := s.observer
			s.mu.Unlock()
			if observer != nil {
				observer.AfterUsageSettlement(workCtx, item, applyResult)
			}
		}
		workCancel()
	}
	if len(items) == s.options.BatchSize {
		s.Wake()
	}
	return nil
}

func (s *DurableSettlement) logBacklogStats(ctx context.Context) {
	if s == nil || s.attempts == nil {
		return
	}
	stats, err := s.attempts.BacklogStats(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Warn("durable settlement backlog stats failed", "error", err)
		}
		return
	}
	oldestAgeSeconds := int64(0)
	if stats.OldestPendingAt != nil {
		oldestAgeSeconds = max(int64(time.Since(*stats.OldestPendingAt).Seconds()), 0)
	}
	slog.Info("durable settlement backlog",
		"mode", s.Mode().String(),
		"started", stats.Started,
		"ready", stats.Ready,
		"processing", stats.Processing,
		"retryable", stats.Retryable,
		"reconciliation_required", stats.ReconciliationRequired,
		"oldest_pending_seconds", oldestAgeSeconds,
		"recovered_total", s.recoveredTotal.Load(),
		"already_settled_total", s.alreadySettledTotal.Load(),
		"retry_total", s.retryTotal.Load(),
		"failed_terminal_total", s.failedTerminalTotal.Load(),
		"reconciliation_total", s.reconciliationTotal.Load(),
		"provider_recovered_total", s.providerRecoveredTotal.Load(),
	)
}

func (s *DurableSettlement) retryDelay(retryCount int) time.Duration {
	if retryCount < 1 {
		retryCount = 1
	}
	delay := s.options.RetryBase
	for index := 1; index < retryCount && delay < s.options.RetryMax; index++ {
		delay *= 2
		if delay > s.options.RetryMax {
			delay = s.options.RetryMax
		}
	}
	return delay
}

func isTerminalSettlementError(err error) bool {
	return errors.Is(err, domain.ErrUsageSettlementFingerprintConflict) ||
		errors.Is(err, domain.ErrUsageSettlementPayloadConflict) ||
		errors.Is(err, domain.ErrUsageSettlementSensitivePayload) ||
		errors.Is(err, domain.ErrUsageBillingRequestConflict)
}
