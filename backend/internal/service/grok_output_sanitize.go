package service

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const grokSanitizedAssistantOutputMaxLen = 8192

var grokOutputPollutionMarkers = []string{
	"\n\n================================================",
	"\n\n====",
	"【步骤",
	"<seed:tool_call",
	"<xai:tool_usage_card",
	"<grok:render",
	"</think>",
	"</think>",
	"｜Assistant｜",
	"[第 ",
	"LLM调用",
	"REASONING_LIVE_2",
	"MULTI_TURN_2",
}

var grokOutputPromptEchoMarkers = []string{
	"\n\nuser:",
	"\n\nuser\n",
	"\n\nUser:",
	"\n\nuser：",
	"\n\nUser\n",
}

// sanitizeGrokAssistantOutputText trims upstream eval rollout / prompt echo tails
// from assistant output_text while preserving the leading valid answer.
func sanitizeGrokAssistantOutputText(text string) string {
	text = strings.TrimRight(text, " \t")
	if text == "" {
		return text
	}
	if cut := grokOutputPollutionCutIndex(text); cut >= 0 {
		text = strings.TrimRight(text[:cut], " \t\n\r")
	}
	if len(text) > grokSanitizedAssistantOutputMaxLen {
		text = text[:grokSanitizedAssistantOutputMaxLen]
	}
	return text
}

func grokOutputPollutionCutIndex(text string) int {
	earliest := len(text)
	for _, marker := range grokOutputPollutionMarkers {
		if idx := strings.Index(text, marker); idx >= 0 && idx < earliest {
			// REASONING_LIVE_2 is only pollution when it appears after a valid T1 prefix.
			if marker == "REASONING_LIVE_2" && !strings.Contains(text[:idx], "REASONING_LIVE_1") {
				continue
			}
			earliest = idx
		}
	}
	for _, marker := range grokOutputPromptEchoMarkers {
		if idx := strings.Index(text, marker); idx >= 0 && idx < earliest {
			earliest = idx
		}
	}
	if earliest < len(text) {
		return earliest
	}
	return -1
}

func sanitizeGrokWSOutboundEvent(eventType string, payload []byte) ([]byte, bool) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return payload, false
	}
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done":
		return sanitizeGrokCompletedEventPayload(payload)
	case "response.output_item.done":
		return sanitizeGrokOutputItemDoneEvent(payload)
	case "response.output_text.done":
		return sanitizeGrokOutputTextDoneEvent(payload)
	default:
		return payload, false
	}
}

func sanitizeGrokCompletedEventPayload(payload []byte) ([]byte, bool) {
	output := gjson.GetBytes(payload, "response.output")
	if !output.IsArray() {
		return payload, false
	}
	changed := false
	updated := payload
	for i, item := range output.Array() {
		if strings.TrimSpace(item.Get("type").String()) != "message" {
			continue
		}
		itemRaw := []byte(item.Raw)
		sanitizedItem, itemChanged := sanitizeGrokMessageOutputItem(itemRaw)
		if !itemChanged {
			continue
		}
		path := "response.output." + itoa(i)
		next, err := sjson.SetRawBytes(updated, path, sanitizedItem)
		if err != nil {
			continue
		}
		updated = next
		changed = true
	}
	return updated, changed
}

func sanitizeGrokOutputItemDoneEvent(payload []byte) ([]byte, bool) {
	item := gjson.GetBytes(payload, "item")
	if !item.Exists() || strings.TrimSpace(item.Get("type").String()) != "message" {
		return payload, false
	}
	sanitizedItem, changed := sanitizeGrokMessageOutputItem([]byte(item.Raw))
	if !changed {
		return payload, false
	}
	updated, err := sjson.SetRawBytes(payload, "item", sanitizedItem)
	if err != nil {
		return payload, false
	}
	return updated, true
}

func sanitizeGrokOutputTextDoneEvent(payload []byte) ([]byte, bool) {
	text := gjson.GetBytes(payload, "text").String()
	if text == "" {
		return payload, false
	}
	sanitized := sanitizeGrokAssistantOutputText(text)
	if sanitized == text {
		return payload, false
	}
	updated, err := sjson.SetBytes(payload, "text", sanitized)
	if err != nil {
		return payload, false
	}
	return updated, true
}

func sanitizeGrokMessageOutputItem(item []byte) ([]byte, bool) {
	if len(item) == 0 || !gjson.ValidBytes(item) {
		return item, false
	}
	content := gjson.GetBytes(item, "content")
	if !content.IsArray() {
		if text := content.String(); text != "" {
			sanitized := sanitizeGrokAssistantOutputText(text)
			if sanitized == text {
				return item, false
			}
			updated, err := sjson.SetBytes(item, "content", sanitized)
			if err != nil {
				return item, false
			}
			return updated, true
		}
		return item, false
	}

	changed := false
	updated := item
	for i, part := range content.Array() {
		partType := strings.TrimSpace(part.Get("type").String())
		if partType != "output_text" && partType != "text" && partType != "" {
			continue
		}
		text := part.Get("text").String()
		if text == "" {
			continue
		}
		sanitized := sanitizeGrokAssistantOutputText(text)
		if sanitized == text {
			continue
		}
		path := "content." + itoa(i) + ".text"
		next, err := sjson.SetBytes(updated, path, sanitized)
		if err != nil {
			continue
		}
		updated = next
		changed = true
	}
	return updated, changed
}

func sanitizeGrokInputAssistantItems(input []any) []any {
	if len(input) == 0 {
		return input
	}
	changed := false
	out := make([]any, 0, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil || !isGrokAssistantInputItem(rawItem) {
			out = append(out, rawItem)
			continue
		}
		sanitizedItem, itemChanged := sanitizeGrokInputAssistantItem(item)
		if itemChanged {
			changed = true
			out = append(out, sanitizedItem)
			continue
		}
		out = append(out, rawItem)
	}
	if !changed {
		return input
	}
	return out
}

func sanitizeGrokInputAssistantItem(item map[string]any) (map[string]any, bool) {
	text := extractGrokInputItemText(item)
	if text == "" {
		return item, false
	}
	sanitized := sanitizeGrokAssistantOutputText(text)
	if sanitized == text {
		return item, false
	}
	updated := cloneStringAnyMap(item)
	switch content := updated["content"].(type) {
	case string:
		updated["content"] = sanitized
	case []any:
		parts := make([]any, 0, len(content))
		replaced := false
		for _, rawPart := range content {
			part, ok := rawPart.(map[string]any)
			if !ok || part == nil {
				parts = append(parts, rawPart)
				continue
			}
			partType := strings.TrimSpace(firstNonEmptyString(part["type"]))
			if partType == "output_text" || partType == "text" || partType == "" || partType == "input_text" {
				nextPart := cloneStringAnyMap(part)
				nextPart["text"] = sanitized
				parts = append(parts, nextPart)
				replaced = true
				continue
			}
			parts = append(parts, rawPart)
		}
		if replaced {
			updated["content"] = parts
		}
	}
	return updated, true
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}