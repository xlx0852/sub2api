package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newTransformedSSEFrameTestContext(t *testing.T) (*OpenAIGatewayService, *gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{
		cfg:           &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		toolCorrector: NewCodexToolCorrector(),
	}
	return svc, c, rec
}

func TestOpenAIStreamingTransformedSplitFrameIsValidatedAndRewrittenAtomically(t *testing.T) {
	svc, c, rec := newTransformedSSEFrameTestContext(t)
	upstream := "event: response.done\n" +
		"data: {\"response\":{\"id\":\"resp_split\",\"model\":\"mapped-model\",\n" +
		"data: \"usage\":{\"input_tokens\":2,\"output_tokens\":3}}}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(upstream)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1}, time.Now(), "client-model", "mapped-model")
	require.NoError(t, err)
	require.Equal(t, "resp_split", result.responseID)
	require.Equal(t, 2, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)

	var payloads [][]byte
	forEachOpenAISSEDataPayload(rec.Body.String(), func(data []byte) {
		payloads = append(payloads, append([]byte(nil), data...))
	})
	require.Len(t, payloads, 1)
	require.JSONEq(t, `{"type":"response.done","response":{"id":"resp_split","model":"client-model","output":[],"usage":{"input_tokens":2,"output_tokens":3}}}`, string(payloads[0]))
}

func TestOpenAIStreamingTransformedMalformedFrameBeforeOutputFailsOverWithoutEmission(t *testing.T) {
	svc, c, rec := newTransformedSSEFrameTestContext(t)
	largePreamble := strings.Repeat("p", 16*1024)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			"event: response.created\n" +
				"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"instructions\":\"" + largePreamble + "\"}}\n\n" +
				"event: response.output_text.delta\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"truncated\n\n",
		)),
		Header: http.Header{"X-Request-Id": []string{"rid-invalid-before-output"}},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformGrok}, time.Now(), "grok", "grok-4.5")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamingTransformedBoundsBufferedPreamble(t *testing.T) {
	svc, c, rec := newTransformedSSEFrameTestContext(t)
	svc.cfg.Gateway.MaxLineSize = 1024
	preamble := `data: {"type":"response.in_progress","response":{"id":"resp_1","instructions":"` + strings.Repeat("p", 600) + "\"}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(preamble + preamble)),
		Header:     http.Header{"X-Request-Id": []string{"rid-large-preamble"}},
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformGrok}, time.Now(), "grok", "grok-4.5")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamingTransformedMalformedFrameAfterOutputEmitsFailureAndDrainsUsage(t *testing.T) {
	svc, c, rec := newTransformedSSEFrameTestContext(t)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			"event: response.output_text.delta\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" +
				"event: response.output_text.delta\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"truncated\n\n" +
				"event: response.done\n" +
				"data: {\"type\":\"response.done\",\"response\":{\"id\":\"resp_drained\",\"usage\":{\"input_tokens\":7,\"output_tokens\":11}}}\n\n",
		)),
		Header: http.Header{"X-Request-Id": []string{"rid-invalid-after-output"}},
	}

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformGrok}, time.Now(), "grok", "grok-4.5")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, 7, result.usage.InputTokens)
	require.Equal(t, 11, result.usage.OutputTokens)

	body := rec.Body.String()
	require.Contains(t, body, `"delta":"partial"`)
	require.NotContains(t, body, "truncated")
	require.NotContains(t, body, "resp_drained")
	require.Equal(t, 1, strings.Count(body, "event: response.failed"))
	var failedPayload []byte
	forEachOpenAISSEDataPayload(body, func(data []byte) {
		if gjson.GetBytes(data, "type").String() == "response.failed" {
			failedPayload = append([]byte(nil), data...)
		}
	})
	require.NotEmpty(t, failedPayload)
	require.Equal(t, "invalid_upstream_sse", gjson.GetBytes(failedPayload, "response.error.code").String())
	require.True(t, gjson.GetBytes(failedPayload, "sequence_number").Exists())
}

func TestOpenAIStreamingTransformedProcessesTerminalFrameBeforeScanError(t *testing.T) {
	svc, c, _ := newTransformedSSEFrameTestContext(t)
	terminal := `event: response.done
data: {"type":"response.done","sequence_number":9,"response":{"id":"resp_terminal_error","usage":{"input_tokens":4,"output_tokens":6}}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(io.MultiReader(
			strings.NewReader(terminal),
			errReadCloser{err: io.ErrUnexpectedEOF},
		)),
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformGrok}, time.Now(), "grok", "grok-4.5")
	require.NoError(t, err)
	require.Equal(t, "resp_terminal_error", result.responseID)
	require.Equal(t, 4, result.usage.InputTokens)
	require.Equal(t, 6, result.usage.OutputTokens)
}

func TestOpenAIStreamingTransformedIgnoresMalformedDataAfterTerminal(t *testing.T) {
	svc, c, rec := newTransformedSSEFrameTestContext(t)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			"event: response.done\n" +
				"data: {\"type\":\"response.done\",\"sequence_number\":4,\"response\":{\"id\":\"resp_terminal\",\"usage\":{\"input_tokens\":4,\"output_tokens\":6}}}\n\n" +
				"event: response.output_text.delta\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"truncated\n\n",
		)),
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformGrok}, time.Now(), "grok", "grok-4.5")
	require.NoError(t, err)
	require.Equal(t, 4, result.usage.InputTokens)
	require.Equal(t, 6, result.usage.OutputTokens)
	require.Contains(t, rec.Body.String(), `"id":"resp_terminal"`)
	require.NotContains(t, rec.Body.String(), "truncated")
	require.NotContains(t, rec.Body.String(), "response.failed")
}

func TestOpenAIStreamingTransformedLargeFrameRemainsValid(t *testing.T) {
	svc, c, rec := newTransformedSSEFrameTestContext(t)
	largeDelta := strings.Repeat("x", 180*1024)
	upstream := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + largeDelta + "\"}\n\n" +
		"event: response.done\n" +
		"data: {\"type\":\"response.done\",\"response\":{\"id\":\"resp_large\",\"usage\":{\"input_tokens\":3,\"output_tokens\":5}}}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(upstream)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformGrok}, time.Now(), "grok", "grok-4.5")
	require.NoError(t, err)
	require.Equal(t, 3, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)
	require.Contains(t, rec.Body.String(), largeDelta)
	require.Equal(t, 2, strings.Count(rec.Body.String(), "data: "))
}

func TestOpenAIStreamingTransformedGrokTranslationEmitsDistinctValidFrames(t *testing.T) {
	svc, c, rec := newTransformedSSEFrameTestContext(t)
	setGrokCodexTranslation(c, newGrokCodexTranslation([]byte(`{"tools":[{"type":"custom","name":"apply_patch"}]}`)))
	upstream := strings.Join([]string{
		`event: response.output_item.added`,
		`data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"apply_patch","status":"in_progress"}}`,
		``,
		`event: response.function_call_arguments.done`,
		`data: {"type":"response.function_call_arguments.done","sequence_number":2,"output_index":0,"item_id":"fc_1",`,
		`data: "call_id":"call_1","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"}"}`,
		``,
		`event: response.output_item.done`,
		`data: {"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"}","status":"completed"}}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","sequence_number":4,"response":{"id":"resp_tools","model":"grok-4.5","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1}}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(upstream)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformGrok}, time.Now(), "grok", "grok-4.5")
	require.NoError(t, err)
	require.Equal(t, "resp_tools", result.responseID)
	body := rec.Body.String()
	require.Contains(t, body, `"type":"response.custom_tool_call_input.delta"`)
	require.Contains(t, body, `"type":"response.custom_tool_call_input.done"`)
	require.NotContains(t, body, `"type":"response.function_call_arguments.done"`)

	sequences := make([]int64, 0, 5)
	for _, rawFrame := range strings.Split(strings.TrimSpace(body), "\n\n") {
		dataLines := make([]string, 0, 1)
		for _, line := range strings.Split(rawFrame, "\n") {
			if data, ok := extractOpenAISSEDataLine(line); ok {
				dataLines = append(dataLines, data)
			}
		}
		require.Len(t, dataLines, 1, "each translated payload must be a distinct SSE frame: %q", rawFrame)
		require.True(t, gjson.Valid(dataLines[0]), "translated frame must contain complete JSON: %q", rawFrame)
		sequences = append(sequences, gjson.Get(dataLines[0], "sequence_number").Int())
	}
	require.Equal(t, []int64{1, 2, 3, 4, 5}, sequences)
}
