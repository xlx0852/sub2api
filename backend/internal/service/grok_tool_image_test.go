package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Regression: xAI's ModelInput cannot attach an image to a tool result, so the translation used
// to drop the pixels and leave "[image]" in the tool output. The model then answered about an
// image it never saw (a live run hallucinated "5 rows / nathan.k@example.net" for a 17-row
// screenshot). Tool images must be re-sent as a follow-up user message.
func TestGrokToolResultImagesAreForwardedAsAUserMessage(t *testing.T) {
	dataURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUg=="
	item := map[string]any{
		"type":    "function_call_output",
		"call_id": "call_1",
		"output": []any{
			map[string]any{"type": "input_text", "text": "screenshot read"},
			map[string]any{"type": "input_image", "image_url": dataURL},
		},
	}
	items := convertGrokFunctionCallOutputItem(item)
	require.Len(t, items, 2, "tool result plus an image carrier message")

	tool := items[0]
	require.Equal(t, "function_call_output", tool["type"])
	require.Equal(t, "call_1", tool["call_id"])
	require.Equal(t, "screenshot read", tool["output"], "text stays in the tool result")

	carrier := items[1]
	require.Equal(t, "user", carrier["role"])
	content, ok := carrier["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	image, ok := content[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "input_image", image["type"])
	require.Equal(t, dataURL, image["image_url"], "the data URL must survive verbatim")
}

func TestGrokToolResultWithoutImagesStillProducesOneItem(t *testing.T) {
	items := convertGrokFunctionCallOutputItem(map[string]any{
		"type": "function_call_output", "call_id": "call_2", "output": "plain text result",
	})
	require.Len(t, items, 1)
	require.Equal(t, "plain text result", items[0]["output"])
}

func TestGrokToolResultImageWithoutURLKeepsContinuityMarker(t *testing.T) {
	// Nothing usable to forward: the marker must remain so the tool chain is obviously intact.
	items := convertGrokFunctionCallOutputItem(map[string]any{
		"type": "function_call_output", "call_id": "call_3",
		"output": []any{map[string]any{"type": "input_image"}},
	})
	require.Len(t, items, 1)
	require.Equal(t, "[image]", items[0]["output"])
}

func TestGrokToolResultImageAcceptsNestedURLObject(t *testing.T) {
	items := convertGrokFunctionCallOutputItem(map[string]any{
		"type": "function_call_output", "call_id": "call_4",
		"output": []any{map[string]any{
			"type":      "input_image",
			"image_url": map[string]any{"url": "https://example.test/a.png"},
		}},
	})
	require.Len(t, items, 2)
	content := items[1]["content"].([]any)
	require.Equal(t, "https://example.test/a.png", content[1].(map[string]any)["image_url"])
}
