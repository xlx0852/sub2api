//go:build unit

package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMarkOpenAIStreamTerminalError_InvalidPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		LogUpstreamErrorBody:         true,
		LogUpstreamErrorBodyMaxBytes: 2048,
	}}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	payload := []byte(`{"type":"response.failed","response":{"instructions":"private customer prompt","output":[{"text":"private model output"}],"error":{"type":"invalid_request_error","code":"invalid_prompt","message":"Invalid prompt"}}}`)
	account := &Account{ID: 97, Name: "openai-97", Platform: PlatformOpenAI}

	svc.markOpenAIStreamTerminalError(c, account, false, "resp_123", payload, "Invalid prompt")

	streamErr, ok := GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, "invalid_request_error", streamErr.ErrType)
	require.Equal(t, http.StatusBadRequest, streamErr.IntendedStatus)
	require.Equal(t, "Invalid prompt", streamErr.Message)

	status, ok := c.Get(OpsUpstreamStatusCodeKey)
	require.True(t, ok)
	require.Equal(t, http.StatusBadRequest, status)
	eventsValue, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := eventsValue.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, int64(97), events[0].AccountID)
	require.Equal(t, "terminal_error", events[0].Kind)
	require.Contains(t, events[0].Detail, "invalid_prompt")
	require.NotContains(t, events[0].Detail, "private customer prompt")
	require.NotContains(t, events[0].Detail, "private model output")
}

func TestMarkOpenAIStreamTerminalError_DeduplicatesErrorAndResponseFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		LogUpstreamErrorBody:         true,
		LogUpstreamErrorBodyMaxBytes: 2048,
	}}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	account := &Account{ID: 97, Name: "openai-97", Platform: PlatformOpenAI}
	message := "Our servers are currently overloaded. Please try again later."

	svc.markOpenAIStreamTerminalError(c, account, false, "resp_123", []byte(
		`{"type":"error","sequence_number":2,"error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`,
	), message)
	svc.markOpenAIStreamTerminalError(c, account, false, "resp_123", []byte(
		`{"type":"response.failed","response":{"id":"resp_123","status":"failed","instructions":"private customer prompt","error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`,
	), message)

	eventsValue, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := eventsValue.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Contains(t, events[0].Detail, `"event_type":"response.failed"`)
	require.Contains(t, events[0].Detail, `"error_code":"server_is_overloaded"`)
	require.NotContains(t, events[0].Detail, "private customer prompt")
}

func TestMarkOpenAIStreamTerminalError_CyberUsesDedicatedSink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"cyber_policy","message":"blocked"}}}`)

	svc.markOpenAIStreamTerminalError(c, &Account{ID: 1}, false, "resp_1", payload, "blocked")

	_, ok := GetOpsStreamError(c)
	require.False(t, ok)
	_, ok = c.Get(OpsUpstreamErrorsKey)
	require.False(t, ok)
}

func TestClearOpsStreamError_AllowsNextWebSocketTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	MarkOpsStreamError(c, "upstream_error", "first", http.StatusBadGateway)
	ClearOpsStreamError(c)
	MarkOpsStreamError(c, "invalid_request_error", "second", http.StatusBadRequest)

	streamErr, ok := GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, "second", streamErr.Message)
}
