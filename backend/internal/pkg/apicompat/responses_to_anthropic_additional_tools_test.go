package apicompat

// Responses 工具摄取的两个线上形状回归测试（2026-09-25 生产问题）：
//
//  1. 新版 Codex（0.155+）**不发顶层 tools**，而是发一个 input 项
//     {"type":"additional_tools","role":"developer","tools":[…]}，
//     里面还套一层 {"type":"namespace","name":"functions","tools":[…]}。
//     Anthropic 上游那条路径原先不认这个形状 → 工具数变 0 → 模型拿不到工具定义，
//     只能按系统提示编造调用文本（deepseek 出 ||DSML|| 标记、kimi 出 |open| 标记、
//     glm 直接编造执行结果），Codex 报 “tool exec invoked with incompatible payload”。
//
//  2. 夹具 testdata/codex_additional_tools_request.json 里的 additional_tools 载体
//     是从**真实 Codex 请求体逐字节取出的**（不是手写形状）。这一点很重要：
//     本次 bug 之所以漏过既有单测，正是因为那些单测用手写的顶层 tools 形状。

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const codexAdditionalToolsFixture = "testdata/codex_additional_tools_request.json"

func loadCodexFixture(t *testing.T) *ResponsesRequest {
	t.Helper()
	raw, err := os.ReadFile(codexAdditionalToolsFixture)
	require.NoError(t, err, "夹具缺失：%s", codexAdditionalToolsFixture)
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal(raw, &req))
	return &req
}

// 核心回归：真实 Codex 形状（additional_tools + namespace）必须被摄取为 Anthropic 工具。
func TestResponsesToAnthropicRequest_IngestsRealCodexAdditionalTools(t *testing.T) {
	req := loadCodexFixture(t)
	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)

	names := make([]string, 0, len(out.Tools))
	for _, tool := range out.Tools {
		names = append(names, tool.Name)
	}

	// 夹具里的三个子工具必须全部出现（exec 是 custom，另两个是 function）
	assert.Contains(t, names, "exec")
	assert.Contains(t, names, "wait")
	assert.Contains(t, names, "request_user_input")

	// namespace 包装本身不能作为工具发给上游
	assert.NotContains(t, names, "functions", "namespace 包装必须被展平，不能当工具发出去")

	// 工具名不得被改写（客户端按 name 匹配回执）
	for _, n := range names {
		assert.NotContains(t, n, ".", "工具名不应被加前缀：%q", n)
	}

	// 工具载体是声明、不是对话消息：messages 里只应剩那条真实用户消息
	require.Len(t, out.Messages, 1, "additional_tools 载体不得进入 messages")
	assert.Equal(t, "user", out.Messages[0].Role)
}

// 顶层 tools 与 additional_tools 并存时合并，且顶层在前。
func TestResponsesToAnthropicRequest_MergesTopLevelAndAdditionalTools(t *testing.T) {
	req := &ResponsesRequest{
		Model: "m",
		Tools: []ResponsesTool{{
			Type: "function", Name: "top_level_tool", Description: "d",
			Parameters: json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"}}}`),
		}},
		Input: json.RawMessage(`[
			{"type":"user","role":"user","content":[{"type":"input_text","text":"hi"}]},
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"function","name":"injected_tool","description":"d","parameters":{"type":"object","properties":{}}}
			]}
		]`),
	}
	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Tools, 2)
	assert.Equal(t, "top_level_tool", out.Tools[0].Name, "顶层 tools 应在前")
	assert.Equal(t, "injected_tool", out.Tools[1].Name, "摄取项应在后")
	require.Len(t, out.Messages, 1, "载体被剔除后只剩用户消息")
}

// 空载体：剔除载体，但不能清空顶层 tools。
func TestResponsesToAnthropicRequest_EmptyAdditionalToolsCarrier(t *testing.T) {
	req := &ResponsesRequest{
		Model: "m",
		Tools: []ResponsesTool{{Type: "function", Name: "kept", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}},
		Input: json.RawMessage(`[
			{"type":"additional_tools","role":"developer","tools":[]},
			{"type":"user","role":"user","content":[{"type":"input_text","text":"hi"}]}
		]`),
	}
	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Tools, 1)
	assert.Equal(t, "kept", out.Tools[0].Name, "空载体不得清空顶层 tools")
	require.Len(t, out.Messages, 1)
}

// 不含该形状的请求：工具语义与消息数量都不变。
func TestResponsesToAnthropicRequest_WithoutAdditionalToolsUnchanged(t *testing.T) {
	req := &ResponsesRequest{
		Model: "m",
		Tools: []ResponsesTool{{Type: "function", Name: "only", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}},
		Input: json.RawMessage(`[{"type":"user","role":"user","content":[{"type":"input_text","text":"hi"}]}]`),
	}
	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Tools, 1)
	assert.Equal(t, "only", out.Tools[0].Name)
	require.Len(t, out.Messages, 1)
}

// namespace 展平：tools 子键、children 子键、多层嵌套、名字不改写。
func TestPrepareResponsesTools_FlattensNamespaces(t *testing.T) {
	t.Run("tools 子键", func(t *testing.T) {
		flat, err := prepareResponsesTools([]ResponsesTool{{
			Type: "namespace", Name: "functions",
			Tools: []ResponsesTool{
				{Type: "custom", Name: "exec"},
				{Type: "function", Name: "wait"},
			},
		}})
		require.NoError(t, err)
		require.Len(t, flat, 2)
		assert.Equal(t, "exec", flat[0].Name)
		assert.Equal(t, "wait", flat[1].Name)
	})

	t.Run("children 子键", func(t *testing.T) {
		flat, err := prepareResponsesTools([]ResponsesTool{{
			Type: "namespace", Name: "functions",
			Children: []ResponsesTool{{Type: "function", Name: "via_children"}},
		}})
		require.NoError(t, err)
		require.Len(t, flat, 1)
		assert.Equal(t, "via_children", flat[0].Name)
	})

	t.Run("多层嵌套", func(t *testing.T) {
		flat, err := prepareResponsesTools([]ResponsesTool{{
			Type: "namespace", Name: "outer",
			Tools: []ResponsesTool{{
				Type: "namespace", Name: "inner",
				Tools: []ResponsesTool{{Type: "function", Name: "leaf"}},
			}},
		}})
		require.NoError(t, err)
		require.Len(t, flat, 1)
		assert.Equal(t, "leaf", flat[0].Name)
	})

	t.Run("同名冲突必须报错", func(t *testing.T) {
		_, err := prepareResponsesTools([]ResponsesTool{
			{Type: "namespace", Name: "a", Tools: []ResponsesTool{{Type: "function", Name: "dup"}}},
			{Type: "namespace", Name: "b", Tools: []ResponsesTool{{Type: "function", Name: "dup"}}},
		})
		require.Error(t, err, "同名工具静默覆盖会让其中一个永远调用不到")
		assert.Contains(t, err.Error(), "dup")
	})
}

// 展平后的 namespace 不能再被当成工具发给上游（兑底路径）。
func TestConvertResponsesToAnthropicTools_NamespaceNotEmittedAsTool(t *testing.T) {
	tools := convertResponsesToAnthropicTools([]ResponsesTool{{
		Type: "namespace", Name: "functions",
		Tools: []ResponsesTool{{Type: "function", Name: "inner", Parameters: json.RawMessage(`{"type":"object","properties":{}}`)}},
	}})
	require.Len(t, tools, 1)
	assert.Equal(t, "inner", tools[0].Name)
}
