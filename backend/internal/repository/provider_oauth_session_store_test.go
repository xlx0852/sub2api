package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newProviderOAuthSessionStoreTest(t *testing.T) (ports.ProviderOAuthSessionStore, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewProviderOAuthSessionStore(rdb), rdb
}

func providerOAuthTestSession(now time.Time, status string) *domain.ProviderOAuthSession {
	return &domain.ProviderOAuthSession{
		ID:                  "session-1",
		Provider:            "test-provider",
		Flow:                domain.ProviderOAuthFlowDeviceCode,
		Status:              status,
		NextPollAtUnixMilli: now.UnixMilli(),
		ExpiresAtUnixMilli:  now.Add(15 * time.Minute).UnixMilli(),
		Payload:             json.RawMessage(`{"device_code":"secret"}`),
	}
}

func TestProviderOAuthSessionStore_PollLeaseSerializesAndCommits(t *testing.T) {
	store, _ := newProviderOAuthSessionStoreTest(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, store.Create(ctx, providerOAuthTestSession(now, domain.ProviderOAuthSessionPending), 15*time.Minute))

	first, err := store.AcquirePollLease(ctx, "session-1", now, 30*time.Second)
	require.NoError(t, err)
	require.True(t, first.Held)
	require.NotEmpty(t, first.ID)
	require.Equal(t, int64(1), first.Session.Version)

	second, err := store.AcquirePollLease(ctx, "session-1", now, 30*time.Second)
	require.NoError(t, err)
	require.False(t, second.Held)
	require.Equal(t, first.ID, second.Session.PollLeaseID)

	updated := *first.Session
	updated.NextPollAtUnixMilli = now.Add(10 * time.Second).UnixMilli()
	committed, err := store.CommitPoll(ctx, first, &updated)
	require.NoError(t, err)
	require.True(t, committed)

	saved, err := store.Get(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, int64(2), saved.Version)
	require.Empty(t, saved.PollLeaseID)
	require.Equal(t, updated.NextPollAtUnixMilli, saved.NextPollAtUnixMilli)
}

func TestProviderOAuthSessionStore_CancelPreventsInflightPollCommit(t *testing.T) {
	store, _ := newProviderOAuthSessionStoreTest(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, store.Create(ctx, providerOAuthTestSession(now, domain.ProviderOAuthSessionPending), 15*time.Minute))

	lease, err := store.AcquirePollLease(ctx, "session-1", now, 30*time.Second)
	require.NoError(t, err)
	require.True(t, lease.Held)

	cancelled, err := store.Cancel(ctx, "session-1", 5*time.Minute)
	require.NoError(t, err)
	require.True(t, cancelled)

	updated := *lease.Session
	updated.Status = domain.ProviderOAuthSessionAuthorized
	updated.Payload = json.RawMessage(`{"access_token":"must-not-survive"}`)
	committed, err := store.CommitPoll(ctx, lease, &updated)
	require.NoError(t, err)
	require.False(t, committed)

	saved, err := store.Get(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, domain.ProviderOAuthSessionCancelled, saved.Status)
	require.Empty(t, saved.Payload)

	consumed, err := store.ConsumeAuthorized(ctx, "session-1")
	require.NoError(t, err)
	require.Nil(t, consumed)
}

func TestProviderOAuthSessionStore_ConsumeAuthorizedExactlyOnce(t *testing.T) {
	store, _ := newProviderOAuthSessionStoreTest(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	session := providerOAuthTestSession(now, domain.ProviderOAuthSessionAuthorized)
	require.NoError(t, store.Create(ctx, session, 15*time.Minute))

	first, err := store.ConsumeAuthorized(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, domain.ProviderOAuthSessionAuthorized, first.Status)

	second, err := store.ConsumeAuthorized(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, second)
}

func TestProviderOAuthSessionStore_PendingConsumeDoesNotDestroySession(t *testing.T) {
	store, _ := newProviderOAuthSessionStoreTest(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	session := providerOAuthTestSession(now, domain.ProviderOAuthSessionPending)
	require.NoError(t, store.Create(ctx, session, 15*time.Minute))

	consumed, err := store.ConsumeAuthorized(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, consumed)

	saved, err := store.Get(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, domain.ProviderOAuthSessionPending, saved.Status)
}

func TestKimiDeviceSessionStore_CancelWinsPollRace(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewKimiDeviceSessionStore(rdb)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	session := &domain.KimiDeviceSession{
		ID: "kimi-session", Status: domain.ProviderOAuthSessionPending,
		DeviceCode: "device-secret", UserCode: "USER-CODE",
		NextPollAt: now, ExpiresAt: now.Add(15 * time.Minute), IntervalSeconds: 5,
	}
	require.NoError(t, store.Create(ctx, session, 15*time.Minute))

	lease, err := store.AcquirePollLease(ctx, session.ID, now, 30*time.Second)
	require.NoError(t, err)
	require.True(t, lease.Held)

	_, err = store.Cancel(ctx, session.ID, 5*time.Minute)
	require.NoError(t, err)
	lease.Session.Status = domain.ProviderOAuthSessionAuthorized
	lease.Session.Token = &domain.KimiOAuthTokenInfo{AccessToken: "must-not-survive"}
	committed, err := store.CommitPoll(ctx, lease, lease.Session)
	require.NoError(t, err)
	require.False(t, committed)
}
