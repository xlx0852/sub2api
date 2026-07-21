//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClampOpenAIResponsesCallID_ShortUnchanged(t *testing.T) {
	require.Equal(t, "fc_abc123", clampOpenAIResponsesCallID("fc_abc123"))
	require.Equal(t, "call_1", clampOpenAIResponsesCallID("call_1"))
	require.Equal(t, "", clampOpenAIResponsesCallID(""))
}

func TestClampOpenAIResponsesCallID_LongDeterministicAndWithinLimit(t *testing.T) {
	// 67-char id mirrors the production upstream rejection.
	longID := "call_" + strings.Repeat("a", 62)
	require.Equal(t, 67, len(longID))

	clamped := clampOpenAIResponsesCallID(longID)
	require.LessOrEqual(t, len(clamped), openAIResponsesCallIDMaxLen)
	require.True(t, strings.HasPrefix(clamped, "call_"))
	// same input must always map to same output so call/output pairing holds
	require.Equal(t, clamped, clampOpenAIResponsesCallID(longID))

	// different long ids should not collide after clamp
	other := "call_" + strings.Repeat("b", 62)
	require.NotEqual(t, clamped, clampOpenAIResponsesCallID(other))
}

func TestFilterCodexInput_ClampsLongCallID_EvenWhenPreserving(t *testing.T) {
	longID := "fc_" + strings.Repeat("x", 64) // 67 chars
	require.Equal(t, 67, len(longID))

	input := []any{
		map[string]any{
			"type":      "function_call",
			"call_id":   longID,
			"name":      "bash",
			"arguments": "{}",
		},
		map[string]any{
			"type":    "function_call_output",
			"call_id": longID,
			"output":  "ok",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{
		PreserveReferences: true,
		PreserveCallIDs:    true,
	})
	require.Len(t, filtered, 2)

	fc := filtered[0].(map[string]any)
	out := filtered[1].(map[string]any)
	require.Equal(t, fc["call_id"], out["call_id"], "call/output pairing must survive clamp")
	require.LessOrEqual(t, len(fc["call_id"].(string)), openAIResponsesCallIDMaxLen)
	require.NotEqual(t, longID, fc["call_id"])
}

func TestFilterCodexInput_ClampsAfterPrefixRewrite(t *testing.T) {
	// call_ + 62 = 67; after rewrite becomes fc_ + 62 = 65, still needs clamp.
	longID := "call_" + strings.Repeat("z", 62)
	require.Equal(t, 67, len(longID))

	input := []any{
		map[string]any{
			"type":    "function_call",
			"call_id": longID,
			"name":    "bash",
		},
		map[string]any{
			"type":    "function_call_output",
			"call_id": longID,
			"output":  "ok",
		},
	}

	filtered := filterCodexInputWithOptions(input, codexInputFilterOptions{})
	require.Len(t, filtered, 2)
	fc := filtered[0].(map[string]any)
	out := filtered[1].(map[string]any)
	require.Equal(t, fc["call_id"], out["call_id"])
	require.LessOrEqual(t, len(fc["call_id"].(string)), openAIResponsesCallIDMaxLen)
	require.True(t, strings.HasPrefix(fc["call_id"].(string), "fc"))
}

func TestClampOpenAIResponsesCallIDsInBody(t *testing.T) {
	longID := "fc_" + strings.Repeat("q", 64)
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"function_call","call_id":"` + longID + `","name":"bash","arguments":"{}"},{"type":"function_call_output","call_id":"` + longID + `","output":"ok"},{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)

	next, changed, err := clampOpenAIResponsesCallIDsInBody(body)
	require.NoError(t, err)
	require.True(t, changed)

	c0 := gjson.GetBytes(next, "input.0.call_id").String()
	c1 := gjson.GetBytes(next, "input.1.call_id").String()
	require.Equal(t, c0, c1)
	require.LessOrEqual(t, len(c0), openAIResponsesCallIDMaxLen)
	require.NotEqual(t, longID, c0)
	// non-tool items untouched
	require.Equal(t, "message", gjson.GetBytes(next, "input.2.type").String())
}

func TestSanitizeOpenAIWSResponseCreateFrame_ClampsCallIDAndStripsMaxOutputTokensForOAuth(t *testing.T) {
	longID := "call_" + strings.Repeat("a", 62)
	require.Equal(t, 67, len(longID))
	frame := []byte(`{"type":"response.create","model":"gpt-5.6-sol","max_output_tokens":2048,"input":[{"type":"function_call","call_id":"` + longID + `","name":"bash","arguments":"{}"},{"type":"function_call_output","call_id":"` + longID + `","output":"ok"}]}`)

	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	next, changed, err := sanitizeOpenAIWSResponseCreateFrame(frame, oauth)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(next, "max_output_tokens").Exists())
	c0 := gjson.GetBytes(next, "input.0.call_id").String()
	c1 := gjson.GetBytes(next, "input.1.call_id").String()
	require.Equal(t, c0, c1)
	require.LessOrEqual(t, len(c0), openAIResponsesCallIDMaxLen)
	require.NotEqual(t, longID, c0)

	// API Key 账号仍 clamp call_id，但不 strip max_output_tokens（官方 API 支持）。
	apikey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	nextAPI, changedAPI, err := sanitizeOpenAIWSResponseCreateFrame(frame, apikey)
	require.NoError(t, err)
	require.True(t, changedAPI)
	require.True(t, gjson.GetBytes(nextAPI, "max_output_tokens").Exists())
	require.LessOrEqual(t, len(gjson.GetBytes(nextAPI, "input.0.call_id").String()), openAIResponsesCallIDMaxLen)
}

func TestClampOpenAIResponsesCallIDsInMap(t *testing.T) {
	longID := "fc_" + strings.Repeat("m", 64)
	body := map[string]any{
		"input": []any{
			map[string]any{"type": "function_call", "call_id": longID, "name": "bash"},
			map[string]any{"type": "function_call_output", "call_id": longID, "output": "ok"},
		},
	}
	require.True(t, clampOpenAIResponsesCallIDsInMap(body))
	items := body["input"].([]any)
	c0 := items[0].(map[string]any)["call_id"].(string)
	c1 := items[1].(map[string]any)["call_id"].(string)
	require.Equal(t, c0, c1)
	require.LessOrEqual(t, len(c0), openAIResponsesCallIDMaxLen)
}
