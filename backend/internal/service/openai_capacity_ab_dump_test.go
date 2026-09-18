package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClassifyOpenAICapacityABLabel(t *testing.T) {
	t.Parallel()

	capacityBody := []byte(`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"Please retry later."}}}`)
	require.Equal(t, "capacity", classifyOpenAICapacityABLabel(http.StatusOK, "", capacityBody))

	atCapacity := []byte(`{"error":{"message":"Selected model is at capacity. Please try a different model.","type":"invalid_request_error"}}`)
	require.Equal(t, "capacity", classifyOpenAICapacityABLabel(http.StatusBadRequest, "", atCapacity))
	require.Equal(t, "capacity", classifyOpenAICapacityABLabel(http.StatusBadRequest, "Selected model is at capacity. Please try a different model.", nil))

	slowDown := []byte(`{"error":{"code":"slow_down"}}`)
	require.Equal(t, "capacity", classifyOpenAICapacityABLabel(http.StatusOK, "", slowDown))

	require.Equal(t, "ok", classifyOpenAICapacityABLabel(http.StatusOK, "", nil))
	require.Equal(t, "", classifyOpenAICapacityABLabel(http.StatusTooManyRequests, "rate limit exceeded", []byte(`{"error":{"code":"rate_limit_exceeded"}}`)))
	require.Equal(t, "", classifyOpenAICapacityABLabel(http.StatusBadRequest, "instructions are required", []byte(`{"error":{"message":"Instructions are required"}}`)))
	require.Equal(t, "", classifyOpenAICapacityABLabel(http.StatusOK, "upstream response failed", []byte(`{"type":"response.failed","response":{"error":{"code":"server_error"}}}`)))
}

func TestMaybeDumpOpenAICapacityABWritesCapacityAndSamplesOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	t.Setenv(openAICapacityABDumpEnv, dir)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAICapacityABDumpOKEvery = 1
	account := &Account{
		ID:   42,
		Type: AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent": 7.0,
			"codex_7d_used_percent": 20.0,
		},
	}
	headers := http.Header{}
	headers.Set("x-request-id", "req_ab")
	headers.Set("cf-ray", "ray_ab")
	headers.Set("x-codex-primary-used-percent", "20")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "7")
	headers.Set("x-codex-secondary-window-minutes", "300")
	headers.Set("authorization", "Bearer secret-token")
	headers.Set("cookie", "session=secret")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("originator", "codex_cli_rs")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1")

	body := []byte(`{"error":{"message":"Selected model is at capacity. Please try a different model.","code":"server_is_overloaded"}}`)
	svc.maybeDumpOpenAICapacityAB(openAICapacityABDumpInput{
		Entry:      "http_responses",
		HTTPStatus: http.StatusBadRequest,
		Model:      "gpt-5.4",
		Message:    "Selected model is at capacity. Please try a different model.",
		Body:       body,
		Headers:    headers,
		Account:    account,
		Client:     c,
	})
	// Same request must not dump twice.
	svc.maybeDumpOpenAICapacityAB(openAICapacityABDumpInput{
		Label:      "ok",
		Entry:      "http_responses",
		HTTPStatus: http.StatusOK,
		Headers:    headers,
		Account:    account,
		Client:     c,
	})

	capacityFiles, err := filepath.Glob(filepath.Join(dir, "capacity_*.json"))
	require.NoError(t, err)
	require.Len(t, capacityFiles, 1)
	okFiles, err := filepath.Glob(filepath.Join(dir, "ok_*.json"))
	require.NoError(t, err)
	require.Empty(t, okFiles)

	raw, err := os.ReadFile(capacityFiles[0])
	require.NoError(t, err)
	var rec openAICapacityABDumpRecord
	require.NoError(t, json.Unmarshal(raw, &rec))
	require.Equal(t, "capacity", rec.Label)
	require.Equal(t, "http_responses", rec.Entry)
	require.Equal(t, "gpt-5.4", rec.Model)
	require.Equal(t, int64(42), rec.AccountID)
	require.Equal(t, "server_is_overloaded", rec.ErrorCode)
	require.Contains(t, rec.ErrorMessage, "at capacity")
	require.Equal(t, "req_ab", rec.Headers["x-request-id"])
	require.Equal(t, "ray_ab", rec.Headers["cf-ray"])
	require.Equal(t, 20.0, rec.CodexSnapshot["primary_used_percent"])
	require.Equal(t, 7.0, rec.AccountQuota["codex_5h_used_percent"])
	require.NotContains(t, string(raw), "secret-token")
	require.NotContains(t, string(raw), "session=secret")
	require.NotContains(t, rec.Headers, "authorization")
	require.NotContains(t, rec.Headers, "cookie")

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc.maybeDumpOpenAICapacityAB(openAICapacityABDumpInput{
		Label:      "ok",
		Entry:      "http_responses",
		HTTPStatus: http.StatusOK,
		Model:      "gpt-5.4",
		Headers:    headers,
		Account:    account,
		Client:     c2,
		Force:      true,
	})
	okFiles, err = filepath.Glob(filepath.Join(dir, "ok_*.json"))
	require.NoError(t, err)
	require.Len(t, okFiles, 1)
}

func TestMaybeDumpOpenAICapacityABDisabledWithoutDir(t *testing.T) {
	t.Setenv(openAICapacityABDumpEnv, "")
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.maybeDumpOpenAICapacityAB(openAICapacityABDumpInput{
		Label:      "capacity",
		HTTPStatus: http.StatusBadRequest,
		Message:    "Selected model is at capacity. Please try a different model.",
	})
}

func TestOpenAICapacityABBodyExcerptOmitsPrompt(t *testing.T) {
	t.Parallel()
	body := []byte(`{"type":"response.failed","response":{"error":{"code":"slow_down","message":"retry"},"instructions":"SYSTEM SECRET","output":[{"content":[{"text":"user prompt"}]}]}}`)
	excerpt := openAICapacityABBodyExcerpt(body, "retry")
	require.Contains(t, excerpt, "slow_down")
	require.NotContains(t, excerpt, "SYSTEM SECRET")
	require.NotContains(t, excerpt, "user prompt")
}

func TestOpenAICapacityABOKSamplingSkip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(openAICapacityABDumpEnv, dir)
	openAICapacityABDumpOKSeq.Store(0)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAICapacityABDumpOKEvery = 3
	headers := http.Header{}
	headers.Set("x-request-id", "ok-sample")

	for i := 0; i < 2; i++ {
		svc.maybeDumpOpenAICapacityAB(openAICapacityABDumpInput{
			Label:      "ok",
			Entry:      "http_responses",
			HTTPStatus: http.StatusOK,
			Headers:    headers,
		})
	}
	okFiles, err := filepath.Glob(filepath.Join(dir, "ok_*.json"))
	require.NoError(t, err)
	require.Empty(t, okFiles)

	svc.maybeDumpOpenAICapacityAB(openAICapacityABDumpInput{
		Label:      "ok",
		Entry:      "http_responses",
		HTTPStatus: http.StatusOK,
		Headers:    headers,
	})
	okFiles, err = filepath.Glob(filepath.Join(dir, "ok_*.json"))
	require.NoError(t, err)
	require.Len(t, okFiles, 1)
}

func TestOpenAICapacityABDumpUsesGinEntryOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	t.Setenv(openAICapacityABDumpEnv, dir)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(openAICapacityABDumpEntryKey, "ws_v2")

	svc.dumpOpenAICapacityABStream(
		c,
		&Account{ID: 7, Type: AccountTypeOAuth},
		"http_stream",
		http.StatusOK,
		nil,
		[]byte(`{"error":{"code":"server_is_overloaded","message":"Please retry later."}}`),
		"Please retry later.",
		"gpt-5.4",
	)
	files, err := filepath.Glob(filepath.Join(dir, "capacity_*.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	raw, err := os.ReadFile(files[0])
	require.NoError(t, err)
	require.Contains(t, string(raw), `"entry": "ws_v2"`)
	require.NotContains(t, string(raw), `"entry": "http_stream"`)
	require.True(t, strings.Contains(string(raw), "server_is_overloaded"))
}
