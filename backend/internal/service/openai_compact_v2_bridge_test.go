package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareOpenAIBodySignalV2UpstreamBody_StripsStreamAndTrigger(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.5",
		"stream":true,
		"store":true,
		"prompt_cache_key":"pck-1",
		"instructions":"sys",
		"input":[
			{"type":"message","role":"user","content":"hello"},
			{"type":"compaction_trigger"}
		]
	}`)
	got, err := prepareOpenAIBodySignalV2UpstreamBody(body)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.5", gjson.GetBytes(got, "model").String())
	require.Equal(t, "sys", gjson.GetBytes(got, "instructions").String())
	require.False(t, gjson.GetBytes(got, "stream").Exists())
	require.False(t, gjson.GetBytes(got, "store").Exists())
	require.False(t, gjson.GetBytes(got, "prompt_cache_key").Exists())
	require.False(t, HasCompactionTriggerInInput(got))
	require.Equal(t, 1, len(gjson.GetBytes(got, "input").Array()))
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
}

func TestExtractOpenAICompactionItemFromLegacyCompactJSON(t *testing.T) {
	body := []byte(`{
		"id":"resp_compact_1",
		"output":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
			{"type":"compaction","encrypted_content":"ENC_SUMMARY"}
		],
		"usage":{"input_tokens":10,"output_tokens":2}
	}`)
	item, responseID, err := extractOpenAICompactionItemFromLegacyCompactJSON(body)
	require.NoError(t, err)
	require.Equal(t, "resp_compact_1", responseID)
	require.Equal(t, "compaction", gjson.GetBytes(item, "type").String())
	require.Equal(t, "ENC_SUMMARY", gjson.GetBytes(item, "encrypted_content").String())
}

func TestWriteOpenAIBodySignalV2BridgeFromLegacyJSON_EmitsTerminalSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetOpenAICompactMode(c, OpenAICompactModeBodySignalV2)

	svc := &OpenAIGatewayService{}
	body := []byte(`{
		"id":"resp_c1",
		"model":"gpt-5.5",
		"output":[{"type":"compaction","encrypted_content":"SUMMARY_V2"}],
		"usage":{"input_tokens":11,"output_tokens":3}
	}`)
	responseID, usage, err := svc.writeOpenAIBodySignalV2BridgeFromLegacyJSON(c, nil, body, "gpt-5.5")
	require.NoError(t, err)
	require.Equal(t, "resp_c1", responseID)
	require.NotNil(t, usage)
	require.Equal(t, 11, usage.InputTokens)
	require.Equal(t, 3, usage.OutputTokens)
	require.True(t, IsOpenAICompactBridgeUsed(c))
	require.True(t, IsOpenAICompactTerminalCommitted(c))
	require.True(t, IsOpenAICompactV2SSEStarted(c))
	require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")

	raw := rec.Body.String()
	require.Contains(t, raw, `"type":"response.output_item.done"`)
	require.Contains(t, raw, `"type":"compaction"`)
	require.Contains(t, raw, `"encrypted_content":"SUMMARY_V2"`)
	require.Contains(t, raw, `"type":"response.completed"`)
	require.Contains(t, raw, `"id":"resp_c1"`)

	// Ensure event order: output_item.done before completed.
	doneIdx := strings.Index(raw, `"type":"response.output_item.done"`)
	completedIdx := strings.Index(raw, `"type":"response.completed"`)
	require.GreaterOrEqual(t, doneIdx, 0)
	require.Greater(t, completedIdx, doneIdx)
}

func TestHandleOpenAIBodySignalV2LegacyCompactResponse_KeepaliveThenBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetOpenAICompactMode(c, OpenAICompactModeBodySignalV2)

	svc := &OpenAIGatewayService{}

	payload := `{"id":"resp_bridge","output":[{"type":"compaction","encrypted_content":"S"}],"usage":{"input_tokens":1,"output_tokens":1}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-bridge"}},
		Body:       io.NopCloser(strings.NewReader(payload)),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	responseID, usage, err := svc.handleOpenAIBodySignalV2LegacyCompactResponse(ctx, c, resp, "gpt-5.5")
	require.NoError(t, err)
	require.Equal(t, "resp_bridge", responseID)
	require.NotNil(t, usage)
	require.True(t, IsOpenAICompactBridgeUsed(c))
	require.True(t, IsOpenAICompactTerminalCommitted(c))
	require.Contains(t, rec.Body.String(), `"type":"response.completed"`)
}

func TestCanRetryOpenAICompactAfterForwardError_KeepaliveDoesNotBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetOpenAICompactMode(c, OpenAICompactModeBodySignalV2)
	MarkOpenAICompactV2SSEStarted(c)
	_, _ = rec.WriteString("data: {\"type\":\"keepalive\"}\n\n")

	require.True(t, CanRetryOpenAICompactAfterForwardError(c, 0))
	MarkOpenAICompactTerminalCommitted(c)
	require.False(t, CanRetryOpenAICompactAfterForwardError(c, 0))
}

func TestBuildOpenAICompactV2CompletedEvent(t *testing.T) {
	evt := buildOpenAICompactV2CompletedEvent("resp_x", "gpt-5.5", &OpenAIUsage{InputTokens: 2, OutputTokens: 4})
	raw, err := json.Marshal(evt)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(raw, "type").String())
	require.Equal(t, "resp_x", gjson.GetBytes(raw, "response.id").String())
	require.Equal(t, int64(6), gjson.GetBytes(raw, "response.usage.total_tokens").Int())
}

func TestWriteOpenAICompactV2Keepalive_SendsDataEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	err := writeOpenAICompactV2Keepalive(c)
	require.NoError(t, err)

	raw := rec.Body.String()
	// Must be a data frame (not a comment frame) so Codex's eventsource parser
	// surfaces it as an event and resets the stream_idle_timeout timer.
	require.Contains(t, raw, "data: ")
	require.NotContains(t, raw, ": compact-keepalive")
	require.Equal(t, "keepalive", gjson.Get(strings.TrimPrefix(strings.TrimSpace(raw), "data: "), "type").String())
}

func TestWriteOpenAICompactV2StreamError_EmitsResponseFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetOpenAICompactMode(c, OpenAICompactModeBodySignalV2)
	MarkOpenAICompactV2SSEStarted(c)

	writeOpenAICompactV2StreamError(c, "something broke")

	raw := rec.Body.String()
	// Must emit response.failed (not {"type":"error"}) so Codex's SSE parser
	// recognises it as a terminal event.
	require.Contains(t, raw, `"type":"response.failed"`)
	require.Contains(t, raw, `"something broke"`)
	require.NotContains(t, raw, `"type":"error"`)
	// Must mark terminal committed to prevent duplicate terminal events and
	// block soft-timeout retry.
	require.True(t, IsOpenAICompactTerminalCommitted(c))
	require.True(t, IsResponseCommitted(c))
}

func TestWriteOpenAICompactV2StreamError_NoopAfterTerminalCommitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetOpenAICompactMode(c, OpenAICompactModeBodySignalV2)
	MarkOpenAICompactV2SSEStarted(c)
	MarkOpenAICompactTerminalCommitted(c)

	writeOpenAICompactV2StreamError(c, "second error")

	// Nothing should be written when terminal is already committed.
	require.Empty(t, rec.Body.String())
}

func TestExtractOpenAICompactionItem_RejectsCompactionWithoutEncryptedContent(t *testing.T) {
	// Compaction item without encrypted_content should be treated as invalid.
	body := []byte(`{
		"id":"resp_bad",
		"output":[{"type":"compaction"}]
	}`)
	_, _, err := extractOpenAICompactionItemFromLegacyCompactJSON(body)
	require.Error(t, err)
	require.Contains(t, err.Error(), "encrypted_content")
}

func TestExtractOpenAICompactionItem_AcceptsCompactionWithEncryptedContent(t *testing.T) {
	body := []byte(`{
		"id":"resp_ok",
		"output":[{"type":"compaction","encrypted_content":"ENC"}]
	}`)
	item, responseID, err := extractOpenAICompactionItemFromLegacyCompactJSON(body)
	require.NoError(t, err)
	require.Equal(t, "resp_ok", responseID)
	require.Equal(t, "ENC", gjson.GetBytes(item, "encrypted_content").String())
}

func TestWriteOpenAICompactV2Keepalive_IsDataEventNotComment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	require.NoError(t, writeOpenAICompactV2Keepalive(c))
	raw := rec.Body.String()
	require.Contains(t, raw, "data: ")
	require.Contains(t, raw, `"type":"keepalive"`)
	require.NotContains(t, raw, ": compact-keepalive")
}

func TestWriteOpenAICompactV2StreamError_UsesResponseFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetOpenAICompactMode(c, OpenAICompactModeBodySignalV2)
	MarkOpenAICompactV2SSEStarted(c)

	writeOpenAICompactV2StreamError(c, "upstream compact failed")
	raw := rec.Body.String()
	require.Contains(t, raw, `"type":"response.failed"`)
	require.Contains(t, raw, `"status":"failed"`)
	require.Contains(t, raw, "upstream compact failed")
	// Codex has no bare {"type":"error"} handler; ensure we don't emit it as the terminal type.
	require.NotContains(t, raw, `"type":"error"`)
	require.True(t, IsOpenAICompactTerminalCommitted(c))
	require.True(t, IsResponseCommitted(c))
}

func TestExtractOpenAICompactionItem_MissingEncryptedContent(t *testing.T) {
	body := []byte(`{
		"id":"resp_bad",
		"output":[{"type":"compaction"}]
	}`)
	_, responseID, err := extractOpenAICompactionItemFromLegacyCompactJSON(body)
	require.Error(t, err)
	require.Equal(t, "resp_bad", responseID)
	require.Contains(t, err.Error(), "encrypted_content")
}

func TestHandleErrorResponse_BodySignalV2SSEStarted_WritesFailedNotJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetOpenAICompactMode(c, OpenAICompactModeBodySignalV2)
	MarkOpenAICompactV2SSEStarted(c)
	// Simulate keepalive already flushed under 200 SSE.
	c.Header("Content-Type", "text/event-stream")
	c.Status(http.StatusOK)
	_, _ = rec.WriteString("data: {\"type\":\"keepalive\"}\n\n")

	svc := &OpenAIGatewayService{}
	account := &Account{
		ID:       42,
		Name:     "acct",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		// Empty error codes → ShouldHandleErrorCode false path still terminates SSE.
	}
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"bad compact","type":"invalid_request_error"}}`)),
	}

	result, err := svc.handleErrorResponse(context.Background(), resp, c, account, []byte(`{"model":"gpt-5.5"}`), "gpt-5.5")
	require.Error(t, err)
	require.Nil(t, result)
	raw := rec.Body.String()
	require.Contains(t, raw, `"type":"response.failed"`)
	// Must not append a raw JSON object body after the SSE stream.
	require.NotContains(t, raw, `{"error":{"type":"upstream_error"`)
	require.True(t, IsOpenAICompactTerminalCommitted(c))
}

func TestWriteOpenAIBodySignalV2Bridge_WriteFailureStillReturnsUsage(t *testing.T) {
	// extract succeeds → usage non-nil even when a later write would fail.
	// Direct unit coverage for extract+usage path used by Forward billing.
	body := []byte(`{
		"id":"resp_bill",
		"output":[{"type":"compaction","encrypted_content":"ENC"}],
		"usage":{"input_tokens":9,"output_tokens":1}
	}`)
	item, responseID, err := extractOpenAICompactionItemFromLegacyCompactJSON(body)
	require.NoError(t, err)
	require.Equal(t, "resp_bill", responseID)
	require.NotEmpty(t, item)
	usageVal, ok := extractOpenAIUsageFromJSONBytes(body)
	require.True(t, ok)
	require.Equal(t, 9, usageVal.InputTokens)
	require.Equal(t, 1, usageVal.OutputTokens)
}
