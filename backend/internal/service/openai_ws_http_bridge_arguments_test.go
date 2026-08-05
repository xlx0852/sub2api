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

// 多轮重放会把 codex 私有工具类型原样注入 input，OpenAI 官方上游不认识这些
// 私有 type（local_shell_call/custom_tool_call/mcp_tool_call 等），报
// "Unknown parameter: 'input[N].arguments'"。桥接 body 应归一化为
// function_call / function_call_output。
func TestPrepareOpenAIWSHTTPBridgeBodyNormalizesPrivateToolTypes(t *testing.T) {
	payload := []byte(`{
		"type": "response.create",
		"model": "gpt-5.6-luna",
		"input": [
			{"type": "message", "role": "user", "content": "run ls"},
			{"type": "local_shell_call", "call_id": "sh_1", "name": "Shell", "arguments": "{\"cmd\": \"ls\"}"},
			{"type": "local_shell_call_output", "call_id": "sh_1", "output": "total 8"},
			{"type": "custom_tool_call", "call_id": "ct_1", "name": "Freeform", "input": "do something"},
			{"type": "custom_tool_call_output", "call_id": "ct_1", "output": "done"},
			{"type": "mcp_tool_call", "call_id": "mcp_1", "name": "McpTool", "arguments": "{}"},
			{"type": "mcp_tool_call_output", "call_id": "mcp_1", "output": "ok"},
			{"type": "tool_search_call", "call_id": "ts_1", "name": "Search", "arguments": "{}"},
			{"type": "tool_search_output", "call_id": "ts_1", "output": "results"}
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
	// 按 call_id + type 分类断言（local_shell_call 与 output 共用 call_id）
	typeByCall := map[string][]map[string]any{}
	for _, raw := range items {
		item := raw.(map[string]any)
		if cid, _ := item["call_id"].(string); cid != "" {
			typeByCall[cid] = append(typeByCall[cid], item)
		}
	}
	// local_shell_call → function_call（arguments 保留）
	if items := typeByCall["sh_1"]; len(items) != 2 {
		t.Fatalf("sh_1 items = %d, want 2", len(items))
	} else {
		var call *map[string]any
		for i := range items {
			if items[i]["type"] == "function_call" {
				call = &items[i]
			}
		}
		if call == nil {
			t.Fatalf("local_shell_call not normalized to function_call: %v", items)
		}
		if (*call)["arguments"] != `{"cmd": "ls"}` {
			t.Fatalf("local_shell_call arguments lost: %v", (*call)["arguments"])
		}
	}
	// custom_tool_call → function_call，input 包进 arguments.input
	if items := typeByCall["ct_1"]; len(items) == 2 {
		var call *map[string]any
		for i := range items {
			if items[i]["type"] == "function_call" {
				call = &items[i]
			}
		}
		if call == nil {
			t.Fatalf("custom_tool_call not normalized: %v", items)
		}
		args, _ := (*call)["arguments"].(string)
		var argsObj map[string]any
		if err := json.Unmarshal([]byte(args), &argsObj); err != nil || argsObj["input"] != "do something" {
			t.Fatalf("custom_tool_call input not wrapped into arguments: %v", (*call)["arguments"])
		}
		if _, has := (*call)["input"]; has {
			t.Fatalf("custom_tool_call input field not removed")
		}
	} else {
		t.Fatalf("ct_1 items = %d, want 2", len(items))
	}
	// mcp/tool_search 私有类型 → function_call
	for _, cid := range []string{"mcp_1", "ts_1"} {
		items := typeByCall[cid]
		if len(items) != 2 {
			t.Fatalf("%s items = %d, want 2", cid, len(items))
		}
		found := false
		for i := range items {
			if items[i]["type"] == "function_call" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s not normalized to function_call", cid)
		}
	}
	// 所有输出项归一化为 function_call_output
	for _, cid := range []string{"sh_1", "ct_1", "mcp_1", "ts_1"} {
		found := false
		for _, raw := range items {
			item := raw.(map[string]any)
			if item["call_id"] == cid && item["type"] == "function_call_output" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("output for %s not normalized to function_call_output", cid)
		}
	}
}
