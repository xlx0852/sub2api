package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanJSONSchema_ConstField(t *testing.T) {
	inputJSON := `{
		"type": "object",
		"properties": {
			"filter": {
				"type": "object",
				"properties": {
					"kind": {
						"description": "the kind of target",
						"const": "file"
					},
					"count": {
						"const": 10
					},
					"active": {
						"const": true
					}
				}
			}
		}
	}`

	var input map[string]any
	require.NoError(t, json.Unmarshal([]byte(inputJSON), &input))

	cleaned := CleanJSONSchema(input)
	cleanedJSON, err := json.Marshal(cleaned)
	require.NoError(t, err)

	t.Logf("Cleaned JSON: %s", string(cleanedJSON))

	// Verify "const" is completely removed
	require.NotContains(t, string(cleanedJSON), `"const"`)

	filter, ok := cleaned["properties"].(map[string]any)["filter"].(map[string]any)
	require.True(t, ok)
	props, ok := filter["properties"].(map[string]any)
	require.True(t, ok)

	// String const
	kind, ok := props["kind"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "string", kind["type"])
	require.Equal(t, []any{"file"}, kind["enum"])
	require.Equal(t, "the kind of target", kind["description"])

	// Int const
	count, ok := props["count"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "number", count["type"]) // json unmarshals numbers to float64
	require.Equal(t, []any{"10"}, count["enum"])

	// Bool const
	active, ok := props["active"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "boolean", active["type"])
	require.Equal(t, []any{"true"}, active["enum"])
}

func TestCleanJSONSchema_DefaultField(t *testing.T) {
	inputJSON := `{
		"type": "object",
		"properties": {
			"mode": {
				"type": "string",
				"default": "fast"
			}
		}
	}`

	var input map[string]any
	require.NoError(t, json.Unmarshal([]byte(inputJSON), &input))

	cleaned := CleanJSONSchema(input)
	cleanedJSON, err := json.Marshal(cleaned)
	require.NoError(t, err)

	require.NotContains(t, string(cleanedJSON), `"default"`)
	mode := cleaned["properties"].(map[string]any)["mode"].(map[string]any)
	require.Contains(t, mode["description"], "[Constraint: default: fast]")
}

func TestCleanJSONSchema_ArrayMissingItems(t *testing.T) {
	inputJSON := `{
		"type": "object",
		"properties": {
			"tags": {
				"type": "array"
			}
		}
	}`

	var input map[string]any
	require.NoError(t, json.Unmarshal([]byte(inputJSON), &input))

	cleaned := CleanJSONSchema(input)
	tags := cleaned["properties"].(map[string]any)["tags"].(map[string]any)
	require.Equal(t, "array", tags["type"])
	items, ok := tags["items"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "string", items["type"])
}

func TestCleanJSONSchema_EmptyObject(t *testing.T) {
	inputJSON := `{
		"type": "object",
		"properties": {
			"meta": {
				"type": "object"
			}
		}
	}`

	var input map[string]any
	require.NoError(t, json.Unmarshal([]byte(inputJSON), &input))

	cleaned := CleanJSONSchema(input)
	meta := cleaned["properties"].(map[string]any)["meta"].(map[string]any)
	require.Equal(t, "object", meta["type"])
	props, ok := meta["properties"].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, props)
}

