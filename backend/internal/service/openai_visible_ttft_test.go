package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIVisibleOutputClassification(t *testing.T) {
	require.False(t, openAIStreamDataStartsVisibleOutput(`{"type":"response.output_item.added","item":{"type":"message","content":[]}}`, "response.output_item.added"))
	require.False(t, openAIStreamDataStartsVisibleOutput(`{"type":"response.output_text.delta","delta":""}`, "response.output_text.delta"))
	require.True(t, openAIStreamDataStartsVisibleOutput(`{"type":"response.output_text.delta","delta":"hello"}`, "response.output_text.delta"))
	require.True(t, openAIStreamDataStartsVisibleOutput(`{"type":"response.image_generation_call.partial_image","partial_image_b64":"abc"}`, "response.image_generation_call.partial_image"))
}
