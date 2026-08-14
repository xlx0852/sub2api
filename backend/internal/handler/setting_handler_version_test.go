package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingHandler_GetPublicVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SettingHandler{version: "0.1.319"}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/version", nil)

	h.GetPublicVersion(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Version            string `json:"version"`
			MinFrontendVersion string `json:"min_frontend_version"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "0.1.319", resp.Data.Version)
	require.Equal(t, "0.1.319", resp.Data.MinFrontendVersion)
}
