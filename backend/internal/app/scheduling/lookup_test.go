package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

type stubAccountLookup struct {
	account *domain.Account
	err     error
	called  bool
}

func (s *stubAccountLookup) GetAccount(context.Context, int64) (*domain.Account, error) {
	s.called = true
	return s.account, s.err
}

func TestConfigOrDefault(t *testing.T) {
	require.Equal(t, DefaultConfig(), ConfigOrDefault(nil))
	cfg := config.GatewaySchedulingConfig{StickySessionMaxWaiting: 9}
	require.Equal(t, cfg, ConfigOrDefault(&cfg))
}

func TestLookupSchedulablePrefersSnapshot(t *testing.T) {
	snapshot := &stubAccountLookup{account: &domain.Account{ID: 1}}
	fallback := &stubAccountLookup{account: &domain.Account{ID: 2}}
	got, err := LookupSchedulable(context.Background(), snapshot, fallback, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.ID)
	require.True(t, snapshot.called)
	require.False(t, fallback.called)
}

func TestLookupSchedulableFallsBackWhenSnapshotNil(t *testing.T) {
	fallback := &stubAccountLookup{account: &domain.Account{ID: 2}}
	got, err := LookupSchedulable(context.Background(), nil, fallback, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), got.ID)
	require.True(t, fallback.called)
}

func TestHydrateSelectedSkipsWhenSnapshotNil(t *testing.T) {
	account := &domain.Account{ID: 3}
	got, err := HydrateSelected(context.Background(), nil, account, "gateway")
	require.NoError(t, err)
	require.Equal(t, account, got)
}

func TestHydrateSelectedErrorsWhenSnapshotMisses(t *testing.T) {
	_, err := HydrateSelected(context.Background(), &stubAccountLookup{}, &domain.Account{ID: 4}, "openai")
	require.EqualError(t, err, "selected openai account 4 not found during hydration")
}

func TestHydrateSelectedPropagatesLookupError(t *testing.T) {
	want := errors.New("cache down")
	_, err := HydrateSelected(context.Background(), &stubAccountLookup{err: want}, &domain.Account{ID: 5}, "gateway")
	require.ErrorIs(t, err, want)
}

func TestNewResultAndModelLimited(t *testing.T) {
	account := &domain.Account{ID: 6}
	plan := WaitPlan{AccountID: 6, Timeout: time.Second}
	got := NewResult(account, false, nil, &plan)
	require.Equal(t, account, got.Account)
	require.False(t, got.Acquired)
	require.Equal(t, &plan, got.WaitPlan)

	limited := NewModelLimitedResult(account)
	require.True(t, limited.ModelLimited)
	require.False(t, limited.Acquired)
	require.Nil(t, limited.WaitPlan)
}

func TestNewResultReleaseIsIdempotent(t *testing.T) {
	calls := 0
	result := NewResult(&domain.Account{ID: 7}, true, func() { calls++ }, nil)

	result.Release()
	result.Release()

	require.Equal(t, 1, calls)
}
