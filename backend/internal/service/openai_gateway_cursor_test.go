package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	cursorpb "github.com/Wei-Shaw/sub2api/internal/pkg/cursor/proto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCursorTokenProvider struct {
	token        string
	refreshCount int32
}

func (m *mockCursorTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	return m.token, nil
}

func (m *mockCursorTokenProvider) ForceRefresh(ctx context.Context, account *Account) (string, error) {
	atomic.AddInt32(&m.refreshCount, 1)
	m.token = "refreshed-token-xyz"
	return m.token, nil
}

func TestForwardCursorChatCompletions_Streaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/aiserver.v1.InferenceService/Stream", r.URL.Path)
		assert.Equal(t, "application/connect+proto", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("x-cursor-checksum"))
		assert.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)

		// Frame 1: Thinking
		f1, _ := cursor.EncodeEnvelope(&cursorpb.InferenceStreamResponse{
			Response: &cursorpb.InferenceStreamResponse_ThinkingPart{
				ThinkingPart: &cursorpb.InferenceThinkingStreamPart{Text: "Thinking deeply..."},
			},
		})
		_, _ = w.Write(f1)

		// Frame 2: Text
		f2, _ := cursor.EncodeEnvelope(&cursorpb.InferenceStreamResponse{
			Response: &cursorpb.InferenceStreamResponse_TextPart{
				TextPart: &cursorpb.InferenceTextStreamPart{Text: "Hello world!"},
			},
		})
		_, _ = w.Write(f2)

		// Frame 3: Extended Usage
		f3, _ := cursor.EncodeEnvelope(&cursorpb.InferenceStreamResponse{
			Response: &cursorpb.InferenceStreamResponse_ExtendedUsage{
				ExtendedUsage: &cursorpb.InferenceExtendedUsageInfo{
					InputTokens:      12,
					OutputTokens:     8,
					CacheReadTokens:  4,
					CacheWriteTokens: 2,
				},
			},
		})
		_, _ = w.Write(f3)

		// Frame 4: EndStream
		endBuf := []byte{cursor.ConnectFlagEndStream, 0, 0, 0, 0}
		_, _ = w.Write(endBuf)
	}))
	defer ts.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"grok-4.5","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:          42,
		Name:        "cursor-account",
		Platform:    PlatformCursor,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":     "test-access-token",
			"machine_id":       "mach-42",
			"base_url":         ts.URL,
			"cursor_transport": "inference",
		},
	}

	svc := &OpenAIGatewayService{
		cfg: &config.Config{},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "grok-4.5")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "grok-4.5", result.Model)
	assert.Equal(t, 12, result.Usage.InputTokens)
	assert.Equal(t, 8, result.Usage.OutputTokens)
	assert.Equal(t, 4, result.Usage.CacheReadInputTokens)
	assert.Equal(t, 2, result.Usage.CacheCreationInputTokens)
	assert.True(t, result.Stream)

	bodyStr := rec.Body.String()
	assert.Contains(t, bodyStr, "Thinking deeply...")
	assert.Contains(t, bodyStr, "Hello world!")
	assert.Contains(t, bodyStr, "data: [DONE]")
}

func TestForwardCursorChatCompletions_NonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)

		// Frame 1: Text
		f1, _ := cursor.EncodeEnvelope(&cursorpb.InferenceStreamResponse{
			Response: &cursorpb.InferenceStreamResponse_TextPart{
				TextPart: &cursorpb.InferenceTextStreamPart{Text: "Non-streaming reply"},
			},
		})
		_, _ = w.Write(f1)

		// Frame 2: Extended Usage
		f2, _ := cursor.EncodeEnvelope(&cursorpb.InferenceStreamResponse{
			Response: &cursorpb.InferenceStreamResponse_ExtendedUsage{
				ExtendedUsage: &cursorpb.InferenceExtendedUsageInfo{
					InputTokens:  15,
					OutputTokens: 6,
				},
			},
		})
		_, _ = w.Write(f2)

		// Frame 3: EndStream
		endBuf := []byte{cursor.ConnectFlagEndStream, 0, 0, 0, 0}
		_, _ = w.Write(endBuf)
	}))
	defer ts.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"grok-4.5","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:          43,
		Name:        "cursor-account",
		Platform:    PlatformCursor,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":     "test-access-token",
			"base_url":         ts.URL,
			"cursor_transport": "inference",
		},
	}

	svc := &OpenAIGatewayService{
		cfg: &config.Config{},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "grok-4.5")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 15, result.Usage.InputTokens)
	assert.Equal(t, 6, result.Usage.OutputTokens)
	assert.False(t, result.Stream)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "chat.completion", resp["object"])
	choices := resp["choices"].([]any)
	require.Len(t, choices, 1)
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	assert.Equal(t, "Non-streaming reply", msg["content"])
	assert.Equal(t, "stop", choice["finish_reason"])
}

func TestForwardCursorChatCompletions_429Failover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer ts.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"grok-4.5","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	account := &Account{
		ID:          44,
		Platform:    PlatformCursor,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":     "test-token",
			"base_url":         ts.URL,
			"cursor_transport": "inference",
		},
	}

	svc := &OpenAIGatewayService{
		cfg: &config.Config{},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "grok-4.5")
	require.Error(t, err)
	require.Nil(t, result)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	assert.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
}

func TestForwardCursorChatCompletions_401TokenRefresh(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var requestCount int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count == 1 {
			// First attempt returns 401
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		// Second attempt with refreshed token succeeds
		assert.Equal(t, "Bearer refreshed-token-xyz", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)

		f1, _ := cursor.EncodeEnvelope(&cursorpb.InferenceStreamResponse{
			Response: &cursorpb.InferenceStreamResponse_TextPart{
				TextPart: &cursorpb.InferenceTextStreamPart{Text: "Success after refresh"},
			},
		})
		_, _ = w.Write(f1)

		endBuf := []byte{cursor.ConnectFlagEndStream, 0, 0, 0, 0}
		_, _ = w.Write(endBuf)
	}))
	defer ts.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"grok-4.5","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	account := &Account{
		ID:          45,
		Platform:    PlatformCursor,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":     "expired-token",
			"base_url":         ts.URL,
			"cursor_transport": "inference",
		},
	}

	tokenProv := &mockCursorTokenProvider{token: "expired-token"}
	svc := &OpenAIGatewayService{
		cfg:                 &config.Config{},
		cursorTokenProvider: tokenProv,
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "grok-4.5")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(1), atomic.LoadInt32(&tokenProv.refreshCount))
	assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
}

func TestForwardCursorChatCompletions_GatewayPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var ensureCount, sendCount, tailCount int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "EnsureSandBox"):
			atomic.AddInt32(&ensureCount, 1)
			assert.Equal(t, "0.44.0", r.Header.Get("x-cursor-client-version"))
			assert.NotEmpty(t, r.Header.Get("x-cursor-checksum"))
			gw := r.Context().Value("gwURL")
			_ = gw
			w.Header().Set("Content-Type", "application/json")
			// gateway URL injected via outer closure below — placeholder replaced after server start
			_, _ = w.Write([]byte(`{"gatewayUrl":"GW_PLACEHOLDER","gatewayToken":"gw-tok","networkToken":"net-tok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	var gwURL string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer gw-tok", r.Header.Get("Authorization"))
		assert.Equal(t, "net-tok", r.Header.Get("x-anyrun-network-token"))
		switch {
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`ok`))
		case r.URL.Path == "/api/setHostSettings":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/api/listAgents":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/api/createAgent":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"agent":{"id":"agent-1","name":"chat-test"}}`))
		case r.URL.Path == "/api/sendPrompt":
			atomic.AddInt32(&sendCount, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"accepted":true}`))
		case r.URL.Path == "/api/getAgentTranscriptTail":
			n := atomic.AddInt32(&tailCount, 1)
			w.WriteHeader(http.StatusOK)
			if n >= 2 {
				ts := time.Now().UnixMilli()
				_, _ = w.Write([]byte(fmt.Sprintf(
					`{"entries":[{"id":"e1","kind":"send-message","timestampMs":%d,"message":{"type":"text","content":"PONG from gateway"}}]}`,
					ts,
				)))
				return
			}
			_, _ = w.Write([]byte(`{"entries":[]}`))
		case r.URL.Path == "/events":
			// SSE not used in this non-stream test; still accept
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			_, _ = fmt.Fprintf(w, "data: {\"channel\":\"agents\",\"payload\":{}}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()
	gwURL = gateway.URL

	// Rebuild backend to inject real gateway URL
	backend.Close()
	backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "EnsureSandBox") {
			atomic.AddInt32(&ensureCount, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(
				`{"gatewayUrl":%q,"gatewayToken":"gw-tok","networkToken":"net-tok"}`,
				gwURL,
			)))
			return
		}
		http.NotFound(w, r)
	}))
	defer backend.Close()

	// Reset counters after rebuild
	atomic.StoreInt32(&ensureCount, 0)
	atomic.StoreInt32(&sendCount, 0)
	atomic.StoreInt32(&tailCount, 0)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"grok-4.6","stream":false,"messages":[{"role":"user","content":"ping"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:       50,
		Platform: PlatformCursor,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "sandbox-token",
			"machine_id":   "mach-gw",
			"base_url":     backend.URL,
		},
	}

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "grok-4.6")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Stream)
	assert.Equal(t, "sand-gateway/sendPrompt", result.UpstreamEndpoint)
	assert.Greater(t, result.Usage.InputTokens, 0)
	assert.Greater(t, result.Usage.OutputTokens, 0)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&sendCount), int32(1))
	assert.GreaterOrEqual(t, atomic.LoadInt32(&ensureCount), int32(1))

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	choices := resp["choices"].([]any)
	require.Len(t, choices, 1)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	assert.Equal(t, "PONG from gateway", msg["content"])
}

func TestForwardCursorChatCompletions_GatewayStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gwURL string
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
			_, _ = w.Write([]byte(`{"id":"agent-stream-1"}`))
		case "/api/sendPrompt":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"accepted":true}`))
		case "/api/getAgentTranscriptTail":
			// poll fallback if SSE not used for completion
			w.WriteHeader(http.StatusOK)
			ts := time.Now().UnixMilli()
			_, _ = w.Write([]byte(fmt.Sprintf(
				`{"entries":[{"id":"e1","kind":"send-message","timestampMs":%d,"message":{"type":"text","content":"stream-ok"}}]}`,
				ts,
			)))
		case "/events":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			ts := time.Now().UnixMilli()
			payload := fmt.Sprintf(
				`{"channel":"transcript","payload":{"agentId":"agent-stream-1","entries":[{"id":"e1","kind":"send-message","timestampMs":%d,"message":{"type":"text","content":"Hello "}},{"id":"e2","kind":"send-message","timestampMs":%d,"message":{"type":"text","content":"Hello SSE"}}]}}`,
				ts, ts,
			)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			if ok {
				flusher.Flush()
			}
			// keep connection briefly
			time.Sleep(50 * time.Millisecond)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()
	gwURL = gateway.URL

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "EnsureSandBox") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(
				`{"gatewayUrl":%q,"gatewayToken":"gw-tok","networkToken":"net-tok"}`, gwURL,
			)))
			return
		}
		http.NotFound(w, r)
	}))
	defer backend.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"grok-4.6","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:       51,
		Platform: PlatformCursor,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "sandbox-token",
			"base_url":     backend.URL,
		},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "grok-4.6")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Stream)
	assert.Contains(t, rec.Body.String(), "data: [DONE]")
	assert.True(t,
		strings.Contains(rec.Body.String(), "Hello") || strings.Contains(rec.Body.String(), "stream-ok"),
		"expected streamed content, got: %s", rec.Body.String(),
	)
}
