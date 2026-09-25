package apicompat

// Anthropic → Responses 流式桥的「终止态文本完整性」回归测试。
//
// 背景（生产问题，2026-09-25）：该桥原先只发 delta，不保留任何文本累加器，
// 于是所有终止态都是空的：
//   - response.output_text.done            缺 Text
//   - response.output_item.done            只有 Type/ID/Status，没有 Content/Summary/Arguments
//   - response.completed.response.output   硬编码 []
//
// 后果：从 delta 拼装的客户端正常；但**从终止态定稿**的客户端（Codex CLI 读
// response.completed.output）拿到空 assistant 消息 —— 表现为「模型有输出 token，
// 但回答是空串」。上游是 Anthropic 协议、客户端要 Responses 的所有组合都受影响。
//
// 这些用例钉住四段一致性：delta 累积 == done 文本 == output_item.done 内容 ==
// response.completed.output。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectAnthropicToResponses 把一串 Anthropic 流式事件喂给状态机，返回全部产出的事件。
func collectAnthropicToResponses(evts ...*AnthropicStreamEvent) []ResponsesStreamEvent {
	state := NewAnthropicEventToResponsesState()
	var out []ResponsesStreamEvent
	for _, evt := range evts {
		out = append(out, AnthropicEventToResponsesEvents(evt, state)...)
	}
	out = append(out, FinalizeAnthropicResponsesStream(state)...)
	return out
}

func findEvent(evts []ResponsesStreamEvent, typ string) (ResponsesStreamEvent, bool) {
	for _, e := range evts {
		if e.Type == typ {
			return e, true
		}
	}
	return ResponsesStreamEvent{}, false
}

func outputItemsOf(evts []ResponsesStreamEvent, typ string) []ResponsesOutput {
	var items []ResponsesOutput
	for _, e := range evts {
		if e.Type == typ && e.Item != nil {
			items = append(items, *e.Item)
		}
	}
	return items
}

func contentText(item ResponsesOutput) string {
	var s string
	for _, c := range item.Content {
		s += c.Text
	}
	return s
}

func summaryText(item ResponsesOutput) string {
	var s string
	for _, p := range item.Summary {
		s += p.Text
	}
	return s
}

// 核心回归：文本流结束后，done / output_item.done / response.completed.output 都必须有文本。
func TestAnthropicToResponsesStream_TerminalArtifactsCarryText(t *testing.T) {
	evts := collectAnthropicToResponses(
		&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "deepseek-v4.1-flash"}},
		&AnthropicStreamEvent{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{Type: "text"}},
		&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "text_delta", Text: "4"}},
		&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "text_delta", Text: "2"}},
		&AnthropicStreamEvent{Type: "content_block_stop"},
		&AnthropicStreamEvent{Type: "message_delta", Usage: &AnthropicUsage{OutputTokens: 7}},
		&AnthropicStreamEvent{Type: "message_stop"},
	)

	// 1) delta 仍在
	var deltas string
	for _, e := range evts {
		if e.Type == "response.output_text.delta" {
			deltas += e.Delta
		}
	}
	assert.Equal(t, "42", deltas)

	// 2) output_text.done 带全文
	done, ok := findEvent(evts, "response.output_text.done")
	require.True(t, ok, "必须发出 response.output_text.done")
	assert.Equal(t, "42", done.Text, "output_text.done 必须带累积文本")

	// 3) output_item.done(message) 带 content
	items := outputItemsOf(evts, "response.output_item.done")
	require.Len(t, items, 1)
	assert.Equal(t, "message", items[0].Type)
	assert.Equal(t, "assistant", items[0].Role)
	assert.Equal(t, "completed", items[0].Status)
	assert.Equal(t, "42", contentText(items[0]), "output_item.done 的 message 必须带 content 文本")

	// 4) response.completed.output 完整（Codex 就是读这里）
	completed, ok := findEvent(evts, "response.completed")
	require.True(t, ok)
	require.NotNil(t, completed.Response)
	require.Len(t, completed.Response.Output, 1, "response.completed.output 不得为空")
	assert.Equal(t, "message", completed.Response.Output[0].Type)
	assert.Equal(t, "42", contentText(completed.Response.Output[0]))
}

// reasoning（thinking_delta）同样要进入终止态。
func TestAnthropicToResponsesStream_ReasoningTerminalText(t *testing.T) {
	evts := collectAnthropicToResponses(
		&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_2", Model: "m"}},
		&AnthropicStreamEvent{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{Type: "thinking"}},
		&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "The user asks "}},
		&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "2+2."}},
		&AnthropicStreamEvent{Type: "content_block_stop"},
		&AnthropicStreamEvent{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{Type: "text"}},
		&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "text_delta", Text: "4"}},
		&AnthropicStreamEvent{Type: "content_block_stop"},
		&AnthropicStreamEvent{Type: "message_stop"},
	)

	summaryDone, ok := findEvent(evts, "response.reasoning_summary_text.done")
	require.True(t, ok)
	assert.Equal(t, "The user asks 2+2.", summaryDone.Text)

	items := outputItemsOf(evts, "response.output_item.done")
	require.Len(t, items, 2, "reasoning + message 两个 item")
	assert.Equal(t, "reasoning", items[0].Type)
	assert.Equal(t, "The user asks 2+2.", summaryText(items[0]))
	assert.Equal(t, "message", items[1].Type)
	assert.Equal(t, "4", contentText(items[1]))

	completed, ok := findEvent(evts, "response.completed")
	require.True(t, ok)
	require.Len(t, completed.Response.Output, 2, "terminate 时必须带上 reasoning + message")
	assert.Equal(t, "reasoning", completed.Response.Output[0].Type)
	assert.Equal(t, "The user asks 2+2.", summaryText(completed.Response.Output[0]))
	assert.Equal(t, "4", contentText(completed.Response.Output[1]))
}

// function_call 的参数增量也要落进终止态（Codex 靠它决定是否执行工具）。
func TestAnthropicToResponsesStream_FunctionCallArguments(t *testing.T) {
	evts := collectAnthropicToResponses(
		&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_3", Model: "m"}},
		&AnthropicStreamEvent{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{
			Type: "tool_use", ID: "toolu_1", Name: "shell",
		}},
		&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"cmd":`}},
		&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `"ls"}`}},
		&AnthropicStreamEvent{Type: "content_block_stop"},
		&AnthropicStreamEvent{Type: "message_stop"},
	)

	argsDone, ok := findEvent(evts, "response.function_call_arguments.done")
	require.True(t, ok)
	assert.Equal(t, `{"cmd":"ls"}`, argsDone.Arguments)

	items := outputItemsOf(evts, "response.output_item.done")
	require.Len(t, items, 1)
	assert.Equal(t, "function_call", items[0].Type)
	assert.Equal(t, "shell", items[0].Name)
	assert.Equal(t, `{"cmd":"ls"}`, items[0].Arguments)

	completed, ok := findEvent(evts, "response.completed")
	require.True(t, ok)
	require.Len(t, completed.Response.Output, 1)
	assert.Equal(t, `{"cmd":"ls"}`, completed.Response.Output[0].Arguments)
}

// 空文本流仍应给出 output=[]（不能因为 nil 变成 null）。
func TestAnthropicToResponsesStream_EmptyStreamKeepsEmptyOutputArray(t *testing.T) {
	evts := collectAnthropicToResponses(
		&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_4", Model: "m"}},
		&AnthropicStreamEvent{Type: "message_stop"},
	)
	completed, ok := findEvent(evts, "response.completed")
	require.True(t, ok)
	require.NotNil(t, completed.Response)
	assert.NotNil(t, completed.Response.Output, "必须是空切片而非 nil")
	assert.Len(t, completed.Response.Output, 0)
}
