package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 账号测试的 system 提示词必须按模型区分：只给 Claude 模型注入 Claude Code 身份。
// 给非 Claude 模型发 "You are Claude Code" 是在测试里伪造身份，
// 会让「连通性 / 降智」判定测到的不是真实模型行为。
func TestTestPayloadSystemPrompt_OnlyClaudeModelsGetClaudeCodeIdentity(t *testing.T) {
	claudeModels := []string{
		"claude-sonnet-4-5",
		"claude-3-5-haiku-20241022",
		"claude-opus-4-1",
		"anthropic/claude-sonnet-4",
		"  Claude-Sonnet-4  ",
	}
	for _, model := range claudeModels {
		require.Equal(t, claudeCodeSystemPrompt, testPayloadSystemPrompt(model),
			"Claude 模型必须保留 Claude Code 身份（真实 Claude 上游与校验依赖它）: %s", model)
	}

	nonClaudeModels := []string{
		"deepseek-v4.1-flash",
		"minimax-m3",
		"kimi-k3",
		"qwen3.8-flash",
		"gpt-5.6-luna",
		"glm-5.3",
		// 名字里含 claude 但不以 claude 开头，不应被误判
		"notclaude-ish",
		"vendor/notclaude",
	}
	for _, model := range nonClaudeModels {
		require.Equal(t, accountTestNeutralSystemPrompt, testPayloadSystemPrompt(model),
			"非 Claude 模型不应拿到 Claude Code 身份: %s", model)
	}
}

// 更强的断言：非 Claude 模型的整个请求体里不能出现任何 Claude 身份文案，
// 避免以后有人只改了一处 system 块却漏掉别处。
func TestCreateTestPayload_NonClaudePayloadHasNoClaudeIdentity(t *testing.T) {
	payload, err := createTestPayload("deepseek-v4.1-flash")
	require.NoError(t, err)

	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	body := string(raw)
	require.NotContains(t, body, "Claude Code")
	require.NotContains(t, body, "Anthropic's official CLI")
	require.Contains(t, body, accountTestNeutralSystemPrompt)
	// 探针题目与非 Claude 身份判定是两件事，题目必须照发。
	require.Contains(t, body, "糖果")
}

// Claude 模型的请求体保持既有形态，不应被这次改动影响。
func TestCreateTestPayload_ClaudePayloadKeepsClaudeCodeIdentity(t *testing.T) {
	payload, err := createTestPayload("claude-sonnet-4-5")
	require.NoError(t, err)

	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	body := string(raw)
	require.Contains(t, body, claudeCodeSystemPrompt)
	require.NotContains(t, body, accountTestNeutralSystemPrompt)
	require.Contains(t, body, "糖果")
}

// system 块必须始终存在且只有一个，避免"非 Claude 就把 system 整个删掉"的写法
// 在不同上游上产生形态差异（Anthropic 协议允许无 system，但我们这里保持形态稳定）。
func TestCreateTestPayload_AlwaysSendsExactlyOneSystemBlock(t *testing.T) {
	for _, model := range []string{"claude-sonnet-4-5", "deepseek-v4.1-flash"} {
		payload, err := createTestPayload(model)
		require.NoError(t, err)

		blocks, ok := payload["system"].([]map[string]any)
		require.True(t, ok, "%s: system 应为块数组", model)
		require.Len(t, blocks, 1, "%s: system 块数应为 1", model)

		text, _ := blocks[0]["text"].(string)
		require.NotEmpty(t, text, "%s: system 文本不应为空", model)
		require.NotContains(t, text, "  ", "%s: system 文本不应有多余空白", model)
		require.Equal(t, text, strings.TrimSpace(text))
	}
}
