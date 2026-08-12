package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesCompletedEventIsEmpty(t *testing.T) {
	require.True(t, openAIResponsesCompletedEventIsEmpty([]byte(`{"type":"response.completed","response":{"output":[]}}`), &OpenAIUsage{}))
	require.False(t, openAIResponsesCompletedEventIsEmpty([]byte(`{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":1}}}`), &OpenAIUsage{}))
	require.False(t, openAIResponsesCompletedEventIsEmpty([]byte(`{"type":"response.completed","response":{"output":[{"type":"message"}]}}`), &OpenAIUsage{}))
}
