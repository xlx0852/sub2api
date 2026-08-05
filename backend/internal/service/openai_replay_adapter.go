package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

type openAIResponsesItemDialect string

const (
	openAIResponsesItemDialectAuto     openAIResponsesItemDialect = "auto"
	openAIResponsesItemDialectStandard openAIResponsesItemDialect = "openai_standard"
	openAIResponsesItemDialectCodex    openAIResponsesItemDialect = "codex_native"
)

const openAIResponsesItemDialectExtraKey = "openai_responses_item_dialect"

func resolveOpenAIResponsesItemDialect(account *Account) openAIResponsesItemDialect {
	if account == nil {
		return openAIResponsesItemDialectStandard
	}
	if account.Extra != nil {
		switch openAIResponsesItemDialect(strings.ToLower(strings.TrimSpace(stringValue(account.Extra[openAIResponsesItemDialectExtraKey])))) {
		case openAIResponsesItemDialectStandard:
			return openAIResponsesItemDialectStandard
		case openAIResponsesItemDialectCodex:
			return openAIResponsesItemDialectCodex
		}
	}
	if account.IsOpenAIOAuth() {
		return openAIResponsesItemDialectCodex
	}
	return openAIResponsesItemDialectAuto
}

func normalizeOpenAIReplayItemsForAccount(account *Account, items []json.RawMessage) []json.RawMessage {
	switch resolveOpenAIResponsesItemDialect(account) {
	case openAIResponsesItemDialectCodex:
		return processOpenAIReplayItems(items, false)
	case openAIResponsesItemDialectStandard:
		return processOpenAIReplayItems(items, true)
	}
	// Auto mode is evidence based: if the upstream itself emitted a native
	// Codex call type, it supports that dialect and replay must preserve it.
	// Otherwise use the portable standard representation. This avoids guessing
	// from provider names or URLs and keeps explicit account configuration in
	// control.
	for _, raw := range items {
		item, ok := decodeOpenAIReplayItem(raw)
		if !ok {
			continue
		}
		switch strings.TrimSpace(stringValue(item["type"])) {
		case "custom_tool_call", "custom_tool_call_output",
			"local_shell_call", "local_shell_call_output",
			"tool_search_call", "tool_search_output",
			"mcp_tool_call", "mcp_tool_call_output":
			return processOpenAIReplayItems(items, false)
		}
	}
	return processOpenAIReplayItems(items, true)
}

// alignOpenAIReplayOutputTypesToCalls repairs only mixed pairs created at the
// replay merge boundary. It leaves complete client-native pairs untouched and
// never fabricates a missing call for a true orphan output.
func alignOpenAIReplayOutputTypesToCalls(items []json.RawMessage) []json.RawMessage {
	if len(items) == 0 {
		return nil
	}
	type callTypeState struct {
		typ       string
		callID    string
		ambiguous bool
	}
	callsByCallID := make(map[string]callTypeState)
	callsByItemID := make(map[string]callTypeState)
	addCall := func(index map[string]callTypeState, key string, state callTypeState) {
		if key == "" {
			return
		}
		previous, exists := index[key]
		if exists && (previous.typ != state.typ || previous.callID != state.callID) {
			previous.ambiguous = true
			index[key] = previous
			return
		}
		index[key] = state
	}
	for _, raw := range items {
		item, ok := decodeOpenAIReplayItem(raw)
		if !ok || !isCodexToolCallContextItemType(strings.TrimSpace(stringValue(item["type"]))) {
			continue
		}
		callID := strings.TrimSpace(stringValue(item["call_id"]))
		if callID == "" {
			continue
		}
		typ := strings.TrimSpace(stringValue(item["type"]))
		state := callTypeState{typ: typ, callID: callID}
		addCall(callsByCallID, callID, state)
		addCall(callsByItemID, strings.TrimSpace(stringValue(item["id"])), state)
	}

	aligned := cloneOpenAIWSRawMessages(items)
	for i, raw := range aligned {
		item, ok := decodeOpenAIReplayItem(raw)
		if !ok || !isOpenAIReplayCallOutputType(strings.TrimSpace(stringValue(item["type"]))) {
			continue
		}
		callID := strings.TrimSpace(stringValue(item["call_id"]))
		state, exists := callsByCallID[callID]
		if !exists {
			state, exists = callsByItemID[callID]
			if exists && !state.ambiguous && state.callID != "" {
				item["call_id"] = state.callID
			}
		}
		if !exists || state.ambiguous || state.typ != "function_call" {
			continue
		}
		if strings.TrimSpace(stringValue(item["type"])) == "function_call_output" &&
			strings.TrimSpace(stringValue(item["call_id"])) == callID {
			continue
		}
		normalized, keep := normalizeOpenAIReplayCallOutput(item)
		if !keep {
			continue
		}
		encoded, err := json.Marshal(normalized)
		if err == nil {
			aligned[i] = encoded
		}
	}
	return aligned
}

// normalizeOpenAIReplayItems adapts only server-collected replay output. Client
// input must not be passed here unless an upstream explicitly rejected an
// indexed item and requested the targeted compatibility repair below.
func normalizeOpenAIReplayItems(items []json.RawMessage) []json.RawMessage {
	return processOpenAIReplayItems(items, true)
}

type openAIReplayDecodedItem struct {
	item map[string]any
}

type openAIReplayAssociationIndex struct {
	callIDByKey map[string]string
	ambiguous   map[string]struct{}
}

func processOpenAIReplayItems(items []json.RawMessage, adaptToStandard bool) []json.RawMessage {
	if len(items) == 0 {
		return nil
	}
	decoded := make([]openAIReplayDecodedItem, 0, len(items))
	for _, raw := range items {
		item, ok := decodeOpenAIReplayItem(raw)
		if ok {
			decoded = append(decoded, openAIReplayDecodedItem{item: item})
		}
	}
	associations := newOpenAIReplayAssociationIndex(decoded)

	type replayCandidate struct {
		item   map[string]any
		callID string
		isCall bool
		isOut  bool
	}
	candidates := make([]replayCandidate, 0, len(items))
	originalCallIDs := make(map[string]struct{})
	keptCallIDs := make(map[string]struct{})
	seenIDs := make(map[string]struct{})
	for _, decodedItem := range decoded {
		item := cloneOpenAIReplayItem(decodedItem.item)
		originalType := strings.TrimSpace(stringValue(item["type"]))
		isCall := isCodexToolCallContextItemType(originalType)
		isOut := isOpenAIReplayCallOutputType(originalType)
		callID := strings.TrimSpace(stringValue(item["call_id"]))
		if callID == "" && isOut {
			callID = associations.resolve(item)
			if callID != "" {
				item["call_id"] = callID
			}
		}
		if isCall && callID != "" {
			originalCallIDs[callID] = struct{}{}
		}

		outputItem := item
		if isCall || isOut {
			validated, keep := normalizeOpenAIReplayItem(item)
			if !keep {
				continue
			}
			if adaptToStandard {
				outputItem = validated
			}
		}
		if id := strings.TrimSpace(stringValue(outputItem["id"])); id != "" {
			if _, duplicate := seenIDs[id]; duplicate {
				// Item id is not the call/output association key. Keep the semantic
				// item and omit only the duplicate id; dropping an output here would
				// break native dialects that reuse one private id for a call pair.
				delete(outputItem, "id")
			} else {
				seenIDs[id] = struct{}{}
			}
		}
		if isCall && callID != "" {
			keptCallIDs[callID] = struct{}{}
		}
		candidates = append(candidates, replayCandidate{item: outputItem, callID: callID, isCall: isCall, isOut: isOut})
	}

	normalized := make([]json.RawMessage, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.isOut && candidate.callID != "" {
			if _, hadCall := originalCallIDs[candidate.callID]; hadCall {
				if _, keptCall := keptCallIDs[candidate.callID]; !keptCall {
					continue
				}
			}
		}
		encoded, err := json.Marshal(candidate.item)
		if err != nil {
			continue
		}
		normalized = append(normalized, encoded)
	}
	return normalized
}

func newOpenAIReplayAssociationIndex(items []openAIReplayDecodedItem) openAIReplayAssociationIndex {
	index := openAIReplayAssociationIndex{
		callIDByKey: make(map[string]string),
		ambiguous:   make(map[string]struct{}),
	}
	for _, decoded := range items {
		item := decoded.item
		if !isCodexToolCallContextItemType(strings.TrimSpace(stringValue(item["type"]))) {
			continue
		}
		callID := strings.TrimSpace(stringValue(item["call_id"]))
		id := strings.TrimSpace(stringValue(item["id"]))
		if callID == "" || id == "" {
			continue
		}
		index.add("exact:"+id, callID)
		if identity := openAIReplayPrivateItemIdentity(id); identity != "" {
			index.add("identity:"+identity, callID)
		}
	}
	return index
}

func (i openAIReplayAssociationIndex) add(key, callID string) {
	if key == "" || callID == "" {
		return
	}
	if _, conflict := i.ambiguous[key]; conflict {
		return
	}
	if previous, exists := i.callIDByKey[key]; exists && previous != callID {
		delete(i.callIDByKey, key)
		i.ambiguous[key] = struct{}{}
		return
	}
	i.callIDByKey[key] = callID
}

func (i openAIReplayAssociationIndex) resolve(item map[string]any) string {
	id := strings.TrimSpace(stringValue(item["id"]))
	if id == "" {
		return ""
	}
	if callID := i.callIDByKey["exact:"+id]; callID != "" {
		return callID
	}
	if identity := openAIReplayPrivateItemIdentity(id); identity != "" {
		return i.callIDByKey["identity:"+identity]
	}
	return ""
}

func openAIReplayPrivateItemIdentity(id string) string {
	trimmed := strings.TrimSpace(id)
	for _, prefix := range []string{"ctco_", "ctc_", "tsco_", "tsc_", "mcp_", "lc_"} {
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimPrefix(trimmed, prefix)
		}
	}
	return trimmed
}

func cloneOpenAIReplayItem(item map[string]any) map[string]any {
	cloned := make(map[string]any, len(item))
	for key, value := range item {
		cloned[key] = value
	}
	return cloned
}

func isOpenAIReplayCallOutputType(typ string) bool {
	return isCodexToolCallOutputItemType(typ) || strings.TrimSpace(typ) == "local_shell_call_output"
}

func decodeOpenAIReplayItem(raw []byte) (map[string]any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var item map[string]any
	if err := decoder.Decode(&item); err != nil || item == nil {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, false
	}
	return item, true
}

func normalizeOpenAIReplayItem(item map[string]any) (map[string]any, bool) {
	typ := strings.TrimSpace(stringValue(item["type"]))
	switch typ {
	case "function_call", "tool_call", "local_shell_call", "tool_search_call", "custom_tool_call", "mcp_tool_call":
		return normalizeOpenAIReplayCall(item, typ)
	case "function_call_output", "local_shell_call_output", "tool_search_output", "custom_tool_call_output", "mcp_tool_call_output":
		return normalizeOpenAIReplayCallOutput(item)
	default:
		return item, true
	}
}

func normalizeOpenAIReplayCall(item map[string]any, typ string) (map[string]any, bool) {
	callID := strings.TrimSpace(stringValue(item["call_id"]))
	if callID == "" {
		return nil, false
	}
	name := strings.TrimSpace(stringValue(item["name"]))
	if name == "" {
		name = strings.TrimSpace(stringValue(item["tool_name"]))
	}
	if name == "" && typ == "local_shell_call" {
		name = "shell"
	}
	if name == "" {
		return nil, false
	}

	var rawArguments any
	switch typ {
	case "local_shell_call":
		rawArguments = item["arguments"]
		if rawArguments == nil {
			rawArguments = item["action"]
		}
	case "custom_tool_call":
		if rawInput, exists := item["input"]; exists {
			rawArguments = map[string]any{"input": rawInput}
		} else {
			rawArguments = item["arguments"]
		}
	default:
		rawArguments = item["arguments"]
	}
	arguments, ok := completeOpenAIReplayArguments(rawArguments)
	if !ok {
		return nil, false
	}

	adapted := map[string]any{
		"type":      "function_call",
		"call_id":   callID,
		"name":      name,
		"arguments": arguments,
	}
	if status := strings.TrimSpace(stringValue(item["status"])); status != "" {
		adapted["status"] = status
	}
	if id := strings.TrimSpace(stringValue(item["id"])); strings.HasPrefix(id, "fc") {
		adapted["id"] = id
	}
	return adapted, true
}

func normalizeOpenAIReplayCallOutput(item map[string]any) (map[string]any, bool) {
	callID := strings.TrimSpace(stringValue(item["call_id"]))
	if callID == "" {
		return nil, false
	}
	rawOutput, exists := item["output"]
	if !exists {
		return nil, false
	}
	output, ok := openAIReplayOutputValue(rawOutput)
	if !ok {
		return nil, false
	}
	// Item id is not the call/output link. Omitting it avoids private prefixes
	// and prevents ctc_x/ctco_x from collapsing to the same synthetic fc_x id.
	return map[string]any{
		"type":    "function_call_output",
		"call_id": callID,
		"output":  output,
	}, true
}

func completeOpenAIReplayArguments(raw any) (string, bool) {
	switch typed := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" || !json.Valid([]byte(trimmed)) {
			return "", false
		}
		return typed, true
	case map[string]any, []any:
		encoded, err := json.Marshal(typed)
		if err != nil || !json.Valid(encoded) {
			return "", false
		}
		return string(encoded), true
	default:
		return "", false
	}
}

func openAIReplayOutputValue(raw any) (any, bool) {
	switch typed := raw.(type) {
	case string:
		return typed, true
	case []any:
		// Responses accepts structured function-call output content. Preserve it
		// instead of collapsing images/files into a lossy JSON string.
		return typed, true
	case nil:
		return nil, false
	default:
		return nil, false
	}
}

// normalizeOpenAIRejectedReplayPairAtIndex applies the replay adapter only to
// the explicitly rejected item and items sharing its call_id.
func normalizeOpenAIRejectedReplayPairAtIndex(body []byte, index int) ([]byte, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var request map[string]any
	if err := decoder.Decode(&request); err != nil {
		return nil, false, fmt.Errorf("decode Responses retry body: %w", err)
	}
	input, ok := request["input"].([]any)
	if !ok || index < 0 || index >= len(input) {
		return nil, false, nil
	}
	target, ok := input[index].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	targetType := strings.TrimSpace(stringValue(target["type"]))
	if !isCodexToolCallContextItemType(targetType) &&
		!isOpenAIReplayCallOutputType(targetType) {
		return nil, false, nil
	}
	decoded := make([]openAIReplayDecodedItem, 0, len(input))
	for _, raw := range input {
		if item, itemOK := raw.(map[string]any); itemOK {
			decoded = append(decoded, openAIReplayDecodedItem{item: item})
		}
	}
	associations := newOpenAIReplayAssociationIndex(decoded)
	targetCallID := strings.TrimSpace(stringValue(target["call_id"]))
	if targetCallID == "" && isOpenAIReplayCallOutputType(targetType) {
		targetCallID = associations.resolve(target)
		if targetCallID == "" {
			return nil, false, nil
		}
	}
	selected := make([]int, 0, 2)
	for i, raw := range input {
		item, itemOK := raw.(map[string]any)
		if !itemOK {
			continue
		}
		itemCallID := strings.TrimSpace(stringValue(item["call_id"]))
		if itemCallID == "" && isOpenAIReplayCallOutputType(strings.TrimSpace(stringValue(item["type"]))) {
			itemCallID = associations.resolve(item)
		}
		if i == index || (targetCallID != "" && itemCallID == targetCallID) {
			selected = append(selected, i)
		}
	}
	if len(selected) == 0 {
		return nil, false, nil
	}

	nextInput := append([]any(nil), input...)
	changed := false
	for _, i := range selected {
		item := cloneOpenAIReplayItem(input[i].(map[string]any))
		if strings.TrimSpace(stringValue(item["call_id"])) == "" &&
			isOpenAIReplayCallOutputType(strings.TrimSpace(stringValue(item["type"]))) {
			if resolvedCallID := associations.resolve(item); resolvedCallID != "" {
				item["call_id"] = resolvedCallID
			}
		}
		normalized, keep := normalizeOpenAIReplayItem(item)
		if !keep {
			return nil, false, nil
		}
		if !reflect.DeepEqual(item, normalized) {
			changed = true
		}
		nextInput[i] = normalized
	}
	if !changed {
		return nil, false, nil
	}
	request["input"] = nextInput
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, false, fmt.Errorf("encode Responses retry body: %w", err)
	}
	return encoded, true, nil
}

// normalizeOpenAIRejectedReplayPairByCallID repairs only an explicitly
// rejected call/output pair. It deliberately refuses true orphan outputs:
// without a real call item there is no safe context to fabricate.
func normalizeOpenAIRejectedReplayPairByCallID(body []byte, callID string) ([]byte, bool, error) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil, false, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var request map[string]any
	if err := decoder.Decode(&request); err != nil {
		return nil, false, fmt.Errorf("decode Responses retry body: %w", err)
	}
	input, ok := request["input"].([]any)
	if !ok {
		return nil, false, nil
	}

	directCalls := make([]int, 0, 1)
	itemIDAliasCalls := make([]int, 0, 1)
	prefixAliasCalls := make([]int, 0, 1)
	exactOutputs := make([]int, 0, 1)
	prefixAliasOutputs := make([]int, 0, 1)
	for i, raw := range input {
		item, itemOK := raw.(map[string]any)
		if !itemOK {
			continue
		}
		typ := strings.TrimSpace(stringValue(item["type"]))
		itemCallID := strings.TrimSpace(stringValue(item["call_id"]))
		if isCodexToolCallContextItemType(typ) {
			switch {
			case itemCallID == callID:
				directCalls = append(directCalls, i)
			case strings.TrimSpace(stringValue(item["id"])) == callID:
				itemIDAliasCalls = append(itemIDAliasCalls, i)
			case openAIReplayCallIDPrefixEquivalent(itemCallID, callID):
				prefixAliasCalls = append(prefixAliasCalls, i)
			}
		}
		if isOpenAIReplayCallOutputType(typ) {
			if itemCallID == callID {
				exactOutputs = append(exactOutputs, i)
			} else if openAIReplayCallIDPrefixEquivalent(itemCallID, callID) {
				prefixAliasOutputs = append(prefixAliasOutputs, i)
			}
		}
	}

	callIndex := -1
	effectiveCallID := ""
	selectedOutputs := exactOutputs
	rewriteCallID := false
	switch {
	case len(directCalls) == 1:
		callIndex = directCalls[0]
		effectiveCallID = callID
	case len(directCalls) > 1:
		return nil, false, nil
	case len(itemIDAliasCalls) == 1:
		callIndex = itemIDAliasCalls[0]
		effectiveCallID = strings.TrimSpace(stringValue(input[callIndex].(map[string]any)["call_id"]))
	case len(itemIDAliasCalls) > 1:
		return nil, false, nil
	case len(prefixAliasCalls) == 1:
		callIndex = prefixAliasCalls[0]
		// The rejecting compatibility layer already canonicalized call_ to fc_.
		// Retry one outbound copy with that exact key, updating both sides of the
		// pair. The client-visible history remains untouched.
		effectiveCallID = callID
		selectedOutputs = prefixAliasOutputs
		rewriteCallID = true
	default:
		return nil, false, nil
	}
	if callIndex < 0 || effectiveCallID == "" || len(selectedOutputs) == 0 {
		return nil, false, nil
	}
	selected := append([]int{callIndex}, selectedOutputs...)

	nextInput := append([]any(nil), input...)
	changed := false
	for _, i := range selected {
		item := cloneOpenAIReplayItem(input[i].(map[string]any))
		if rewriteCallID || isOpenAIReplayCallOutputType(strings.TrimSpace(stringValue(item["type"]))) {
			item["call_id"] = effectiveCallID
		}
		normalized, keep := normalizeOpenAIReplayItem(item)
		if !keep {
			return nil, false, nil
		}
		if !reflect.DeepEqual(item, normalized) {
			changed = true
		}
		nextInput[i] = normalized
	}
	if !changed {
		return nil, false, nil
	}
	request["input"] = nextInput
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, false, fmt.Errorf("encode Responses retry body: %w", err)
	}
	return encoded, true, nil
}

func openAIReplayCallIDPrefixEquivalent(left, right string) bool {
	leftPrefix, leftSuffix := splitOpenAIReplayCallIDPrefix(left)
	rightPrefix, rightSuffix := splitOpenAIReplayCallIDPrefix(right)
	return leftSuffix != "" && leftSuffix == rightSuffix && leftPrefix != rightPrefix
}

func splitOpenAIReplayCallIDPrefix(callID string) (string, string) {
	trimmed := strings.TrimSpace(callID)
	for _, prefix := range []string{"call_", "fc_"} {
		if strings.HasPrefix(trimmed, prefix) {
			return prefix, strings.TrimPrefix(trimmed, prefix)
		}
	}
	return "", trimmed
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}
