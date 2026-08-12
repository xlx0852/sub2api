package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatReasoningAliasResponsesOutput(t *testing.T) {
	msg := ChatMessage{Reasoning: "alias reasoning", Content: json.RawMessage(`""`)}
	outputs := chatMessageToResponsesOutput(msg, nil)
	require.NotEmpty(t, outputs)
	require.Equal(t, "reasoning", outputs[0].Type)
	require.Equal(t, "alias reasoning", outputs[0].Summary[0].Text)
}

func TestChatReasoningContentTakesPrecedence(t *testing.T) {
	msg := ChatMessage{ReasoningContent: "canonical", Reasoning: "alias"}
	require.Equal(t, "canonical", msg.reasoningText())
}
