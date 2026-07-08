package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	grokReasoningReplayCacheTTL         = 1 * time.Hour
	grokReasoningReplayCacheMaxEntries  = 10240
	grokReasoningReplayCacheEvictBatch  = 128
)

type grokReasoningReplayEntry struct {
	Items     [][]byte
	Timestamp time.Time
}

var (
	grokReasoningReplayMu      sync.Mutex
	grokReasoningReplayEntries = make(map[string]grokReasoningReplayEntry)
)

func grokReasoningReplayCacheKey(modelName, sessionKey string) string {
	modelName = strings.TrimSpace(modelName)
	sessionKey = strings.TrimSpace(sessionKey)
	if modelName == "" || sessionKey == "" {
		return ""
	}
	return strings.Join([]string{"grok-reasoning-replay", modelName, sessionKey}, "\x00")
}

func grokReasoningReplaySessionKey(body []byte, headers http.Header) string {
	if len(body) > 0 {
		if promptCacheKey := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String()); promptCacheKey != "" {
			return "prompt-cache:" + promptCacheKey
		}
		if windowID := strings.TrimSpace(gjson.GetBytes(body, "client_metadata.x-codex-window-id").String()); windowID != "" {
			return "window:" + windowID
		}
		if turnMetadata := strings.TrimSpace(gjson.GetBytes(body, "client_metadata.x-codex-turn-metadata").String()); turnMetadata != "" {
			if key := grokReasoningReplaySessionKeyFromTurnMetadata(turnMetadata); key != "" {
				return key
			}
		}
	}
	if headers == nil {
		return ""
	}
	if turnMetadata := strings.TrimSpace(headers.Get("X-Codex-Turn-Metadata")); turnMetadata != "" {
		if key := grokReasoningReplaySessionKeyFromTurnMetadata(turnMetadata); key != "" {
			return key
		}
	}
	if windowID := strings.TrimSpace(grokHeaderValue(headers, "X-Codex-Window-Id")); windowID != "" {
		return "window:" + windowID
	}
	for _, headerName := range []string{"Session_id", "session_id", "Session-Id"} {
		if value := strings.TrimSpace(grokHeaderValue(headers, headerName)); value != "" {
			return "session-id:" + value
		}
	}
	if conversationID := strings.TrimSpace(grokHeaderValue(headers, "Conversation_id")); conversationID != "" {
		return "conversation_id:" + conversationID
	}
	return ""
}

func grokHeaderValue(headers http.Header, name string) string {
	if headers == nil {
		return ""
	}
	if value := strings.TrimSpace(headers.Get(name)); value != "" {
		return value
	}
	name = strings.ToLower(strings.TrimSpace(name))
	for key, values := range headers {
		if strings.ToLower(strings.TrimSpace(key)) != name || len(values) == 0 {
			continue
		}
		if value := strings.TrimSpace(values[0]); value != "" {
			return value
		}
	}
	return ""
}

func grokReasoningReplaySessionKeyFromTurnMetadata(turnMetadata string) string {
	if promptCacheKey := strings.TrimSpace(gjson.Get(turnMetadata, "prompt_cache_key").String()); promptCacheKey != "" {
		return "prompt-cache:" + promptCacheKey
	}
	if windowID := strings.TrimSpace(gjson.Get(turnMetadata, "window_id").String()); windowID != "" {
		return "window:" + windowID
	}
	return ""
}

func cacheGrokReasoningReplayItems(modelName, sessionKey string, items [][]byte) bool {
	key := grokReasoningReplayCacheKey(modelName, sessionKey)
	if key == "" {
		return false
	}
	normalized, ok := normalizeGrokReasoningReplayItems(items)
	if !ok {
		return false
	}
	now := time.Now()
	grokReasoningReplayMu.Lock()
	defer grokReasoningReplayMu.Unlock()
	grokReasoningReplayEntries[key] = grokReasoningReplayEntry{
		Items:     normalized,
		Timestamp: now,
	}
	if len(grokReasoningReplayEntries) > grokReasoningReplayCacheMaxEntries {
		evictOldestGrokReasoningReplayEntriesLocked(grokReasoningReplayCacheEvictBatch)
	}
	return true
}

func getGrokReasoningReplayItems(modelName, sessionKey string) ([][]byte, bool) {
	key := grokReasoningReplayCacheKey(modelName, sessionKey)
	if key == "" {
		return nil, false
	}
	now := time.Now()
	grokReasoningReplayMu.Lock()
	defer grokReasoningReplayMu.Unlock()
	entry, ok := grokReasoningReplayEntries[key]
	if !ok {
		return nil, false
	}
	if now.Sub(entry.Timestamp) > grokReasoningReplayCacheTTL {
		delete(grokReasoningReplayEntries, key)
		return nil, false
	}
	entry.Timestamp = now
	grokReasoningReplayEntries[key] = entry
	return cloneGrokReasoningReplayItems(entry.Items), true
}

func deleteGrokReasoningReplayItems(modelName, sessionKey string) {
	key := grokReasoningReplayCacheKey(modelName, sessionKey)
	if key == "" {
		return
	}
	grokReasoningReplayMu.Lock()
	delete(grokReasoningReplayEntries, key)
	grokReasoningReplayMu.Unlock()
}

func cacheGrokReasoningReplayFromCompleted(modelName string, requestBody, completedEvent []byte) {
	sessionKey := grokReasoningReplaySessionKey(requestBody, nil)
	if sessionKey == "" {
		return
	}
	output := gjson.GetBytes(completedEvent, "response.output")
	if !output.IsArray() {
		return
	}
	items := make([][]byte, 0, len(output.Array()))
	for _, item := range output.Array() {
		switch strings.TrimSpace(item.Get("type").String()) {
		case "reasoning", "function_call", "custom_tool_call":
			items = append(items, []byte(item.Raw))
		default:
			continue
		}
	}
	cached := cacheGrokReasoningReplayItems(modelName, sessionKey, items)
	storeReason := "ok"
	if !cached {
		deleteGrokReasoningReplayItems(modelName, sessionKey)
		storeReason = diagnoseGrokReasoningReplayStoreFailure(items)
	}
	observeGrokEvent(nil, "reasoning_replay_cache", map[string]string{
		"replay_action":      map[bool]string{true: "store", false: "store_failed"}[cached],
		"replay_session_key": sessionKey,
		"replay_items":       fmt.Sprintf("%d", len(items)),
		"upstream_model":     modelName,
		"store_reason":       storeReason,
	})
}

func diagnoseGrokReasoningReplayStoreFailure(items [][]byte) string {
	if len(items) == 0 {
		return "no_replay_items_in_completed_output"
	}
	reasons := make([]string, 0, len(items))
	for _, item := range items {
		itemResult := gjson.ParseBytes(item)
		itemType := strings.TrimSpace(itemResult.Get("type").String())
		switch itemType {
		case "reasoning":
			encrypted := itemResult.Get("encrypted_content")
			switch {
			case encrypted.Type == gjson.Null:
				if grokReasoningReplaySummaryText(itemResult) != "" {
					reasons = append(reasons, "reasoning_summary_fallback_available")
				} else {
					reasons = append(reasons, "reasoning_encrypted_content_null")
				}
			case encrypted.Type != gjson.String:
				reasons = append(reasons, "reasoning_encrypted_content_not_string")
			case strings.TrimSpace(encrypted.String()) == "":
				reasons = append(reasons, "reasoning_encrypted_content_empty")
			case strings.HasPrefix(strings.TrimSpace(encrypted.String()), "gAAAA"):
				reasons = append(reasons, "reasoning_encrypted_content_codex_blob")
			default:
				if _, err := xai.InspectGrokEncryptedContent(encrypted.String()); err != nil {
					reasons = append(reasons, "reasoning_encrypted_content_invalid:"+truncateGrokObserveValue(err.Error()))
				} else {
					reasons = append(reasons, "reasoning_normalize_rejected")
				}
			}
		case "function_call", "custom_tool_call":
			reasons = append(reasons, itemType+"_normalize_rejected")
		default:
			reasons = append(reasons, "unsupported_item_type:"+itemType)
		}
	}
	if len(reasons) == 0 {
		return "normalize_failed_unknown"
	}
	return strings.Join(reasons, ",")
}

func applyGrokReasoningReplayCache(body []byte, upstreamModel string, headers http.Header) []byte {
	return applyGrokReasoningReplayCacheObserved(body, upstreamModel, headers, nil)
}

func applyGrokReasoningReplayCacheObserved(body []byte, upstreamModel string, headers http.Header, span *grokObserveSpan) []byte {
	sessionKey := grokReasoningReplaySessionKey(body, headers)
	if sessionKey == "" {
		observeGrokEvent(span, "reasoning_replay", map[string]string{
			"replay_action": "skip_no_session_key",
		})
		return body
	}
	items, ok := getGrokReasoningReplayItems(upstreamModel, sessionKey)
	if !ok || len(items) == 0 {
		observeGrokEvent(span, "reasoning_replay", map[string]string{
			"replay_action":      "miss",
			"replay_session_key": sessionKey,
		})
		return body
	}
	cachedCount := len(items)
	items = filterGrokReasoningReplayItemsForInput(body, items)
	if len(items) == 0 {
		observeGrokEvent(span, "reasoning_replay", map[string]string{
			"replay_action":      "skip_filtered_empty",
			"replay_session_key": sessionKey,
			"replay_items":       fmt.Sprintf("%d", cachedCount),
		})
		return body
	}
	beforeItems := len(gjson.GetBytes(body, "input").Array())
	updated, changed := insertGrokReasoningReplayItems(body, items)
	if !changed {
		observeGrokEvent(span, "reasoning_replay", map[string]string{
			"replay_action":      "skip_insert_failed",
			"replay_session_key": sessionKey,
			"replay_items":       fmt.Sprintf("%d", len(items)),
		})
		return body
	}
	afterItems := len(gjson.GetBytes(updated, "input").Array())
	observeGrokEvent(span, "reasoning_replay", map[string]string{
		"replay_action":      "inject",
		"replay_session_key": sessionKey,
		"replay_items":       fmt.Sprintf("%d", len(items)),
		"replay_injected":    fmt.Sprintf("%d", afterItems-beforeItems),
		"input_items_before": fmt.Sprintf("%d", beforeItems),
		"input_items_after":  fmt.Sprintf("%d", afterItems),
	})
	return updated
}

func grokInputHasValidReasoningEncryptedContent(body []byte) bool {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	for _, item := range input.Array() {
		if strings.TrimSpace(item.Get("type").String()) != "reasoning" {
			continue
		}
		encryptedContent := item.Get("encrypted_content")
		if encryptedContent.Type != gjson.String {
			continue
		}
		if _, err := xai.InspectGrokEncryptedContent(encryptedContent.String()); err == nil {
			return true
		}
	}
	return false
}

func filterGrokReasoningReplayItemsForInput(body []byte, items [][]byte) [][]byte {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return nil
	}

	hasInputReasoning := grokInputHasValidReasoningEncryptedContent(body)
	existingCalls := make(map[string]bool)
	existingOutputs := make(map[string]bool)
	for _, inputItem := range input.Array() {
		itemType := strings.TrimSpace(inputItem.Get("type").String())
		if itemType == "function_call_output" || itemType == "custom_tool_call_output" {
			callID := strings.TrimSpace(inputItem.Get("call_id").String())
			if callID != "" {
				for _, candidate := range grokReplayComparableCallIDs(callID) {
					existingOutputs[candidate] = true
				}
			}
		}
		for _, key := range grokReplayToolCallKeys(inputItem) {
			existingCalls[key] = true
		}
	}

	filtered := make([][]byte, 0, len(items))
	for _, item := range items {
		itemResult := gjson.ParseBytes(item)
		switch strings.TrimSpace(itemResult.Get("type").String()) {
		case "reasoning":
			if hasInputReasoning {
				continue
			}
		case "function_call", "custom_tool_call":
			keys := grokReplayToolCallKeys(itemResult)
			if len(keys) == 0 || grokReplayAnyToolCallKeyExists(existingCalls, keys) {
				continue
			}
			hasMatchingOutput := false
			callID := strings.TrimSpace(itemResult.Get("call_id").String())
			if callID != "" {
				for _, candidate := range grokReplayComparableCallIDs(callID) {
					if existingOutputs[candidate] {
						hasMatchingOutput = true
						break
					}
				}
			}
			if !hasMatchingOutput {
				continue
			}
			for _, key := range keys {
				existingCalls[key] = true
			}
		default:
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func insertGrokReasoningReplayItems(body []byte, replayItems [][]byte) ([]byte, bool) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() || len(replayItems) == 0 {
		return body, false
	}
	inputItems := input.Array()
	insertIndex := grokReasoningReplayInsertIndex(inputItems, replayItems)
	replayItems = grokAlignReasoningReplayToolCallIDs(inputItems, replayItems)
	items := make([]string, 0, len(inputItems)+len(replayItems))
	for i, inputItem := range inputItems {
		if i == insertIndex {
			for _, replayItem := range replayItems {
				items = append(items, string(replayItem))
			}
		}
		items = append(items, inputItem.Raw)
	}
	if insertIndex == len(inputItems) {
		for _, replayItem := range replayItems {
			items = append(items, string(replayItem))
		}
	}
	updated, err := sjson.SetRawBytes(body, "input", []byte("["+strings.Join(items, ",")+"]"))
	if err != nil {
		return body, false
	}
	return updated, true
}

func grokReasoningReplayInsertIndex(inputItems []gjson.Result, replayItems [][]byte) int {
	replayCallIDs := make(map[string]bool)
	for _, replayItem := range replayItems {
		itemResult := gjson.ParseBytes(replayItem)
		itemType := strings.TrimSpace(itemResult.Get("type").String())
		if itemType != "function_call" && itemType != "custom_tool_call" {
			continue
		}
		for _, callID := range grokReplayComparableCallIDs(itemResult.Get("call_id").String()) {
			replayCallIDs[callID] = true
		}
	}
	if len(replayCallIDs) > 0 {
		for index, inputItem := range inputItems {
			itemType := strings.TrimSpace(inputItem.Get("type").String())
			if itemType != "function_call_output" && itemType != "custom_tool_call_output" {
				continue
			}
			callID := strings.TrimSpace(inputItem.Get("call_id").String())
			if callID == "" || replayCallIDs[callID] {
				return index
			}
		}
	}
	for index := len(inputItems) - 1; index >= 0; index-- {
		inputItem := inputItems[index]
		if strings.TrimSpace(inputItem.Get("type").String()) == "message" && strings.TrimSpace(inputItem.Get("role").String()) == "assistant" {
			return index
		}
	}
	for index, inputItem := range inputItems {
		if shouldInsertGrokReasoningReplayBefore(inputItem) {
			return index
		}
	}
	return len(inputItems)
}

func grokAlignReasoningReplayToolCallIDs(inputItems []gjson.Result, replayItems [][]byte) [][]byte {
	outputCallIDs := grokReplayOutputCallIDs(inputItems)
	if len(outputCallIDs) == 0 {
		return replayItems
	}

	aligned := make([][]byte, 0, len(replayItems))
	for _, replayItem := range replayItems {
		itemResult := gjson.ParseBytes(replayItem)
		itemType := strings.TrimSpace(itemResult.Get("type").String())
		if itemType != "function_call" && itemType != "custom_tool_call" {
			aligned = append(aligned, replayItem)
			continue
		}

		callID := strings.TrimSpace(itemResult.Get("call_id").String())
		outputCallID := ""
		for _, candidate := range grokReplayComparableCallIDs(callID) {
			if value := outputCallIDs[candidate]; value != "" {
				outputCallID = value
				break
			}
		}
		if outputCallID == "" || outputCallID == callID {
			aligned = append(aligned, replayItem)
			continue
		}

		updated, err := sjson.SetBytes(replayItem, "call_id", outputCallID)
		if err != nil {
			aligned = append(aligned, replayItem)
			continue
		}
		aligned = append(aligned, updated)
	}
	return aligned
}

func grokReplayOutputCallIDs(inputItems []gjson.Result) map[string]string {
	outputCallIDs := make(map[string]string)
	for _, inputItem := range inputItems {
		itemType := strings.TrimSpace(inputItem.Get("type").String())
		if itemType != "function_call_output" && itemType != "custom_tool_call_output" {
			continue
		}
		callID := strings.TrimSpace(inputItem.Get("call_id").String())
		if callID == "" {
			continue
		}
		for _, candidate := range grokReplayComparableCallIDs(callID) {
			outputCallIDs[candidate] = callID
		}
	}
	return outputCallIDs
}

func shouldInsertGrokReasoningReplayBefore(item gjson.Result) bool {
	if strings.TrimSpace(item.Get("type").String()) != "message" {
		return true
	}
	switch strings.TrimSpace(item.Get("role").String()) {
	case "developer", "system":
		return false
	default:
		return true
	}
}

func grokReplayToolCallKeys(item gjson.Result) []string {
	itemType := strings.TrimSpace(item.Get("type").String())
	if itemType != "function_call" && itemType != "custom_tool_call" {
		return nil
	}
	callIDs := grokReplayComparableCallIDs(item.Get("call_id").String())
	if len(callIDs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(callIDs))
	for _, callID := range callIDs {
		keys = append(keys, itemType+":"+callID)
	}
	return keys
}

func grokReplayAnyToolCallKeyExists(existing map[string]bool, keys []string) bool {
	for _, key := range keys {
		if existing[key] {
			return true
		}
	}
	return false
}

func grokReplayComparableCallIDs(callID string) []string {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil
	}
	return []string{callID}
}

func normalizeGrokReasoningReplayItems(items [][]byte) ([][]byte, bool) {
	normalized := make([][]byte, 0, len(items))
	for _, item := range items {
		normalizedItem, ok := normalizeGrokReasoningReplayItem(item)
		if ok {
			normalized = append(normalized, normalizedItem)
		}
	}
	return normalized, len(normalized) > 0
}

func normalizeGrokReasoningReplayItem(item []byte) ([]byte, bool) {
	itemResult := gjson.ParseBytes(item)
	switch strings.TrimSpace(itemResult.Get("type").String()) {
	case "reasoning":
		return normalizeGrokReasoningReplayReasoningItem(itemResult)
	case "function_call":
		return normalizeGrokReasoningReplayFunctionCallItem(itemResult)
	case "custom_tool_call":
		return normalizeGrokReasoningReplayCustomToolCallItem(itemResult)
	default:
		return nil, false
	}
}

func normalizeGrokReasoningReplayReasoningItem(itemResult gjson.Result) ([]byte, bool) {
	encryptedContentResult := itemResult.Get("encrypted_content")
	if encryptedContentResult.Type == gjson.String {
		encryptedContent := encryptedContentResult.String()
		if encryptedContent == strings.TrimSpace(encryptedContent) {
			if _, err := xai.InspectGrokEncryptedContent(encryptedContent); err == nil {
				normalized := []byte(`{"type":"reasoning","summary":[],"content":null}`)
				normalized, _ = sjson.SetBytes(normalized, "encrypted_content", encryptedContent)
				return normalized, true
			}
		}
	}
	if summaryText := grokReasoningReplaySummaryText(itemResult); summaryText != "" {
		normalized := []byte(`{"type":"reasoning","summary":[],"content":null}`)
		summaryItem := []byte(`{"type":"summary_text","text":""}`)
		summaryItem, _ = sjson.SetBytes(summaryItem, "text", summaryText)
		normalized, _ = sjson.SetRawBytes(normalized, "summary.0", summaryItem)
		return normalized, true
	}
	return nil, false
}

func grokReasoningReplaySummaryText(itemResult gjson.Result) string {
	const maxSummaryChars = 2048
	chunks := make([]string, 0, 4)
	for _, part := range itemResult.Get("content").Array() {
		if strings.TrimSpace(part.Get("type").String()) != "reasoning_text" {
			continue
		}
		if text := strings.TrimSpace(part.Get("text").String()); text != "" {
			chunks = append(chunks, text)
		}
	}
	if len(chunks) == 0 {
		for _, part := range itemResult.Get("summary").Array() {
			if text := strings.TrimSpace(part.Get("text").String()); text != "" {
				chunks = append(chunks, text)
			}
		}
	}
	text := strings.TrimSpace(strings.Join(chunks, "\n"))
	if text == "" {
		return ""
	}
	if len(text) > maxSummaryChars {
		text = text[:maxSummaryChars]
	}
	return text
}

func normalizeGrokReasoningReplayFunctionCallItem(itemResult gjson.Result) ([]byte, bool) {
	callID := strings.TrimSpace(itemResult.Get("call_id").String())
	name := strings.TrimSpace(itemResult.Get("name").String())
	arguments := itemResult.Get("arguments")
	if callID == "" || name == "" || arguments.Type != gjson.String {
		return nil, false
	}

	normalized := []byte(`{"type":"function_call"}`)
	normalized, _ = sjson.SetBytes(normalized, "call_id", callID)
	normalized, _ = sjson.SetBytes(normalized, "name", name)
	normalized, _ = sjson.SetBytes(normalized, "arguments", arguments.String())
	return normalized, true
}

func normalizeGrokReasoningReplayCustomToolCallItem(itemResult gjson.Result) ([]byte, bool) {
	callID := strings.TrimSpace(itemResult.Get("call_id").String())
	name := strings.TrimSpace(itemResult.Get("name").String())
	input := itemResult.Get("input")
	if callID == "" || name == "" || !input.Exists() {
		return nil, false
	}

	normalized := []byte(`{"type":"custom_tool_call","status":"completed"}`)
	if status := strings.TrimSpace(itemResult.Get("status").String()); status != "" {
		normalized, _ = sjson.SetBytes(normalized, "status", status)
	}
	normalized, _ = sjson.SetBytes(normalized, "call_id", callID)
	normalized, _ = sjson.SetBytes(normalized, "name", name)
	if input.Type == gjson.String {
		normalized, _ = sjson.SetBytes(normalized, "input", input.String())
	} else {
		normalized, _ = sjson.SetRawBytes(normalized, "input", []byte(input.Raw))
	}
	return normalized, true
}

func cloneGrokReasoningReplayItems(items [][]byte) [][]byte {
	cloned := make([][]byte, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, append([]byte(nil), item...))
	}
	return cloned
}

func evictOldestGrokReasoningReplayEntriesLocked(count int) {
	if count <= 0 || len(grokReasoningReplayEntries) == 0 {
		return
	}
	type candidate struct {
		key       string
		timestamp time.Time
	}
	candidates := make([]candidate, 0, len(grokReasoningReplayEntries))
	for key, entry := range grokReasoningReplayEntries {
		candidates = append(candidates, candidate{key: key, timestamp: entry.Timestamp})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].timestamp.Before(candidates[j].timestamp)
	})
	if count > len(candidates) {
		count = len(candidates)
	}
	for i := 0; i < count; i++ {
		delete(grokReasoningReplayEntries, candidates[i].key)
	}
}

func sanitizeGrokInputEncryptedContent(body []byte) []byte {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || !input.IsArray() {
		return body
	}
	items := make([]json.RawMessage, 0, len(input.Array()))
	changed := false
	for _, item := range input.Array() {
		itemType := strings.TrimSpace(item.Get("type").String())
		if itemType != "reasoning" && itemType != "compaction" {
			items = append(items, json.RawMessage(item.Raw))
			continue
		}
		encryptedContent := item.Get("encrypted_content")
		if !encryptedContent.Exists() {
			items = append(items, json.RawMessage(item.Raw))
			continue
		}
		reason := ""
		switch encryptedContent.Type {
		case gjson.String:
			if _, err := xai.InspectGrokEncryptedContent(encryptedContent.String()); err != nil {
				reason = err.Error()
			}
		case gjson.Null:
			reason = "encrypted_content is null"
		default:
			reason = "encrypted_content must be a string"
		}
		if reason == "" {
			items = append(items, json.RawMessage(item.Raw))
			continue
		}
		if itemType == "compaction" {
			changed = true
			continue
		}
		next, err := sjson.DeleteBytes([]byte(item.Raw), "encrypted_content")
		if err != nil {
			items = append(items, json.RawMessage(item.Raw))
			continue
		}
		items = append(items, json.RawMessage(next))
		changed = true
	}
	if !changed {
		return body
	}
	rawInput, err := json.Marshal(items)
	if err != nil {
		return body
	}
	updated, err := sjson.SetRawBytes(body, "input", rawInput)
	if err != nil {
		return body
	}
	return updated
}