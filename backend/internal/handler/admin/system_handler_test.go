package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetVersionReturnsLocalBuildOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/version", nil)

	handler := NewSystemHandler("0.1.391-test", nil, nil)
	handler.GetVersion(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "0.1.391-test", data["version"])
	_, hasUpdate := data["has_update"]
	require.False(t, hasUpdate)
	_, latest := data["latest_version"]
	require.False(t, latest)
}
