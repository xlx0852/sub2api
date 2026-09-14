package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIResponsesLiteDemotedContextKey = "openai_responses_lite_demoted"
	openAIBaseInstructionsKind           = "model.base_instructions"
)

// openAIModelUsesResponsesLite reports whether the catalog marks the model as a
// Responses-Lite dialect model. Unknown models are treated as non-Lite.
func openAIModelUsesResponsesLite(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, entry := range openai.CurrentDefaultModels() {
		if strings.EqualFold(strings.TrimSpace(entry.ID), model) {
			return entry.UseResponsesLite
		}
	}
	return false
}

// isOpenAIResponsesLiteInbound detects the client-side Responses-Lite pair:
// HTTP header and/or WS client_metadata key.
func isOpenAIResponsesLiteInbound(c *gin.Context, body []byte) bool {
	if c != nil && isOpenAIResponsesLiteHeader(c.GetHeader(responsesLiteHeader)) {
		return true
	}
	return isOpenAIResponsesLiteWebSocketPayload(body)
}

// shouldDemoteOpenAIResponsesLite is true when the client spoke Lite but the
// upstream model does not accept the Lite dialect (e.g. sol → gpt-5.5).
// Lite→Lite mapping only rewrites the model slug and keeps the Lite pair.
func shouldDemoteOpenAIResponsesLite(liteIn bool, upstreamModel string) bool {
	return liteIn && !openAIModelUsesResponsesLite(upstreamModel)
}

func markOpenAIResponsesLiteDemoted(c *gin.Context) {
	if c != nil {
		c.Set(openAIResponsesLiteDemotedContextKey, true)
	}
}

func isOpenAIResponsesLiteDemoted(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(openAIResponsesLiteDemotedContextKey)
	if !ok {
		return false
	}
	flag, _ := v.(bool)
	return flag
}

// applyOpenAIResponsesLiteForUpstream demotes a Lite request body when the
// resolved upstream model is not Lite-capable. Callers must still strip the
// HTTP Lite header on the upstream request (see stripOpenAIResponsesLiteHeader).
//
// forceStandard=true 用于 external bridge（Devin/Cursor）：即使上游模型 slug 被
// 标成 Lite（如 gpt-6-astra），桥接侧也不理解 additional_tools / base_instructions
// 这套 Lite 方言，必须强制降级为标准 Responses，否则 tools 会整包丢失。
func applyOpenAIResponsesLiteForUpstream(c *gin.Context, body []byte, upstreamModel string, forceStandard ...bool) ([]byte, bool, error) {
	force := len(forceStandard) > 0 && forceStandard[0]
	liteIn := isOpenAIResponsesLiteInbound(c, body)
	if !force && !shouldDemoteOpenAIResponsesLite(liteIn, upstreamModel) {
		return body, false, nil
	}
	if force && !liteIn && !bytes.Contains(body, []byte(`"additional_tools"`)) &&
		!bytes.Contains(body, []byte(openAIBaseInstructionsKind)) &&
		!bytes.Contains(body, []byte(`"all_turns"`)) &&
		!bytes.Contains(body, []byte(responsesLiteWSMetadataKey)) {
		// 非 Lite 请求且无 Lite 特征：无需处理。
		return body, false, nil
	}
	next, changed, err := demoteOpenAIResponsesLiteBody(body)
	if err != nil {
		return body, false, err
	}
	if !changed {
		return body, false, nil
	}
	markOpenAIResponsesLiteDemoted(c)
	return next, true, nil
}

func stripOpenAIResponsesLiteHeader(header interface{ Del(string) }) {
	if header == nil {
		return
	}
	header.Del(responsesLiteHeader)
}

// demoteOpenAIResponsesLiteBody reverses Codex Responses-Lite request shaping
// into standard Responses form:
//   - additional_tools input items → top-level tools
//   - developer base-instructions message → instructions
//   - reasoning.context=all_turns dropped
//   - WS Lite client_metadata key dropped
func demoteOpenAIResponsesLiteBody(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	needsWork := bytes.Contains(body, []byte(`"additional_tools"`)) ||
		bytes.Contains(body, []byte(openAIBaseInstructionsKind)) ||
		bytes.Contains(body, []byte(`"all_turns"`)) ||
		bytes.Contains(body, []byte(responsesLiteWSMetadataKey))
	if !needsWork {
		return body, false, nil
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body, false, fmt.Errorf("decode responses lite body: %w", err)
	}
	modified := demoteOpenAIResponsesLiteRequestMap(req)
	if !modified {
		return body, false, nil
	}
	out, err := marshalOpenAIUpstreamJSON(req)
	if err != nil {
		return body, false, fmt.Errorf("encode demoted responses body: %w", err)
	}
	return out, true, nil
}

func demoteOpenAIResponsesLiteRequestMap(req map[string]any) bool {
	if req == nil {
		return false
	}
	modified := false

	if demoteOpenAIResponsesLiteTools(req) {
		modified = true
	}
	if demoteOpenAIResponsesLiteBaseInstructions(req) {
		modified = true
	}
	if demoteOpenAIResponsesLiteReasoningContext(req) {
		modified = true
	}
	if demoteOpenAIResponsesLiteClientMetadata(req) {
		modified = true
	}
	return modified
}

func demoteOpenAIResponsesLiteTools(req map[string]any) bool {
	rawInput, ok := req["input"]
	if !ok || rawInput == nil {
		return false
	}
	input, ok := rawInput.([]any)
	if !ok {
		return false
	}

	var extracted []any
	filtered := make([]any, 0, len(input))
	modified := false
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || strings.TrimSpace(firstNonEmptyString(item["type"])) != "additional_tools" {
			filtered = append(filtered, rawItem)
			continue
		}
		modified = true
		if tools, ok := item["tools"].([]any); ok {
			for _, tool := range tools {
				if tool != nil {
					extracted = append(extracted, tool)
				}
			}
		}
	}
	if !modified {
		return false
	}
	req["input"] = filtered

	if len(extracted) == 0 {
		return true
	}
	existing, _ := req["tools"].([]any)
	merged := make([]any, 0, len(existing)+len(extracted))
	merged = append(merged, existing...)
	merged = append(merged, extracted...)
	req["tools"] = merged
	return true
}

func demoteOpenAIResponsesLiteBaseInstructions(req map[string]any) bool {
	rawInput, ok := req["input"]
	if !ok || rawInput == nil {
		return false
	}
	input, ok := rawInput.([]any)
	if !ok {
		return false
	}

	var baseText string
	filtered := make([]any, 0, len(input))
	modified := false
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || !isOpenAIResponsesLiteBaseInstructionsMessage(item) {
			filtered = append(filtered, rawItem)
			continue
		}
		modified = true
		if baseText == "" {
			baseText = extractOpenAIResponsesLiteMessageText(item)
		}
	}
	if !modified {
		return false
	}
	req["input"] = filtered

	existing := strings.TrimSpace(firstNonEmptyString(req["instructions"]))
	if existing == "" && strings.TrimSpace(baseText) != "" {
		req["instructions"] = baseText
	}
	return true
}

func isOpenAIResponsesLiteBaseInstructionsMessage(item map[string]any) bool {
	if strings.TrimSpace(firstNonEmptyString(item["type"])) != "message" {
		return false
	}
	if strings.TrimSpace(firstNonEmptyString(item["role"])) != "developer" {
		return false
	}
	meta, ok := item["internal_chat_message_metadata_passthrough"].(map[string]any)
	if !ok {
		return false
	}
	kinds, ok := meta["content_item_kinds"].([]any)
	if !ok {
		return false
	}
	for _, raw := range kinds {
		if strings.TrimSpace(firstNonEmptyString(raw)) == openAIBaseInstructionsKind {
			return true
		}
	}
	return false
}

func extractOpenAIResponsesLiteMessageText(item map[string]any) string {
	content, ok := item["content"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, raw := range content {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.TrimSpace(firstNonEmptyString(part["type"]))
		if typ != "" && typ != "input_text" && typ != "text" && typ != "output_text" {
			continue
		}
		if text := strings.TrimSpace(firstNonEmptyString(part["text"])); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func demoteOpenAIResponsesLiteReasoningContext(req map[string]any) bool {
	rawReasoning, ok := req["reasoning"]
	if !ok || rawReasoning == nil {
		return false
	}
	reasoning, ok := rawReasoning.(map[string]any)
	if !ok {
		return false
	}
	ctxVal := strings.TrimSpace(firstNonEmptyString(reasoning["context"]))
	if !strings.EqualFold(ctxVal, "all_turns") {
		return false
	}
	delete(reasoning, "context")
	if len(reasoning) == 0 {
		delete(req, "reasoning")
	} else {
		req["reasoning"] = reasoning
	}
	return true
}

func demoteOpenAIResponsesLiteClientMetadata(req map[string]any) bool {
	rawMeta, ok := req["client_metadata"]
	if !ok || rawMeta == nil {
		return false
	}
	meta, ok := rawMeta.(map[string]any)
	if !ok {
		return false
	}
	if _, exists := meta[responsesLiteWSMetadataKey]; !exists {
		return false
	}
	delete(meta, responsesLiteWSMetadataKey)
	if len(meta) == 0 {
		delete(req, "client_metadata")
	} else {
		req["client_metadata"] = meta
	}
	return true
}
