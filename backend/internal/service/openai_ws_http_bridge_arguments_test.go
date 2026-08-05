package service

import (
	"encoding/json"
	"strings"
	"testing"
)

// 客户端 payload 原样保留：codex 把 call_id 当作不透明关联键，发出原生
// custom_tool_call/custom_tool_call_output 对。prepareOpenAIWSHTTPBridgeBody
// 只剥 WS 信封字段，不归一化客户端输入（兼容适配只作用于重放项或上游显式
// 拒绝后）。
func TestPrepareOpenAIWSHTTPBridgeBodyPreservesClientInput(t *testing.T) {
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
	// 缺字段 → 保持缺失（不补）
	if _, exists := items[0].(map[string]any)["arguments"]; exists {
		t.Fatalf("missing client arguments must remain missing")
	}
	// 空字符串 → 保持空串
	if got := items[1].(map[string]any)["arguments"]; got != "" {
		t.Fatalf("empty client arguments mutated: %v", got)
	}
	// 已有 "{}" → 保持
	if got := items[2].(map[string]any)["arguments"]; got != "{}" {
		t.Fatalf("call_3 arguments=%v, want \"{}\"", got)
	}
	// 对象形态（新版 codex）→ 保持
	if _, ok := items[3].(map[string]any)["arguments"].(map[string]any); !ok {
		t.Fatalf("call_4 object arguments mutated: %v", items[3])
	}
}

// 客户端私有工具类型原样保留：codex 的 custom_tool_call / local_shell_call 等
// 原生形态不动（type、id、call_id 均保持），兼容适配只作用于服务端收集的
// 重放项。
func TestPrepareOpenAIWSHTTPBridgeBodyPreservesClientPrivateToolTypes(t *testing.T) {
	payload := []byte(`{
		"type": "response.create",
		"model": "gpt-5.6-luna",
		"input": [
			{"type": "message", "role": "user", "content": "run ls"},
			{"type": "local_shell_call", "id": "lc_ab1", "call_id": "sh_1", "name": "Shell", "arguments": "{\"cmd\": \"ls\"}"},
			{"type": "local_shell_call_output", "id": "ctco_019fd0d8", "call_id": "sh_1", "output": "total 8"},
			{"type": "custom_tool_call", "id": "ctc_cd2", "call_id": "ct_1", "name": "Freeform", "input": "do something"},
			{"type": "custom_tool_call_output", "id": "ctco_cd2", "call_id": "ct_1", "output": "done"},
			{"type": "mcp_tool_call", "id": "mcp_ef3", "call_id": "mcp_1", "name": "McpTool", "arguments": "{}"},
			{"type": "mcp_tool_call_output", "id": "ctco_ef3", "call_id": "mcp_1", "output": "ok"},
			{"type": "tool_search_call", "id": "tsc_gh4", "call_id": "ts_1", "name": "Search", "arguments": "{}"},
			{"type": "tool_search_output", "id": "tsco_gh4", "call_id": "ts_1", "output": "results"}
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
	typeByCall := map[string][]map[string]any{}
	for _, raw := range items {
		item := raw.(map[string]any)
		if cid, _ := item["call_id"].(string); cid != "" {
			typeByCall[cid] = append(typeByCall[cid], item)
		}
	}
	// local_shell_call 原样保留
	if items := typeByCall["sh_1"]; len(items) != 2 {
		t.Fatalf("sh_1 items = %d, want 2", len(items))
	} else {
		if items[0]["type"] != "local_shell_call" || items[0]["id"] != "lc_ab1" {
			t.Fatalf("local_shell_call mutated: %v", items[0])
		}
		if items[1]["type"] != "local_shell_call_output" {
			t.Fatalf("local_shell_call_output mutated: %v", items[1])
		}
	}
	// custom_tool_call 原样保留
	if items := typeByCall["ct_1"]; len(items) == 2 {
		if items[0]["type"] != "custom_tool_call" || items[0]["id"] != "ctc_cd2" {
			t.Fatalf("custom_tool_call mutated: %v", items[0])
		}
		if _, has := items[0]["input"]; !has {
			t.Fatalf("custom_tool_call input removed: %v", items[0])
		}
	}
	// mcp/tool_search 原样保留
	for _, cid := range []string{"mcp_1", "ts_1"} {
		items := typeByCall[cid]
		if len(items) != 2 {
			t.Fatalf("%s items = %d, want 2", cid, len(items))
		}
		if items[0]["type"] == "function_call" || items[1]["type"] == "function_call_output" {
			t.Fatalf("%s normalized (should preserve): %v", cid, items)
		}
	}
}

// call_id 映射：归一化后上游自生成 fc_ call_id，需按工具名还原为 codex call_id。
func TestBridgeCallIDMapperRestoresUpstreamCallID(t *testing.T) {
	body := []byte(`{
		"input": [
			{"type": "function_call", "name": "mcp_read", "call_id": "ctco_019fd0d8", "arguments": "{}"},
			{"type": "function_call", "name": "mcp_write", "call_id": "ctco_write01", "arguments": "{}"}
		]
	}`)
	m := newBridgeCallIDMapper(body)

	// 上游响应 function_call：观察映射（不改写）
	callEvent := []byte(`{"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","call_id":"fc_upstream_abc","name":"mcp_read","arguments":"{}"}}`)
	out, changed := m.restoreBridgeResponseCallIDs(callEvent)
	if changed {
		t.Fatalf("function_call observe should not rewrite, got %s", string(out))
	}
	// 上游响应 function_call_output：fc_upstream_abc 应还原为 ctco_019fd0d8
	outEvent := []byte(`{"type":"response.output_item.done","item":{"id":"fc_2","type":"function_call_output","call_id":"fc_upstream_abc","output":"ok"}}`)
	restored, changed := m.restoreBridgeResponseCallIDs(outEvent)
	if !changed {
		t.Fatalf("function_call_output should be rewritten")
	}
	if !strings.Contains(string(restored), `"call_id":"ctco_019fd0d8"`) {
		t.Fatalf("call_id not restored to codex: %s", string(restored))
	}
	if strings.Contains(string(restored), "fc_upstream_abc") {
		t.Fatalf("upstream call_id leaked: %s", string(restored))
	}
}

// 同名工具多次调用：按 name 队列顺序配对，还原各自 codex call_id。
func TestBridgeCallIDMapperSameNamePairing(t *testing.T) {
	body := []byte(`{
		"input": [
			{"type": "function_call", "name": "Read", "call_id": "ctco_r1", "arguments": "{}"},
			{"type": "function_call", "name": "Read", "call_id": "ctco_r2", "arguments": "{}"}
		]
	}`)
	m := newBridgeCallIDMapper(body)
	m.restoreBridgeResponseCallIDs([]byte(`{"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","call_id":"fc_a","name":"Read","arguments":"{}"}}`))
	m.restoreBridgeResponseCallIDs([]byte(`{"type":"response.output_item.done","item":{"id":"fc_2","type":"function_call","call_id":"fc_b","name":"Read","arguments":"{}"}}`))
	out1, _ := m.restoreBridgeResponseCallIDs([]byte(`{"type":"response.output_item.done","item":{"id":"fc_3","type":"function_call_output","call_id":"fc_a","output":"a"}}`))
	out2, _ := m.restoreBridgeResponseCallIDs([]byte(`{"type":"response.output_item.done","item":{"id":"fc_4","type":"function_call_output","call_id":"fc_b","output":"b"}}`))
	if !strings.Contains(string(out1), `"call_id":"ctco_r1"`) {
		t.Fatalf("fc_a should map to ctco_r1: %s", string(out1))
	}
	if !strings.Contains(string(out2), `"call_id":"ctco_r2"`) {
		t.Fatalf("fc_b should map to ctco_r2: %s", string(out2))
	}
}

// 客户端直接发的 function_call（call_id 无私有前缀）不建映射，原样回传。
func TestBridgeCallIDMapperPassthroughWhenNoMapping(t *testing.T) {
	body := []byte(`{"input": [{"type": "function_call", "name": "Read", "call_id": "call_x", "arguments": "{}"}]}`)
	m := newBridgeCallIDMapper(body)
	out, changed := m.restoreBridgeResponseCallIDs([]byte(`{"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call_output","call_id":"call_x","output":"ok"}}`))
	_ = out
	if changed {
		t.Fatalf("call_x already codex call_id, should not change: %s", string(out))
	}
}
