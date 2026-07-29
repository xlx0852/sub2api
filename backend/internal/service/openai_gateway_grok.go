package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	grokComposerImageBridgeVisionModel     = "grok-build-0.1"
	grokComposerImageBridgeMaxOutputTokens = 512
	grokCodexAppNamespaceName              = "codex_app"
	grokAutomationUpdateToolName           = "automation_update"
)

// xAI rejects function tools whose object schemas omit additionalProperties:false
// (Cursor/Claude Desktop often send open object schemas). Keep empty open-ended
// fallbacks closed as well so automation_update and similar tools stay valid.
var grokSafeFunctionParameters = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)

func (s *OpenAIGatewayService) forwardGrokResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	originalModel string,
	reqStream bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	if account.Type != AccountTypeOAuth && account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("grok account type %s is not supported by responses forwarding", account.Type)
	}

	upstreamModel := account.GetMappedModel(originalModel)
	if strings.TrimSpace(upstreamModel) == "" {
		upstreamModel = xai.CurrentDefaultChatModel()
	}
	cacheIdentity := resolveGrokCacheIdentity(c, body, "", upstreamModel)
	patchedBody, translation, err := patchGrokResponsesBodyWithTranslation(body, upstreamModel)
	if err != nil {
		return nil, err
	}
	setGrokCodexTranslation(c, translation)
	patchedBody, err = applyGrokResponsesCacheIdentity(patchedBody, body, cacheIdentity, account.IsGrokOAuth())
	if err != nil {
		return nil, fmt.Errorf("apply grok prompt cache identity: %w", err)
	}
	patchedBody, err = s.optimizeGrokPayload(account, patchedBody, grokPayloadResponses)
	if err != nil {
		if handleGrokPayloadOptimizationError(c, grokPayloadResponses, err) {
			return nil, err
		}
		return nil, fmt.Errorf("optimize Grok Responses payload: %w", err)
	}

	token, _, credential, err := s.getGrokAccessTokenWithDescriptor(ctx, account)
	if err != nil {
		return nil, err
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	upstreamReq, err := buildGrokResponsesRequest(upstreamCtx, c, account, patchedBody, token, cacheIdentity)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		s.updateGrokUsageSnapshot(ctx, account, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
		}
		attribution := s.attributeGrokUnauthorized(ctx, c, account, resp.StatusCode, credential)
		retryDisabled := grokUpstreamExplicitlyDisablesRetry(resp.Header)
		if retryDisabled {
			return s.handleErrorResponse(ctx, resp, c, account, patchedBody, upstreamModel)
		}
		opsEvent := OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
			Kind:               "failover",
			Message:            upstreamMsg,
		}
		applyGrokUnauthorizedAttribution(&opsEvent, attribution)
		appendOpsUpstreamError(c, opsEvent)
		grokSameAccountRetry := s.isGrokSameAccountRetry(c, resp.StatusCode)
		penalizeAccount := shouldPenalizeGrokUnauthorized(resp.StatusCode, attribution)
		if !grokSameAccountRetry && penalizeAccount {
			s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:                resp.StatusCode,
				ResponseBody:              respBody,
				ResponseHeaders:           resp.Header.Clone(),
				RetryableOnSameAccount:    grokSameAccountRetry || (account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)),
				GrokSameAccountRetry:      grokSameAccountRetry,
				GrokAccountPenaltyApplied: !grokSameAccountRetry && penalizeAccount,
				GrokStaleCredential:       !penalizeAccount,
			}
		}
		return s.handleErrorResponse(ctx, resp, c, account, patchedBody, upstreamModel)
	}

	s.updateGrokUsageSnapshot(ctx, account, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))

	var usage *OpenAIUsage
	var firstTokenMs *int
	responseID := ""
	if reqStream {
		streamResult, err := s.handleStreamingResponse(ctx, resp, c, account, startTime, originalModel, upstreamModel)
		if err != nil {
			return nil, err
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
		responseID = strings.TrimSpace(streamResult.responseID)
	} else {
		nonStreamResult, err := s.handleNonStreamingResponse(ctx, resp, c, account, originalModel, upstreamModel)
		if err != nil {
			return nil, err
		}
		usage = nonStreamResult.usage
		responseID = strings.TrimSpace(nonStreamResult.responseID)
	}

	if usage == nil {
		usage = &OpenAIUsage{}
	}
	reasoningEffort := extractOpenAIReasoningEffortFromBody(patchedBody, originalModel)
	return &OpenAIForwardResult{
		RequestID:       firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
		ResponseID:      responseID,
		Usage:           *usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		Stream:          reqStream,
		OpenAIWSMode:    false,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
	}, nil
}

func patchGrokResponsesBody(body []byte, upstreamModel string) ([]byte, error) {
	out, _, err := patchGrokResponsesBodyWithTranslation(body, upstreamModel)
	return out, err
}

func patchGrokResponsesBodyWithTranslation(body []byte, upstreamModel string) ([]byte, *grokCodexTranslation, error) {
	if !json.Valid(body) {
		return nil, nil, fmt.Errorf("invalid json request body")
	}
	translation := newGrokCodexTranslation(body)
	out, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return nil, nil, err
	}
	for _, unsupportedField := range []string{
		"previous_response_id",
		"context_management",
		"prompt_cache_retention",
		"safety_identifier",
		"stream_options",
	} {
		if gjson.GetBytes(out, unsupportedField).Exists() {
			out, err = sjson.DeleteBytes(out, unsupportedField)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	if strings.EqualFold(upstreamModel, "grok-4.5") {
		for _, unsupportedField := range []string{"presence_penalty", "presencePenalty", "frequency_penalty", "frequencyPenalty", "stop"} {
			if gjson.GetBytes(out, unsupportedField).Exists() {
				out, err = sjson.DeleteBytes(out, unsupportedField)
				if err != nil {
					return nil, nil, err
				}
			}
		}
	}
	out, err = sanitizeGrokResponsesUnsupportedFields(out)
	if err != nil {
		return nil, nil, err
	}
	out, err = sanitizeGrokResponsesModelCapabilities(out, upstreamModel)
	if err != nil {
		return nil, nil, err
	}
	out, err = sanitizeGrokReasoningNullContent(out)
	if err != nil {
		return nil, nil, err
	}
	out, err = normalizeGrokResponsesInput(out, translation)
	if err != nil {
		return nil, nil, err
	}
	out, err = sanitizeGrokResponsesTools(out, translation)
	if err != nil {
		return nil, nil, err
	}
	out, err = normalizeGrokResponsesTopLevel(out)
	if err != nil {
		return nil, nil, err
	}
	return out, translation, nil
}

// sanitizeGrokResponsesModelCapabilities strips reasoning controls that certain
// Grok Composer models reject (422).
func sanitizeGrokResponsesModelCapabilities(body []byte, upstreamModel string) ([]byte, error) {
	if !grokModelRejectsReasoningEffort(upstreamModel) {
		return body, nil
	}
	out := body
	for _, field := range []string{"reasoning", "reasoning_effort", "reasoningEffort"} {
		if !gjson.GetBytes(out, field).Exists() {
			continue
		}
		var err error
		out, err = sjson.DeleteBytes(out, field)
		if err != nil {
			return nil, fmt.Errorf("remove unsupported Grok Composer %s: %w", field, err)
		}
	}
	return out, nil
}

func grokModelRejectsReasoningEffort(model string) bool {
	model = strings.TrimSpace(strings.ToLower(model))
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		model = strings.TrimSpace(model[slash+1:])
	}
	switch model {
	case "grok-composer", "grok-composer-2.5-fast", "composer-2.5":
		return true
	default:
		return false
	}
}

// sanitizeGrokReasoningNullContent deletes reasoning items' "content": null.
// xAI's untagged enum deserializer rejects that field with 422. Local
// normalizeGrokResponsesInput already rewrites reasoning into assistant text,
// but keep this strip as a defensive pre-pass for any retained reasoning shape.
func sanitizeGrokReasoningNullContent(body []byte) ([]byte, error) {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || !input.IsArray() {
		return body, nil
	}
	items := input.Array()
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if strings.TrimSpace(item.Get("type").String()) != "reasoning" {
			continue
		}
		contentResult := item.Get("content")
		if contentResult.Exists() && contentResult.Type == gjson.Null {
			var err error
			body, err = sjson.DeleteBytes(body, fmt.Sprintf("input.%d.content", i))
			if err != nil {
				return nil, err
			}
		}
	}
	return body, nil
}

func normalizeGrokResponsesTopLevel(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := decodeGrokJSON(body, &payload); err != nil {
		return nil, err
	}
	if instructions, ok := payload["instructions"]; !ok || instructions == nil {
		payload["instructions"] = ""
	}
	if include, ok := payload["include"].([]any); ok {
		filtered := include[:0]
		for _, item := range include {
			value, _ := item.(string)
			if strings.TrimSpace(value) == "reasoning.encrypted_content" {
				continue
			}
			filtered = append(filtered, item)
		}
		if len(filtered) == 0 {
			delete(payload, "include")
		} else {
			payload["include"] = filtered
		}
	}
	return json.Marshal(payload)
}

// xAI ModelInput only reliably accepts easy messages plus function_call /
// function_call_output on the Grok OAuth / CLI proxy surface. Search/MCP/shell
// call items from Codex history fail untagged-enum decode even when the type
// name looks OpenAI-compatible.
var grokResponsesKeepInputTypes = map[string]struct{}{
	"message":              {},
	"function_call":        {},
	"function_call_output": {},
}

var grokResponsesDropInputTypes = map[string]struct{}{
	"computer_call":           {},
	"computer_call_output":    {},
	"tool_search_call":        {},
	"tool_search_output":      {},
	"apply_patch_call":        {},
	"apply_patch_call_output": {},
}

var grokResponsesCollapseInputTypes = map[string]string{
	"web_search_call":       "web search",
	"x_search_call":         "x search",
	"file_search_call":      "file search",
	"code_interpreter_call": "code interpreter",
	"code_execution_call":   "code execution",
	"mcp_call":              "mcp",
	"shell_call":            "shell",
}

func normalizeGrokResponsesInput(body []byte, translation *grokCodexTranslation) ([]byte, error) {
	var payload map[string]any
	if err := decodeGrokJSON(body, &payload); err != nil {
		return nil, err
	}

	switch input := payload["input"].(type) {
	case []any:
		payload["input"] = normalizeGrokResponsesInputItems(input, translation)
	case map[string]any:
		normalized := normalizeGrokResponsesInputItems([]any{input}, translation)
		switch len(normalized) {
		case 0:
			delete(payload, "input")
		case 1:
			payload["input"] = normalized[0]
		default:
			payload["input"] = normalized
		}
	default:
		return body, nil
	}
	return json.Marshal(payload)
}

func normalizeGrokResponsesInputItems(input []any, translation *grokCodexTranslation) []any {
	normalized := make([]any, 0, len(input))
	localShellCallIDs := make(map[string]any)
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || strings.TrimSpace(fmt.Sprint(item["type"])) != "local_shell_call" {
			continue
		}
		itemID := strings.TrimSpace(fmt.Sprint(item["id"]))
		if itemID != "" {
			localShellCallIDs[itemID] = firstNonEmptyAny(item["call_id"], item["id"], "call_shell")
		}
	}
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok {
			// Bare strings are valid easy ModelInput text.
			if text, ok := rawItem.(string); ok {
				normalized = append(normalized, text)
			}
			continue
		}

		itemType, _ := item["type"].(string)
		itemType = strings.TrimSpace(itemType)

		// Easy message: {role, content} without type — keep with cleaned content only.
		if itemType == "" && item["role"] != nil {
			if cleaned := normalizeGrokMessageItem(item); cleaned != nil {
				normalized = append(normalized, cleaned)
			}
			continue
		}

		switch itemType {
		case "compaction", "ghost_snapshot":
			// Opaque foreign state — never forward.
			continue
		case "reasoning":
			// Convert human-readable summary into plain assistant text so multi-turn
			// context is not completely lost, but drop the opaque reasoning item itself.
			if summary := extractGrokReasoningSummaryText(item); summary != "" {
				normalized = append(normalized, grokAssistantTextItem("[reasoning] "+summary))
			}
			continue
		case "message":
			if cleaned := normalizeGrokMessageItem(item); cleaned != nil {
				normalized = append(normalized, cleaned)
			}
			continue
		case "custom_tool_call":
			if converted := convertGrokCustomToolCallItem(item, translation); converted != nil {
				normalized = append(normalized, converted)
			}
			continue
		case "custom_tool_call_output":
			if converted := convertGrokFunctionCallOutputItem(item); converted != nil {
				normalized = append(normalized, converted)
			}
			continue
		case "function_call":
			if converted := convertGrokFunctionCallItem(item); converted != nil {
				normalized = append(normalized, converted)
			}
			continue
		case "function_call_output":
			if converted := convertGrokFunctionCallOutputItem(item); converted != nil {
				normalized = append(normalized, converted)
			}
			continue
		case "local_shell_call":
			// Codex shell history uses local_shell_call; xAI ModelInput does not accept it.
			// Preserve continuity by rewriting into a plain function_call.
			if converted := convertGrokLocalShellCallItem(item); converted != nil {
				normalized = append(normalized, converted)
			}
			continue
		case "local_shell_call_output":
			callID := firstNonEmptyAny(localShellCallIDs[strings.TrimSpace(fmt.Sprint(item["id"]))], item["call_id"], item["id"], "call_unknown")
			if converted := convertGrokFunctionCallOutputItemWithCallID(item, callID); converted != nil {
				normalized = append(normalized, converted)
			}
			continue
		case "image_generation_call":
			path := firstNonEmptyStringField(item, "path", "result", "output", "revised_prompt")
			if path == "" {
				path = "image"
			}
			normalized = append(normalized, grokAssistantTextItem("[image generated: "+path+"]"))
			continue
		case "web_search_call", "x_search_call", "file_search_call", "code_interpreter_call", "code_execution_call", "mcp_call", "shell_call":
			// These OpenAI call item types are not accepted by xAI ModelInput on the
			// Grok proxy. Collapse them to a short assistant note so history remains.
			label := grokResponsesCollapseInputTypes[itemType]
			if label == "" {
				label = itemType
			}
			detail := firstNonEmptyStringField(item, "query")
			if detail == "" {
				if action, ok := item["action"].(map[string]any); ok {
					detail = firstNonEmptyStringField(action, "query", "url", "pattern")
				}
			}
			text := "[" + label + "]"
			if detail != "" {
				text = "[" + label + ": " + detail + "]"
			}
			normalized = append(normalized, grokAssistantTextItem(text))
			continue
		case "output_text", "input_text":
			if text, ok := item["text"]; ok && text != nil {
				normalized = append(normalized, map[string]any{
					"role": "user",
					"content": []any{
						map[string]any{"type": "input_text", "text": text},
					},
				})
			}
			continue
		}

		if _, drop := grokResponsesDropInputTypes[itemType]; drop {
			continue
		}

		// Unknown tagged type: keep only if it looks like a message.
		if item["role"] != nil {
			if cleaned := normalizeGrokMessageItem(item); cleaned != nil {
				normalized = append(normalized, cleaned)
			}
		}
	}
	return normalized
}

func grokAssistantTextItem(text string) map[string]any {
	return map[string]any{
		"role": "assistant",
		"content": []any{
			map[string]any{"type": "input_text", "text": text},
		},
	}
}

func normalizeGrokMessageItem(item map[string]any) map[string]any {
	role := strings.TrimSpace(fmt.Sprint(item["role"]))
	if role == "" || role == "<nil>" {
		role = "user"
	}
	content := normalizeGrokContentValue(item["content"])
	if content == nil {
		return nil
	}
	return map[string]any{
		"type":    "message",
		"role":    role,
		"content": content,
	}
}

func normalizeGrokContentValue(raw any) any {
	switch typed := raw.(type) {
	case string:
		return typed
	case []any:
		parts := make([]any, 0, len(typed))
		for _, rawPart := range typed {
			switch part := rawPart.(type) {
			case string:
				if strings.TrimSpace(part) == "" {
					continue
				}
				parts = append(parts, map[string]any{"type": "input_text", "text": part})
			case map[string]any:
				if cleaned := normalizeGrokContentPart(part); cleaned != nil {
					parts = append(parts, cleaned)
				}
			}
		}
		if len(parts) == 0 {
			return nil
		}
		return parts
	default:
		return nil
	}
}

func normalizeGrokContentPart(part map[string]any) map[string]any {
	partType := strings.TrimSpace(fmt.Sprint(part["type"]))
	switch partType {
	case "output_text", "text":
		partType = "input_text"
	case "output_image":
		partType = "input_image"
	case "input_text", "input_image":
		// keep
	default:
		// Unknown content part types are not safe for xAI ModelInput.
		if text, ok := part["text"].(string); ok && strings.TrimSpace(text) != "" {
			return map[string]any{"type": "input_text", "text": text}
		}
		return nil
	}
	out := map[string]any{"type": partType}
	switch partType {
	case "input_text":
		text, _ := part["text"].(string)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		out["text"] = text
	case "input_image":
		imageURL := firstNonEmptyStringField(part, "image_url", "url")
		if imageURL == "" {
			// image_url may itself be an object {"url": "..."}
			if nested, ok := part["image_url"].(map[string]any); ok {
				imageURL = firstNonEmptyStringField(nested, "url", "image_url")
			}
		}
		if imageURL == "" {
			return nil
		}
		out["image_url"] = imageURL
	}
	return out
}

func convertGrokCustomToolCallItem(item map[string]any, translation *grokCodexTranslation) map[string]any {
	name := strings.TrimSpace(fmt.Sprint(item["name"]))
	if name == "" || name == "<nil>" {
		name = "custom_tool"
	}
	if translation != nil {
		translation.customTools[name] = struct{}{}
	}
	callID := firstNonEmptyAny(item["call_id"], item["id"], "call_"+name)

	var arguments string
	switch {
	case item["input"] != nil:
		arguments = encodeGrokCustomToolInput(item["input"])
	default:
		arguments = normalizeGrokFunctionArguments(item["arguments"])
	}

	return map[string]any{
		"type":      "function_call",
		"call_id":   callID,
		"name":      name,
		"arguments": arguments,
	}
}

func convertGrokFunctionCallItem(item map[string]any) map[string]any {
	name := strings.TrimSpace(fmt.Sprint(item["name"]))
	if name == "" || name == "<nil>" {
		name = "function"
	}
	return map[string]any{
		"type":      "function_call",
		"call_id":   firstNonEmptyAny(item["call_id"], item["id"], "call_"+name),
		"name":      name,
		"arguments": normalizeGrokFunctionArguments(item["arguments"]),
	}
}

func convertGrokLocalShellCallItem(item map[string]any) map[string]any {
	callID := firstNonEmptyAny(item["call_id"], item["id"], "call_shell")
	arguments := "{}"
	if action, ok := item["action"].(map[string]any); ok {
		encoded, err := json.Marshal(action)
		if err == nil {
			arguments = string(encoded)
		}
	} else if rawArgs := item["arguments"]; rawArgs != nil {
		arguments = normalizeGrokFunctionArguments(rawArgs)
	}
	return map[string]any{
		"type":      "function_call",
		"call_id":   callID,
		"name":      "shell",
		"arguments": arguments,
	}
}

func convertGrokFunctionCallOutputItem(item map[string]any) map[string]any {
	callID := firstNonEmptyAny(item["call_id"], item["id"], "call_unknown")
	return convertGrokFunctionCallOutputItemWithCallID(item, callID)
}

func convertGrokFunctionCallOutputItemWithCallID(item map[string]any, callID any) map[string]any {
	output := stringifyGrokToolOutput(item["output"])
	return map[string]any{
		"type":    "function_call_output",
		"call_id": callID,
		"output":  output,
	}
}

func normalizeGrokFunctionArguments(raw any) string {
	switch typed := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return "{}"
		}
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return typed
		}
		return encodeGrokCustomToolInput(typed)
	case map[string]any, []any:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "{}"
		}
		return string(encoded)
	case nil:
		return "{}"
	default:
		return encodeGrokCustomToolInput(typed)
	}
}

func stringifyGrokToolOutput(raw any) string {
	switch typed := raw.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		// Codex may encode function_call_output.output as content items:
		// [{"type":"input_text","text":"..."}, {"type":"input_image", ...}]
		// xAI ModelInput expects a plain string here.
		parts := make([]string, 0, len(typed))
		for _, rawPart := range typed {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			switch strings.TrimSpace(fmt.Sprint(part["type"])) {
			case "input_text", "output_text", "text":
				if text, ok := part["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			case "input_image", "output_image":
				// Drop image payloads; keep a short marker so tool continuity remains.
				parts = append(parts, "[image]")
			}
		}
		if len(parts) == 0 {
			encoded, err := json.Marshal(typed)
			if err != nil {
				return ""
			}
			return string(encoded)
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(encoded)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(encoded)
	}
}

func firstNonEmptyStringField(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := item[key].(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func firstNonEmptyAny(values ...any) any {
	for _, value := range values {
		switch typed := value.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(typed) == "" || typed == "<nil>" {
				continue
			}
			return typed
		default:
			text := strings.TrimSpace(fmt.Sprint(typed))
			if text == "" || text == "<nil>" {
				continue
			}
			return typed
		}
	}
	return "call_unknown"
}

func stripGrokEncryptedAnywhere(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "encrypted_content")
		for _, child := range typed {
			stripGrokEncryptedAnywhere(child)
		}
	case []any:
		for _, child := range typed {
			stripGrokEncryptedAnywhere(child)
		}
	}
}

func extractGrokReasoningSummaryText(item map[string]any) string {
	summary, ok := item["summary"].([]any)
	if !ok || len(summary) == 0 {
		return ""
	}
	parts := make([]string, 0, len(summary))
	for _, rawPart := range summary {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		text, ok := part["text"].(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func decodeGrokJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	return decoder.Decode(target)
}

var grokResponsesUnsupportedRecursiveFields = map[string]struct{}{
	"external_web_access": {},
}

func sanitizeGrokResponsesUnsupportedFields(body []byte) ([]byte, error) {
	if !bytes.Contains(body, []byte(`"external_web_access"`)) {
		return body, nil
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if !deleteJSONFields(payload, grokResponsesUnsupportedRecursiveFields) {
		return body, nil
	}
	return json.Marshal(payload)
}

func deleteJSONFields(value any, fields map[string]struct{}) bool {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		for field := range fields {
			if _, ok := typed[field]; ok {
				delete(typed, field)
				changed = true
			}
		}
		for _, child := range typed {
			if deleteJSONFields(child, fields) {
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for _, child := range typed {
			if deleteJSONFields(child, fields) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

var grokResponsesSupportedToolTypes = map[string]struct{}{
	"code_execution":     {},
	"code_interpreter":   {},
	"collections_search": {},
	"file_search":        {},
	"function":           {},
	"mcp":                {},
	"shell":              {},
	"web_search":         {},
	"x_search":           {},
}

func sanitizeGrokResponsesTools(body []byte, translation *grokCodexTranslation) ([]byte, error) {
	tools := gjson.GetBytes(body, "tools")
	filteredTools := make([]json.RawMessage, 0)
	var err error

	if tools.Exists() && tools.IsArray() {
		rawTools := tools.Array()
		filteredTools = make([]json.RawMessage, 0, len(rawTools))
		for _, tool := range rawTools {
			toolType := strings.TrimSpace(tool.Get("type").String())
			if toolType == "namespace" {
				namespaceName := firstNonEmpty(tool.Get("name").String(), tool.Get("namespace").String())
				nestedTools := tool.Get("tools")
				if !nestedTools.IsArray() {
					nestedTools = tool.Get("children")
				}
				for _, nested := range nestedTools.Array() {
					normalized, keep, err := normalizeGrokResponsesTool(nested, namespaceName, translation)
					if err != nil {
						return nil, err
					}
					if keep {
						filteredTools = append(filteredTools, normalized)
					}
				}
				continue
			}
			normalized, keep, err := normalizeGrokResponsesTool(tool, "", translation)
			if err != nil {
				return nil, err
			}
			if keep {
				filteredTools = append(filteredTools, normalized)
			}
		}

		if len(filteredTools) != len(rawTools) || !rawJSONMessagesEqual(filteredTools, rawTools) {
			if len(filteredTools) == 0 {
				body, err = sjson.DeleteBytes(body, "tools")
			} else {
				var encoded []byte
				encoded, err = json.Marshal(filteredTools)
				if err != nil {
					return nil, err
				}
				body, err = sjson.SetRawBytes(body, "tools", encoded)
			}
			if err != nil {
				return nil, err
			}
		}
	} else if tools.Exists() && !tools.IsArray() {
		// Invalid tools shape: drop it so orphan tool_choice cleanup can run.
		body, err = sjson.DeleteBytes(body, "tools")
		if err != nil {
			return nil, err
		}
		filteredTools = nil
	}

	toolChoice := gjson.GetBytes(body, "tool_choice")
	if toolChoice.IsObject() && strings.TrimSpace(toolChoice.Get("type").String()) == "custom" {
		var choice map[string]any
		if err := decodeGrokJSON([]byte(toolChoice.Raw), &choice); err != nil {
			return nil, err
		}
		choice["type"] = "function"
		encoded, err := json.Marshal(choice)
		if err != nil {
			return nil, err
		}
		body, err = sjson.SetRawBytes(body, "tool_choice", encoded)
		if err != nil {
			return nil, err
		}
		toolChoice = gjson.GetBytes(body, "tool_choice")
	}
	if toolChoice.Exists() && shouldDropGrokToolChoice(toolChoice, filteredTools) {
		body, err = sjson.DeleteBytes(body, "tool_choice")
		if err != nil {
			return nil, err
		}
	}
	if len(filteredTools) == 0 && gjson.GetBytes(body, "parallel_tool_calls").Exists() {
		body, err = sjson.DeleteBytes(body, "parallel_tool_calls")
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func normalizeGrokResponsesTool(tool gjson.Result, namespaceName string, translation *grokCodexTranslation) (json.RawMessage, bool, error) {
	var item map[string]any
	if err := decodeGrokJSON([]byte(tool.Raw), &item); err != nil {
		return nil, false, err
	}
	toolType := strings.TrimSpace(tool.Get("type").String())
	switch toolType {
	case "tool_search", "image_generation":
		return nil, false, nil
	case "custom":
		name := strings.TrimSpace(tool.Get("name").String())
		if name != "" && translation != nil {
			translation.customTools[name] = struct{}{}
		}
		item["type"] = "function"
		item["parameters"] = grokCustomFunctionParameters()
		delete(item, "format")
		toolType = "function"
	}
	if _, supported := grokResponsesSupportedToolTypes[toolType]; !supported {
		return nil, false, nil
	}
	delete(item, "external_web_access")
	if toolType == "function" {
		if _, exists := item["parameters"]; !exists {
			item["parameters"] = map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			}
		}
		if strings.EqualFold(strings.TrimSpace(namespaceName), grokCodexAppNamespaceName) &&
			strings.EqualFold(strings.TrimSpace(tool.Get("name").String()), grokAutomationUpdateToolName) {
			var parameters map[string]any
			if err := json.Unmarshal(grokSafeFunctionParameters, &parameters); err != nil {
				return nil, false, err
			}
			item["parameters"] = parameters
			if strict, ok := item["strict"].(bool); ok && strict {
				item["strict"] = false
			}
		}
		// Force closed object schemas for every function tool. Nested object /
		// array-item schemas are walked so Cursor-style tools (e.g.
		// run_terminal_cmd_v2) stop failing xAI's additionalProperties gate.
		if params, ok := item["parameters"].(map[string]any); ok {
			enforceGrokClosedObjectSchemas(params)
			item["parameters"] = params
		}
	}
	normalized, err := json.Marshal(item)
	return normalized, err == nil, err
}

// sanitizeGrokChatTools enforces xAI tool-schema rules on Chat Completions
// bodies (tools[].function.parameters and legacy tools[].parameters).
func sanitizeGrokChatTools(body []byte) ([]byte, error) {
	tools := gjson.GetBytes(body, "tools")
	if !tools.Exists() || !tools.IsArray() {
		return body, nil
	}
	changed := false
	normalizedTools := make([]json.RawMessage, 0, len(tools.Array()))
	for _, tool := range tools.Array() {
		var item map[string]any
		if err := decodeGrokJSON([]byte(tool.Raw), &item); err != nil {
			return nil, err
		}
		toolChanged := false

		// OpenAI Chat Completions shape: {type:function, function:{name,parameters}}
		if fn, ok := item["function"].(map[string]any); ok {
			if params, ok := fn["parameters"].(map[string]any); ok {
				if enforceGrokClosedObjectSchemas(params) {
					fn["parameters"] = params
					item["function"] = fn
					toolChanged = true
				}
			} else if _, exists := fn["parameters"]; !exists {
				fn["parameters"] = map[string]any{
					"type":                 "object",
					"properties":           map[string]any{},
					"additionalProperties": false,
				}
				item["function"] = fn
				toolChanged = true
			}
		}

		// Responses-like / flattened shape on chat endpoint.
		if params, ok := item["parameters"].(map[string]any); ok {
			if enforceGrokClosedObjectSchemas(params) {
				item["parameters"] = params
				toolChanged = true
			}
		}

		encoded, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		if toolChanged || !bytes.Equal(encoded, []byte(tool.Raw)) {
			changed = true
		}
		normalizedTools = append(normalizedTools, encoded)
	}
	if !changed {
		return body, nil
	}
	encoded, err := json.Marshal(normalizedTools)
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(body, "tools", encoded)
}

// enforceGrokClosedObjectSchemas walks a JSON Schema fragment and forces every
// object node to declare additionalProperties:false. Returns whether any node changed.
func enforceGrokClosedObjectSchemas(schema map[string]any) bool {
	if schema == nil {
		return false
	}
	changed := false
	if isJSONSchemaObjectType(schema["type"]) || schemaHasObjectShape(schema) {
		if raw, exists := schema["additionalProperties"]; !exists {
			schema["additionalProperties"] = false
			changed = true
		} else if asBool, ok := raw.(bool); ok && asBool {
			// xAI currently rejects open object schemas on tools.
			schema["additionalProperties"] = false
			changed = true
		} else if nested, ok := raw.(map[string]any); ok {
			if enforceGrokClosedObjectSchemas(nested) {
				changed = true
			}
		}
	}
	for _, key := range []string{"properties", "$defs", "definitions", "patternProperties"} {
		nestedMap, ok := schema[key].(map[string]any)
		if !ok {
			continue
		}
		for childKey, child := range nestedMap {
			childMap, ok := child.(map[string]any)
			if !ok {
				continue
			}
			if enforceGrokClosedObjectSchemas(childMap) {
				nestedMap[childKey] = childMap
				changed = true
			}
		}
		schema[key] = nestedMap
	}
	for _, key := range []string{"items", "contains", "not"} {
		if childMap, ok := schema[key].(map[string]any); ok {
			if enforceGrokClosedObjectSchemas(childMap) {
				schema[key] = childMap
				changed = true
			}
		}
	}
	for _, key := range []string{"anyOf", "oneOf", "allOf", "prefixItems"} {
		list, ok := schema[key].([]any)
		if !ok {
			continue
		}
		for i, child := range list {
			childMap, ok := child.(map[string]any)
			if !ok {
				continue
			}
			if enforceGrokClosedObjectSchemas(childMap) {
				list[i] = childMap
				changed = true
			}
		}
		schema[key] = list
	}
	return changed
}

func isJSONSchemaObjectType(raw any) bool {
	switch typed := raw.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "object")
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.EqualFold(strings.TrimSpace(s), "object") {
				return true
			}
		}
	}
	return false
}

func schemaHasObjectShape(schema map[string]any) bool {
	if schema == nil {
		return false
	}
	if _, ok := schema["properties"]; ok {
		return true
	}
	if _, ok := schema["additionalProperties"]; ok {
		return true
	}
	if _, ok := schema["patternProperties"]; ok {
		return true
	}
	return false
}

func rawJSONMessagesEqual(normalized []json.RawMessage, original []gjson.Result) bool {
	if len(normalized) != len(original) {
		return false
	}
	for i := range normalized {
		if !bytes.Equal(normalized[i], []byte(original[i].Raw)) {
			return false
		}
	}
	return true
}

func shouldDropGrokToolChoice(toolChoice gjson.Result, tools []json.RawMessage) bool {
	if len(tools) == 0 {
		return true
	}
	if !toolChoice.IsObject() {
		return false
	}
	choiceType := strings.TrimSpace(toolChoice.Get("type").String())
	if choiceType == "" {
		return false
	}
	if _, ok := grokResponsesSupportedToolTypes[choiceType]; !ok {
		return true
	}
	if choiceType == "function" {
		choiceName := strings.TrimSpace(toolChoice.Get("name").String())
		if choiceName == "" {
			choiceName = strings.TrimSpace(toolChoice.Get("function.name").String())
		}
		if choiceName == "" {
			return false
		}
		for _, tool := range tools {
			var item struct {
				Type     string `json:"type"`
				Name     string `json:"name"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if err := json.Unmarshal(tool, &item); err != nil {
				continue
			}
			name := strings.TrimSpace(item.Name)
			if name == "" {
				name = strings.TrimSpace(item.Function.Name)
			}
			if strings.TrimSpace(item.Type) == "function" && name == choiceName {
				return false
			}
		}
		return true
	}
	return false
}

func (s *OpenAIGatewayService) bridgeGrokComposerImageInputs(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) ([]byte, OpenAIUsage, bool, error) {
	if !shouldBridgeGrokComposerImageInputs(body) {
		return body, OpenAIUsage{}, false, nil
	}

	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return body, OpenAIUsage{}, false, fmt.Errorf("parse grok composer image bridge request: %w", err)
	}

	imageURLs := collectGrokComposerImageURLs(reqBody)
	if len(imageURLs) == 0 {
		return body, OpenAIUsage{}, false, nil
	}

	descriptions := make([]string, 0, len(imageURLs))
	var bridgeUsage OpenAIUsage
	for index, imageURL := range imageURLs {
		description, usage, err := s.describeGrokComposerImage(ctx, c, account, token, imageURL, index+1)
		if err != nil {
			return body, bridgeUsage, false, err
		}
		descriptions = append(descriptions, description)
		addOpenAIUsage(&bridgeUsage, usage)
	}

	if !rewriteGrokComposerImagesAsText(reqBody, descriptions) {
		return body, bridgeUsage, false, nil
	}
	bridgedBody, err := marshalOpenAIUpstreamJSON(reqBody)
	if err != nil {
		return body, bridgeUsage, false, fmt.Errorf("serialize grok composer image bridge request: %w", err)
	}
	return bridgedBody, bridgeUsage, true, nil
}

func shouldBridgeGrokComposerImageInputs(body []byte) bool {
	if len(body) == 0 || !isGrokComposerModel(gjson.GetBytes(body, "model").String()) {
		return false
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() {
		return false
	}
	return openAIJSONValueMayContainImageInput(messages)
}

func isGrokComposerModel(model string) bool {
	model = strings.TrimSpace(strings.ToLower(model))
	if model == "" {
		return false
	}
	if strings.Contains(model, "/") {
		parts := strings.Split(model, "/")
		model = strings.TrimSpace(parts[len(parts)-1])
	}
	return strings.Contains(model, "composer")
}

func collectGrokComposerImageURLs(reqBody map[string]any) []string {
	messages, ok := reqBody["messages"].([]any)
	if !ok {
		return nil
	}

	var imageURLs []string
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := msgMap["content"].([]any)
		if !ok {
			continue
		}
		for _, part := range parts {
			if imageURL := grokComposerImageURLFromPart(part); imageURL != "" {
				imageURLs = append(imageURLs, imageURL)
			}
		}
	}
	return imageURLs
}

func grokComposerImageURLFromPart(part any) string {
	partMap, ok := part.(map[string]any)
	if !ok {
		return ""
	}
	if strings.TrimSpace(strings.ToLower(fmt.Sprint(partMap["type"]))) != "image_url" {
		return ""
	}
	switch imageURL := partMap["image_url"].(type) {
	case string:
		return normalizeGrokComposerImageURL(imageURL)
	case map[string]any:
		raw, _ := imageURL["url"].(string)
		return normalizeGrokComposerImageURL(raw)
	default:
		return ""
	}
}

func normalizeGrokComposerImageURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || isEmptyBase64DataURI(trimmed) {
		return ""
	}
	return trimmed
}

func (s *OpenAIGatewayService) describeGrokComposerImage(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	imageURL string,
	index int,
) (string, OpenAIUsage, error) {
	body, err := buildGrokComposerImageDescriptionBody(imageURL, index)
	if err != nil {
		return "", OpenAIUsage{}, err
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	// Image-description probes are auxiliary requests, not conversation turns.
	// Do not bind them to the caller's Grok prompt-cache identity.
	upstreamReq, err := buildGrokResponsesRequest(upstreamCtx, c, account, body, token, "")
	releaseUpstreamCtx()
	if err != nil {
		return "", OpenAIUsage{}, fmt.Errorf("build grok composer image bridge request: %w", err)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return "", OpenAIUsage{}, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		s.updateGrokUsageSnapshot(ctx, account, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI image bridge upstream returned status %d", resp.StatusCode)
		}
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
			Kind:               "failover",
			Message:            upstreamMsg,
		})
		s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return "", OpenAIUsage{}, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return "", OpenAIUsage{}, fmt.Errorf("grok composer image bridge upstream error: %s", upstreamMsg)
	}

	s.updateGrokUsageSnapshot(ctx, account, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, nil)
	if err != nil {
		return "", OpenAIUsage{}, fmt.Errorf("read grok composer image bridge response: %w", err)
	}

	var parsed apicompat.ResponsesResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", OpenAIUsage{}, fmt.Errorf("parse grok composer image bridge response: %w", err)
	}
	description := strings.TrimSpace(grokResponsesOutputText(&parsed))
	if description == "" {
		return "", copyOpenAIUsageFromResponsesUsage(parsed.Usage), fmt.Errorf("grok composer image bridge returned empty description")
	}
	return description, copyOpenAIUsageFromResponsesUsage(parsed.Usage), nil
}

func buildGrokComposerImageDescriptionBody(imageURL string, index int) ([]byte, error) {
	prompt := fmt.Sprintf("Describe image %d in concise, factual text for a downstream coding/composer model. Include visible text, UI elements, diagrams, errors, and spatial relationships. Do not mention that you are an image analysis bridge.", index)
	req := map[string]any{
		"model":             grokComposerImageBridgeVisionModel,
		"stream":            false,
		"store":             false,
		"max_output_tokens": grokComposerImageBridgeMaxOutputTokens,
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": prompt},
					map[string]any{"type": "input_image", "image_url": imageURL},
				},
			},
		},
	}
	return marshalOpenAIUpstreamJSON(req)
}

func grokResponsesOutputText(resp *apicompat.ResponsesResponse) string {
	if resp == nil {
		return ""
	}
	var parts []string
	for _, output := range resp.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" || content.Type == "text" || content.Type == "input_text" {
				if text := strings.TrimSpace(content.Text); text != "" {
					parts = append(parts, text)
				}
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func rewriteGrokComposerImagesAsText(reqBody map[string]any, descriptions []string) bool {
	messages, ok := reqBody["messages"].([]any)
	if !ok {
		return false
	}

	imageIndex := 0
	changed := false
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := msgMap["content"].([]any)
		if !ok {
			continue
		}
		var textParts []string
		messageChanged := false
		for _, part := range parts {
			if imageURL := grokComposerImageURLFromPart(part); imageURL != "" {
				if imageIndex < len(descriptions) {
					textParts = append(textParts, fmt.Sprintf("Image %d description: %s", imageIndex+1, strings.TrimSpace(descriptions[imageIndex])))
				}
				imageIndex++
				messageChanged = true
				continue
			}
			if text := grokComposerTextFromPart(part); text != "" {
				textParts = append(textParts, text)
			}
		}
		if messageChanged {
			msgMap["content"] = strings.Join(textParts, "\n\n")
			changed = true
		}
	}
	return changed
}

func grokComposerTextFromPart(part any) string {
	partMap, ok := part.(map[string]any)
	if !ok {
		return ""
	}
	partType := strings.TrimSpace(strings.ToLower(fmt.Sprint(partMap["type"])))
	switch partType {
	case "text", "input_text":
		text, _ := partMap["text"].(string)
		return strings.TrimSpace(text)
	default:
		return ""
	}
}

func addOpenAIUsage(dst *OpenAIUsage, usage OpenAIUsage) {
	if dst == nil {
		return
	}
	dst.InputTokens += usage.InputTokens
	dst.ImageInputTokens += usage.ImageInputTokens
	dst.OutputTokens += usage.OutputTokens
	dst.CacheCreationInputTokens += usage.CacheCreationInputTokens
	dst.CacheReadInputTokens += usage.CacheReadInputTokens
	dst.ImageOutputTokens += usage.ImageOutputTokens
}

func buildGrokResponsesRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, cacheIdentity string) (*http.Request, error) {
	targetURL, err := xai.BuildResponsesURL(account.GetGrokChatBaseURL())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileGrok))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "sub2api-grok/1.0")
	if xai.IsCLIChatProxyBaseURL(account.GetGrokChatBaseURL()) {
		xai.ApplyGrokCLIChatHeaders(req)
	}
	applyGrokCacheHeaders(req.Header, cacheIdentity)
	if c != nil {
		if v := c.GetHeader("OpenAI-Beta"); strings.TrimSpace(v) != "" {
			req.Header.Set("OpenAI-Beta", v)
		}
	}
	applyGrokRequestIdentityHeaders(req.Header, c, cacheIdentity, gjson.GetBytes(body, "model").String())
	// Header overrides must run after built-in defaults so operators can patch
	// relay-required headers without losing CLI/auth identity headers.
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

func (s *OpenAIGatewayService) updateGrokUsageSnapshot(ctx context.Context, account *Account, snapshot *xai.QuotaSnapshot) {
	if s == nil || s.accountRepo == nil || account == nil || account.ID <= 0 || snapshot == nil {
		return
	}
	if s.codexSnapshotThrottle != nil && !s.codexSnapshotThrottle.Allow(account.ID, time.Now()) {
		return
	}
	updates := map[string]any{
		grokQuotaSnapshotExtraKey: snapshot,
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		return
	}
}

func (s *OpenAIGatewayService) handleGrokAccountUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte) {
	if s == nil || account == nil {
		return
	}
	switch statusCode {
	case http.StatusUnauthorized:
		s.tempUnscheduleGrok(ctx, account, 10*time.Minute, "grok oauth token unauthorized")
	case http.StatusForbidden:
		reason := "grok entitlement or subscription tier denied"
		if isGrokSpendingLimitError(responseBody, "") {
			reason = grokSpendingLimitTempUnschedReason
		}
		s.tempUnscheduleGrok(ctx, account, 30*time.Minute, reason)
	case http.StatusTooManyRequests:
		cooldown := 2 * time.Minute
		if snapshot := xai.ParseQuotaHeaders(headers, statusCode); snapshot != nil && snapshot.RetryAfterSeconds != nil && *snapshot.RetryAfterSeconds > 0 {
			cooldown = time.Duration(*snapshot.RetryAfterSeconds) * time.Second
		}
		s.tempUnscheduleGrok(ctx, account, cooldown, "grok rate limited")
	default:
		if statusCode >= 500 {
			s.tempUnscheduleGrok(ctx, account, 2*time.Minute, "grok upstream temporary error")
		}
	}
}

func (s *OpenAIGatewayService) isGrokSameAccountRetry(c *gin.Context, statusCode int) bool {
	if s == nil || s.cfg == nil || c == nil || c.Request == nil || c.Request.URL == nil ||
		c.Request.Method != http.MethodPost || isWebSocketUpgradeRequest(c.Request) {
		return false
	}
	switch c.Request.URL.Path {
	case "/v1/responses", "/responses", "/backend-api/codex/responses",
		"/v1/chat/completions", "/chat/completions":
	default:
		return false
	}
	if statusCode != http.StatusTooManyRequests && (statusCode < 500 || statusCode > 599) {
		return false
	}
	retryCfg := s.cfg.Gateway.GrokSameAccountRetry
	if !retryCfg.Enabled || retryCfg.MaxRetries <= 0 {
		return false
	}
	for _, configuredStatus := range retryCfg.Statuses {
		if configuredStatus == statusCode {
			return true
		}
	}
	return false
}

func isWebSocketUpgradeRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
}

// ApplyGrokSameAccountRetryPenalty applies the deferred health penalty after a
// pinned Grok retry fails. It intentionally accepts the original failure so
// Retry-After and the upstream body remain available to the existing policy.
func (s *OpenAIGatewayService) ApplyGrokSameAccountRetryPenalty(ctx context.Context, account *Account, failoverErr *UpstreamFailoverError) {
	if failoverErr == nil || !failoverErr.GrokSameAccountRetry {
		return
	}
	s.handleGrokAccountUpstreamError(ctx, account, failoverErr.StatusCode, failoverErr.ResponseHeaders, failoverErr.ResponseBody)
}

func (s *OpenAIGatewayService) tempUnscheduleGrok(ctx context.Context, account *Account, cooldown time.Duration, reason string) {
	if s == nil || account == nil {
		return
	}
	until := time.Now().Add(cooldown)
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(until) {
		until = *account.TempUnschedulableUntil
	}
	s.BlockAccountScheduling(account, until, reason)
	if s.accountRepo != nil {
		stateCtx, cancel := openAIAccountStateContext(ctx)
		defer cancel()
		_ = s.accountRepo.SetTempUnschedulable(stateCtx, account.ID, until, reason)
	}
}

func ptrStringOrNil(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
