package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

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
			"access_token": "test-access-token",
			"machine_id":   "mach-42",
			"base_url":     ts.URL,
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
			"access_token": "test-access-token",
			"base_url":     ts.URL,
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
		Credentials: map[string]any{"access_token": "test-token", "base_url": ts.URL},
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
		Credentials: map[string]any{"access_token": "expired-token", "base_url": ts.URL},
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
