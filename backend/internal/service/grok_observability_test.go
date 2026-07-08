package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectGrokOutputAnomalies_EvalRollout(t *testing.T) {
	raw := []byte(`{"response":{"output":[{"type":"message","content":[{"type":"output_text","text":"REASONING_LIVE_1\n\n============================================================\n【步骤 2】 2026-03-16 12:40:01"}]}]}}`)
	anomalies := detectGrokOutputAnomalies(raw)
	require.Contains(t, anomalies, "eval_step_rollout")
	require.Contains(t, anomalies, "multi_turn_eval_banner")
}

func TestAnalyzeGrokCompletedResponse_CharsAndHint(t *testing.T) {
	raw := []byte(`{
		"sequence_number": 7431,
		"response": {
			"output": [
				{"type":"reasoning","content":[{"type":"reasoning_text","text":"short reasoning"}]},
				{"type":"message","content":[{"type":"output_text","text":"REASONING_LIVE_1\n\n【步骤 2】"}]}
			]
		}
	}`)
	analysis := analyzeGrokCompletedResponse(raw)
	require.Equal(t, 7431, int(analysis.SequenceNumber))
	require.Greater(t, analysis.OutputTextChars, 0)
	require.Greater(t, analysis.ReasoningTextChars, 0)
	require.Contains(t, analysis.Anomalies, "eval_step_rollout")
	require.Equal(t, "upstream_eval_rollout_regurgitation", grokAnomalyRootCauseHint(analysis.Anomalies, analysis.OutputTextChars, analysis.ReasoningTextChars))
}

func TestGrokCollapseStrategyName(t *testing.T) {
	recallInput := []any{
		map[string]any{"role": "user", "content": "我刚才的结果是多少"},
	}
	require.Equal(t, "recall_dialogue", grokCollapseStrategyName(recallInput))

	trailingInput := []any{
		map[string]any{"role": "user", "content": "hello"},
		map[string]any{"role": "assistant", "content": "hi"},
		map[string]any{"role": "user", "content": "下一个问题"},
	}
	require.Equal(t, "trailing_user_only", grokCollapseStrategyName(trailingInput))
}