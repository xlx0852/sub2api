//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokSupportsNativeImageGeneration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		want  bool
	}{
		{model: "", want: false},
		{model: "grok-4.5", want: false},
		{model: "xai/grok-4.5", want: false},
		{model: "grok-4.20-0309-reasoning", want: false},
		{model: "grok-4.6", want: true},
		{model: "xai/grok-4.6", want: true},
		{model: "grok-4.6-beta", want: true},
		{model: "grok-4.7", want: true},
		{model: "xai/grok-4.7", want: true},
		{model: "grok-5", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			require.Equal(t, tt.want, grokSupportsNativeImageGeneration(tt.model))
		})
	}
}

func TestPatchGrokResponsesBodyImageGenerationByModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		model    string
		wantKeep bool
	}{
		{name: "missing model still strips", model: "", wantKeep: false},
		{name: "grok-4.5 strips", model: "grok-4.5", wantKeep: false},
		{name: "grok-4.20 strips", model: "grok-4.20-0309-reasoning", wantKeep: false},
		{name: "grok-4.6 keeps", model: "grok-4.6", wantKeep: true},
		{name: "grok-4.7 keeps", model: "grok-4.7", wantKeep: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte("{\"model\":\"" + tt.model + "\",\"input\":\"draw\",\"tools\":[{\"type\":\"image_generation\",\"action\":\"generate\"},{\"type\":\"web_search\"}]}")
			patched, err := patchGrokResponsesBody(body, tt.model)
			require.NoError(t, err)
			require.True(t, json.Valid(patched))
			require.True(t, gjson.GetBytes(patched, "tools.#(type==\"web_search\")").Exists())
			require.Equal(t, tt.wantKeep, gjson.GetBytes(patched, "tools.#(type==\"image_generation\")").Exists())
			if tt.wantKeep {
				require.Equal(t, "generate", gjson.GetBytes(patched, "tools.#(type==\"image_generation\").action").String())
			}
		})
	}
}

func TestPatchGrokResponsesBodyRewritesForcedImageGenerationChoice(t *testing.T) {
	t.Parallel()

	body := []byte("{\"model\":\"grok-4.6\",\"input\":\"draw a cat\",\"tools\":[{\"type\":\"image_generation\",\"action\":\"generate\"},{\"type\":\"web_search\"}],\"tool_choice\":{\"type\":\"image_generation\"}}")
	patched, err := patchGrokResponsesBody(body, "grok-4.6")
	require.NoError(t, err)
	require.True(t, json.Valid(patched))
	require.Equal(t, "required", gjson.GetBytes(patched, "tool_choice").String())
	require.Len(t, gjson.GetBytes(patched, "tools").Array(), 1)
	require.Equal(t, "image_generation", gjson.GetBytes(patched, "tools.0.type").String())
	require.Equal(t, "generate", gjson.GetBytes(patched, "tools.0.action").String())
}

func TestPatchGrokResponsesBodyDropsOrphanedImageGenerationChoice(t *testing.T) {
	t.Parallel()

	body := []byte("{\"model\":\"grok-4.5\",\"input\":\"draw a cat\",\"tools\":[{\"type\":\"image_generation\"},{\"type\":\"web_search\"}],\"tool_choice\":{\"type\":\"image_generation\"}}")
	patched, err := patchGrokResponsesBody(body, "grok-4.5")
	require.NoError(t, err)
	require.True(t, json.Valid(patched))
	require.True(t, gjson.GetBytes(patched, "tools.#(type==\"web_search\")").Exists())
	require.False(t, gjson.GetBytes(patched, "tools.#(type==\"image_generation\")").Exists())
	require.False(t, gjson.GetBytes(patched, "tool_choice").Exists())
}

func TestPatchGrokResponsesBodyRewritesImageOnlyAllowedToolsChoice(t *testing.T) {
	t.Parallel()

	body := []byte("{\"model\":\"grok-4.6\",\"input\":\"draw a cat\",\"tools\":[{\"type\":\"image_generation\"},{\"type\":\"web_search\"}],\"tool_choice\":{\"type\":\"allowed_tools\",\"mode\":\"required\",\"tools\":[{\"type\":\"image_generation\"}]}}")
	patched, err := patchGrokResponsesBody(body, "grok-4.6")
	require.NoError(t, err)
	require.True(t, json.Valid(patched))
	require.Equal(t, "required", gjson.GetBytes(patched, "tool_choice").String())
	require.Len(t, gjson.GetBytes(patched, "tools").Array(), 1)
	require.Equal(t, "image_generation", gjson.GetBytes(patched, "tools.0.type").String())
}
