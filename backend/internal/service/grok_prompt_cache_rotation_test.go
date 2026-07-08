//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyGrokCollapsedInputPromptCacheKeyRotation_IsolatesDesktopTurn(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"prompt_cache_key":     "session-abc",
		"previous_response_id": "resp_prev",
	}
	fullInput := []any{
		map[string]any{
			"role":    "user",
			"type":    "message",
			"content": []any{map[string]any{"type": "input_text", "text": "分析 bridge_wechat JSON 转义"}},
		},
		map[string]any{
			"role":    "assistant",
			"type":    "message",
			"content": []any{map[string]any{"type": "output_text", "text": stringsLongBridgeWechatAnalysis()}},
		},
		map[string]any{
			"role":    "user",
			"type":    "message",
			"content": []any{map[string]any{"type": "input_text", "text": "看下桌面都有什么文件"}},
		},
	}
	collapsed := keepTrailingGrokUserMessages(fullInput)

	rotated := applyGrokCollapsedInputPromptCacheKeyRotation(payload, fullInput, collapsed)
	require.True(t, rotated)
	require.Contains(t, payload["prompt_cache_key"], "session-abc"+grokIsolatedTurnPromptCacheKeyPrefix)
	require.NotContains(t, payload, "previous_response_id")
}

func TestApplyGrokCollapsedInputPromptCacheKeyRotation_SkipsShortAssistantHistory(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"prompt_cache_key":     "session-abc",
		"previous_response_id": "resp_prev",
	}
	fullInput := []any{
		map[string]any{"role": "user", "type": "message", "content": []any{map[string]any{"type": "input_text", "text": "1+1等于几"}}},
		map[string]any{"role": "assistant", "type": "message", "content": []any{map[string]any{"type": "output_text", "text": "2"}}},
		map[string]any{"role": "user", "type": "message", "content": []any{map[string]any{"type": "input_text", "text": "天空是什么颜色"}}},
	}
	collapsed := keepTrailingGrokUserMessages(fullInput)

	rotated := applyGrokCollapsedInputPromptCacheKeyRotation(payload, fullInput, collapsed)
	require.False(t, rotated)
}

func TestCollapseGrokResponsesInputForContinuation_KeepsRecallDialogue(t *testing.T) {
	t.Parallel()

	input := []any{
		map[string]any{"role": "user", "type": "message", "content": []any{map[string]any{"type": "input_text", "text": "17+25等于几？只回答数字"}}},
		map[string]any{"role": "assistant", "type": "message", "content": []any{map[string]any{"type": "output_text", "text": "42"}}},
		map[string]any{"role": "user", "type": "message", "content": []any{map[string]any{"type": "input_text", "text": "月亮是什么形状？"}}},
		map[string]any{"role": "assistant", "type": "message", "content": []any{map[string]any{"type": "output_text", "text": "圆形"}}},
		map[string]any{"role": "user", "type": "message", "content": []any{map[string]any{"type": "input_text", "text": "我刚才问的加法结果是多少？"}}},
	}
	collapsed := collapseGrokResponsesInputForContinuation(nil, input, nil)
	require.Len(t, collapsed, 5)
	require.Equal(t, "42", extractGrokInputItemText(collapsed[1].(map[string]any)))
	require.Equal(t, "我刚才问的加法结果是多少？", extractGrokInputItemText(collapsed[4].(map[string]any)))
}

func TestApplyGrokCollapsedInputPromptCacheKeyRotation_KeepsRecallTurn(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"prompt_cache_key":     "session-abc",
		"previous_response_id": "resp_prev",
	}
	fullInput := []any{
		map[string]any{"role": "user", "type": "message", "content": []any{map[string]any{"type": "input_text", "text": "1+1等于几"}}},
		map[string]any{"role": "assistant", "type": "message", "content": []any{map[string]any{"type": "output_text", "text": "2"}}},
		map[string]any{"role": "user", "type": "message", "content": []any{map[string]any{"type": "input_text", "text": "天空是什么颜色"}}},
		map[string]any{"role": "assistant", "type": "message", "content": []any{map[string]any{"type": "output_text", "text": "蓝色"}}},
		map[string]any{"role": "user", "type": "message", "content": []any{map[string]any{"type": "input_text", "text": "我刚才问的加法结果是多少"}}},
	}
	collapsed := keepTrailingGrokUserMessages(fullInput)

	rotated := applyGrokCollapsedInputPromptCacheKeyRotation(payload, fullInput, collapsed)
	require.False(t, rotated)
	require.Equal(t, "session-abc", payload["prompt_cache_key"])
	require.Equal(t, "resp_prev", payload["previous_response_id"])
}

func stringsLongBridgeWechatAnalysis() string {
	return "bridge_wechat json.dumps 双重转义 " + stringsRepeat("x", grokPromptCacheRotationMinDroppedAssistant)
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}