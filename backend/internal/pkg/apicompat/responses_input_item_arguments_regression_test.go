package apicompat

import (
	"encoding/json"
	"strings"
	"testing"
)

// 复现 issue #2913 完整场景：新版 codex 的 input 里
//   - function_call.arguments = JSON 对象（issue 原文："arguments 为对象时报类似错误"）
//   - function_call_output.output = JSON 数组
// 二者任意一个都会让整个 input 数组 unmarshal 失败 → Anthropic 上游 502。
func TestResponsesInputItemArgumentsObjectRegression2913(t *testing.T) {
	body := `{
		"model": "codex-1",
		"instructions": "look around",
		"input": [
			{
				"type": "function_call",
				"call_id": "call_1",
				"name": "Read",
				"arguments": {"path": "/etc/hosts", "pages": [1, 2]}
			},
			{
				"type": "function_call_output",
				"call_id": "call_1",
				"output": [{"type": "input_text", "text": "127.0.0.1 localhost"}]
			}
		]
	}`
	var req ResponsesRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("parse request failed: %v", err)
	}
	_, _, err := convertResponsesInputToAnthropic(req.Instructions, req.Input)
	if err != nil {
		t.Fatalf("convertResponsesInputToAnthropic FAILED (此即线上 502 根因): %v", err)
	}
	_ = strings.TrimSpace // keep import
}
