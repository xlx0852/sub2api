package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAntigravityResponsesWebSocketFallsBackToHTTP(t *testing.T) {
	router := newGatewayRoutesTestRouter(service.PlatformAntigravity)
	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUpgradeRequired, w.Code)
	require.Contains(t, w.Body.String(), "websocket_not_supported")
}
