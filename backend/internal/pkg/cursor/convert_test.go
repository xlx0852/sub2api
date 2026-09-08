package cursor

import (
	"encoding/json"
	"testing"

	cursorpb "github.com/Wei-Shaw/sub2api/internal/pkg/cursor/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapOpenAIRequestToProto_Basic(t *testing.T) {
	req := &OpenAIChatRequest{
		Model: "grok-4.5",
		Messages: []OpenAIMessage{
			{
				Role:    "system",
				Content: json.RawMessage(`"You are a helpful assistant."`),
			},
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello!"`),
			},
		},
	}

	protoReq, err := MapOpenAIRequestToProto(req)
	require.NoError(t, err)
	assert.Equal(t, "grok-4.5", *protoReq.ModelId)
	assert.True(t, protoReq.RequestedModel.MaxMode)
	require.Len(t, protoReq.RequestedModel.Parameters, 2)
	assert.Equal(t, "effort", protoReq.RequestedModel.Parameters[0].Id)
	assert.Equal(t, "high", protoReq.RequestedModel.Parameters[0].Value)

	require.Len(t, protoReq.Messages, 2)
	assert.Equal(t, cursorpb.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_SYSTEM, protoReq.Messages[0].Role)
	assert.Equal(t, "You are a helpful assistant.", protoReq.Messages[0].GetText())

	assert.Equal(t, cursorpb.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER, protoReq.Messages[1].Role)
	assert.Equal(t, "Hello!", protoReq.Messages[1].GetText())
}

func TestMapOpenAIRequestToProto_ToolsAndToolCalls(t *testing.T) {
	req := &OpenAIChatRequest{
		Model: "default",
		Tools: []OpenAITool{
			{
				Type: "function",
				Function: OpenAIFunction{
					Name:        "get_weather",
					Description: "Get weather for city",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
				},
			},
		},
		Messages: []OpenAIMessage{
			{
				Role:    "user",
				Content: json.RawMessage(`"What is the weather in Tokyo?"`),
			},
			{
				Role:    "assistant",
				Content: json.RawMessage(`""`),
				ToolCalls: []OpenAIToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: OpenAIFunctionCallDesc{
							Name:      "get_weather",
							Arguments: `{"city":"Tokyo"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				ToolCallID: "call_123",
				Content:    json.RawMessage(`"{\"temp\": 25}"`),
			},
		},
	}

	protoReq, err := MapOpenAIRequestToProto(req)
	require.NoError(t, err)

	require.Len(t, protoReq.Tools, 1)
	assert.Equal(t, "get_weather", protoReq.Tools[0].Name)
	assert.Equal(t, "Get weather for city", protoReq.Tools[0].Description)

	require.Len(t, protoReq.Messages, 3)
	// Assistant
	asst := protoReq.Messages[1]
	assert.Equal(t, cursorpb.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT, asst.Role)
	require.Len(t, asst.ToolCalls, 1)
	assert.Equal(t, "call_123", asst.ToolCalls[0].ToolCallId)
	assert.Equal(t, "get_weather", asst.ToolCalls[0].ToolName)
	assert.Equal(t, "Tokyo", asst.ToolCalls[0].Args.Fields["city"].GetStringValue())

	// Tool result
	toolMsg := protoReq.Messages[2]
	assert.Equal(t, cursorpb.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL, toolMsg.Role)
	toolContent := toolMsg.GetToolContent()
	require.NotNil(t, toolContent)
	require.Len(t, toolContent.Parts, 1)
	assert.Equal(t, "call_123", toolContent.Parts[0].ToolCallId)
	assert.Equal(t, "get_weather", toolContent.Parts[0].ToolName)
}

func TestResolveRequestedModel(t *testing.T) {
	m, max, p := ResolveRequestedModel("grok-4.5")
	assert.Equal(t, "grok-4.5", m)
	assert.True(t, max)
	assert.Len(t, p, 2)

	m, max, p = ResolveRequestedModel("claude-3.5-sonnet+max")
	assert.Equal(t, "claude-3.5-sonnet", m)
	assert.True(t, max)
	assert.Nil(t, p)

	m, max, p = ResolveRequestedModel("gpt-4o")
	assert.Equal(t, "gpt-4o", m)
	assert.False(t, max)
	assert.Nil(t, p)
}
