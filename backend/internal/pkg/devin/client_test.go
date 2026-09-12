package devin

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	devinpb "github.com/Wei-Shaw/sub2api/internal/pkg/devin/proto"
)

func TestBuildRequestPreservesConversationAndTools(t *testing.T) {
	client, err := NewClient("https://example.test", "devin-session-token$test", "glm-5-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	req, err := client.BuildRequest(&apicompat.ChatCompletionsRequest{
		Model: "glm-5-2",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: json.RawMessage(`"system"`)},
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
		Tools: []apicompat.ChatTool{{Type: "function", Function: &apicompat.ChatFunction{Name: "exec", Description: "run command", Parameters: json.RawMessage(`{"type":"object"}`)}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.GetChatModelUid() != "glm-5-2" || req.GetRequestType() != devinpb.ChatMessageRequestType_CHAT_MESSAGE_REQUEST_TYPE_CASCADE {
		t.Fatalf("request identity = %q/%v", req.GetChatModelUid(), req.GetRequestType())
	}
	if len(req.GetChatMessagePrompts()) != 2 || req.GetChatMessagePrompts()[1].GetPrompt() != "hello" {
		t.Fatalf("prompts = %#v", req.GetChatMessagePrompts())
	}
	if len(req.GetTools()) != 1 || req.GetTools()[0].GetName() != "exec" || req.GetTools()[0].GetJsonSchemaString() != `{"type":"object"}` {
		t.Fatalf("tools = %#v", req.GetTools())
	}
	if req.GetMetadata().GetApiKey() != "devin-session-token$test" {
		t.Fatalf("metadata token = %q", req.GetMetadata().GetApiKey())
	}
}

func TestDecodeResponseMapsThinkingToolUsageAndStop(t *testing.T) {
	call := &devinpb.ChatToolCall{Id: "call-1", Name: "exec", ArgumentsJson: `{"command":"ls"}`}
	delta := DecodeResponse(&devinpb.GetChatMessageResponse{
		DeltaThinking:  "inspect",
		DeltaText:      "done",
		DeltaToolCalls: []*devinpb.ChatToolCall{call},
		Usage:          &devinpb.ModelUsageStats{InputTokens: 10, OutputTokens: 4, CacheReadTokens: 2},
		StopReason:     devinpb.StopReason_STOP_REASON_FUNCTION_CALL,
	}, 0)
	if delta.Reasoning != "inspect" || delta.Text != "done" || delta.ToolCallID != "call-1" || delta.ToolName != "exec" || delta.FinishReason != "tool_calls" {
		t.Fatalf("delta = %#v", delta)
	}
	if delta.Usage == nil || delta.Usage.InputTokens != 10 || delta.Usage.CacheReadTokens != 2 {
		t.Fatalf("usage = %#v", delta.Usage)
	}
}
