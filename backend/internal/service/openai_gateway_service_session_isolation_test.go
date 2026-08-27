package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsolateOpenAISessionID(t *testing.T) {
	t.Run("empty_raw_returns_empty", func(t *testing.T) {
		assert.Equal(t, "", isolateOpenAISessionID(1, ""))
		assert.Equal(t, "", isolateOpenAISessionID(1, "   "))
	})

	t.Run("deterministic", func(t *testing.T) {
		a := isolateOpenAISessionID(42, "sess_abc123")
		b := isolateOpenAISessionID(42, "sess_abc123")
		assert.Equal(t, a, b)
	})

	t.Run("different_apiKeyID_different_result", func(t *testing.T) {
		a := isolateOpenAISessionID(1, "same_session")
		b := isolateOpenAISessionID(2, "same_session")
		require.NotEqual(t, a, b, "不同 API Key 使用相同 session_id 应产生不同隔离值")
	})

	t.Run("different_raw_different_result", func(t *testing.T) {
		a := isolateOpenAISessionID(1, "session_a")
		b := isolateOpenAISessionID(1, "session_b")
		require.NotEqual(t, a, b)
	})

	t.Run("format_is_uuid", func(t *testing.T) {
		result := isolateOpenAISessionID(99, "test_session")
		// 官方 Codex 客户端的 session-id 是标准 UUID（36 字符带连字符），
		// 隔离值也应保持该形态，避免 16 位 hex 一眼暴露网关特征。
		assert.Len(t, result, 36, "应为 36 字符的 UUID 字符串")
		_, err := uuid.Parse(result)
		assert.NoError(t, err, "应为合法 UUID")
	})

	t.Run("zero_apiKeyID_still_works", func(t *testing.T) {
		result := isolateOpenAISessionID(0, "session")
		assert.NotEmpty(t, result)
		// apiKeyID=0 与 apiKeyID=1 应产生不同结果
		other := isolateOpenAISessionID(1, "session")
		assert.NotEqual(t, result, other)
	})
}
