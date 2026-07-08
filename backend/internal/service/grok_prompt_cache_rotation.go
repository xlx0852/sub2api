package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	grokIsolatedTurnPromptCacheKeyPrefix      = "iso_"
	grokPromptCacheRotationMinDroppedAssistant = 512
)

// applyGrokCollapsedInputPromptCacheKeyRotation breaks Grok server-side conversation
// affinity when Codex sends a fresh user turn after a long assistant block but keeps
// the same prompt_cache_key. Without rotation, upstream may answer from stale cache
// even though ingress collapse only forwards the latest user message.
func applyGrokCollapsedInputPromptCacheKeyRotation(payload map[string]any, fullInput, collapsed []any) bool {
	if payload == nil || len(fullInput) == 0 || len(collapsed) == 0 {
		return false
	}
	if grokCollapsedInputEqual(fullInput, collapsed) {
		return false
	}
	if !isGrokTrailingUserContinuation(fullInput) {
		return false
	}
	latestUserText := grokLatestUserMessageText(collapsed)
	if latestUserText == "" {
		return false
	}
	if grokUserMessageReferencesPriorContext(latestUserText) {
		return false
	}
	if grokDroppedAssistantTextLen(fullInput, collapsed) < grokPromptCacheRotationMinDroppedAssistant {
		return false
	}

	baseKey := strings.TrimSpace(firstNonEmptyString(payload["prompt_cache_key"]))
	if baseKey == "" {
		return false
	}
	rotated := deriveGrokIsolatedTurnPromptCacheKey(baseKey, latestUserText)
	payload["prompt_cache_key"] = rotated
	delete(payload, "previous_response_id")
	return true
}

func deriveGrokIsolatedTurnPromptCacheKey(baseKey, latestUserText string) string {
	baseKey = strings.TrimSpace(baseKey)
	latestUserText = strings.TrimSpace(latestUserText)
	if baseKey == "" {
		return ""
	}
	if latestUserText == "" {
		return baseKey
	}
	return fmt.Sprintf("%s%s%s", baseKey, grokIsolatedTurnPromptCacheKeyPrefix, hashSensitiveValueForLog(latestUserText))
}

func grokLatestUserMessageText(input []any) string {
	for i := len(input) - 1; i >= 0; i-- {
		item, ok := input[i].(map[string]any)
		if !ok || item == nil {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(item["role"])) != "user" {
			continue
		}
		return strings.TrimSpace(extractGrokInputItemText(item))
	}
	return ""
}

func grokDroppedAssistantTextLen(fullInput, collapsed []any) int {
	if len(fullInput) <= len(collapsed) {
		return 0
	}
	collapsedSet := make(map[int]struct{}, len(collapsed))
	for i, item := range collapsed {
		for j, fullItem := range fullInput {
			if grokInputItemsEqual(item, fullItem) {
				collapsedSet[j] = struct{}{}
				break
			}
		}
		_ = i
	}
	total := 0
	for j, rawItem := range fullInput {
		if _, kept := collapsedSet[j]; kept {
			continue
		}
		if !isGrokAssistantInputItem(rawItem) {
			continue
		}
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			continue
		}
		total += len(strings.TrimSpace(extractGrokInputItemText(item)))
	}
	return total
}

func grokInputItemsEqual(a, b any) bool {
	aJSON, err1 := json.Marshal(a)
	bJSON, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}

func grokUserMessageReferencesPriorContext(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return false
	}
	markers := []string{
		"刚才", "刚刚", "上面", "上文", "之前", "先前", "继续", "接着", "还是",
		"刚才问", "刚才说", "上面问", "上面说", "之前问", "之前说",
		"the previous", "previous", "earlier", "above", "continue", "as i said",
		"as mentioned", "follow up", "follow-up",
	}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}