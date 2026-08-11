package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const validOpenAIStatusFixture = `{
  "page":{"id":"page-1","name":"OpenAI","url":"https://status.openai.com/","updated_at":"2026-08-10T15:07:54Z"},
  "status":{"description":"Partial System Degradation","indicator":"minor"},
  "components":[
    {"id":"responses","name":"Responses","status":"operational","updated_at":"2026-08-10T15:00:00Z"},
    {"id":"conversations","name":"Conversations","status":"degraded_performance","updated_at":"2026-08-10T15:07:54Z"}
  ],
  "incidents":[{
    "id":"incident-1","name":"Increased error rates","status":"monitoring","impact":"minor",
    "created_at":"2026-08-10T14:04:37Z","updated_at":"2026-08-10T15:07:54Z","monitoring_at":"2026-08-10T15:07:54Z",
    "incident_updates":[{"id":"update-1","status":"monitoring","body":"Mitigation applied","created_at":"2026-08-10T15:07:54Z","updated_at":"2026-08-10T15:07:54Z"}]
  }]
}`

func TestOpenAIStatusClientFetchNormalizesAndHashesContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(validOpenAIStatusFixture))
	}))
	defer server.Close()

	client := NewOpenAIStatusClientForTest(server.Client(), server.URL, 64*1024)
	first, err := client.Fetch(context.Background(), time.Date(2026, 8, 10, 15, 8, 0, 0, time.UTC))
	require.NoError(t, err)
	second, err := client.Fetch(context.Background(), time.Date(2026, 8, 10, 15, 9, 0, 0, time.UTC))
	require.NoError(t, err)

	require.Equal(t, "minor", first.OverallIndicator)
	require.Equal(t, first.ContentHash, second.ContentHash)
	require.NotEqual(t, first.FetchedAt, second.FetchedAt)
	require.Contains(t, string(first.ComponentsJSON), "degraded_performance")
	require.Contains(t, string(first.IncidentsJSON), "Mitigation applied")
}

func TestOpenAIStatusClientRejectsOversizedAndMalformedPayloads(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", 1025)))
		}))
		defer server.Close()
		client := NewOpenAIStatusClientForTest(server.Client(), server.URL, 1024)
		_, err := client.Fetch(context.Background(), time.Now())
		require.ErrorContains(t, err, "exceeds")
	})

	t.Run("missing required fields", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":{}}`))
		}))
		defer server.Close()
		client := NewOpenAIStatusClientForTest(server.Client(), server.URL, 4096)
		_, err := client.Fetch(context.Background(), time.Now())
		require.ErrorContains(t, err, "missing required fields")
	})
}
