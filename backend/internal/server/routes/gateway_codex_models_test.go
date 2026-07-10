package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGatewayRoutesCodexModelsManifestPathIsRegistered(t *testing.T) {
	router := newGatewayRoutesTestRouter()

	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		if route.Method == http.MethodGet {
			registered[route.Path] = true
		}
	}

	require.True(t, registered["/backend-api/codex/models"], "GET /backend-api/codex/models should be registered")
	require.True(t, registered["/v1/models"], "GET /v1/models should be registered")
	require.True(t, registered["/models"], "GET /models should be registered for custom provider base_url roots")
}

// TestGatewayRoutesCodexModelsClientVersionDispatchesToCodexHandler verifies that
// custom-provider style GET /models?client_version=... is routed to CodexModels
// (not the generic OpenAI model list path). With a nil service the handler returns
// a stable 500 rather than 404, which is enough to assert dispatch.
func TestGatewayRoutesCodexModelsClientVersionDispatchesToCodexHandler(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformOpenAI)

	for _, path := range []string{
		"/models?client_version=0.144.0",
		"/v1/models?client_version=0.144.0",
		"/backend-api/codex/models?client_version=0.144.0",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s should hit CodexModels", path)
		// Empty OpenAIGatewayHandler has nil gatewayService; CodexModels nil-guards to 500.
		require.Equal(t, http.StatusInternalServerError, w.Code, "path=%s body=%s", path, w.Body.String())
		require.Contains(t, w.Body.String(), "gateway service unavailable")
	}

	// Non-OpenAI groups must not claim the Codex-only manifest route semantics
	// for the dedicated backend-api path (platform gate).
	grokRouter := newGatewayRoutesTestRouter(service.PlatformGrok)
	req := httptest.NewRequest(http.MethodGet, "/backend-api/codex/models?client_version=0.144.0", nil)
	w := httptest.NewRecorder()
	grokRouter.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "only available for OpenAI groups")
}
