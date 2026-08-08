//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractOpenAIResponseModelFromJSONBytes(t *testing.T) {
	// Responses API: response.model 优先
	body := []byte(`{"id":"resp_1","response":{"id":"resp_1","model":"gpt-5.6-luna"},"model":"gpt-5.6-sol"}`)
	require.Equal(t, "gpt-5.6-luna", extractOpenAIResponseModelFromJSONBytes(body))

	// chat.completions: 顶层 model
	body = []byte(`{"id":"chatcmpl_1","model":"gpt-5.6-sol"}`)
	require.Equal(t, "gpt-5.6-sol", extractOpenAIResponseModelFromJSONBytes(body))

	// 无 model 字段 → 空
	require.Equal(t, "", extractOpenAIResponseModelFromJSONBytes([]byte(`{"id":"x"}`)))
	// 非法 JSON → 空
	require.Equal(t, "", extractOpenAIResponseModelFromJSONBytes([]byte(`{not-json`)))
	// 空 body → 空
	require.Equal(t, "", extractOpenAIResponseModelFromJSONBytes(nil))
}
