package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSanitizeGrokAssistantOutputText_TruncatesEvalRollout(t *testing.T) {
	raw := "MULTI_TURN_1\n\n============================================================\n【步骤 2】"
	got := sanitizeGrokAssistantOutputText(raw)
	require.Equal(t, "MULTI_TURN_1", got)
}

func TestSanitizeGrokAssistantOutputText_TruncatesPromptEcho(t *testing.T) {
	raw := "REASONING_LIVE_1\n\nuser: 请只回复 exactly: REASONING_LIVE_2，不要多说任何字。"
	got := sanitizeGrokAssistantOutputText(raw)
	require.Equal(t, "REASONING_LIVE_1", got)
}

func TestSanitizeGrokAssistantOutputText_TruncatesToolCallTemplate(t *testing.T) {
	raw := `<seed:tool_call:execute_bash><parameter=command>echo "=== 第一轮 multi-turn 对话 ==="</parameter></seed:tool_call>`
	got := sanitizeGrokAssistantOutputText(raw)
	require.Equal(t, "", got)
}

func TestSanitizeGrokAssistantOutputText_PreservesCleanShortAnswer(t *testing.T) {
	for _, raw := range []string{"2", "蓝色", "REASONING_LIVE_1", "42"} {
		require.Equal(t, raw, sanitizeGrokAssistantOutputText(raw))
	}
}

func TestSanitizeGrokCompletedEventPayload(t *testing.T) {
	payload := []byte(`{"type":"response.completed","response":{"output":[{"type":"reasoning","content":[{"type":"reasoning_text","text":"thinking"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"42\n\nuser: 请只回复 exactly: REASONING_LIVE_1"}]}]}}`)
	got, changed := sanitizeGrokCompletedEventPayload(payload)
	require.True(t, changed)
	require.Equal(t, "42", gjson.GetBytes(got, "response.output.1.content.0.text").String())
}

func TestSanitizeGrokInputAssistantItems(t *testing.T) {
	input := []any{
		map[string]any{"role": "user", "type": "message", "content": []any{map[string]any{"type": "input_text", "text": "hi"}}},
		map[string]any{"role": "assistant", "type": "message", "content": []any{map[string]any{"type": "output_text", "text": "REASONING_LIVE_1\n\n===="}}},
	}
	got := sanitizeGrokInputAssistantItems(input)
	assistant := got[1].(map[string]any)
	content := assistant["content"].([]any)[0].(map[string]any)
	require.Equal(t, "REASONING_LIVE_1", content["text"])
}

func TestSanitizeGrokWSOutboundEvent_Completed(t *testing.T) {
	payload := []byte(`{"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"output_text","text":"2\n\nuser:\nfoo"}]}]}}`)
	got, changed := sanitizeGrokWSOutboundEvent("response.completed", payload)
	require.True(t, changed)
	require.Equal(t, "2", gjson.GetBytes(got, "response.output.0.content.0.text").String())
}

func TestSanitizeGrokAssistantOutputText_HardCap(t *testing.T) {
	raw := "OK" + strings.Repeat("x", grokSanitizedAssistantOutputMaxLen+100)
	got := sanitizeGrokAssistantOutputText(raw)
	require.Len(t, got, grokSanitizedAssistantOutputMaxLen)
}