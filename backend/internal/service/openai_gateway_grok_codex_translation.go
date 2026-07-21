package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

const grokCodexTranslationContextKey = "grok_codex_translation"

type grokCodexTranslation struct {
	customTools map[string]struct{}

	mu          sync.Mutex
	streamCalls map[string]string
	streamArgs  map[string]strings.Builder
}

func newGrokCodexTranslation(body []byte) *grokCodexTranslation {
	t := &grokCodexTranslation{
		customTools: make(map[string]struct{}),
		streamCalls: make(map[string]string),
		streamArgs:  make(map[string]strings.Builder),
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) == nil {
		collectGrokCustomToolNames(payload["tools"], t.customTools)
	}
	return t
}

func collectGrokCustomToolNames(raw any, out map[string]struct{}) {
	tools, ok := raw.([]any)
	if !ok {
		return
	}
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		typeName := strings.TrimSpace(fmt.Sprint(tool["type"]))
		if typeName == "custom" {
			if name := strings.TrimSpace(fmt.Sprint(tool["name"])); name != "" {
				out[name] = struct{}{}
			}
		}
		if typeName == "namespace" {
			collectGrokCustomToolNames(tool["tools"], out)
			collectGrokCustomToolNames(tool["children"], out)
		}
	}
}

func (t *grokCodexTranslation) hasCustomTools() bool {
	return t != nil && len(t.customTools) > 0
}

func (t *grokCodexTranslation) isCustom(name string) bool {
	if t == nil {
		return false
	}
	_, ok := t.customTools[strings.TrimSpace(name)]
	return ok
}

func setGrokCodexTranslation(c *gin.Context, t *grokCodexTranslation) {
	if c != nil && t != nil && t.hasCustomTools() {
		c.Set(grokCodexTranslationContextKey, t)
	}
}

func getGrokCodexTranslation(c *gin.Context) *grokCodexTranslation {
	if c == nil {
		return nil
	}
	value, ok := c.Get(grokCodexTranslationContextKey)
	if !ok {
		return nil
	}
	t, _ := value.(*grokCodexTranslation)
	return t
}

func grokCustomFunctionParameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "The raw input for this tool, passed through verbatim.",
			},
		},
		"required":             []any{"input"},
		"additionalProperties": false,
	}
}

func encodeGrokCustomToolInput(input any) string {
	encoded, err := json.Marshal(map[string]any{"input": input})
	if err != nil {
		return `{"input":""}`
	}
	return string(encoded)
}

func decodeGrokCustomToolInput(arguments any) string {
	var value any
	switch typed := arguments.(type) {
	case string:
		if json.Unmarshal([]byte(typed), &value) != nil {
			return typed
		}
	default:
		value = typed
	}
	if object, ok := value.(map[string]any); ok {
		if input, exists := object["input"]; exists {
			if text, ok := input.(string); ok {
				return text
			}
			encoded, _ := json.Marshal(input)
			return string(encoded)
		}
		if len(object) == 1 {
			for _, only := range object {
				if text, ok := only.(string); ok {
					return text
				}
			}
		}
	}
	if text, ok := value.(string); ok {
		return text
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func rewriteGrokCustomCallsInJSON(body []byte, t *grokCodexTranslation) ([]byte, bool) {
	if t == nil || !t.hasCustomTools() || !json.Valid(body) {
		return body, false
	}
	var payload any
	if json.Unmarshal(body, &payload) != nil || !rewriteGrokCustomCallsValue(payload, t) {
		return body, false
	}
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return body, false
	}
	return rewritten, true
}

func rewriteGrokCustomCallsValue(value any, t *grokCodexTranslation) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]any:
		if strings.TrimSpace(fmt.Sprint(typed["type"])) == "function_call" && t.isCustom(fmt.Sprint(typed["name"])) {
			typed["type"] = "custom_tool_call"
			typed["input"] = decodeGrokCustomToolInput(typed["arguments"])
			delete(typed, "arguments")
			if _, exists := typed["id"]; !exists {
				callID := strings.TrimSpace(fmt.Sprint(typed["call_id"]))
				if callID != "" && callID != "<nil>" {
					typed["id"] = "ctc_" + callID
				}
			}
			if _, exists := typed["status"]; !exists {
				typed["status"] = "completed"
			}
			changed = true
		}
		for _, child := range typed {
			if rewriteGrokCustomCallsValue(child, t) {
				changed = true
			}
		}
	case []any:
		for _, child := range typed {
			if rewriteGrokCustomCallsValue(child, t) {
				changed = true
			}
		}
	}
	return changed
}

func grokStreamCallKey(payload map[string]any) string {
	if itemID := strings.TrimSpace(fmt.Sprint(payload["item_id"])); itemID != "" && itemID != "<nil>" {
		return "item:" + itemID
	}
	if callID := strings.TrimSpace(fmt.Sprint(payload["call_id"])); callID != "" && callID != "<nil>" {
		return "call:" + callID
	}
	if index, ok := payload["output_index"]; ok {
		return "index:" + fmt.Sprint(index)
	}
	return ""
}

func grokStreamItemKey(payload map[string]any, item map[string]any) string {
	if id := strings.TrimSpace(fmt.Sprint(item["id"])); id != "" && id != "<nil>" {
		return "item:" + id
	}
	if callID := strings.TrimSpace(fmt.Sprint(item["call_id"])); callID != "" && callID != "<nil>" {
		return "call:" + callID
	}
	return grokStreamCallKey(payload)
}

// rewriteGrokCodexSSEData returns zero or more complete SSE data payloads.
// Function argument deltas for custom tools are buffered until the done event,
// then emitted as one custom input delta followed by the custom input done event.
func rewriteGrokCodexSSEData(data []byte, t *grokCodexTranslation) ([][]byte, bool) {
	if t == nil || !t.hasCustomTools() || !json.Valid(data) {
		return [][]byte{data}, false
	}
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		return [][]byte{data}, false
	}
	eventType := strings.TrimSpace(fmt.Sprint(payload["type"]))

	t.mu.Lock()
	defer t.mu.Unlock()

	if rawItem, ok := payload["item"].(map[string]any); ok {
		name := strings.TrimSpace(fmt.Sprint(rawItem["name"]))
		if strings.TrimSpace(fmt.Sprint(rawItem["type"])) == "function_call" && t.isCustom(name) {
			key := grokStreamItemKey(payload, rawItem)
			if key != "" {
				t.streamCalls[key] = name
			}
			rawItem["type"] = "custom_tool_call"
			if arguments, exists := rawItem["arguments"]; exists {
				rawItem["input"] = decodeGrokCustomToolInput(arguments)
				delete(rawItem, "arguments")
			}
			encoded, _ := json.Marshal(payload)
			return [][]byte{encoded}, true
		}
	}

	key := grokStreamCallKey(payload)
	_, customCall := t.streamCalls[key]
	if !customCall {
		if rewritten, changed := rewriteGrokCustomCallsInJSON(data, t); changed {
			return [][]byte{rewritten}, true
		}
		return [][]byte{data}, false
	}

	switch eventType {
	case "response.function_call_arguments.delta":
		builder := t.streamArgs[key]
		builder.WriteString(fmt.Sprint(payload["delta"]))
		t.streamArgs[key] = builder
		return nil, true
	case "response.function_call_arguments.done":
		arguments := fmt.Sprint(payload["arguments"])
		if builder, ok := t.streamArgs[key]; ok && strings.TrimSpace(arguments) == "" {
			arguments = builder.String()
		}
		delete(t.streamArgs, key)
		input := decodeGrokCustomToolInput(arguments)
		delta := cloneGrokSSEPayload(payload)
		delta["type"] = "response.custom_tool_call_input.delta"
		delta["delta"] = input
		delete(delta, "arguments")
		done := cloneGrokSSEPayload(payload)
		done["type"] = "response.custom_tool_call_input.done"
		done["input"] = input
		delete(done, "arguments")
		encodedDelta, _ := json.Marshal(delta)
		encodedDone, _ := json.Marshal(done)
		return [][]byte{encodedDelta, encodedDone}, true
	default:
		if rewritten, changed := rewriteGrokCustomCallsInJSON(data, t); changed {
			return [][]byte{rewritten}, true
		}
		return [][]byte{data}, false
	}
}

func rewriteGrokCodexSSEBody(body []byte, t *grokCodexTranslation) ([]byte, bool) {
	if t == nil || !t.hasCustomTools() {
		return body, false
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var out strings.Builder
	changed := false
	for scanner.Scan() {
		line := scanner.Text()
		data, ok := extractOpenAISSEDataLine(line)
		if !ok {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		payloads, rewritten := rewriteGrokCodexSSEData([]byte(data), t)
		if !rewritten {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		changed = true
		for _, payload := range payloads {
			out.WriteString("data: ")
			out.Write(payload)
			out.WriteString("\n\n")
		}
	}
	if scanner.Err() != nil || !changed {
		return body, false
	}
	return []byte(out.String()), true
}

func cloneGrokSSEPayload(payload map[string]any) map[string]any {
	copy := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		copy[key] = value
	}
	return copy
}
