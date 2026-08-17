package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpsHandlerGetOpenAICompatibilityFallbackMetrics(t *testing.T) {
	h := NewOpsHandler(newRuntimeOpsService(t))
	router := newOpsRuntimeRouter(h, true)
	router.GET("/runtime/openai-compatibility-fallbacks", h.GetOpenAICompatibilityFallbackMetrics)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/runtime/openai-compatibility-fallbacks", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Code int `json:"code"`
		Data struct {
			ObservedAt             time.Time      `json:"observed_at"`
			Scope                  string         `json:"scope"`
			ReadOldFallbackEnabled bool           `json:"read_old_fallback_enabled"`
			DualWriteOldEnabled    bool           `json:"dual_write_old_enabled"`
			LegacyTTLMaxSeconds    int            `json:"legacy_ttl_max_seconds"`
			Metrics                map[string]any `json:"metrics"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Zero(t, payload.Code)
	require.False(t, payload.Data.ObservedAt.IsZero())
	require.Equal(t, "process", payload.Data.Scope)
	require.False(t, payload.Data.ReadOldFallbackEnabled)
	require.False(t, payload.Data.DualWriteOldEnabled)
	require.Equal(t, 600, payload.Data.LegacyTTLMaxSeconds)
	require.Contains(t, payload.Data.Metrics, "session_hash_legacy_read_fallback_total")
	require.Contains(t, payload.Data.Metrics, "session_hash_legacy_read_fallback_hit")
	require.Contains(t, payload.Data.Metrics, "session_hash_legacy_dual_write_total")
}
