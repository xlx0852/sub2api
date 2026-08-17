package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVertexServiceAccountCreationBindingIsRetired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", bytes.NewBufferString(
		`{"name":"vertex","platform":"anthropic","type":"service_account","credentials":{"service_account_json":"not-written"}}`,
	))
	ctx.Request.Header.Set("Content-Type", "application/json")

	var request CreateAccountRequest
	require.Error(t, ctx.ShouldBindJSON(&request))
}

func TestVertexServiceAccountBatchCreationFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	adminSvc.createAccountErr = service.ErrVertexServiceAccountOnboardingDisabled
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/accounts/batch", handler.BatchCreate)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch", bytes.NewBufferString(
		`{"accounts":[{"name":"vertex","platform":"gemini","type":"service_account","credentials":{"service_account_json":"not-written"}}]}`,
	))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "vertex service account onboarding has been retired")
	require.Contains(t, recorder.Body.String(), `"failed":1`)
}
