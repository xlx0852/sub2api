package billing

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/stretchr/testify/require"
)

type durableSettlementRepoFake struct {
	mu            sync.Mutex
	claim         []domain.UsageSettlementAttempt
	claimCount    int
	retryCount    int
	terminalCount int
	startedCount  int
	readyCount    int
	expireCount   int
	statsCount    int
	lastClaim     ports.UsageSettlementClaimRequest
	lastStarted   *domain.UsageSettlementAttempt
}

func (f *durableSettlementRepoFake) CreateStarted(_ context.Context, attempt *domain.UsageSettlementAttempt) (*domain.UsageSettlementAttempt, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startedCount++
	copy := *attempt
	f.lastStarted = &copy
	return attempt, true, nil
}
func (f *durableSettlementRepoFake) ObserveProviderRequestID(_ context.Context, _ string, _ int64, _, _ string) (*domain.UsageSettlementAttempt, error) {
	return &domain.UsageSettlementAttempt{}, nil
}
func (f *durableSettlementRepoFake) MarkReady(_ context.Context, _ string, _ int64, _ string, _ *domain.UsageSettlementPayloadV1) (*domain.UsageSettlementAttempt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readyCount++
	return &domain.UsageSettlementAttempt{}, nil
}
func (f *durableSettlementRepoFake) Claim(_ context.Context, request ports.UsageSettlementClaimRequest) ([]domain.UsageSettlementAttempt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimCount++
	f.lastClaim = request
	items := f.claim
	f.claim = nil
	return items, nil
}
func (f *durableSettlementRepoFake) ExpireStarted(context.Context, time.Time, int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.expireCount++
	return 0, nil
}
func (f *durableSettlementRepoFake) ListExpiredRecoverable(context.Context, time.Time, int) ([]domain.UsageSettlementAttempt, error) {
	return nil, nil
}
func (f *durableSettlementRepoFake) BacklogStats(context.Context) (*ports.UsageSettlementBacklogStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statsCount++
	return &ports.UsageSettlementBacklogStats{}, nil
}
func (f *durableSettlementRepoFake) MarkRetryable(_ context.Context, _ int64, _ string, _ time.Time, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retryCount++
	return nil
}
func (f *durableSettlementRepoFake) MarkReconciliationRequired(context.Context, string, int64, string, string, string) error {
	return nil
}
func (f *durableSettlementRepoFake) MarkFailedTerminal(_ context.Context, _ int64, _, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminalCount++
	return nil
}
func (f *durableSettlementRepoFake) MarkSettled(context.Context, int64, string) error { return nil }
func (f *durableSettlementRepoFake) GetByIdentity(context.Context, string, int64) (*domain.UsageSettlementAttempt, error) {
	return nil, errors.New("not implemented")
}

type durableSettlementBillingFake struct {
	mu      sync.Mutex
	calls   int
	err     error
	started chan struct{}
	release chan struct{}
}

type durableSettlementObserverFake struct {
	mu    sync.Mutex
	calls int
}

func (f *durableSettlementObserverFake) AfterUsageSettlement(context.Context, *domain.UsageSettlementAttempt, *domain.UsageSettlementApplyResult) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
}

type durableSettlementSettingRepoFake struct {
	value      string
	err        error
	scopeValue string
	scopeErr   error
}

func (f *durableSettlementSettingRepoFake) Get(context.Context, string) (*domain.Setting, error) {
	return nil, f.err
}
func (f *durableSettlementSettingRepoFake) GetValue(_ context.Context, key string) (string, error) {
	if key == SettingKeySettlementDurabilityGroupIDs {
		if f.scopeErr != nil {
			return "", f.scopeErr
		}
		if strings.TrimSpace(f.scopeValue) == "" {
			return "*", nil
		}
		return f.scopeValue, nil
	}
	if key != SettingKeySettlementDurabilityMode {
		return "", errors.New("unexpected setting key")
	}
	return f.value, f.err
}

func TestParseSettlementRolloutScope(t *testing.T) {
	all, err := parseSettlementRolloutScope("*")
	require.NoError(t, err)
	require.True(t, all.matches(nil))

	scope, err := parseSettlementRolloutScope(`[31, 32, 31]`)
	require.NoError(t, err)
	group31, group32, group33 := int64(31), int64(32), int64(33)
	require.True(t, scope.matches(&group31))
	require.True(t, scope.matches(&group32))
	require.False(t, scope.matches(&group33))
	require.Equal(t, []int64{31, 32}, scope.claimGroupIDs())

	for _, raw := range []string{"", "[]", "[0]", "[-1]", "not-json"} {
		_, parseErr := parseSettlementRolloutScope(raw)
		require.Error(t, parseErr, raw)
	}
}

func TestDurableSettlementGroupScopeControlsWritesAuthorityAndClaims(t *testing.T) {
	repo := &durableSettlementRepoFake{}
	worker := NewDurableSettlement(repo, &durableSettlementBillingFake{}, DurableSettlementOptions{})
	scope, err := parseSettlementRolloutScope(`[41]`)
	require.NoError(t, err)
	worker.setRollout(SettlementDurabilityAuthoritative, scope)
	selected, other := int64(41), int64(42)
	require.True(t, worker.EnabledFor(&selected))
	require.True(t, worker.AuthoritativeFor(&selected))
	require.False(t, worker.EnabledFor(&other))
	require.False(t, worker.AuthoritativeFor(&other))

	_, inserted, err := worker.StartAttempt(context.Background(), &domain.UsageSettlementAttempt{GroupID: &other})
	require.NoError(t, err)
	require.False(t, inserted)
	require.Zero(t, repo.startedCount)
	_, inserted, err = worker.StartAttempt(context.Background(), &domain.UsageSettlementAttempt{GroupID: &selected})
	require.NoError(t, err)
	require.True(t, inserted)
	require.Equal(t, 1, repo.startedCount)
	require.NotNil(t, repo.lastStarted)
	require.True(t, repo.lastStarted.Authoritative)

	require.NoError(t, worker.drainOnce(context.Background()))
	require.Equal(t, []int64{41}, repo.lastClaim.GroupIDs)
}
func (f *durableSettlementSettingRepoFake) Set(context.Context, string, string) error { return nil }
func (f *durableSettlementSettingRepoFake) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}
func (f *durableSettlementSettingRepoFake) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (f *durableSettlementSettingRepoFake) GetAll(context.Context) (map[string]string, error) {
	return nil, nil
}
func (f *durableSettlementSettingRepoFake) Delete(context.Context, string) error { return nil }

func (f *durableSettlementBillingFake) ApplySettlement(ctx context.Context, _ int64, _ string) (*domain.UsageSettlementApplyResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &domain.UsageSettlementApplyResult{}, f.err
}

func TestDurableSettlementModesGateWritesAndClaims(t *testing.T) {
	repo := &durableSettlementRepoFake{claim: []domain.UsageSettlementAttempt{{ID: 1}}}
	billing := &durableSettlementBillingFake{}
	worker := NewDurableSettlement(repo, billing, DurableSettlementOptions{PollInterval: time.Hour})

	_, inserted, err := worker.StartAttempt(context.Background(), &domain.UsageSettlementAttempt{})
	require.NoError(t, err)
	require.False(t, inserted)
	require.Equal(t, 0, repo.startedCount)

	worker.SetMode(SettlementDurabilityShadow)
	_, inserted, err = worker.StartAttempt(context.Background(), &domain.UsageSettlementAttempt{})
	require.NoError(t, err)
	require.True(t, inserted)
	_, err = worker.HandoffReady(context.Background(), "id", 1, "fp", &domain.UsageSettlementPayloadV1{})
	require.NoError(t, err)
	require.Equal(t, 1, repo.readyCount)
	worker.Start(context.Background())
	worker.Wake()
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, 0, repo.claimCount)
	require.Equal(t, 1, repo.expireCount, "shadow mode must retain missing-terminal attempts as auditable reconciliation records")
	worker.logBacklogStats(context.Background())
	require.Equal(t, 1, repo.statsCount)
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, worker.StopAndWait(stopCtx))
}

func TestProvideDurableSettlementLoadsDatabaseModeAndFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		err        error
		scopeRaw   string
		scopeErr   error
		want       SettlementDurabilityMode
		wantGroup  int64
		wantGlobal bool
	}{
		{name: "missing", err: domain.ErrSettingNotFound, want: SettlementDurabilityOff},
		{name: "read failure", err: errors.New("database unavailable"), want: SettlementDurabilityOff},
		{name: "invalid", raw: "enabled", want: SettlementDurabilityOff},
		{name: "shadow", raw: "shadow", scopeRaw: `[61]`, want: SettlementDurabilityShadow, wantGroup: 61},
		{name: "authoritative", raw: "authoritative", scopeRaw: "*", want: SettlementDurabilityAuthoritative, wantGlobal: true},
		{name: "active missing scope", raw: "authoritative", scopeErr: domain.ErrSettingNotFound, want: SettlementDurabilityOff},
		{name: "active invalid scope", raw: "shadow", scopeRaw: `[]`, want: SettlementDurabilityOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := ProvideDurableSettlement(
				&durableSettlementRepoFake{},
				&durableSettlementBillingFake{},
				&durableSettlementSettingRepoFake{value: tt.raw, err: tt.err, scopeValue: tt.scopeRaw, scopeErr: tt.scopeErr},
			)
			require.Equal(t, tt.want, worker.Mode())
			if tt.wantGroup > 0 {
				require.True(t, worker.EnabledFor(&tt.wantGroup))
				other := tt.wantGroup + 1
				require.False(t, worker.EnabledFor(&other))
			}
			if tt.wantGlobal {
				require.True(t, worker.EnabledFor(nil))
			}
			stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			require.NoError(t, worker.StopAndWait(stopCtx))
		})
	}
}

func TestDurableSettlementWorkerRetriesAndClassifiesTerminalErrors(t *testing.T) {
	repo := &durableSettlementRepoFake{claim: []domain.UsageSettlementAttempt{{ID: 1}}}
	billing := &durableSettlementBillingFake{err: errors.New("temporary")}
	worker := NewDurableSettlement(repo, billing, DurableSettlementOptions{})
	require.NoError(t, worker.drainOnce(context.Background()))
	require.Equal(t, 1, repo.retryCount)

	repo.claim = []domain.UsageSettlementAttempt{{ID: 2}}
	billing.err = domain.ErrUsageBillingRequestConflict
	require.NoError(t, worker.drainOnce(context.Background()))
	require.Equal(t, 1, repo.terminalCount)
}

func TestDurableSettlementObserverRunsOnlyAfterSuccessfulCommit(t *testing.T) {
	repo := &durableSettlementRepoFake{claim: []domain.UsageSettlementAttempt{{ID: 1}}}
	billing := &durableSettlementBillingFake{}
	observer := &durableSettlementObserverFake{}
	worker := NewDurableSettlement(repo, billing, DurableSettlementOptions{})
	worker.SetAppliedObserver(observer)
	require.NoError(t, worker.drainOnce(context.Background()))
	require.Equal(t, 1, observer.calls)

	repo.claim = []domain.UsageSettlementAttempt{{ID: 2}}
	billing.err = errors.New("commit failed")
	require.NoError(t, worker.drainOnce(context.Background()))
	require.Equal(t, 1, observer.calls)
}

func TestDurableSettlementStopWaitsForInFlightApply(t *testing.T) {
	repo := &durableSettlementRepoFake{claim: []domain.UsageSettlementAttempt{{ID: 1}}}
	billing := &durableSettlementBillingFake{started: make(chan struct{}, 1), release: make(chan struct{})}
	worker := NewDurableSettlement(repo, billing, DurableSettlementOptions{PollInterval: time.Hour})
	worker.SetMode(SettlementDurabilityAuthoritative)
	worker.Start(context.Background())
	worker.Wake()
	select {
	case <-billing.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start apply")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, worker.StopAndWait(stopCtx), context.DeadlineExceeded)
	close(billing.release)
	finalCtx, finalCancel := context.WithTimeout(context.Background(), time.Second)
	defer finalCancel()
	require.NoError(t, worker.StopAndWait(finalCtx))
}
