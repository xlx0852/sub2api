//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryProviderStatusSnapshots(t *testing.T) {
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, "TRUNCATE provider_status_snapshots RESTART IDENTITY")
	require.NoError(t, err)
	repo := NewOpsRepository(integrationDB)
	now := time.Now().UTC().Truncate(time.Second)
	first := &service.ProviderStatusSnapshotRecord{
		Provider: service.ProviderStatusOpenAI, SourceURL: service.OpenAIStatusSummaryURL,
		OverallIndicator: "none", OverallDescription: "Fully operational",
		ComponentsJSON: json.RawMessage(`[]`), IncidentsJSON: json.RawMessage(`[]`),
		FetchedAt: now, ContentHash: "hash-1",
	}
	inserted, err := repo.InsertProviderStatusSnapshot(ctx, first)
	require.NoError(t, err)
	require.True(t, inserted)
	inserted, err = repo.InsertProviderStatusSnapshot(ctx, first)
	require.NoError(t, err)
	require.False(t, inserted)

	second := *first
	second.OverallIndicator = "minor"
	second.OverallDescription = "Partial System Degradation"
	second.FetchedAt = now.Add(time.Minute)
	second.ContentHash = "hash-2"
	inserted, err = repo.InsertProviderStatusSnapshot(ctx, &second)
	require.NoError(t, err)
	require.True(t, inserted)

	latest, err := repo.GetLatestProviderStatusSnapshot(ctx, service.ProviderStatusOpenAI)
	require.NoError(t, err)
	require.Equal(t, "minor", latest.OverallIndicator)
	history, err := repo.ListProviderStatusSnapshots(ctx, &service.ProviderStatusHistoryFilter{
		Provider: service.ProviderStatusOpenAI, StartTime: &now, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, history, 2)
}
