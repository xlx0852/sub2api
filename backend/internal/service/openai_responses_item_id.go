package service

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// hasOverlongOpenAIResponsesReplayItemID only covers replay items whose id is
// optional. Never strip/rewrite item_reference or unknown item types: their id
// may be the only link to a stored upstream object. call_id is a separate key.
func hasOverlongOpenAIResponsesReplayItemID(itemType, id string) bool {
	if len(id) <= 64 || utf8.RuneCountInString(id) <= 64 {
		return false
	}
	switch itemType {
	case "message", "reasoning", "compaction_summary":
		return true
	default:
		return isCodexToolCallItemType(itemType)
	}
}

// Invalid replayed IDs are removed rather than rewritten because a fabricated
// ID may point at a different upstream object.
func shouldStripOpenAIResponsesInputItemID(itemType, id string) bool {
	if id == "" {
		return false
	}
	if hasOverlongOpenAIResponsesReplayItemID(itemType, id) {
		return true
	}
	switch itemType {
	case "message":
		return !strings.HasPrefix(id, "msg")
	case "reasoning":
		return !strings.HasPrefix(id, "rs")
	default:
		return isCodexToolCallInputType(itemType) && !strings.HasPrefix(id, "fc")
	}
}

func sanitizeOpenAIResponsesInputItemIDs(body []byte) ([]byte, bool, error) {
	return stripOpenAIResponsesInputItemIDs(body, shouldStripOpenAIResponsesInputItemID)
}

func stripOverlongOpenAIResponsesInputItemIDs(body []byte) ([]byte, bool, error) {
	return stripOpenAIResponsesInputItemIDs(body, hasOverlongOpenAIResponsesReplayItemID)
}

func stripOpenAIResponsesInputItemIDs(body []byte, shouldStrip func(string, string) bool) ([]byte, bool, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}

	items := make([][]byte, 0)
	changed := false
	var sanitizeErr error
	index := 0
	input.ForEach(func(_, item gjson.Result) bool {
		currentIndex := index
		index++
		itemBody := []byte(item.Raw)
		if item.IsObject() {
			itemType := item.Get("type")
			id := item.Get("id")
			if itemType.Type == gjson.String && id.Type == gjson.String &&
				shouldStrip(itemType.String(), id.String()) {
				itemBody, sanitizeErr = sjson.DeleteBytes(itemBody, "id")
				if sanitizeErr != nil {
					sanitizeErr = fmt.Errorf("delete input.%d.id: %w", currentIndex, sanitizeErr)
					return false
				}
				changed = true
			}
		}
		items = append(items, itemBody)
		return true
	})
	if sanitizeErr != nil {
		return nil, false, sanitizeErr
	}
	if !changed {
		return body, false, nil
	}

	rebuiltInput := make([]byte, 0, len(input.Raw))
	rebuiltInput = append(rebuiltInput, '[')
	for i, item := range items {
		if i > 0 {
			rebuiltInput = append(rebuiltInput, ',')
		}
		rebuiltInput = append(rebuiltInput, item...)
	}
	rebuiltInput = append(rebuiltInput, ']')

	sanitized, err := sjson.SetRawBytes(body, "input", rebuiltInput)
	if err != nil {
		return nil, false, fmt.Errorf("replace sanitized input: %w", err)
	}
	return sanitized, true, nil
}

// Copy on write: WS payload construction shallow-copies the request map, so its
// input slice and item maps may still belong to the caller.
func stripOverlongOpenAIResponsesInputItemIDsInMap(body map[string]any) bool {
	items, ok := body["input"].([]any)
	if !ok {
		return false
	}
	var next []any
	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := item["type"].(string)
		id, _ := item["id"].(string)
		if !hasOverlongOpenAIResponsesReplayItemID(typ, id) {
			continue
		}
		if next == nil {
			next = append([]any(nil), items...)
		}
		copyItem := make(map[string]any, len(item)-1)
		for key, value := range item {
			if key != "id" {
				copyItem[key] = value
			}
		}
		next[i] = copyItem
	}
	if next == nil {
		return false
	}
	body["input"] = next
	return true
}
