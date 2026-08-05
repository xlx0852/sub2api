package apicompat

import (
	"encoding/json"
	"testing"
)

// tool_search_call 线上形态：arguments 是 JSON 对象、带 execution 字段
// （codex 工具搜索模式），Unmarshal 应容错对象，Marshal 应还原对象形态。
func TestResponsesOutputToolSearchCallRoundTrip(t *testing.T) {
	wire := `{
		"type": "tool_search_call",
		"id": "ts_1",
		"call_id": "call_ts1",
		"execution": "client",
		"arguments": {"query": "find function", "limit": 5}
	}`
	var out ResponsesOutput
	if err := json.Unmarshal([]byte(wire), &out); err != nil {
		t.Fatalf("unmarshal tool_search_call failed: %v", err)
	}
	if out.Type != "tool_search_call" || out.Arguments == "" {
		t.Fatalf("unexpected decode: type=%q args=%q", out.Type, out.Arguments)
	}
	rebuilt, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(rebuilt, &m); err != nil {
		t.Fatalf("rebuilt not valid json: %v", err)
	}
	if m["execution"] != "client" {
		t.Fatalf("execution missing on wire: %s", string(rebuilt))
	}
	if _, ok := m["arguments"].(map[string]any); !ok {
		t.Fatalf("arguments should be an object on wire: %s", string(rebuilt))
	}
}
