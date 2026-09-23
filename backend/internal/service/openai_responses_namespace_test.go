package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldFlattenOpenAIResponsesNamespaces(t *testing.T) {
	oauth := &Account{Type: AccountTypeOAuth}
	apiKey := &Account{Type: AccountTypeAPIKey}
	externalBridge := &Account{
		Type: AccountTypeAPIKey,
		Extra: map[string]any{
			"external_bridge":      true,
			"external_bridge_kind": "devin",
		},
	}

	tests := []struct {
		name               string
		account            *Account
		transport          OpenAIUpstreamTransport
		passthroughEnabled bool
		want               bool
	}{
		{name: "oauth_http", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, want: true},
		{name: "oauth_http_passthrough", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, passthroughEnabled: true, want: true},
		// WSv2 出口原样转发上游事件、不做回程还原，摊平会让客户端收到无法匹配的平名。
		{name: "oauth_wsv2", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, want: false},
		// 透传账号先于 WSv2 分支经 HTTP 转发返回，仍需摊平。
		{name: "oauth_wsv2_passthrough", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, passthroughEnabled: true, want: true},
		{name: "apikey_http", account: apiKey, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		// Devin 等 external bridge 必须摊平 namespace，否则 tools 在桥接侧被整表丢弃。
		{name: "external_bridge_http", account: externalBridge, transport: OpenAIUpstreamTransportHTTPSSE, passthroughEnabled: true, want: true},
		{name: "nil_account", account: nil, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldFlattenOpenAIResponsesNamespaces(tt.account, tt.transport, tt.passthroughEnabled))
		})
	}
}

func TestDowngradeExternalBridgeResponsesTools_RoundTrip(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	body := []byte(`{
		"model": "gpt-6-astra",
		"input": [
			{"type": "additional_tools", "tools": [
				{"type": "namespace", "name": "functions", "tools": [
					{"type": "custom", "name": "exec", "description": "run", "format": {"type": "grammar"}},
					{"type": "function", "name": "wait", "parameters": {}}
				]}
			]},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "hi"}]}
		]
	}`)

	// 上游 demotion 后的形态：additional_tools 已提升为顶层 tools，namespace 仍在。
	demoted, changed, err := applyOpenAIResponsesLiteForUpstream(c, body, "gpt-6-astra", true)
	require.NoError(t, err)
	require.True(t, changed)

	shaped, err := downgradeExternalBridgeResponsesTools(c, demoted, true)
	require.NoError(t, err)

	var shapedBody map[string]any
	require.NoError(t, json.Unmarshal(shaped, &shapedBody))
	tools, _ := shapedBody["tools"].([]any)
	require.Len(t, tools, 2)
	for _, raw := range tools {
		tool := raw.(map[string]any)
		require.Equal(t, "function", tool["type"], "custom/namespace children must be flattened to functions")
	}
	names := openAIResponsesNamespaceNames(c)
	require.Contains(t, names, "functions__exec")
	customNames := openAIResponsesCustomToolNames(c)
	require.Equal(t, map[string]bool{"functions__exec": true}, customNames)

	// 回程：上游只会回 function_call，必须还原成 custom_tool_call + namespace 身份。
	upstream := []byte(`{"type":"response.output_item.done","item":{"type":"function_call","name":"functions__exec","call_id":"ctc_9","arguments":"{\"input\":\"ls\"}"}}`)
	restored, err := restoreOpenAIResponsesNamespacePayload(c, upstream)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"response.output_item.done","item":{"type":"custom_tool_call","name":"exec","namespace":"functions","call_id":"ctc_9","input":"ls"}}`, string(restored))
}

func TestTruncateExternalBridgeToolDescriptions(t *testing.T) {
	long := strings.Repeat("x", externalBridgeToolDescriptionLimit+500)
	requestBody := map[string]any{
		"tools": []any{
			map[string]any{"type": "function", "name": "functions__exec", "description": long},
			map[string]any{"type": "function", "name": "short", "description": "ok"},
			map[string]any{"type": "function", "name": "nodesc"},
		},
	}

	require.True(t, truncateExternalBridgeToolDescriptions(requestBody))
	tools := requestBody["tools"].([]any)
	truncated := tools[0].(map[string]any)["description"].(string)
	require.LessOrEqual(t, len([]rune(truncated)), externalBridgeToolDescriptionLimit)
	require.Contains(t, truncated, "truncated by the gateway")
	require.Equal(t, "ok", tools[1].(map[string]any)["description"])
	require.NotContains(t, tools[2].(map[string]any), "description")
	require.False(t, truncateExternalBridgeToolDescriptions(requestBody))
}

func TestTruncateExternalBridgeToolDescriptionPreferLineBoundary(t *testing.T) {
	line := "line of description\n"
	description := strings.Repeat(line, 400) // 8000 字符，远超声明的上限
	got := truncateExternalBridgeToolDescription(description)
	require.LessOrEqual(t, len([]rune(got)), externalBridgeToolDescriptionLimit)
	require.True(t, strings.HasSuffix(got, externalBridgeToolDescriptionTruncationMarker))
	// 截断处必须是完整行，而不是半句。
	body := strings.TrimSuffix(got, externalBridgeToolDescriptionTruncationMarker)
	require.False(t, strings.HasSuffix(strings.TrimSpace(body), "line of descri"))
	require.True(t, strings.HasSuffix(strings.TrimSpace(body), "line of description"))
	short := description[:100]
	require.Equal(t, short, truncateExternalBridgeToolDescription(short))
}

func TestDevinToolDescriptionUTF8ByteLimit(t *testing.T) {
	for _, text := range []string{strings.Repeat("中文🙂", 900), strings.Repeat("é", 2500), strings.Repeat("x", 4001)} {
		got := truncateExternalBridgeToolDescription(text)
		require.LessOrEqual(t, len(got), externalBridgeToolDescriptionLimit)
		require.True(t, utf8.ValidString(got))
		require.True(t, strings.HasSuffix(got, externalBridgeToolDescriptionTruncationMarker))
		require.Equal(t, got, truncateExternalBridgeToolDescription(got))
	}
	short := strings.Repeat("x", 4000)
	require.Equal(t, short, truncateExternalBridgeToolDescription(short))
}

func TestExternalBridgeToolsCursorDoesNotUseDevinPolicy(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body, err := json.Marshal(map[string]any{"tools": []any{map[string]any{"type": "custom", "name": "exec", "description": strings.Repeat("x", 6000), "format": map[string]any{"type": "text"}}}})
	require.NoError(t, err)
	got, err := downgradeExternalBridgeResponsesTools(c, body, false)
	require.NoError(t, err)
	require.Equal(t, string(body), string(got))
	require.Empty(t, openAIResponsesCustomToolNames(c))
}

func TestDevinPlainFunctionLongDescriptionIsCapped(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body, err := json.Marshal(map[string]any{"tools": []any{map[string]any{"type": "function", "name": "shell", "description": strings.Repeat("中文🙂", 900)}}})
	require.NoError(t, err)
	got, err := downgradeExternalBridgeResponsesTools(c, body, true)
	require.NoError(t, err)
	var decoded struct {
		Tools []struct {
			Description string `json:"description"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(got, &decoded))
	require.LessOrEqual(t, len(decoded.Tools[0].Description), externalBridgeToolDescriptionLimit)
	require.True(t, utf8.ValidString(decoded.Tools[0].Description))
}
