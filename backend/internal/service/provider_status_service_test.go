package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpsServiceGetProviderStatusCurrentFreshAndStale(t *testing.T) {
	now := time.Now().UTC()
	components, _ := json.Marshal([]ProviderStatusComponent{{ID: "responses", Name: "Responses", Status: "operational"}})
	repo := &opsRepoMock{}
	repo.GetLatestProviderStatusSnapshotFn = func(context.Context, string) (*ProviderStatusSnapshotRecord, error) {
		return &ProviderStatusSnapshotRecord{
			ID: 1, Provider: ProviderStatusOpenAI, SourceURL: OpenAIStatusSummaryURL,
			OverallIndicator: "none", OverallDescription: "Fully operational",
			ComponentsJSON: components, IncidentsJSON: json.RawMessage(`[]`), FetchedAt: now.Add(-2 * time.Minute),
		}, nil
	}
	svc := NewOpsService(repo, nil, &config.Config{Ops: config.OpsConfig{Enabled: true, ProviderStatus: config.OpsProviderStatusConfig{
		Enabled: true, PollIntervalSeconds: 60, StaleAfterSeconds: 180,
	}}}, nil, nil, nil, nil, nil, nil, nil, nil)

	current, err := svc.GetProviderStatusCurrent(context.Background(), ProviderStatusOpenAI)
	require.NoError(t, err)
	require.Equal(t, ProviderStatusFresh, current.Freshness)
	require.Len(t, current.Snapshot.Components, 1)

	repo.GetLatestProviderStatusSnapshotFn = func(context.Context, string) (*ProviderStatusSnapshotRecord, error) {
		return &ProviderStatusSnapshotRecord{ID: 2, Provider: ProviderStatusOpenAI, ComponentsJSON: json.RawMessage(`[]`), IncidentsJSON: json.RawMessage(`[]`), FetchedAt: now.Add(-4 * time.Minute)}, nil
	}
	current, err = svc.GetProviderStatusCurrent(context.Background(), ProviderStatusOpenAI)
	require.NoError(t, err)
	require.Equal(t, ProviderStatusStale, current.Freshness)
}
