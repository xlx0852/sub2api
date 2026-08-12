package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractOpenAIUsageNestedDataEnvelope(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{"data":{"usage":{"input_tokens":3,"output_tokens":2}}}`))
	require.True(t, ok)
	require.Equal(t, 3, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)
}

func TestExtractOpenAIUsagePreservesResponsePrecedence(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{"response":{"usage":{"input_tokens":7}},"data":{"usage":{"input_tokens":9}}}`))
	require.True(t, ok)
	require.Equal(t, 7, usage.InputTokens)
}
