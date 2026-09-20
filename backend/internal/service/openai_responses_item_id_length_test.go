package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesReplayItemIDLengthBoundary(t *testing.T) {
	for _, typ := range []string{"message", "reasoning", "compaction_summary", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output", "mcp_tool_call", "mcp_tool_call_output", "tool_search_output"} {
		for _, size := range []int{0, 63, 64, 65, 83} {
			t.Run(fmt.Sprintf("%s/%d", typ, size), func(t *testing.T) {
				require.Equal(t, size > 64, hasOverlongOpenAIResponsesReplayItemID(typ, strings.Repeat("x", size)))
			})
		}
	}
	for _, typ := range []string{"item_reference", "unknown", "computer_call"} {
		require.False(t, hasOverlongOpenAIResponsesReplayItemID(typ, strings.Repeat("x", 83)))
	}
	// The upstream limit is characters, not UTF-8 bytes.
	require.False(t, hasOverlongOpenAIResponsesReplayItemID("message", strings.Repeat("界", 64)))
	require.True(t, hasOverlongOpenAIResponsesReplayItemID("message", strings.Repeat("界", 65)))
}

// Mirror input[384].id length=83 without retaining any production request data.
func TestResponsesReplayItemIDLength_AllUpstreamPaths(t *testing.T) {
	input := make([]any, 0, 385)
	for i := 0; i < 383; i++ {
		input = append(input, map[string]any{"type": "message", "role": "user", "content": "test"})
	}
	input = append(input,
		map[string]any{"type": "function_call", "id": "fc_" + strings.Repeat("a", 80), "call_id": "call_pair", "name": "example", "arguments": "{}"},
		map[string]any{"type": "function_call_output", "id": "fco_" + strings.Repeat("b", 79), "call_id": "call_pair", "output": "unchanged output"},
	)
	body, err := json.Marshal(map[string]any{"type": "response.create", "model": "gpt-5.6-luna", "input": input})
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(body, "input.384.id").String(), 83)

	paths := map[string]func([]byte) ([]byte, error){
		"apikey_http": func(b []byte) ([]byte, error) {
			next, changed, err := sanitizeOpenAIResponsesInputItemIDs(b)
			require.True(t, changed)
			return next, err
		},
		"oauth_passthrough": func(b []byte) ([]byte, error) {
			next, _, err := normalizeOpenAIPassthroughOAuthBody(b, false)
			return next, err
		},
		"ws_frame_oauth": func(b []byte) ([]byte, error) {
			next, _, err := sanitizeOpenAIWSResponseCreateFrame(b, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth})
			return next, err
		},
		"ws_frame_apikey": func(b []byte) ([]byte, error) {
			next, _, err := sanitizeOpenAIWSResponseCreateFrame(b, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey})
			return next, err
		},
		"ws_http_bridge_after_replay": prepareOpenAIWSHTTPBridgeBody,
		"ws_map": func(b []byte) ([]byte, error) {
			var req map[string]any
			require.NoError(t, json.Unmarshal(b, &req))
			return json.Marshal((&OpenAIGatewayService{}).buildOpenAIWSCreatePayload(req, &Account{Type: AccountTypeOAuth}))
		},
		"codex_transform": func(b []byte) ([]byte, error) {
			var req map[string]any
			require.NoError(t, json.Unmarshal(b, &req))
			req["input"] = filterCodexInputWithOptions(req["input"].([]any), codexInputFilterOptions{PreserveReferences: true, PreserveCallIDs: true})
			return json.Marshal(req)
		},
	}
	for name, run := range paths {
		t.Run(name, func(t *testing.T) {
			next, err := run(body)
			require.NoError(t, err)
			require.Equal(t, int64(385), gjson.GetBytes(next, "input.#").Int())
			require.False(t, gjson.GetBytes(next, "input.383.id").Exists())
			require.False(t, gjson.GetBytes(next, "input.384.id").Exists())
			require.Equal(t, "call_pair", gjson.GetBytes(next, "input.383.call_id").String())
			require.Equal(t, "call_pair", gjson.GetBytes(next, "input.384.call_id").String())
			require.Equal(t, "{}", gjson.GetBytes(next, "input.383.arguments").String())
			require.Equal(t, "unchanged output", gjson.GetBytes(next, "input.384.output").String())
		})
	}
	require.Len(t, gjson.GetBytes(body, "input.384.id").String(), 83, "original request must not be mutated")
}

func TestResponsesReplayItemIDLength_PreservesReferencesAndPayload(t *testing.T) {
	longID := "fc_" + strings.Repeat("a", 80)
	body := []byte(`{"previous_response_id":"resp_keep","input":[{"type":"item_reference","id":"` + longID + `"},{"type":"function_call","id":"fc_` + strings.Repeat("b", 61) + `","call_id":"call_keep","arguments":"{}"},{"type":"reasoning","id":"rs_` + strings.Repeat("c", 80) + `","encrypted_content":"synthetic_encrypted_context","summary":[]},{"type":"future_item","id":"` + longID + `","data":{"id":"` + longID + `"}}]}`)
	next, changed, err := stripOverlongOpenAIResponsesInputItemIDs(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "resp_keep", gjson.GetBytes(next, "previous_response_id").String())
	require.Equal(t, longID, gjson.GetBytes(next, "input.0.id").String())
	require.Len(t, gjson.GetBytes(next, "input.1.id").String(), 64)
	require.False(t, gjson.GetBytes(next, "input.2.id").Exists())
	require.Equal(t, "synthetic_encrypted_context", gjson.GetBytes(next, "input.2.encrypted_content").String())
	require.Equal(t, longID, gjson.GetBytes(next, "input.3.id").String())
	require.Equal(t, longID, gjson.GetBytes(next, "input.3.data.id").String())
	again, changed, err := stripOverlongOpenAIResponsesInputItemIDs(next)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, next, again)
}

func TestResponsesReplayItemIDLength_MapCopyOnWrite(t *testing.T) {
	longID := "fc_" + strings.Repeat("x", 80)
	item := map[string]any{"type": "function_call", "id": longID, "call_id": "call_keep", "arguments": "{}"}
	input := []any{item}
	body := map[string]any{"input": input}
	require.True(t, stripOverlongOpenAIResponsesInputItemIDsInMap(body))
	require.Equal(t, longID, item["id"])
	require.Same(t, &input[0], &input[0])
	require.NotContains(t, body["input"].([]any)[0], "id")
	require.Equal(t, "call_keep", body["input"].([]any)[0].(map[string]any)["call_id"])
	require.False(t, stripOverlongOpenAIResponsesInputItemIDsInMap(body))
}

func TestResponsesReplayItemIDLength_CallIDDerivedBeforeStripping(t *testing.T) {
	longID := "fc_" + strings.Repeat("x", 80)
	input := []any{
		map[string]any{"type": "function_call", "id": longID, "name": "example", "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": longID, "output": "ok"},
	}
	out := filterCodexInputWithOptions(input, codexInputFilterOptions{PreserveReferences: true, PreserveCallIDs: true})
	call, result := out[0].(map[string]any), out[1].(map[string]any)
	require.NotContains(t, call, "id")
	require.Equal(t, clampOpenAIResponsesCallID(longID), call["call_id"])
	require.Equal(t, call["call_id"], result["call_id"])
	require.Equal(t, longID, input[0].(map[string]any)["id"])
}
