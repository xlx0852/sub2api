package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func newCompactBodySignalTestContext(t *testing.T, path string, body []byte) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

// Body-signal v2 must keep the client /responses streaming contract.
// Upstream still uses /responses/compact via force-suffix + SSE bridge.
func TestNormalizeOpenAIResponsesCompactRequest_BodySignalV2KeepsClientPath(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{
		"model":"gpt-5.5",
		"stream":true,
		"store":true,
		"prompt_cache_key":"pck-signal-1",
		"input":[
			{"type":"message","role":"user","content":"hello"},
			{"type":"compaction_trigger"}
		]
	}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses", body)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)

	require.Equal(t, "/v1/responses", c.Request.URL.Path)
	require.False(t, isOpenAIRemoteCompactPath(c))
	require.Equal(t, service.OpenAICompactModeBodySignalV2, service.OpenAICompactMode(c))
	require.True(t, service.IsOpenAICompactRequest(c))
	require.True(t, service.IsOpenAIBodySignalCompactV2(c))

	// Client body keeps stream + trigger; upstream path is forced separately.
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
	require.True(t, gjson.GetBytes(normalized, "prompt_cache_key").Exists())
	require.True(t, service.HasCompactionTriggerInInput(normalized))

	reqStream, streamOK := parseOpenAICompatibleStream(normalized)
	require.True(t, streamOK)
	require.True(t, reqStream)

	// Upstream suffix forced to legacy compact.
	require.True(t, service.IsOpenAIResponsesCompactPathForTest(c))

	seed, exists := c.Get(service.OpenAICompactSessionSeedKeyForTest())
	require.True(t, exists)
	require.Equal(t, "pck-signal-1", seed)
}

func TestNormalizeOpenAIResponsesCompactRequest_BodySignalTrailingSlash(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses/", body)

	_, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses/", c.Request.URL.Path)
	require.Equal(t, service.OpenAICompactModeBodySignalV2, service.OpenAICompactMode(c))
}

func TestNormalizeOpenAIResponsesCompactRequest_CodexDirectAliasBodySignal(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := newCompactBodySignalTestContext(t, "/backend-api/codex/responses", body)

	_, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/backend-api/codex/responses", c.Request.URL.Path)
	require.Equal(t, service.OpenAICompactModeBodySignalV2, service.OpenAICompactMode(c))
}

func TestNormalizeOpenAIResponsesCompactRequest_NoTriggerUntouched(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"message","role":"user","content":"hello"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses", body)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses", c.Request.URL.Path)
	require.Equal(t, service.OpenAICompactModeNone, service.OpenAICompactMode(c))
	require.False(t, service.IsOpenAICompactRequest(c))
	require.Equal(t, body, normalized)
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
}

func TestNormalizeOpenAIResponsesCompactRequest_PathBasedLegacyJSON(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","stream":true,"store":true,"input":[{"type":"message","role":"user","content":"hello"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses/compact", body)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses/compact", c.Request.URL.Path)
	require.Equal(t, service.OpenAICompactModeLegacyPath, service.OpenAICompactMode(c))
	require.False(t, gjson.GetBytes(normalized, "stream").Exists())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
}

func TestNormalizeOpenAIResponsesCompactRequest_SubpathNotPromoted(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses/resp_123/cancel", body)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses/resp_123/cancel", c.Request.URL.Path)
	require.Equal(t, body, normalized)
	require.Equal(t, service.OpenAICompactModeNone, service.OpenAICompactMode(c))
}

// 回归 #3875：body-signal 原始请求 stream:true 时必须标记 client-stream，
// 供响应写回阶段把上游 unary JSON 合成回 Codex remote compact v2 所需的 SSE。
func TestNormalizeOpenAIResponsesCompactRequest_BodySignalStreamTrueMarksClientStream(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"compaction_trigger"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses", body)

	_, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)

	marked, exists := c.Get(service.OpenAICompactClientStreamKeyForTest())
	require.True(t, exists)
	require.Equal(t, true, marked)
}

func TestNormalizeOpenAIResponsesCompactRequest_BodySignalStreamFalseNotMarked(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	for name, body := range map[string][]byte{
		"stream_false":  []byte(`{"model":"gpt-5.5","stream":false,"input":[{"type":"compaction_trigger"}]}`),
		"stream_absent": []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`),
	} {
		c := newCompactBodySignalTestContext(t, "/v1/responses", body)
		_, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
		require.True(t, ok, name)
		require.Equal(t, "/v1/responses/compact", c.Request.URL.Path, name)
		_, exists := c.Get(service.OpenAICompactClientStreamKeyForTest())
		require.False(t, exists, "case %s 不应标记 client-stream", name)
	}
}

// path-based compact（Codex v1 unary 协议）即使 body 带 stream:true 也不标记，
// 保持 JSON 写回行为不变。
func TestNormalizeOpenAIResponsesCompactRequest_PathBasedStreamTrueNotMarked(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"message","role":"user","content":"hello"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses/compact", body)

	_, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	_, exists := c.Get(service.OpenAICompactClientStreamKeyForTest())
	require.False(t, exists)
}
