package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeCollectedPartsToResponse_TextMerging(t *testing.T) {
	initial := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{"text": ""},
					},
					"role": "model",
				},
				"finishReason": "STOP",
			},
		},
	}

	collected := []map[string]any{
		{"text": "Hello, "},
		{"text": "world!"},
	}

	result := mergeCollectedPartsToResponse(initial, collected)
	parts := extractGeminiParts(result)
	require.Len(t, parts, 1)
	require.Equal(t, "Hello, world!", parts[0]["text"])
}

func TestMergeCollectedPartsToResponse_FunctionCallPreserved(t *testing.T) {
	initial := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{"text": ""},
					},
					"role": "model",
				},
				"finishReason": "STOP",
			},
		},
	}

	collected := []map[string]any{
		{
			"functionCall": map[string]any{
				"name": "get_weather",
				"args": map[string]any{"city": "Tokyo"},
			},
			"thoughtSignature": "sig_fc_12345",
		},
	}

	result := mergeCollectedPartsToResponse(initial, collected)
	parts := extractGeminiParts(result)
	require.Len(t, parts, 1)
	fc, ok := parts[0]["functionCall"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "get_weather", fc["name"])
	require.Equal(t, "sig_fc_12345", parts[0]["thoughtSignature"])
}

func TestMergeCollectedPartsToResponse_ThoughtAndTextMerged(t *testing.T) {
	initial := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"role": "model",
				},
				"finishReason": "STOP",
			},
		},
	}

	collected := []map[string]any{
		{"text": "Let me ", "thought": true},
		{"text": "think about this.", "thought": true, "thoughtSignature": "sig_thought_abc"},
		{"text": "Here is ", "thought": false},
		{"text": "the solution.", "thought": false},
	}

	result := mergeCollectedPartsToResponse(initial, collected)
	parts := extractGeminiParts(result)
	require.Len(t, parts, 2)

	// Part 0: Thought
	require.Equal(t, "Let me think about this.", parts[0]["text"])
	require.Equal(t, true, parts[0]["thought"])
	require.Equal(t, "sig_thought_abc", parts[0]["thoughtSignature"])

	// Part 1: Regular text
	require.Equal(t, "Here is the solution.", parts[1]["text"])
	require.Nil(t, parts[1]["thought"])
}

func TestMergeCollectedPartsToResponse_ThoughtFunctionCallAndText(t *testing.T) {
	initial := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"role": "model",
				},
				"finishReason": "STOP",
			},
		},
	}

	collected := []map[string]any{
		{"text": "Checking function calling...", "thought": true, "thoughtSignature": "sig_t"},
		{
			"functionCall": map[string]any{
				"name": "lookup",
				"args": map[string]any{"q": "test"},
			},
			"thoughtSignature": "sig_fc",
		},
	}

	result := mergeCollectedPartsToResponse(initial, collected)
	parts := extractGeminiParts(result)
	require.Len(t, parts, 2)

	// Part 0: Thought
	require.Equal(t, "Checking function calling...", parts[0]["text"])
	require.Equal(t, true, parts[0]["thought"])
	require.Equal(t, "sig_t", parts[0]["thoughtSignature"])

	// Part 1: Function call
	require.NotNil(t, parts[1]["functionCall"])
	require.Equal(t, "sig_fc", parts[1]["thoughtSignature"])
}
