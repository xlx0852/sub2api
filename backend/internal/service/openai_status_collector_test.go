package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIStatusCollectorDeduplicatesAndCreatesTransitionEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(validOpenAIStatusFixture))
	}))
	defer server.Close()

	var mu sync.Mutex
	records := []*ProviderStatusSnapshotRecord{{
		ID: 1, Provider: ProviderStatusOpenAI, OverallIndicator: "none", OverallDescription: "Fully operational",
		FetchedAt: time.Now().UTC().Add(-time.Minute), ContentHash: "old",
	}}
	seen := map[string]struct{}{"old": {}}
	createdEvents := 0
	repo := &opsRepoMock{}
	repo.InsertProviderStatusSnapshotFn = func(_ context.Context, input *ProviderStatusSnapshotRecord) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		if _, ok := seen[input.ContentHash]; ok {
			return false, nil
		}
		seen[input.ContentHash] = struct{}{}
		copy := *input
		copy.ID = int64(len(records) + 1)
		records = append(records, &copy)
		return true, nil
	}
	repo.ListProviderStatusSnapshotsFn = func(_ context.Context, _ *ProviderStatusHistoryFilter) ([]*ProviderStatusSnapshotRecord, error) {
		mu.Lock()
		defer mu.Unlock()
		out := append([]*ProviderStatusSnapshotRecord(nil), records...)
		sort.Slice(out, func(i, j int) bool { return out[i].FetchedAt.After(out[j].FetchedAt) })
		return out, nil
	}
	repo.CreateAlertEventFn = func(_ context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
		createdEvents++
		return event, nil
	}

	cfg := &config.Config{Ops: config.OpsConfig{Enabled: true, ProviderStatus: config.OpsProviderStatusConfig{
		Enabled: true, PollIntervalSeconds: 60, StaleAfterSeconds: 180, RequestTimeoutSeconds: 5, MaxBodyBytes: 64 * 1024,
	}}}
	collector := NewOpenAIStatusCollectorService(repo, nil, cfg)
	collector.client = NewOpenAIStatusClientForTest(server.Client(), server.URL, 64*1024)

	collector.collectOnce()
	collector.collectOnce()

	require.Len(t, records, 2, "unchanged second poll must not create another snapshot")
	require.Equal(t, 1, createdEvents, "degradation transition must be deduplicated")
}
