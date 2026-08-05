//go:build unit

package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandler_GetSettings_ReturnsModelConcurrencyLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyModelConcurrencyLimits: `{"gpt-5.6-luna":6}`,
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)

	handler.GetSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	limits, ok := data["model_concurrency_limits"].(map[string]any)
	require.True(t, ok, "model_concurrency_limits must be present in GetSettings response")
	require.Equal(t, float64(6), limits["gpt-5.6-luna"])
}

func TestSettingHandler_UpdateSettings_PersistsModelConcurrencyLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{
		values: map[string]string{
			service.SettingKeyModelConcurrencyLimits: `{"gpt-5.6-luna":8}`,
		},
	}
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)

	body := map[string]any{
		"model_concurrency_limits": map[string]int{"gpt-5.6-luna": 6},
	}
	rawBody, err := json.Marshal(body)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, `{"gpt-5.6-luna":6}`, repo.values[service.SettingKeyModelConcurrencyLimits])

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	limits, ok := data["model_concurrency_limits"].(map[string]any)
	require.True(t, ok, "update response must echo model_concurrency_limits")
	require.Equal(t, float64(6), limits["gpt-5.6-luna"])
}

func TestDiffSettings_DetectsModelConcurrencyLimitsChange(t *testing.T) {
	before := &service.SystemSettings{
		ModelConcurrencyLimits: map[string]int{"gpt-5.6-luna": 8},
	}
	after := &service.SystemSettings{
		ModelConcurrencyLimits: map[string]int{"gpt-5.6-luna": 6},
	}
	changed := diffSettings(before, after, nil, nil, UpdateSettingsRequest{})
	found := false
	for _, key := range changed {
		if key == service.SettingKeyModelConcurrencyLimits {
			found = true
			break
		}
	}
	require.True(t, found, "expected model_concurrency_limits change detection, got %v", changed)
}
