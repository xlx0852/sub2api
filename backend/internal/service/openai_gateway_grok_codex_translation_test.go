//go:build unit

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

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPatchGrokResponsesBodyTranslatesCustomToolHistoryAndChoice(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.6-sol",
		"input":[
			{"type":"custom_tool_call","call_id":"call_1","name":"apply_patch","input":"*** Begin Patch"},
			{"type":"custom_tool_call_output","call_id":"call_1","output":"Done"}
		],
		"tools":[{"type":"custom","name":"apply_patch","description":"patch files"}],
		"tool_choice":{"type":"custom","name":"apply_patch"}
	}`)

	patched, translation, err := patchGrokResponsesBodyWithTranslation(body, "grok-4.5")
	require.NoError(t, err)
	require.True(t, translation.isCustom("apply_patch"))
	require.Equal(t, "function_call", gjson.GetBytes(patched, "input.0.type").String())
	require.Equal(t, "call_1", gjson.GetBytes(patched, "input.0.call_id").String())
	require.JSONEq(t, `{"input":"*** Begin Patch"}`, gjson.GetBytes(patched, "input.0.arguments").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(patched, "input.1.type").String())
	require.Equal(t, "call_1", gjson.GetBytes(patched, "input.1.call_id").String())
	require.Equal(t, "function", gjson.GetBytes(patched, "tools.0.type").String())
	require.Equal(t, "string", gjson.GetBytes(patched, "tools.0.parameters.properties.input.type").String())
	require.Equal(t, "function", gjson.GetBytes(patched, "tool_choice.type").String())
	require.Equal(t, "apply_patch", gjson.GetBytes(patched, "tool_choice.name").String())
}

func TestRewriteGrokCustomCallsInJSONRestoresOnlyDeclaredCustomTools(t *testing.T) {
	translation := newGrokCodexTranslation([]byte(`{
		"tools":[
			{"type":"custom","name":"exec"},
			{"type":"function","name":"lookup","parameters":{"type":"object"}}
		]
	}`))
	body := []byte(`{
		"id":"resp_1",
		"output":[
			{"type":"function_call","id":"fc_1","call_id":"call_1","name":"exec","arguments":"{\"input\":\"pwd\"}"},
			{"type":"function_call","id":"fc_2","call_id":"call_2","name":"lookup","arguments":"{\"q\":\"x\"}"}
		]
	}`)

	rewritten, changed := rewriteGrokCustomCallsInJSON(body, translation)
	require.True(t, changed)
	require.Equal(t, "custom_tool_call", gjson.GetBytes(rewritten, "output.0.type").String())
	require.Equal(t, "pwd", gjson.GetBytes(rewritten, "output.0.input").String())
	require.False(t, gjson.GetBytes(rewritten, "output.0.arguments").Exists())
	require.Equal(t, "function_call", gjson.GetBytes(rewritten, "output.1.type").String())
	require.Equal(t, `{"q":"x"}`, gjson.GetBytes(rewritten, "output.1.arguments").String())
}

func TestRewriteGrokCodexSSEDataRestoresCustomToolLifecycle(t *testing.T) {
	translation := newGrokCodexTranslation([]byte(`{"tools":[{"type":"custom","name":"apply_patch"}]}`))

	added := []byte(`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"apply_patch","status":"in_progress"}}`)
	addedOut, changed := rewriteGrokCodexSSEData(added, translation)
	require.True(t, changed)
	require.Len(t, addedOut, 1)
	require.Equal(t, "custom_tool_call", gjson.GetBytes(addedOut[0], "item.type").String())

	delta := []byte(`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"input\":\"*** Begin"}`)
	deltaOut, changed := rewriteGrokCodexSSEData(delta, translation)
	require.True(t, changed)
	require.Empty(t, deltaOut)

	done := []byte(`{"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_1","call_id":"call_1","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"}"}`)
	doneOut, changed := rewriteGrokCodexSSEData(done, translation)
	require.True(t, changed)
	require.Len(t, doneOut, 2)
	require.Equal(t, "response.custom_tool_call_input.delta", gjson.GetBytes(doneOut[0], "type").String())
	require.Equal(t, "*** Begin Patch", gjson.GetBytes(doneOut[0], "delta").String())
	require.Equal(t, "response.custom_tool_call_input.done", gjson.GetBytes(doneOut[1], "type").String())
	require.Equal(t, "*** Begin Patch", gjson.GetBytes(doneOut[1], "input").String())

	terminal := []byte(`{"type":"response.completed","response":{"id":"resp_1","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"}"}]}}`)
	terminalOut, changed := rewriteGrokCodexSSEData(terminal, translation)
	require.True(t, changed)
	require.Equal(t, "custom_tool_call", gjson.GetBytes(terminalOut[0], "response.output.0.type").String())
	require.Equal(t, "*** Begin Patch", gjson.GetBytes(terminalOut[0], "response.output.0.input").String())
}

func TestGrokTranslationStateIsRequestScoped(t *testing.T) {
	custom := newGrokCodexTranslation([]byte(`{"tools":[{"type":"custom","name":"lookup"}]}`))
	function := newGrokCodexTranslation([]byte(`{"tools":[{"type":"function","name":"lookup"}]}`))
	body := []byte(`{"output":[{"type":"function_call","name":"lookup","arguments":"{\"q\":\"x\"}"}]}`)

	customBody, changed := rewriteGrokCustomCallsInJSON(body, custom)
	require.True(t, changed)
	require.Equal(t, "custom_tool_call", gjson.GetBytes(customBody, "output.0.type").String())

	functionBody, changed := rewriteGrokCustomCallsInJSON(body, function)
	require.False(t, changed)
	require.True(t, json.Valid(functionBody))
	require.Equal(t, "function_call", gjson.GetBytes(functionBody, "output.0.type").String())
}

func TestHandleNonStreamingResponseRestoresGrokCustomToolCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	setGrokCodexTranslation(c, newGrokCodexTranslation([]byte(`{"tools":[{"type":"custom","name":"exec"}]}`)))

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_1","model":"grok-4.5","status":"completed",
			"output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"exec","arguments":"{\"input\":\"pwd\"}"}],
			"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}
		}`)),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, toolCorrector: NewCodexToolCorrector()}

	result, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformGrok}, "grok", "grok-4.5")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "custom_tool_call", gjson.Get(recorder.Body.String(), "output.0.type").String())
	require.Equal(t, "pwd", gjson.Get(recorder.Body.String(), "output.0.input").String())
}

func TestHandleStreamingResponseRestoresGrokCustomToolLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	setGrokCodexTranslation(c, newGrokCodexTranslation([]byte(`{"tools":[{"type":"custom","name":"apply_patch"}]}`)))

	stream := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"apply_patch","status":"in_progress"}}`,
		``,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"input\":\"*** Begin Patch\"}"}`,
		``,
		`data: {"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_1","call_id":"call_1","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"}"}`,
		``,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"}","status":"completed"}}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"grok-4.5","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"}"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(stream)),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, toolCorrector: NewCodexToolCorrector()}

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformGrok}, time.Now(), "grok", "grok-4.5")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, recorder.Body.String(), `"type":"custom_tool_call"`)
	require.Contains(t, recorder.Body.String(), `"type":"response.custom_tool_call_input.delta"`)
	require.Contains(t, recorder.Body.String(), `"type":"response.custom_tool_call_input.done"`)
	require.NotContains(t, recorder.Body.String(), `"type":"response.function_call_arguments.delta"`)
}
