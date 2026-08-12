package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSanitizeOpenAIResponsesInputItemIDs(t *testing.T) {
	body := []byte(`{"input":[{"type":"reasoning","id":"item_bad","summary":[]},{"type":"reasoning","id":"rs_ok","summary":[]},{"type":"message","id":"item_bad"},{"type":"function_call","id":"item_bad"},{"type":"web_search_call","id":"item_allowed"}]}`)
	sanitized, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "input.0.id").Exists())
	require.Equal(t, "rs_ok", gjson.GetBytes(sanitized, "input.1.id").String())
	require.False(t, gjson.GetBytes(sanitized, "input.2.id").Exists())
	require.False(t, gjson.GetBytes(sanitized, "input.3.id").Exists())
	require.Equal(t, "item_allowed", gjson.GetBytes(sanitized, "input.4.id").String())
}
