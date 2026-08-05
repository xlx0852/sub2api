package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesRejectedFieldRetryStateRejectsDuplicateBodyAndCap(t *testing.T) {
	initialBody := []byte(`{"model":"gpt-5.5"}`)
	state := newOpenAIResponsesRejectedFieldRetryState(initialBody)

	require.False(t, state.Allow(initialBody))
	for attempt := 0; attempt < maxOpenAIResponsesRejectedFieldRetries; attempt++ {
		nextBody := []byte(fmt.Sprintf(`{"model":"gpt-5.5","variant":%d}`, attempt))
		require.True(t, state.Allow(nextBody))
		require.False(t, state.Allow(nextBody))
	}
	require.False(t, state.Allow([]byte(`{"model":"gpt-5.5","variant":"overflow"}`)))
}

func TestOpenAIResponsesRejectedFieldRetryStateAllowsOnlyOneIndexedRepair(t *testing.T) {
	state := newOpenAIResponsesRejectedFieldRetryState([]byte(`{"input":[]}`))

	require.True(t, state.AllowForReason([]byte(`{"input":[1]}`), "indexed arguments compatibility rejection"))
	require.False(t, state.AllowForReason([]byte(`{"input":[2]}`), "indexed id compatibility rejection"))
	// Unrelated legacy compatibility retries retain their existing bounded path.
	require.True(t, state.AllowForReason([]byte(`{"input":[3]}`), "indexed namespace parameter rejection"))
}

func TestOpenAIResponsesRejectedFieldRetryStateSharesReplayRepairBudget(t *testing.T) {
	state := newOpenAIResponsesRejectedFieldRetryState([]byte(`{"input":[]}`))

	require.True(t, state.AllowForReason([]byte(`{"input":[1]}`), "tool call pairing compatibility rejection"))
	require.False(t, state.AllowForReason([]byte(`{"input":[2]}`), "indexed arguments compatibility rejection"))
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRejectsAmbiguousErrors(t *testing.T) {
	tests := []struct {
		name         string
		body         []byte
		responseBody []byte
	}{
		{
			name:         "namespace belongs to message",
			body:         []byte(`{"input":[{"type":"message","namespace":"keep"}]}`),
			responseBody: []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[0].namespace'.","param":"input[0].namespace"}}`),
		},
		{
			name:         "max output tokens only mentioned",
			body:         []byte(`{"max_output_tokens":4096}`),
			responseBody: []byte(`{"error":{"code":"invalid_request_error","message":"max_output_tokens must be positive","param":"max_output_tokens"}}`),
		},
		{
			name:         "structured param overrides namespace mention",
			body:         []byte(`{"input":[{"type":"function_call","namespace":"keep","arguments":"{}"}]}`),
			responseBody: []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[0].namespace'.","param":"tools"}}`),
		},
		{
			name:         "nested max output tokens param is not top level",
			body:         []byte(`{"max_output_tokens":4096,"input":[{"type":"message","content":{"max_output_tokens":"keep"}}]}`),
			responseBody: []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: input[0].content.max_output_tokens","param":"input[0].content.max_output_tokens"}}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, tt.body, tt.responseBody)
			require.NoError(t, err)
			require.False(t, changed)
			require.Nil(t, retryBody)
		})
	}
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyFindsNamespacePathInMessage(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call","namespace":"keep","arguments":"{}"},{"type":"function_call","namespace":"remove","arguments":"{}"}]}`)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"input[0] was accepted; Unknown parameter: 'input[1].namespace'."}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "keep", gjson.GetBytes(retryBody, "input.0.namespace").String())
	require.False(t, gjson.GetBytes(retryBody, "input.1.namespace").Exists())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyBindsNamespacePathToRejectionPhrase(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call","namespace":"keep","arguments":"{}"},{"type":"function_call","namespace":"remove","arguments":"{}"}]}`)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"input[0].namespace is supported; Unknown parameter: input[1].namespace."}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "keep", gjson.GetBytes(retryBody, "input.0.namespace").String())
	require.False(t, gjson.GetBytes(retryBody, "input.1.namespace").Exists())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyDoesNotTreatMaxOutputTokensSuggestionAsRejection(t *testing.T) {
	body := []byte(`{"max_tokens":4096,"max_output_tokens":2048}`)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: max_tokens. Use max_output_tokens instead."}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.False(t, changed)
	require.Nil(t, retryBody)
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyBindsMaxOutputTokensToRejectionPhrase(t *testing.T) {
	body := []byte(`{"max_output_tokens":2048}`)
	responseBody := []byte(`{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: max_output_tokens."}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(retryBody, "max_output_tokens").Exists())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRepairsIndexedPrivateArguments(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[
		{"type":"message","role":"user","content":"keep"},
		{"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"patch","input":"x","arguments":"private"},
		{"type":"custom_tool_call_output","id":"ctco_1","call_id":"call_1","output":"ok"}
	]}`)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[1].arguments'.","param":"input[1].arguments"}}`)

	retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, reason, "arguments")
	require.Equal(t, "keep", gjson.GetBytes(retryBody, "input.0.content").String())
	require.Equal(t, "function_call", gjson.GetBytes(retryBody, "input.1.type").String())
	require.JSONEq(t, `{"input":"x"}`, gjson.GetBytes(retryBody, "input.1.arguments").String())
	require.False(t, gjson.GetBytes(retryBody, "input.1.id").Exists())
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.2.type").String())
	require.False(t, gjson.GetBytes(retryBody, "input.2.id").Exists())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRepairsIndexedInvalidID(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[
		{"type":"function_call","id":"ctc_1","call_id":"call_1","name":"patch","arguments":"{}"},
		{"type":"function_call_output","id":"ctco_1","call_id":"call_1","output":"ok"}
	]}`)
	responseBody := []byte(`{"error":{"code":"invalid_value","message":"Invalid 'input[1].id': 'ctco_1'. Expected an ID that begins with 'fc'","param":"input[1].id"}}`)

	retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, reason, "id")
	require.False(t, gjson.GetBytes(retryBody, "input.0.id").Exists())
	require.False(t, gjson.GetBytes(retryBody, "input.1.id").Exists())
	require.Equal(t, "call_1", gjson.GetBytes(retryBody, "input.0.call_id").String())
	require.Equal(t, "call_1", gjson.GetBytes(retryBody, "input.1.call_id").String())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRepairsMissingCustomToolCallPair(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"gpt-5.6-sol","input":[
		{"type":"function_call","call_id":"fc_yxHnti6s5fgUU2ZJISVoSHN6","name":"apply_patch","arguments":"{}"},
		{"type":"custom_tool_call_output","call_id":"fc_yxHnti6s5fgUU2ZJISVoSHN6","output":"Done"}
	]}`)
	responseBody := []byte(`{"error":{"code":"invalid_request_error","message":"No tool call found for custom tool call output with call_id fc_yxHnti6s5fgUU2ZJISVoSHN6."}}`)

	retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, reason, "tool call pairing")
	require.Equal(t, "function_call", gjson.GetBytes(retryBody, "input.0.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.1.type").String())
	require.Equal(t, "fc_yxHnti6s5fgUU2ZJISVoSHN6", gjson.GetBytes(retryBody, "input.1.call_id").String())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRepairsItemIDAliasPair(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[
		{"type":"function_call","id":"fc_alias","call_id":"call_real","name":"apply_patch","arguments":"{}"},
		{"type":"custom_tool_call_output","call_id":"fc_alias","output":"Done"}
	]}`)
	responseBody := []byte(`{"error":{"code":"invalid_request_error","message":"No tool call found for custom tool call output with call_id fc_alias."}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.1.type").String())
	require.Equal(t, "call_real", gjson.GetBytes(retryBody, "input.1.call_id").String())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyDoesNotFabricateMissingCall(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[{"type":"custom_tool_call_output","call_id":"fc_orphan","output":"Done"}]}`)
	responseBody := []byte(`{"error":{"code":"invalid_request_error","message":"No tool call found for custom tool call output with call_id fc_orphan."}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.False(t, changed)
	require.Nil(t, retryBody)
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRepairsLongHistoryIndex109(t *testing.T) {
	t.Parallel()

	input := make([]map[string]any, 111)
	for i := 0; i < 109; i++ {
		input[i] = map[string]any{
			"type":    "message",
			"role":    "user",
			"content": fmt.Sprintf("keep-%d", i),
		}
	}
	input[109] = map[string]any{
		"type":    "custom_tool_call",
		"id":      "ctc_109",
		"call_id": "call_109",
		"name":    "patch",
		"input":   "x",
	}
	input[110] = map[string]any{
		"type":    "custom_tool_call_output",
		"id":      "ctco_109",
		"call_id": "call_109",
		"output":  "ok",
	}
	body, err := json.Marshal(map[string]any{"model": "gpt-5.5", "input": input})
	require.NoError(t, err)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[109].arguments'.","param":"input[109].arguments"}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "keep-108", gjson.GetBytes(retryBody, "input.108.content").String())
	require.Equal(t, "function_call", gjson.GetBytes(retryBody, "input.109.type").String())
	require.JSONEq(t, `{"input":"x"}`, gjson.GetBytes(retryBody, "input.109.arguments").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.110.type").String())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyDoesNotRepairUnrelatedInvalidError(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call","id":"ctc_1","call_id":"call_1","name":"patch","arguments":"{}"}]}`)
	responseBody := []byte(`{"error":{"code":"invalid_request_error","message":"Invalid model input[0].id is shown only as context","param":"model"}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.False(t, changed)
	require.Nil(t, retryBody)
}

func TestOpenAIGatewayService_RetriesRejectedIndexedNamespaceField(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"input":[{"type":"function_call","name":"first","namespace":"keep","arguments":"{}"},{"type":"custom_tool_call","name":"second","namespace":"remove","input":"{}"}]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[1].namespace'.","param":"input[1].namespace","type":"invalid_request_error"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(),
		newOpenAIRejectedFieldTestContext(body),
		newOpenAIRejectedFieldTestAccount(),
		body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "keep", gjson.GetBytes(upstream.bodies[1], "input.0.namespace").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "input.1.namespace").Exists())
}

func TestOpenAIGatewayService_RetriesRejectedIndexedPrivateArguments(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"input":[
		{"type":"message","role":"user","content":"keep"},
		{"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"patch","input":"x"},
		{"type":"custom_tool_call_output","id":"ctco_1","call_id":"call_1","output":"ok"},
		{"type":"tool_search_call","id":"tsc_2","call_id":"call_2","name":"search","arguments":"{\"q\":\"keep\"}"}
	]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[1].arguments'.","param":"input[1].arguments"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(),
		newOpenAIRejectedFieldTestContext(body),
		newOpenAIRejectedFieldTestAccount(),
		body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "custom_tool_call", gjson.GetBytes(upstream.bodies[0], "input.1.type").String())
	require.Equal(t, "function_call", gjson.GetBytes(upstream.bodies[1], "input.1.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.bodies[1], "input.2.type").String())
	require.Equal(t, "tool_search_call", gjson.GetBytes(upstream.bodies[1], "input.3.type").String())
}

func TestOpenAIGatewayService_RetriesExplicitMaxOutputTokensRejection(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"max_output_tokens":4096,"input":[{"type":"message","role":"user","content":{"max_output_tokens":"keep"}}]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: max_output_tokens","param":"max_output_tokens","type":"invalid_request_error"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	// Local Forward still pre-strips max_output_tokens for non-Codex clients; use
	// Codex CLI UA so the field reaches upstream and the rejection retry path runs.
	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(),
		newOpenAIRejectedFieldTestContextWithUA(body, codexCLIUserAgent),
		newOpenAIRejectedFieldTestAccount(),
		body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, int64(4096), gjson.GetBytes(upstream.bodies[0], "max_output_tokens").Int())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "max_output_tokens").Exists())
	require.Equal(t, "keep", gjson.GetBytes(upstream.bodies[1], "input.0.content.max_output_tokens").String())
}

func TestOpenAIGatewayService_ComposesDistinctRejectedFieldRetries(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"max_output_tokens":2048,"input":[{"type":"function_call","name":"first","namespace":"keep","arguments":"{}"},{"type":"custom_tool_call","name":"second","namespace":"remove","input":"{}"}]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[1].namespace'.","param":"input[1].namespace"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: max_output_tokens","param":"max_output_tokens"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(),
		newOpenAIRejectedFieldTestContextWithUA(body, codexCLIUserAgent),
		newOpenAIRejectedFieldTestAccount(),
		body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 3)
	require.True(t, gjson.GetBytes(upstream.bodies[0], "input.1.namespace").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "input.1.namespace").Exists())
	require.Equal(t, int64(2048), gjson.GetBytes(upstream.bodies[1], "max_output_tokens").Int())
	require.False(t, gjson.GetBytes(upstream.bodies[2], "input.1.namespace").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[2], "max_output_tokens").Exists())
}

func newOpenAIRejectedFieldTestService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		}},
		httpUpstream: upstream,
	}
}

func newOpenAIRejectedFieldTestContext(body []byte) *gin.Context {
	return newOpenAIRejectedFieldTestContextWithUA(body, "curl/8.0")
}

func newOpenAIRejectedFieldTestContextWithUA(body []byte, userAgent string) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", userAgent)
	return c
}

func newOpenAIRejectedFieldTestAccount() *Account {
	return &Account{
		ID:          5107,
		Name:        "responses-compatible",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://compat.example",
		},
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
			openai_compat.ExtraKeyResponsesSupported: true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func newOpenAIRejectedFieldTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
