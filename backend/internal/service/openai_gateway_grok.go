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

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func (s *OpenAIGatewayService) forwardGrokResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	originalModel string,
	reqStream bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	if account.Type != AccountTypeOAuth {
		return nil, fmt.Errorf("grok account type %s is not supported by subscription forwarding", account.Type)
	}

	upstreamModel := account.GetMappedModel(originalModel)
	if strings.TrimSpace(upstreamModel) == "" {
		upstreamModel = "grok-4.3"
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	sessionID := s.GenerateSessionHashForOpenAIRequest(c, body)
	var grokIDMapper *grokWSRequestIDMapper
	if sessionID != "" {
		grokIDMapper = newGrokWSRequestIDMapper(sessionID, body)
		defer deleteGrokWSIDState(sessionID)
	}

	preparedBody, err := s.prepareGrokUpstreamResponsesBody(ctx, c, account, body, upstreamModel, token, proxyURL)
	if err != nil {
		return nil, err
	}
	patchedBody := preparedBody.Body
	preprocessUsage := preparedBody.PreprocessUsage
	if grokIDMapper != nil {
		patchedBody = grokIDMapper.upstreamRequestPayload(patchedBody)
		setGrokWSRequestIDMapper(c, grokIDMapper)
		setGrokWSUpstreamRequestPayload(c, patchedBody)
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	upstreamReq, err := buildGrokResponsesRequest(upstreamCtx, c, account, patchedBody, token)
	if err != nil {
		return nil, err
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
		s.updateGrokUsageSnapshot(ctx, account.ID, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
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
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleErrorResponse(ctx, resp, c, account, patchedBody, upstreamModel)
	}

	s.updateGrokUsageSnapshot(ctx, account.ID, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))

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
	if preprocessUsage != nil {
		addOpenAIUsage(usage, *preprocessUsage)
	}
	return &OpenAIForwardResult{
		RequestID:       firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
		ResponseID:      responseID,
		Usage:           *usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: ptrStringOrNil(normalizeOpenAIReasoningEffort(gjson.GetBytes(patchedBody, "reasoning.effort").String())),
		Stream:          reqStream,
		OpenAIWSMode:    false,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
	}, nil
}

const (
	grokComposerVisionPreprocessModel           = "grok-build"
	grokComposerVisionPreprocessMaxContextChars = 4000
)

type grokVisionPreprocessResult struct {
	Description string
	Usage       OpenAIUsage
}

type grokPreparedResponsesBody struct {
	Body            []byte
	PreprocessUsage *OpenAIUsage
}

func (s *OpenAIGatewayService) prepareGrokUpstreamResponsesBody(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	upstreamModel string,
	token string,
	proxyURL string,
) (*grokPreparedResponsesBody, error) {
	var preprocessUsage *OpenAIUsage
	if shouldPreprocessGrokComposerImages(upstreamModel, body) {
		preprocessResult, err := s.preprocessGrokComposerImages(ctx, c, account, body, token, proxyURL)
		if err != nil {
			return nil, err
		}
		rewrittenBody, err := rewriteGrokComposerBodyWithVisionDescription(body, preprocessResult.Description)
		if err != nil {
			return nil, err
		}
		body = rewrittenBody
		preprocessUsage = &preprocessResult.Usage
	}

	patchedBody, err := patchGrokResponsesBody(body, upstreamModel)
	if err != nil {
		return nil, err
	}
	return &grokPreparedResponsesBody{
		Body:            patchedBody,
		PreprocessUsage: preprocessUsage,
	}, nil
}

func shouldPreprocessGrokComposerImages(upstreamModel string, body []byte) bool {
	return isGrokComposerModel(upstreamModel) && openAIRequestBodyMayContainImageInput(body)
}

func isGrokComposerModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "grok-composer-")
}

func (s *OpenAIGatewayService) preprocessGrokComposerImages(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
	proxyURL string,
) (*grokVisionPreprocessResult, error) {
	visionBody, err := buildGrokComposerVisionPreprocessBody(body)
	if err != nil {
		return nil, err
	}
	patchedVisionBody, err := patchGrokResponsesBody(visionBody, grokComposerVisionPreprocessModel)
	if err != nil {
		return nil, err
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	upstreamReq, err := buildGrokResponsesRequest(upstreamCtx, c, account, patchedVisionBody, token)
	if err != nil {
		return nil, err
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
		s.updateGrokUsageSnapshot(ctx, account.ID, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI vision preprocess returned status %d", resp.StatusCode)
		}
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
			Kind:               "grok_vision_preprocess",
			Message:            upstreamMsg,
		})
		s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		_, err := s.handleErrorResponse(ctx, resp, c, account, patchedVisionBody, grokComposerVisionPreprocessModel)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("grok vision preprocess returned status %d", resp.StatusCode)
	}

	s.updateGrokUsageSnapshot(ctx, account.ID, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	description := strings.TrimSpace(extractOpenAIResponsesText(respBody))
	if description == "" {
		return nil, fmt.Errorf("grok vision preprocess returned empty description")
	}
	usage, _ := extractOpenAIUsageFromJSONBytes(respBody)
	return &grokVisionPreprocessResult{
		Description: description,
		Usage:       usage,
	}, nil
}

func buildGrokComposerVisionPreprocessBody(body []byte) ([]byte, error) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	var images []string
	collectGrokResponsesImageURLs(payload, &images)
	if len(images) == 0 {
		return nil, fmt.Errorf("grok composer image preprocess requires at least one image")
	}

	var textContext strings.Builder
	collectGrokResponsesInputText(payload, &textContext)
	contextText := trimGrokComposerVisionContext(textContext.String())
	prompt := "Describe the attached image(s) for a coding assistant that cannot receive images directly. " +
		"Focus on UI state, visible error text, logs, diagrams, code snippets, and anything needed to answer the user's request. " +
		"Return a concise but complete textual description."
	if contextText != "" {
		prompt += "\n\nOriginal user text:\n" + contextText
	}

	content := make([]any, 0, len(images)+1)
	content = append(content, map[string]any{
		"type": "input_text",
		"text": prompt,
	})
	for _, imageURL := range images {
		if strings.TrimSpace(imageURL) == "" {
			continue
		}
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": imageURL,
		})
	}
	if len(content) == 1 {
		return nil, fmt.Errorf("grok composer image preprocess requires at least one non-empty image")
	}

	return marshalOpenAIUpstreamJSON(map[string]any{
		"model":  grokComposerVisionPreprocessModel,
		"stream": false,
		"input": []any{
			map[string]any{
				"role":    "user",
				"content": content,
			},
		},
	})
}

func rewriteGrokComposerBodyWithVisionDescription(body []byte, description string) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	stripGrokResponsesImagesFromPayload(payload)
	prependGrokVisionDescription(payload, description)
	return marshalOpenAIUpstreamJSON(payload)
}

func collectGrokResponsesImageURLs(value any, out *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		imageURL := strings.TrimSpace(firstNonEmptyString(typed["image_url"]))
		if imageURL == "" {
			if imageObject, ok := typed["image_url"].(map[string]any); ok {
				imageURL = strings.TrimSpace(firstNonEmptyString(imageObject["url"]))
			}
		}
		if imageURL != "" {
			*out = append(*out, imageURL)
		}
		for _, child := range typed {
			collectGrokResponsesImageURLs(child, out)
		}
	case []any:
		for _, child := range typed {
			collectGrokResponsesImageURLs(child, out)
		}
	}
}

func collectGrokResponsesInputText(value any, out *strings.Builder) {
	switch typed := value.(type) {
	case map[string]any:
		if text := strings.TrimSpace(firstNonEmptyString(typed["text"])); text != "" {
			if out.Len() > 0 {
				out.WriteString("\n")
			}
			out.WriteString(text)
		}
		for _, key := range []string{"instructions", "input", "messages", "content"} {
			if child, ok := typed[key]; ok {
				collectGrokResponsesInputText(child, out)
			}
		}
	case []any:
		for _, child := range typed {
			collectGrokResponsesInputText(child, out)
		}
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			if out.Len() > 0 {
				out.WriteString("\n")
			}
			out.WriteString(text)
		}
	}
}

func trimGrokComposerVisionContext(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= grokComposerVisionPreprocessMaxContextChars {
		return value
	}
	return value[:grokComposerVisionPreprocessMaxContextChars]
}

func stripGrokResponsesImagesFromPayload(payload map[string]any) {
	input, ok := payload["input"].([]any)
	if !ok {
		return
	}
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			continue
		}
		content, ok := item["content"].([]any)
		if !ok {
			continue
		}
		normalized := make([]any, 0, len(content))
		for _, rawPart := range content {
			part, ok := rawPart.(map[string]any)
			if !ok || part == nil {
				normalized = append(normalized, rawPart)
				continue
			}
			partType := strings.TrimSpace(firstNonEmptyString(part["type"]))
			if partType == "input_image" {
				continue
			}
			delete(part, "image_url")
			normalized = append(normalized, part)
		}
		item["content"] = normalized
	}
	payload["input"] = input
}

func prependGrokVisionDescription(payload map[string]any, description string) {
	visionText := "Image analysis from Grok Builder-style preprocessing:\n" +
		strings.TrimSpace(description) +
		"\n\nUse this as the visual context for the user's request. The original image inputs are not attached to this Composer request."
	visionPart := map[string]any{
		"type": "input_text",
		"text": visionText,
	}

	input, _ := payload["input"].([]any)
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			continue
		}
		if role := strings.TrimSpace(firstNonEmptyString(item["role"])); role != "" && role != "user" {
			continue
		}
		switch content := item["content"].(type) {
		case []any:
			item["content"] = append([]any{visionPart}, content...)
			payload["input"] = input
			return
		case string:
			item["content"] = []any{
				visionPart,
				map[string]any{"type": "input_text", "text": content},
			}
			payload["input"] = input
			return
		}
	}

	payload["input"] = append(input, map[string]any{
		"role": "user",
		"content": []any{
			visionPart,
		},
	})
}

func addOpenAIUsage(dst *OpenAIUsage, src OpenAIUsage) {
	if dst == nil {
		return
	}
	dst.InputTokens += src.InputTokens
	dst.ImageInputTokens += src.ImageInputTokens
	dst.OutputTokens += src.OutputTokens
	dst.CacheCreationInputTokens += src.CacheCreationInputTokens
	dst.CacheReadInputTokens += src.CacheReadInputTokens
	dst.ImageOutputTokens += src.ImageOutputTokens
	dst.CostInUSDTicks += src.CostInUSDTicks
	if len(src.ServerSideToolUsage) > 0 {
		if dst.ServerSideToolUsage == nil {
			dst.ServerSideToolUsage = map[string]int{}
		}
		for name, count := range src.ServerSideToolUsage {
			dst.ServerSideToolUsage[name] += count
		}
	}
}

func sanitizeGrokResponsesInclude(body []byte) ([]byte, error) {
	include := gjson.GetBytes(body, "include")
	if !include.Exists() || !include.IsArray() {
		return body, nil
	}
	filtered := make([]any, 0, len(include.Array()))
	changed := false
	for _, item := range include.Array() {
		if strings.TrimSpace(item.String()) == "reasoning.encrypted_content" {
			changed = true
			continue
		}
		filtered = append(filtered, item.Value())
	}
	if !changed {
		return body, nil
	}
	if len(filtered) == 0 {
		return sjson.DeleteBytes(body, "include")
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(body, "include", encoded)
}

func sanitizeGrokResponsesInput(body []byte) ([]byte, error) {
	if !gjson.GetBytes(body, "input").Exists() {
		return body, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	input, ok := payload["input"].([]any)
	if !ok {
		return body, nil
	}
	input = stripGrokIncompatibleCompactionInputItems(input)
	mergeInstructions := strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()) == ""
	input = relocateGrokSystemDeveloperInputItems(payload, input, mergeInstructions)
	input = dropGrokEmptyInputItems(input)
	if normalizedToolRoleInput, modified := normalizeCodexToolRoleMessages(input); modified {
		input = normalizedToolRoleInput
	}
	if normalizedMessageInput, modified := normalizeCodexMessageContentText(input); modified {
		input = normalizedMessageInput
	}
	dropReasoning := isGrokComposerModel(strings.TrimSpace(gjson.GetBytes(body, "model").String()))
	input = sanitizeGrokCodexInputItems(input, dropReasoning)
	input = wrapGrokStandaloneInputContentItems(input)
	normalizedInput, ok := normalizeGrokResponsesImageParts(input).([]any)
	if !ok {
		return body, nil
	}
	normalizedInput = rewriteGrokResponsesFunctionCallOutputImages(normalizedInput)
	if strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()) != "" {
		normalizedInput = collapseGrokResponsesInputForContinuation(normalizedInput)
	}
	payload["input"] = normalizedInput
	return json.Marshal(payload)
}

func collapseGrokResponsesInputForContinuation(input []any) []any {
	if len(input) == 0 {
		return input
	}
	lastAssistantIdx := -1
	for i, rawItem := range input {
		if isGrokAssistantInputItem(rawItem) {
			lastAssistantIdx = i
		}
	}
	collapsed := input
	if lastAssistantIdx >= 0 && lastAssistantIdx+1 < len(input) {
		collapsed = input[lastAssistantIdx+1:]
	}
	collapsed = dropGrokAssistantInputItems(collapsed)
	if len(collapsed) == 0 {
		collapsed = keepTrailingGrokContinuationInputItems(input)
	}
	return collapsed
}

func isGrokAssistantInputItem(rawItem any) bool {
	item, ok := rawItem.(map[string]any)
	if !ok || item == nil {
		return false
	}
	return strings.TrimSpace(firstNonEmptyString(item["role"])) == "assistant"
}

func dropGrokAssistantInputItems(input []any) []any {
	if len(input) == 0 {
		return input
	}
	filtered := make([]any, 0, len(input))
	for _, rawItem := range input {
		if isGrokAssistantInputItem(rawItem) {
			continue
		}
		filtered = append(filtered, rawItem)
	}
	return filtered
}

func keepTrailingGrokContinuationInputItems(input []any) []any {
	if len(input) == 0 {
		return input
	}
	for i := len(input) - 1; i >= 0; i-- {
		item, ok := input[i].(map[string]any)
		if !ok || item == nil {
			continue
		}
		role := strings.TrimSpace(firstNonEmptyString(item["role"]))
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		switch {
		case role == "user":
			return []any{item}
		case itemType == "function_call", itemType == "function_call_output":
			return input[i:]
		}
	}
	return []any{input[len(input)-1]}
}

func relocateGrokSystemDeveloperInputItems(payload map[string]any, input []any, mergeInstructions bool) []any {
	if len(input) == 0 {
		return input
	}
	instructionParts := make([]string, 0)
	filtered := make([]any, 0, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			filtered = append(filtered, rawItem)
			continue
		}
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		role := strings.TrimSpace(firstNonEmptyString(item["role"]))
		if itemType == "message" {
			switch role {
			case "developer", "system":
			default:
				filtered = append(filtered, rawItem)
				continue
			}
		} else if role != "developer" && role != "system" {
			filtered = append(filtered, rawItem)
			continue
		}
		if text := strings.TrimSpace(extractTextFromContent(item["content"])); text != "" {
			instructionParts = append(instructionParts, text)
		}
	}
	if mergeInstructions && len(instructionParts) > 0 {
		existing := strings.TrimSpace(firstNonEmptyString(payload["instructions"]))
		mergedParts := make([]string, 0, len(instructionParts)+1)
		if existing != "" {
			mergedParts = append(mergedParts, existing)
		}
		mergedParts = append(mergedParts, instructionParts...)
		payload["instructions"] = strings.Join(mergedParts, "\n\n")
	}
	return filtered
}

func dropGrokEmptyInputItems(input []any) []any {
	if len(input) == 0 {
		return input
	}
	filtered := make([]any, 0, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			filtered = append(filtered, rawItem)
			continue
		}
		if content, ok := item["content"].(string); ok && strings.TrimSpace(content) == "" {
			continue
		}
		filtered = append(filtered, rawItem)
	}
	return filtered
}

func wrapGrokStandaloneInputContentItems(input []any) []any {
	if len(input) == 0 {
		return input
	}
	wrapped := make([]any, 0, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			wrapped = append(wrapped, rawItem)
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(item["role"])) != "" {
			wrapped = append(wrapped, item)
			continue
		}
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		switch itemType {
		case "input_text":
			text := strings.TrimSpace(firstNonEmptyString(item["text"]))
			if text == "" {
				continue
			}
			wrapped = append(wrapped, map[string]any{
				"role":    "user",
				"content": text,
			})
		case "input_image":
			wrapped = append(wrapped, map[string]any{
				"role": "user",
				"content": []any{
					item,
				},
			})
		default:
			wrapped = append(wrapped, item)
		}
	}
	return wrapped
}

func stripGrokIncompatibleCompactionInputItems(input []any) []any {
	if len(input) == 0 {
		return input
	}
	filtered := make([]any, 0, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			filtered = append(filtered, rawItem)
			continue
		}
		if isGrokIncompatibleCompactionInputItem(item) {
			continue
		}
		filtered = append(filtered, rawItem)
	}
	return filtered
}

var grokCodexToolCallTypeAliases = map[string]string{
	"local_shell_call": "function_call",
	"custom_tool_call": "function_call",
	"mcp_tool_call":    "function_call",
	"tool_search_call": "function_call",
}

var grokCodexToolOutputTypeAliases = map[string]string{
	"custom_tool_call_output": "function_call_output",
	"mcp_tool_call_output":    "function_call_output",
	"tool_search_output":      "function_call_output",
}

var grokDroppedCodexInputItemTypes = map[string]struct{}{
	"item_reference":         {},
	"web_search_call":        {},
	"file_search_call":       {},
	"computer_call":          {},
	"code_interpreter_call":  {},
	"image_generation_call":  {},
}

func sanitizeGrokCodexInputItems(input []any, dropReasoning bool) []any {
	if len(input) == 0 {
		return input
	}
	filtered := make([]any, 0, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			filtered = append(filtered, rawItem)
			continue
		}
		normalized, keep := normalizeGrokCodexInputItem(item, dropReasoning)
		if !keep {
			continue
		}
		filtered = append(filtered, normalized)
	}
	return filtered
}

func normalizeGrokCodexInputItem(item map[string]any, dropReasoning bool) (map[string]any, bool) {
	itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
	if itemType == "" {
		if role := strings.TrimSpace(firstNonEmptyString(item["role"])); role != "" {
			return normalizeGrokCodexMessageItem(item), true
		}
		if strings.TrimSpace(firstNonEmptyString(item["text"])) != "" {
			return item, true
		}
		return item, true
	}
	if _, drop := grokDroppedCodexInputItemTypes[itemType]; drop {
		return nil, false
	}
	if targetType, ok := grokCodexToolCallTypeAliases[itemType]; ok {
		return normalizeGrokCodexFunctionCallItem(item, targetType), true
	}
	if targetType, ok := grokCodexToolOutputTypeAliases[itemType]; ok {
		return normalizeGrokCodexFunctionCallOutputItem(item, targetType), true
	}
	switch itemType {
	case "reasoning":
		if dropReasoning {
			return nil, false
		}
		return normalizeGrokCodexReasoningItem(item)
	case "message":
		return normalizeGrokCodexMessageItem(item), true
	case "function_call":
		return normalizeGrokCodexFunctionCallItem(item, "function_call"), true
	case "function_call_output":
		return normalizeGrokCodexFunctionCallOutputItem(item, "function_call_output"), true
	case "input_text", "input_image":
		return item, true
	default:
		return nil, false
	}
}

func normalizeGrokCodexFunctionCallItem(item map[string]any, targetType string) map[string]any {
	normalized := make(map[string]any, len(item)+3)
	for key, value := range item {
		normalized[key] = value
	}
	normalized["type"] = targetType
	callID := strings.TrimSpace(firstNonEmptyString(
		normalized["call_id"],
		normalized["id"],
	))
	if callID != "" {
		normalized["call_id"] = callID
	}
	delete(normalized, "id")
	name := strings.TrimSpace(firstNonEmptyString(
		normalized["name"],
		normalized["tool_name"],
	))
	if name == "" {
		name = "tool"
	}
	normalized["name"] = name
	delete(normalized, "tool_name")
	if _, ok := normalized["arguments"]; !ok {
		if rawArguments := normalized["input"]; rawArguments != nil {
			switch typed := rawArguments.(type) {
			case string:
				normalized["arguments"] = typed
			default:
				if encoded, err := json.Marshal(typed); err == nil {
					normalized["arguments"] = string(encoded)
				}
			}
		}
	}
	delete(normalized, "input")
	if rawArguments, ok := normalized["arguments"]; ok && rawArguments != nil {
		switch typed := rawArguments.(type) {
		case string:
		default:
			if encoded, err := json.Marshal(typed); err == nil {
				normalized["arguments"] = string(encoded)
			}
		}
	}
	for _, field := range []string{"status", "parsed_arguments", "queue_id"} {
		delete(normalized, field)
	}
	return normalized
}

func normalizeGrokCodexFunctionCallOutputItem(item map[string]any, targetType string) map[string]any {
	normalized := make(map[string]any, len(item)+2)
	for key, value := range item {
		normalized[key] = value
	}
	normalized["type"] = targetType
	callID := strings.TrimSpace(firstNonEmptyString(
		normalized["call_id"],
		normalized["id"],
	))
	if callID != "" {
		normalized["call_id"] = callID
	}
	delete(normalized, "id")
	if _, ok := normalized["output"]; !ok {
		if output := extractTextFromContent(normalized["content"]); output != "" {
			normalized["output"] = output
		}
	}
	delete(normalized, "content")
	for _, field := range []string{"status"} {
		delete(normalized, field)
	}
	return normalized
}

func normalizeGrokCodexReasoningItem(item map[string]any) (map[string]any, bool) {
	normalized := make(map[string]any, len(item))
	for key, value := range item {
		if key == "encrypted_content" || key == "id" {
			continue
		}
		normalized[key] = value
	}
	if summary, ok := normalized["summary"]; !ok || summary == nil {
		normalized["summary"] = []any{}
	}
	if len(normalized) <= 1 {
		return nil, false
	}
	return normalized, true
}

func normalizeGrokCodexMessageItem(item map[string]any) map[string]any {
	normalized := make(map[string]any, len(item))
	for key, value := range item {
		normalized[key] = value
	}
	delete(normalized, "type")
	delete(normalized, "id")
	delete(normalized, "status")
	role := strings.TrimSpace(firstNonEmptyString(normalized["role"]))
	switch role {
	case "tool":
		callID := strings.TrimSpace(firstNonEmptyString(normalized["call_id"], normalized["tool_call_id"], normalized["id"]))
		output := extractTextFromContent(normalized["content"])
		if output == "" {
			output = strings.TrimSpace(firstNonEmptyString(normalized["output"]))
		}
		if callID != "" {
			return map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  output,
			}
		}
		normalized["role"] = "user"
	}
	normalized["content"] = flattenGrokCodexMessageContent(normalized["content"])
	return normalized
}

func flattenGrokCodexMessageContent(content any) any {
	switch typed := content.(type) {
	case string:
		return typed
	case []any:
		parts := stripGrokCodexMessageContentFields(typed)
		textChunks := make([]string, 0, len(parts))
		imageParts := make([]any, 0)
		for _, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok || part == nil {
				continue
			}
			partType := strings.TrimSpace(firstNonEmptyString(part["type"]))
			switch partType {
			case "input_image":
				imageParts = append(imageParts, part)
			case "input_text", "output_text", "text", "":
				if text := strings.TrimSpace(firstNonEmptyString(part["text"])); text != "" {
					textChunks = append(textChunks, text)
				}
			default:
				if text := strings.TrimSpace(firstNonEmptyString(part["text"])); text != "" {
					textChunks = append(textChunks, text)
				}
			}
		}
		if len(imageParts) > 0 {
			if len(textChunks) > 0 {
				imageParts = append([]any{map[string]any{"type": "input_text", "text": strings.Join(textChunks, "\n")}}, imageParts...)
			}
			return imageParts
		}
		if len(textChunks) == 1 {
			return textChunks[0]
		}
		if len(textChunks) > 1 {
			return strings.Join(textChunks, "\n")
		}
		return parts
	default:
		return content
	}
}

func stripGrokCodexMessageContentFields(parts []any) []any {
	if len(parts) == 0 {
		return parts
	}
	out := make([]any, len(parts))
	for i, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok || part == nil {
			out[i] = rawPart
			continue
		}
		cleaned := make(map[string]any, len(part))
		for key, value := range part {
			switch key {
			case "nonce":
				continue
			default:
				cleaned[key] = value
			}
		}
		if text, ok := cleaned["text"]; ok {
			if _, isString := text.(string); !isString {
				cleaned["text"] = stringifyCodexContentText(text)
			}
		}
		out[i] = cleaned
	}
	return out
}

func isGrokIncompatibleCompactionInputItem(item map[string]any) bool {
	itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
	switch itemType {
	case "compaction", "compaction_summary":
		return true
	}
	itemID := strings.TrimSpace(firstNonEmptyString(item["id"]))
	if strings.HasPrefix(itemID, "cmp_") && strings.TrimSpace(firstNonEmptyString(item["encrypted_content"])) != "" {
		return true
	}
	return false
}

func normalizeGrokResponsesImageParts(value any) any {
	switch typed := value.(type) {
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeGrokResponsesImageParts(item)
		}
		return out
	case map[string]any:
		if typed == nil {
			return typed
		}
		obj := typed
		partType := strings.TrimSpace(firstNonEmptyString(obj["type"]))
		switch partType {
		case "image":
			mimeType := strings.TrimSpace(firstNonEmptyString(obj["mimeType"]))
			data := strings.TrimSpace(firstNonEmptyString(obj["data"]))
			if mimeType != "" && data != "" {
				detail := strings.TrimSpace(firstNonEmptyString(obj["detail"]))
				if detail == "" {
					detail = "auto"
				}
				return map[string]any{
					"type":      "input_image",
					"image_url": "data:" + mimeType + ";base64," + data,
					"detail":    detail,
				}
			}
		case "image_url":
			imageURL, detail := grokResponsesImageURLAndDetail(obj)
			normalized := map[string]any{
				"type":      "input_image",
				"image_url": imageURL,
			}
			if detail != "" {
				normalized["detail"] = detail
			} else {
				normalized["detail"] = "auto"
			}
			return normalized
		case "input_image":
			imageURL, detail := grokResponsesImageURLAndDetail(obj)
			if imageURL != "" {
				obj["image_url"] = imageURL
			}
			if detail != "" {
				obj["detail"] = detail
			} else {
				obj["detail"] = "auto"
			}
		}
		for _, key := range []string{"content", "output"} {
			if child, ok := obj[key]; ok {
				obj[key] = normalizeGrokResponsesImageParts(child)
			}
		}
		return obj
	default:
		return value
	}
}

func grokResponsesImageURLAndDetail(obj map[string]any) (imageURL string, detail string) {
	if nested, ok := obj["image_url"].(map[string]any); ok && nested != nil {
		imageURL = strings.TrimSpace(firstNonEmptyString(nested["url"]))
		detail = strings.TrimSpace(firstNonEmptyString(nested["detail"]))
		return imageURL, detail
	}
	imageURL = strings.TrimSpace(firstNonEmptyString(obj["image_url"]))
	detail = strings.TrimSpace(firstNonEmptyString(obj["detail"]))
	return imageURL, detail
}

func rewriteGrokResponsesFunctionCallOutputImages(input []any) []any {
	if len(input) == 0 {
		return input
	}
	rewritten := make([]any, 0, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			rewritten = append(rewritten, rawItem)
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(item["type"])) != "function_call_output" {
			rewritten = append(rewritten, item)
			continue
		}
		outputParts, ok := item["output"].([]any)
		if !ok {
			rewritten = append(rewritten, item)
			continue
		}
		imageParts := make([]any, 0)
		textChunks := make([]string, 0)
		for _, rawPart := range outputParts {
			part, ok := rawPart.(map[string]any)
			if ok && part != nil && strings.TrimSpace(firstNonEmptyString(part["type"])) == "input_image" {
				imageParts = append(imageParts, normalizeGrokResponsesImageParts(part))
				continue
			}
			switch typed := rawPart.(type) {
			case string:
				if text := strings.TrimSpace(typed); text != "" {
					textChunks = append(textChunks, text)
				}
			case map[string]any:
				if text := strings.TrimSpace(firstNonEmptyString(typed["text"])); text != "" {
					textChunks = append(textChunks, text)
				}
			}
		}
		if len(imageParts) == 0 {
			rewritten = append(rewritten, item)
			continue
		}
		outputText := strings.Join(textChunks, "\n")
		if outputText == "" {
			outputText = "(tool returned no text output)"
		}
		flattened := make(map[string]any, len(item))
		for key, value := range item {
			flattened[key] = value
		}
		flattened["output"] = outputText
		rewritten = append(rewritten, flattened)

		callID := strings.TrimSpace(firstNonEmptyString(item["call_id"]))
		label := "The previous tool result"
		if callID != "" {
			label += " (" + callID + ")"
		}
		imageCount := len(imageParts)
		if imageCount == 1 {
			label += " included 1 image. Use the attached image as the visual output from that tool."
		} else {
			label += fmt.Sprintf(" included %d images. Use the attached images as the visual output from that tool.", imageCount)
		}
		userContent := make([]any, 0, len(imageParts)+1)
		userContent = append(userContent, map[string]any{"type": "input_text", "text": label})
		userContent = append(userContent, imageParts...)
		rewritten = append(rewritten, map[string]any{
			"role":    "user",
			"content": userContent,
		})
	}
	return rewritten
}

func patchGrokResponsesBody(body []byte, upstreamModel string) ([]byte, error) {
	if !json.Valid(body) {
		return nil, fmt.Errorf("invalid json request body")
	}
	out, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return nil, err
	}
	for _, unsupportedField := range []string{"prompt_cache_retention", "safety_identifier"} {
		if gjson.GetBytes(out, unsupportedField).Exists() {
			out, err = sjson.DeleteBytes(out, unsupportedField)
			if err != nil {
				return nil, err
			}
		}
	}
	out, err = sanitizeGrokResponsesUnsupportedFields(out)
	if err != nil {
		return nil, err
	}
	if isGrokComposerModel(upstreamModel) {
		for _, unsupportedField := range []string{"reasoning", "reasoning_effort", "reasoningEffort"} {
			if gjson.GetBytes(out, unsupportedField).Exists() {
				out, err = sjson.DeleteBytes(out, unsupportedField)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	if strings.TrimSpace(gjson.GetBytes(out, "previous_response_id").String()) != "" {
		if next, deleteErr := sjson.DeleteBytes(out, "instructions"); deleteErr == nil {
			out = next
		}
	}
	out, err = sanitizeGrokResponsesInclude(out)
	if err != nil {
		return nil, err
	}
	out, err = sanitizeGrokResponsesInput(out)
	if err != nil {
		return nil, err
	}
	out, err = sanitizeGrokResponsesTools(out)
	if err != nil {
		return nil, err
	}
	return out, nil
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

func sanitizeGrokResponsesTools(body []byte) ([]byte, error) {
	tools := gjson.GetBytes(body, "tools")
	if !tools.Exists() || !tools.IsArray() {
		return body, nil
	}

	rawTools := tools.Array()
	filteredTools := make([]json.RawMessage, 0, len(rawTools))
	for _, tool := range rawTools {
		toolType := strings.TrimSpace(tool.Get("type").String())
		if _, ok := grokResponsesSupportedToolTypes[toolType]; ok {
			filteredTools = append(filteredTools, json.RawMessage(tool.Raw))
		}
	}

	var err error
	if len(filteredTools) != len(rawTools) {
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

	toolChoice := gjson.GetBytes(body, "tool_choice")
	if !toolChoice.Exists() {
		return body, nil
	}
	if shouldDropGrokToolChoice(toolChoice, filteredTools) {
		body, err = sjson.DeleteBytes(body, "tool_choice")
		if err != nil {
			return nil, err
		}
	}
	return body, nil
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

func buildGrokResponsesRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string) (*http.Request, error) {
	targetURL, err := xai.BuildResponsesURL(account.GetGrokBaseURL())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "sub2api-grok/1.0")
	modelID := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if xai.IsCLIChatProxyBaseURL(account.GetGrokBaseURL()) || xai.IsGrokCLIModel(modelID) {
		xai.SetGrokCLIRequestHeaders(req.Header, modelID)
	}
	if c != nil {
		xai.ForwardGrokCLIRequestHeaders(req.Header, c.Request.Header, gjson.GetBytes(body, "prompt_cache_key").String())
		if v := c.GetHeader("OpenAI-Beta"); strings.TrimSpace(v) != "" {
			req.Header.Set("OpenAI-Beta", v)
		}
	}
	return req, nil
}

func (s *OpenAIGatewayService) updateGrokUsageSnapshot(ctx context.Context, accountID int64, snapshot *xai.QuotaSnapshot) {
	if s == nil || s.accountRepo == nil || accountID <= 0 || snapshot == nil {
		return
	}
	if s.codexSnapshotThrottle != nil && !s.codexSnapshotThrottle.Allow(accountID, time.Now()) {
		return
	}
	_ = s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{
		grokQuotaSnapshotExtraKey: snapshot,
	})
}

func (s *OpenAIGatewayService) handleGrokAccountUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte) {
	if s == nil || account == nil {
		return
	}
	switch statusCode {
	case http.StatusUnauthorized:
		s.tempUnscheduleGrok(ctx, account, 10*time.Minute, "grok oauth token unauthorized")
	case http.StatusForbidden:
		s.tempUnscheduleGrok(ctx, account, 30*time.Minute, "grok entitlement or subscription tier denied")
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
	_ = responseBody
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
