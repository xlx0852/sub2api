package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newProviderOAuthSessionStoreTest(t *testing.T) (service.ProviderOAuthSessionStore, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewProviderOAuthSessionStore(rdb), rdb
}

func providerOAuthTestSession(now time.Time, status string) *service.ProviderOAuthSession {
	return &service.ProviderOAuthSession{
		ID:                  "session-1",
		Provider:            "test-provider",
		Flow:                service.ProviderOAuthFlowDeviceCode,
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
	require.NoError(t, store.Create(ctx, providerOAuthTestSession(now, service.ProviderOAuthSessionPending), 15*time.Minute))

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
	require.NoError(t, store.Create(ctx, providerOAuthTestSession(now, service.ProviderOAuthSessionPending), 15*time.Minute))

	lease, err := store.AcquirePollLease(ctx, "session-1", now, 30*time.Second)
	require.NoError(t, err)
	require.True(t, lease.Held)

	cancelled, err := store.Cancel(ctx, "session-1", 5*time.Minute)
	require.NoError(t, err)
	require.True(t, cancelled)

	updated := *lease.Session
	updated.Status = service.ProviderOAuthSessionAuthorized
	updated.Payload = json.RawMessage(`{"access_token":"must-not-survive"}`)
	committed, err := store.CommitPoll(ctx, lease, &updated)
	require.NoError(t, err)
	require.False(t, committed)

	saved, err := store.Get(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, service.ProviderOAuthSessionCancelled, saved.Status)
	require.Empty(t, saved.Payload)

	consumed, err := store.ConsumeAuthorized(ctx, "session-1")
	require.NoError(t, err)
	require.Nil(t, consumed)
}

func TestProviderOAuthSessionStore_ConsumeAuthorizedExactlyOnce(t *testing.T) {
	store, _ := newProviderOAuthSessionStoreTest(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	session := providerOAuthTestSession(now, service.ProviderOAuthSessionAuthorized)
	require.NoError(t, store.Create(ctx, session, 15*time.Minute))

	first, err := store.ConsumeAuthorized(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, service.ProviderOAuthSessionAuthorized, first.Status)

	second, err := store.ConsumeAuthorized(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, second)
}

func TestProviderOAuthSessionStore_PendingConsumeDoesNotDestroySession(t *testing.T) {
	store, _ := newProviderOAuthSessionStoreTest(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	session := providerOAuthTestSession(now, service.ProviderOAuthSessionPending)
	require.NoError(t, store.Create(ctx, session, 15*time.Minute))

	consumed, err := store.ConsumeAuthorized(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, consumed)

	saved, err := store.Get(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, service.ProviderOAuthSessionPending, saved.Status)
}

func TestKimiDeviceSessionStore_CancelWinsPollRace(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewKimiDeviceSessionStore(rdb)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	session := &service.KimiDeviceSession{
		ID: "kimi-session", Status: service.ProviderOAuthSessionPending,
		DeviceCode: "device-secret", UserCode: "USER-CODE",
		NextPollAt: now, ExpiresAt: now.Add(15 * time.Minute), IntervalSeconds: 5,
	}
	require.NoError(t, store.Create(ctx, session, 15*time.Minute))

	lease, err := store.AcquirePollLease(ctx, session.ID, now, 30*time.Second)
	require.NoError(t, err)
	require.True(t, lease.Held)

	_, err = store.Cancel(ctx, session.ID, 5*time.Minute)
	require.NoError(t, err)
	lease.Session.Status = service.ProviderOAuthSessionAuthorized
	lease.Session.Token = &service.KimiOAuthTokenInfo{AccessToken: "must-not-survive"}
	committed, err := store.CommitPoll(ctx, lease, lease.Session)
	require.NoError(t, err)
	require.False(t, committed)
}
