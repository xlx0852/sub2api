//go:build integration

package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerCacheSnapshotUsesSlimMetadataButKeepsFullAccount(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := NewSchedulerCache(rdb)

	bucket := domain.SchedulerBucket{GroupID: 2, Platform: domain.PlatformGemini, Mode: domain.SchedulerModeSingle}
	now := time.Now().UTC().Truncate(time.Second)
	limitReset := now.Add(10 * time.Minute)
	overloadUntil := now.Add(2 * time.Minute)
	tempUnschedUntil := now.Add(3 * time.Minute)
	windowEnd := now.Add(5 * time.Hour)

	account := domain.Account{
		ID:          101,
		Name:        "gemini-heavy",
		Platform:    domain.PlatformGemini,
		Type:        domain.AccountTypeOAuth,
		Status:      domain.StatusActive,
		Schedulable: true,
		Concurrency: 3,
		Priority:    7,
		LastUsedAt:  &now,
		Credentials: map[string]any{
			"api_key":       "gemini-api-key",
			"access_token":  "secret-access-token",
			"project_id":    "proj-1",
			"oauth_type":    "ai_studio",
			"model_mapping": map[string]any{"gemini-2.5-pro": "gemini-2.5-pro"},
			"huge_blob":     strings.Repeat("x", 4096),
		},
		Extra: map[string]any{
			"mixed_scheduling":             true,
			"unused_large_field":           strings.Repeat("y", 4096),
		},
		RateLimitResetAt:       &limitReset,
		OverloadUntil:          &overloadUntil,
		TempUnschedulableUntil: &tempUnschedUntil,
		SessionWindowStart:     &now,
		SessionWindowEnd:       &windowEnd,
		SessionWindowStatus:    "active",
		GroupIDs:               []int64{bucket.GroupID},
		AccountGroups: []domain.AccountGroup{
			{
				AccountID: 101,
				GroupID:   bucket.GroupID,
				Priority:  5,
				Group:     &domain.Group{ID: bucket.GroupID, Name: "gemini-group"},
			},
		},
	}

	require.NoError(t, cache.SetSnapshot(ctx, bucket, []domain.Account{account}))

	snapshot, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, snapshot, 1)

	got := snapshot[0]
	require.NotNil(t, got)
	require.Equal(t, "gemini-api-key", got.GetCredential("api_key"))
	require.Equal(t, "proj-1", got.GetCredential("project_id"))
	require.Equal(t, "ai_studio", got.GetCredential("oauth_type"))
	require.NotEmpty(t, got.GetModelMapping())
	require.Empty(t, got.GetCredential("access_token"))
	require.Empty(t, got.GetCredential("huge_blob"))
	require.Equal(t, true, got.Extra["mixed_scheduling"])
	require.Nil(t, got.Extra["unused_large_field"])
	require.Equal(t, []int64{bucket.GroupID}, got.GroupIDs)
	require.Len(t, got.AccountGroups, 1)
	require.Equal(t, account.ID, got.AccountGroups[0].AccountID)
	require.Equal(t, bucket.GroupID, got.AccountGroups[0].GroupID)
	require.Nil(t, got.AccountGroups[0].Group)

	full, err := cache.GetAccount(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, full)
	require.Equal(t, "secret-access-token", full.GetCredential("access_token"))
	require.Equal(t, strings.Repeat("x", 4096), full.GetCredential("huge_blob"))
	require.Len(t, full.AccountGroups, 1)
	require.NotNil(t, full.AccountGroups[0].Group)
}
