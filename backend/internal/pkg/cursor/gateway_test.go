package cursor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatMessagesForGateway_ToolsAndRoles(t *testing.T) {
	tools := []OpenAITool{{
		Type: "function",
		Function: OpenAIFunction{
			Name:        "lookup",
			Description: "look something up",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		},
	}}
	msgs := []OpenAIMessage{
		{Role: "system", Content: json.RawMessage(`"be brief"`)},
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{
			Role:    "assistant",
			Content: json.RawMessage(`""`),
			ToolCalls: []OpenAIToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: OpenAIFunctionCallDesc{
					Name:      "lookup",
					Arguments: `{"q":"x"}`,
				},
			}},
		},
		{Role: "tool", ToolCallID: "call_1", Content: json.RawMessage(`"result-data"`)},
	}
	out := FormatMessagesForGateway(msgs, tools)
	assert.Contains(t, out, "[AVAILABLE_TOOLS]")
	assert.Contains(t, out, "lookup")
	assert.Contains(t, out, "[SYSTEM]")
	assert.Contains(t, out, "[USER]")
	assert.Contains(t, out, "[ASSISTANT]")
	assert.Contains(t, out, "[TOOL lookup]")
	assert.Contains(t, out, "result-data")
}

func TestEstimateTokens(t *testing.T) {
	assert.Equal(t, 0, EstimateTokens(""))
	assert.Greater(t, EstimateTokens("hello world this is a test"), 0)
	assert.Greater(t, EstimateTokens("你好世界测试中文内容"), 0)
}

func TestParseTranscriptEntries_SendMessageAndToolCall(t *testing.T) {
	raw := json.RawMessage(`{
		"entries":[
			{"id":"1","kind":"send-message","timestampMs":100,"message":{"type":"text","content":"hi"}},
			{"id":"2","kind":"tool-call","timestampMs":101,"name":"lookup","toolCallId":"c1","args":{"q":1},"status":"done"}
		]
	}`)
	entries := parseTranscriptEntries(raw)
	require.Len(t, entries, 2)
	assert.Equal(t, "send-message", entries[0].Kind)
	assert.Equal(t, "hi", extractSendMessageText(entries[0]))
	assert.Equal(t, "tool-call", entries[1].Kind)
	assert.Equal(t, "lookup", entries[1].Name)

	agg := aggregateTranscriptSince(entries, time.UnixMilli(0))
	assert.Equal(t, "hi", agg.Text)
	require.Len(t, agg.ToolCalls, 1)
	assert.Equal(t, "lookup", agg.ToolCalls[0].Function.Name)
}

func TestDiffTranscript_IncrementalText(t *testing.T) {
	start := time.Now().Add(-time.Second)
	seen := ""
	tools := map[string]string{}

	e1 := []TranscriptEntry{{
		ID: "1", Kind: "send-message", TimestampMs: time.Now().UnixMilli(), IsStreaming: true,
		Message: &TranscriptMsg{Type: "text", Content: "Hel"},
	}}
	// streaming incomplete
	d1, done1 := diffTranscript(e1, start, &seen, tools)
	assert.False(t, done1)
	if len(d1) > 0 {
		assert.Equal(t, "Hel", d1[0].Text)
	}

	e2 := []TranscriptEntry{{
		ID: "1", Kind: "send-message", TimestampMs: time.Now().UnixMilli(), IsStreaming: false,
		Message: &TranscriptMsg{Type: "text", Content: "Hello"},
	}}
	d2, done2 := diffTranscript(e2, start, &seen, tools)
	assert.True(t, done2)
	require.NotEmpty(t, d2)
	// delta should be remaining suffix "lo" if "Hel" was seen, or full "Hello"
	joined := ""
	for _, d := range d2 {
		joined += d.Text
	}
	assert.True(t, strings.Contains(joined, "lo") || joined == "Hello" || seen == "Hello")
}

func TestEnsureSandBoxAndGatewayChat(t *testing.T) {
	var ensureHits, sendHits int32
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
			_, _ = w.Write([]byte(`{"agent":{"id":"a1"}}`))
		case "/api/sendPrompt":
			atomic.AddInt32(&sendHits, 1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			assert.Equal(t, "a1", body["agentId"])
			assert.NotEmpty(t, body["prompt"])
			assert.Contains(t, body["prompt"], "[USER]")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"accepted":true}`))
		case "/api/getAgentTranscriptTail":
			w.WriteHeader(http.StatusOK)
			ts := time.Now().UnixMilli()
			_, _ = fmt.Fprintf(w, `{"entries":[{"id":"e","kind":"send-message","timestampMs":%d,"message":{"type":"text","content":"OK"}}]}`, ts)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()
	gwURL = gateway.URL

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/aiserver.v1.GrokBotService/EnsureSandBox", r.URL.Path)
		assert.Equal(t, GatewayClientVersion, r.Header.Get(HeaderCursorClientVersion))
		atomic.AddInt32(&ensureHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"gatewayUrl":%q,"gatewayToken":"t","networkToken":"n"}`, gwURL)
	}))
	defer backend.Close()

	client := NewClient("tok", "mach", backend.Client(), backend.URL)
	// Use gateway's client for gateway calls — same transport is fine for httptest
	client.httpClient = http.DefaultClient

	sess, err := client.EnsureHealthySandbox(context.Background())
	require.NoError(t, err)
	require.Equal(t, gwURL, sess.GatewayURL)
	assert.Equal(t, int32(1), atomic.LoadInt32(&ensureHits))

	// Cached second call
	sess2, err := client.EnsureSandBox(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, sess.GatewayToken, sess2.GatewayToken)
	assert.Equal(t, int32(1), atomic.LoadInt32(&ensureHits), "should use cache")

	result, err := client.ChatViaGatewaySync(context.Background(), &GatewayChatRequest{
		Messages: []OpenAIMessage{{Role: "user", Content: json.RawMessage(`"ping"`)}},
		Model:    "grok-4.6",
		Stream:   false,
		Timeout:  5 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, "OK", result.Text)
	assert.Equal(t, "a1", result.AgentID)
	assert.Greater(t, result.Usage.InputTokens, 0)
	assert.Greater(t, result.Usage.OutputTokens, 0)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&sendHits), int32(1))
}

func TestGatewayEventStream_ParseSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/events", r.URL.Path)
		assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "data: {\"channel\":\"transcript\",\"payload\":{\"entries\":[]}}\n\n")
		_, _ = fmt.Fprintf(w, "data: {\"channel\":\"agents\",\"payload\":{}}\n\n")
	}))
	defer srv.Close()

	client := NewClient("t", "m", srv.Client(), srv.URL)
	sess := &SandboxSession{GatewayURL: srv.URL, GatewayToken: "gt"}
	es, err := client.OpenGatewayEvents(context.Background(), sess)
	require.NoError(t, err)
	defer es.Close()

	ev1, err := es.Recv()
	require.NoError(t, err)
	assert.Equal(t, "transcript", ev1.Channel)

	ev2, err := es.Recv()
	require.NoError(t, err)
	assert.Equal(t, "agents", ev2.Channel)
}

func TestBindAgentDefaultModel_Body(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/setHostSettings", r.URL.Path)
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := NewClient("t", "m", srv.Client(), srv.URL)
	sess := &SandboxSession{GatewayURL: srv.URL, GatewayToken: "gt"}
	err := client.BindAgentDefaultModel(context.Background(), sess, "grok-4.5")
	require.NoError(t, err)
	model := gotBody["agentDefaultModel"].(map[string]any)
	assert.Equal(t, "grok-4.5", model["modelId"])
	assert.Equal(t, true, model["maxMode"])
}

func TestCheckGatewayHealth_RetryPath(t *testing.T) {
	var healthHits int32
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			n := atomic.AddInt32(&healthHits, 1)
			if n == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/api/listAgents" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer gw.Close()

	var ensureHits int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&ensureHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"gatewayUrl":%q,"gatewayToken":"t","networkToken":"n"}`, gw.URL)
	}))
	defer backend.Close()

	client := NewClient("tok-health", "m", http.DefaultClient, backend.URL)
	InvalidateSandboxCache("tok-health")
	sess, err := client.EnsureHealthySandbox(context.Background())
	require.NoError(t, err)
	require.NotNil(t, sess)
	// First health fail → invalidate → ensure again → health or listAgents ok
	assert.GreaterOrEqual(t, atomic.LoadInt32(&ensureHits), int32(1))
}

func TestLiveGatewayChat(t *testing.T) {
	if os.Getenv("CURSOR_LIVE_GATEWAY") == "" {
		t.Skip("set CURSOR_LIVE_GATEWAY=1 to run")
	}
	tokPath := os.Getenv("CURSOR_TOKEN_FILE")
	if tokPath == "" {
		// backend/internal/pkg/cursor → repo root
		tokPath = filepath.Join("..", "..", "..", "..", "grok_bot_2api_temp", "local-api", "local-token.json")
	}
	b, err := os.ReadFile(tokPath)
	if err != nil {
		t.Skip("token file missing:", err)
	}
	var tok struct {
		AccessToken string `json:"accessToken"`
		MachineID   string `json:"machineId"`
	}
	require.NoError(t, json.Unmarshal(b, &tok))
	require.NotEmpty(t, tok.AccessToken)

	client := NewClient(tok.AccessToken, tok.MachineID, nil, DefaultBackendURL)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	res, err := client.ChatViaGatewaySync(ctx, &GatewayChatRequest{
		Messages: []OpenAIMessage{{
			Role:    "user",
			Content: json.RawMessage(`"Reply with exactly the word PONG and nothing else."`),
		}},
		Model:   "grok-4.6",
		Timeout: 80 * time.Second,
	})
	require.NoError(t, err)
	t.Logf("reply=%q agent=%s usage=%+v", res.Text, res.AgentID, res.Usage)
	require.NotEmpty(t, res.Text)
}
