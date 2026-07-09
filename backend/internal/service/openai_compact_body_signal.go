package service

import "github.com/tidwall/gjson"

// HasCompactionTriggerInInput detects the Codex remote compact v2 body signal:
// an input item with type "compaction_trigger". Official Codex (default since
// 2026-06-11) sends this on ordinary POST /v1/responses with stream=true and
// waits for SSE terminal events:
//
//	response.output_item.done (item.type=compaction)
//	response.completed
//
// sub2api must preserve that client-facing SSE contract. Upstream may still use
// legacy POST /responses/compact JSON, in which case the gateway bridges the
// result into the v2 SSE sequence (see openai_compact_v2_bridge.go).
func HasCompactionTriggerInInput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "compaction_trigger" {
			found = true
			return false
		}
		return true
	})
	return found
}
