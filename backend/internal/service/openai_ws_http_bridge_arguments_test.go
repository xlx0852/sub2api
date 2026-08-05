package service

import (
	"encoding/json"
	"testing"
)

// 多轮会话重放时 function_call 缺 arguments 会触发上游 400
// "Missing required parameter: 'input[N].arguments'"（issue 场景）。
// prepareOpenAIWSHTTPBridgeBody 应给所有 function_call 系列项兜底补 "{}"。
func TestPrepareOpenAIWSHTTPBridgeBodyEnsuresArguments(t *testing.T) {
	payload := []byte(`{
		"type": "response.create",
		"model": "gpt-5.6-luna",
		"input": [
			{"type": "function_call", "call_id": "call_1", "name": "Read"},
			{"type": "function_call", "call_id": "call_2", "name": "Write", "arguments": ""},
			{"type": "function_call", "call_id": "call_3", "name": "Search", "arguments": "{}"},
			{"type": "function_call", "call_id": "call_4", "name": "Exec", "arguments": {"cmd": "ls"}},
			{"type": "function_call_output", "call_id": "call_1", "output": "ok"},
			{"type": "message", "role": "user", "content": "hi"}
		]
	}`)
	body, err := prepareOpenAIWSHTTPBridgeBody(payload)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("output not json: %v", err)
	}
	items := parsed["input"].([]any)
	argsByCall := map[string]any{}
	for _, raw := range items {
		item := raw.(map[string]any)
		if item["type"] == "function_call" {
			argsByCall[item["call_id"].(string)] = item["arguments"]
		}
	}
	// 缺字段 → 补 "{}"
	if got, ok := argsByCall["call_1"].(string); !ok || got != "{}" {
		t.Fatalf("call_1 (missing) arguments=%v, want \"{}\"", argsByCall["call_1"])
	}
	// 空字符串 → 补 "{}"
	if got, ok := argsByCall["call_2"].(string); !ok || got != "{}" {
		t.Fatalf("call_2 (empty) arguments=%v, want \"{}\"", argsByCall["call_2"])
	}
	// 已有 "{}" → 保持
	if got, ok := argsByCall["call_3"].(string); !ok || got != "{}" {
		t.Fatalf("call_3 arguments=%v, want \"{}\"", argsByCall["call_3"])
	}
	// 对象形态（新版 codex）→ 保持
	if _, ok := argsByCall["call_4"].(map[string]any); !ok {
		t.Fatalf("call_4 object arguments mutated: %v", argsByCall["call_4"])
	}
}
