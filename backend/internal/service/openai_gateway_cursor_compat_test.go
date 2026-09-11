package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCursorSandboxFixture spins up a fake Cursor backend (EnsureSandBox) plus a
// fake sandbox gateway serving the given transcript entries.
func newCursorSandboxFixture(t *testing.T, replyText string) *httptest.Server {
	t.Helper()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/setHostSettings":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case "/api/listAgents":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case "/api/createAgent":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"agent":{"id":"agent-1"}}`))
		case "/api/sendPrompt":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"accepted":true}`))
		case "/api/getAgentTranscriptTail":
			w.WriteHeader(http.StatusOK)
			ts := time.Now().UnixMilli()
			_, _ = w.Write([]byte(fmt.Sprintf(
				`{"entries":[{"id":"e1","kind":"send-message","timestampMs":%d,"message":{"type":"text","content":%q}}]}`,
				ts, replyText,
			)))
		case "/events":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			ts := time.Now().UnixMilli()
			payload := fmt.Sprintf(
				`{"channel":"transcript","payload":{"agentId":"agent-1","entries":[{"id":"e1","kind":"send-message","timestampMs":%d,"message":{"type":"text","content":%q}}]}}`,
				ts, replyText,
			)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(gateway.Close)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "EnsureSandBox") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(
				`{"gatewayUrl":%q,"gatewayToken":"gw-tok","networkToken":"net-tok"}`,
				gateway.URL,
			)))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(backend.Close)
	return backend
}

func cursorSandboxAccount(backendURL string) *Account {
	return &Account{
		ID:       90,
		Platform: PlatformCursor,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "sandbox-token",
			"base_url":     backendURL,
		},
	}
}

func TestForwardCursorResponses_NonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	backend := newCursorSandboxFixture(t, "pong from responses")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"cursor-grok-4.6-low","input":"say pong","stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	result, err := svc.Forward(context.Background(), c, cursorSandboxAccount(backend.URL), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Stream)
	assert.Equal(t, "cursor-grok-4.6-low", result.Model)
	assert.Equal(t, "sand-gateway/sendPrompt", result.UpstreamEndpoint)
	assert.Greater(t, result.Usage.InputTokens, 0)
	assert.Greater(t, result.Usage.OutputTokens, 0)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "response", resp["object"])
	assert.True(t, strings.HasPrefix(resp["id"].(string), "resp_"))
	assert.Equal(t, "completed", resp["status"])
	assert.Equal(t, "cursor-grok-4.6-low", resp["model"])
	output := resp["output"].([]any)
	require.Len(t, output, 1)
	item := output[0].(map[string]any)
	assert.Equal(t, "message", item["type"])
	parts := item["content"].([]any)
	require.Len(t, parts, 1)
	assert.Equal(t, "output_text", parts[0].(map[string]any)["type"])
	assert.Equal(t, "pong from responses", parts[0].(map[string]any)["text"])
}

func TestForwardCursorResponses_Streaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	backend := newCursorSandboxFixture(t, "stream-pong")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"cursor-grok-4.6-low","input":"hi","stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	result, err := svc.Forward(context.Background(), c, cursorSandboxAccount(backend.URL), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Stream)

	out := rec.Body.String()
	assert.Contains(t, out, "event: response.created")
	assert.Contains(t, out, "event: response.output_text.delta")
	assert.Contains(t, out, "event: response.completed")
	assert.Contains(t, out, "stream-pong")
}

func TestForwardCursorAnthropic_NonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	backend := newCursorSandboxFixture(t, "anthropic pong")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"cursor-grok-4.6-low","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"say pong"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	result, err := svc.ForwardAsAnthropic(context.Background(), c, cursorSandboxAccount(backend.URL), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Stream)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "message", resp["type"])
	assert.Equal(t, "assistant", resp["role"])
	blocks := resp["content"].([]any)
	require.Len(t, blocks, 1)
	assert.Equal(t, "text", blocks[0].(map[string]any)["type"])
	assert.Equal(t, "anthropic pong", blocks[0].(map[string]any)["text"])
}

func TestForwardCursorAnthropic_Streaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	backend := newCursorSandboxFixture(t, "anthropic stream pong")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"cursor-grok-4.6-low","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	result, err := svc.ForwardAsAnthropic(context.Background(), c, cursorSandboxAccount(backend.URL), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Stream)

	out := rec.Body.String()
	assert.Contains(t, out, "event: message_start")
	assert.Contains(t, out, "anthropic stream pong")
	assert.Contains(t, out, "event: message_stop")
}

// Live e2e probe: exercises forwardCursorResponses against the real Cursor
// sandbox gateway using a real account token.
//
//	CURSOR_LIVE_TOKEN=... CURSOR_LIVE_MACHINE_ID=... go test ./internal/service -run TestLiveCursorResponses -v
func TestLiveCursorResponsesProbe(t *testing.T) {
	token := strings.TrimSpace(getenvCursorLive(t, "CURSOR_LIVE_TOKEN"))
	if token == "" {
		t.Skip("CURSOR_LIVE_TOKEN not set")
	}
	machineID := strings.TrimSpace(getenvCursorLive(t, "CURSOR_LIVE_MACHINE_ID"))
	gin.SetMode(gin.TestMode)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{
		ID:       9001,
		Platform: PlatformCursor,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": token,
			"machine_id":   machineID,
		},
	}

	t.Run("stream", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body := []byte(`{"model":"cursor-grok-4.6-low","input":"Reply with exactly: pong","stream":true}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		result, err := svc.Forward(context.Background(), c, account, body)
		require.NoError(t, err)
		require.NotNil(t, result)
		out := rec.Body.String()
		t.Logf("events: %s", out[:min(len(out), 2000)])
		assert.Contains(t, out, "response.completed")
	})

	t.Run("anthropic", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body := []byte(`{"model":"cursor-grok-4.6-low","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"Reply with exactly: pong"}]}`)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
		require.NoError(t, err)
		require.NotNil(t, result)
		t.Logf("anthropic body: %s", rec.Body.String()[:min(len(rec.Body.String()), 1000)])
	})
}

func getenvCursorLive(t *testing.T, key string) string {
	t.Helper()
	return strings.TrimSpace(os.Getenv(key))
}
