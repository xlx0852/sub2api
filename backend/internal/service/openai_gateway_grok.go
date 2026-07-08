package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	if sessionID != "" && !s.shouldUseGrokBuildHTTPRelay(c, account) {
		grokIDMapper = newGrokWSRequestIDMapper(sessionID, body)
	}

	prepOpts := grokPreparedResponsesOptions{}
	if s.shouldUseGrokBuildHTTPRelay(c, account) {
		prepOpts.UseGrokBuildHTTPRelay = true
		prepOpts.PromptCacheKey = sessionID
	}
	preparedBody, err := s.prepareGrokUpstreamResponsesBody(ctx, c, account, body, upstreamModel, token, proxyURL, prepOpts)
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
	captureGrokTransform(
		"http_relay",
		account.ID,
		"http",
		0,
		sessionID,
		body,
		patchedBody,
		map[string]string{
			"prompt_cache_key":     strings.TrimSpace(gjson.GetBytes(patchedBody, "prompt_cache_key").String()),
			"previous_response_id": strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()),
		},
	)

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
		if resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusBadRequest {
			logOpenAIWSModeInfo(
				"grok_http_upstream_rejected account_id=%d inbound_bytes=%d upstream_bytes=%d previous_response_id=%s input_items=%d input_summary=%s message=%s",
				account.ID,
				len(body),
				len(patchedBody),
				truncateOpenAIWSLogValue(gjson.GetBytes(body, "previous_response_id").String(), 64),
				len(gjson.GetBytes(patchedBody, "input").Array()),
				truncateOpenAIWSLogValue(summarizeGrokResponsesInputItems(gjson.GetBytes(patchedBody, "input")), 256),
				truncateOpenAIWSLogValue(upstreamMsg, openAIWSLogValueMaxLen),
			)
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

// grokPreparedResponsesOptions controls how Codex/Grok upstream bodies are normalized.
type grokPreparedResponsesOptions struct {
	// UseGrokBuildHTTPRelay applies native Grok Build HTTP semantics for Codex HTTP
	// continuation: strip previous_response_id, keep full sanitized input, and pin
	// prompt_cache_key for server-side conversation affinity.
	UseGrokBuildHTTPRelay bool
	PromptCacheKey        string
}

type grokResponsesPatchOptions struct {
	dropReplayedAssistantMessages bool
	collapseForGrokBuildHTTPRelay bool
	forceIncrementalInput         bool
	requestHeaders                http.Header
	observe                       *grokObserveSpan
}

func (s *OpenAIGatewayService) shouldUseGrokBuildHTTPRelay(c *gin.Context, account *Account) bool {
	if account != nil && account.IsGrokOAuth() {
		return true
	}
	return s.shouldUseCodexHighFidelityRelay(c)
}

func applyGrokBuildHTTPRelayBody(body []byte, promptCacheKey string) ([]byte, error) {
	if !json.Valid(body) {
		return nil, fmt.Errorf("invalid json request body")
	}
	out := body
	var err error
	if strings.TrimSpace(gjson.GetBytes(out, "previous_response_id").String()) != "" {
		out, err = sjson.DeleteBytes(out, "previous_response_id")
		if err != nil {
			return nil, err
		}
	}
	promptCacheKey = strings.TrimSpace(promptCacheKey)
	if promptCacheKey != "" && !gjson.GetBytes(out, "prompt_cache_key").Exists() {
		out, err = sjson.SetBytes(out, "prompt_cache_key", promptCacheKey)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *OpenAIGatewayService) prepareGrokUpstreamResponsesBody(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	upstreamModel string,
	token string,
	proxyURL string,
	opts grokPreparedResponsesOptions,
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

	if opts.UseGrokBuildHTTPRelay {
		var relayErr error
		body, relayErr = applyGrokBuildHTTPRelayBody(body, opts.PromptCacheKey)
		if relayErr != nil {
			return nil, relayErr
		}
	}

	patchOpts := grokResponsesPatchOptions{}
	if c != nil && c.Request != nil {
		patchOpts.requestHeaders = c.Request.Header
	}
	if opts.UseGrokBuildHTTPRelay {
		patchOpts.dropReplayedAssistantMessages = true
		patchOpts.collapseForGrokBuildHTTPRelay = true
	}
	patchedBody, err := patchGrokResponsesBodyWithOptions(body, upstreamModel, patchOpts)
	if err != nil {
		return nil, err
	}
	return &grokPreparedResponsesBody{
		Body:            patchedBody,
		PreprocessUsage: preprocessUsage,
	}, nil
}

func shouldPreprocessGrokComposerImages(upstreamModel string, body []byte) bool {
	return grokPayloadRequiresComposerImagePreprocess(upstreamModel, body)
}

func grokPayloadRequiresComposerImagePreprocess(upstreamModel string, body []byte) bool {
	if !isGrokComposerModel(upstreamModel) {
		return false
	}
	if openAIWSRawPayloadHasToolCallOutput(body) {
		return false
	}
	if grokPayloadHasToolContinuationInTrailingInput(body) {
		return false
	}
	return grokPayloadHasTrailingImageInput(body)
}

func grokPayloadHasTrailingImageInput(body []byte) bool {
	if len(body) == 0 || !json.Valid(body) {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) == 0 {
		return false
	}
	trailing := trailingGrokItemsForImageDetection(input)
	if len(trailing) == 0 {
		return false
	}
	var images []string
	for _, item := range trailing {
		collectGrokResponsesImageURLs(item, &images)
	}
	return len(images) > 0
}

func grokPayloadHasToolContinuationInTrailingInput(body []byte) bool {
	if len(body) == 0 || !json.Valid(body) {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) == 0 {
		return false
	}
	for _, rawItem := range trailingGrokItemsForImageDetection(input) {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			continue
		}
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		switch {
		case itemType == "function_call", itemType == "function_call_output",
			isCodexToolCallContextItemType(itemType), isCodexToolCallOutputItemType(itemType):
			return true
		}
	}
	return false
}

func trailingGrokItemsForImageDetection(input []any) []any {
	if len(input) == 0 {
		return nil
	}
	lastAssistantIdx := -1
	for i, rawItem := range input {
		if isGrokAssistantInputItem(rawItem) {
			lastAssistantIdx = i
		}
	}
	if lastAssistantIdx >= 0 && lastAssistantIdx+1 < len(input) {
		return input[lastAssistantIdx+1:]
	}
	if isGrokTrailingUserContinuation(input) {
		if kept := keepTrailingGrokUserMessages(input); len(kept) > 0 {
			return kept
		}
	}
	if needsGrokTrailingToolContinuation(input) {
		return keepTrailingGrokToolContinuationItems(input)
	}
	return []any{input[len(input)-1]}
}

func isGrokComposerModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "grok-composer-")
}

func isGrokBuildModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == "grok-build" || strings.HasPrefix(model, "grok-build-")
}

func grokModelRejectsReasoningEffort(model string) bool {
	return isGrokComposerModel(model) || isGrokBuildModel(model)
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
		"Return a concise but complete textual description with enough detail for the assistant to fully answer the user's request without seeing the image."
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
	rebuildGrokComposerInputAfterVisionPreprocess(payload, description)
	return marshalOpenAIUpstreamJSON(payload)
}

func grokResponsesImageDataURL(obj map[string]any) string {
	if obj == nil {
		return ""
	}
	partType := strings.TrimSpace(firstNonEmptyString(obj["type"]))
	imageURL := strings.TrimSpace(firstNonEmptyString(obj["image_url"]))
	if imageURL == "" {
		if imageObject, ok := obj["image_url"].(map[string]any); ok && imageObject != nil {
			imageURL = strings.TrimSpace(firstNonEmptyString(imageObject["url"]))
		}
	}
	switch partType {
	case "input_image", "image_url":
		return imageURL
	case "image":
		if imageURL != "" {
			return imageURL
		}
		mimeType := strings.TrimSpace(firstNonEmptyString(obj["mimeType"], obj["mime_type"]))
		data := strings.TrimSpace(firstNonEmptyString(obj["data"]))
		if mimeType != "" && data != "" {
			return "data:" + mimeType + ";base64," + data
		}
		if source, ok := obj["source"].(map[string]any); ok && source != nil {
			mediaType := strings.TrimSpace(firstNonEmptyString(source["media_type"], source["mime_type"], source["mimeType"]))
			sourceData := strings.TrimSpace(firstNonEmptyString(source["data"]))
			if mediaType != "" && sourceData != "" {
				return "data:" + mediaType + ";base64," + sourceData
			}
		}
	}
	if imageURL != "" {
		return imageURL
	}
	return ""
}

func collectGrokResponsesImageURLs(value any, out *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		if imageURL := grokResponsesImageDataURL(typed); imageURL != "" {
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

const grokVisionTurnUserTextMarker = "Image analysis from Grok vision preprocessing:"

func stripGrokResponsesImagesFromPayload(payload map[string]any) {
	input, ok := payload["input"].([]any)
	if !ok {
		return
	}
	normalized := make([]any, 0, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			normalized = append(normalized, rawItem)
			continue
		}
		partType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		if partType == "input_image" || partType == "image" || partType == "image_url" || item["image_url"] != nil {
			continue
		}
		content, ok := item["content"].([]any)
		if !ok {
			normalized = append(normalized, item)
			continue
		}
		normalizedContent := make([]any, 0, len(content))
		for _, rawPart := range content {
			part, ok := rawPart.(map[string]any)
			if !ok || part == nil {
				normalizedContent = append(normalizedContent, rawPart)
				continue
			}
			partType := strings.TrimSpace(firstNonEmptyString(part["type"]))
			if partType == "input_image" || partType == "image" || partType == "image_url" {
				continue
			}
			delete(part, "image_url")
			normalizedContent = append(normalizedContent, part)
		}
		item["content"] = normalizedContent
		normalized = append(normalized, item)
	}
	payload["input"] = normalized
}

func rebuildGrokComposerInputAfterVisionPreprocess(payload map[string]any, description string) {
	userText := strings.TrimSpace(extractTrailingGrokUserRequestText(payload))
	payload["input"] = []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{
					"type": "input_text",
					"text": buildGrokVisionTurnUserText(description, userText),
				},
			},
		},
	}
}

func buildGrokVisionTurnUserText(description string, userText string) string {
	var b strings.Builder
	b.WriteString(grokVisionTurnUserTextMarker)
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(description))
	b.WriteString("\n\n")
	if userText != "" {
		b.WriteString("User request:\n")
		b.WriteString(userText)
		b.WriteString("\n\n")
	}
	b.WriteString("Using ONLY the image analysis above, fully answer the user request now in this response. ")
	b.WriteString("Do not say you are still viewing, loading, or waiting for the image. ")
	b.WriteString("Provide the complete analysis immediately.")
	return b.String()
}

func extractTrailingGrokUserRequestText(payload map[string]any) string {
	input, _ := payload["input"].([]any)
	for i := len(input) - 1; i >= 0; i-- {
		item, ok := input[i].(map[string]any)
		if !ok || item == nil {
			continue
		}
		role := strings.TrimSpace(firstNonEmptyString(item["role"]))
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		switch {
		case role == "user" || (itemType == "message" && role != "assistant" && role != "developer" && role != "system"):
			if text := strings.TrimSpace(extractGrokInputItemText(item)); text != "" {
				return text
			}
		case itemType == "input_text":
			if text := strings.TrimSpace(firstNonEmptyString(item["text"])); text != "" && !strings.Contains(text, grokVisionTurnUserTextMarker) {
				return text
			}
		}
	}
	return ""
}

func extractGrokInputItemText(item map[string]any) string {
	if item == nil {
		return ""
	}
	switch content := item["content"].(type) {
	case string:
		text := strings.TrimSpace(content)
		if strings.Contains(text, grokVisionTurnUserTextMarker) {
			return ""
		}
		return text
	case []any:
		chunks := make([]string, 0, len(content))
		for _, rawPart := range content {
			part, ok := rawPart.(map[string]any)
			if !ok || part == nil {
				continue
			}
			partType := strings.TrimSpace(firstNonEmptyString(part["type"]))
			if partType != "input_text" && partType != "output_text" && partType != "" && partType != "text" {
				continue
			}
			text := strings.TrimSpace(firstNonEmptyString(part["text"]))
			if text == "" || strings.Contains(text, grokVisionTurnUserTextMarker) {
				continue
			}
			chunks = append(chunks, text)
		}
		return strings.Join(chunks, "\n")
	default:
		return ""
	}
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
	return sanitizeGrokResponsesInputWithOptions(body, grokResponsesPatchOptions{})
}

func sanitizeGrokResponsesInputWithOptions(body []byte, opts grokResponsesPatchOptions) ([]byte, error) {
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
	dropReasoning := opts.collapseForGrokBuildHTTPRelay
	input = sanitizeGrokCodexInputItems(input, dropReasoning)
	input = wrapGrokStandaloneInputContentItems(input)
	normalizedInput, ok := normalizeGrokResponsesImageParts(input).([]any)
	if !ok {
		return body, nil
	}
	normalizedInput = rewriteGrokResponsesFunctionCallOutputImages(normalizedInput)
	normalizedInput = finalizeGrokFunctionCallOutputStrings(normalizedInput)
	inputBeforeCollapse := append([]any(nil), normalizedInput...)
	forceTrailingUserOnly := opts.forceIncrementalInput || opts.collapseForGrokBuildHTTPRelay
	switch {
	case strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()) != "":
		normalizedInput = collapseGrokResponsesInputForContinuation(payload, normalizedInput, opts.observe, grokCollapseInputOpts{
			forceTrailingUserOnly: forceTrailingUserOnly,
		})
	case opts.forceIncrementalInput && len(normalizedInput) > 1:
		normalizedInput = collapseGrokResponsesInputForContinuation(payload, normalizedInput, opts.observe, grokCollapseInputOpts{
			forceTrailingUserOnly: true,
		})
	case opts.collapseForGrokBuildHTTPRelay && len(normalizedInput) > 1:
		normalizedInput = collapseGrokResponsesInputForContinuation(payload, normalizedInput, opts.observe, grokCollapseInputOpts{
			forceTrailingUserOnly: true,
		})
	case opts.dropReplayedAssistantMessages:
		normalizedInput = dropGrokAssistantInputItems(normalizedInput)
	}
	cacheKeyBefore := strings.TrimSpace(firstNonEmptyString(payload["prompt_cache_key"]))
	if applyGrokCollapsedInputPromptCacheKeyRotation(payload, inputBeforeCollapse, normalizedInput) {
		cacheKeyAfter := strings.TrimSpace(firstNonEmptyString(payload["prompt_cache_key"]))
		observeGrokEvent(opts.observe, "cache_key_rotation", map[string]string{
			"rotation_reason":         "isolated_user_turn",
			"cache_key_before":        cacheKeyBefore,
			"cache_key_after":         cacheKeyAfter,
			"dropped_assistant_chars": fmt.Sprintf("%d", grokDroppedAssistantTextLen(inputBeforeCollapse, normalizedInput)),
		})
		logOpenAIWSModeInfo(
			"grok_prompt_cache_key_rotated reason=isolated_user_turn prompt_cache_key=%s previous_response_id_stripped=true",
			truncateOpenAIWSLogValue(cacheKeyAfter, 96),
		)
	}
	payload["input"] = normalizedInput
	return json.Marshal(payload)
}

func summarizeGrokResponsesInputItems(input gjson.Result) string {
	if !input.IsArray() {
		return "-"
	}
	parts := make([]string, 0, len(input.Array()))
	for i, item := range input.Array() {
		itemType := strings.TrimSpace(item.Get("type").String())
		role := strings.TrimSpace(item.Get("role").String())
		switch {
		case itemType != "" && role != "":
			parts = append(parts, fmt.Sprintf("%d:%s/%s", i, itemType, role))
		case itemType != "":
			parts = append(parts, fmt.Sprintf("%d:%s", i, itemType))
		case role != "":
			parts = append(parts, fmt.Sprintf("%d:%s", i, role))
		default:
			parts = append(parts, fmt.Sprintf("%d:?", i))
		}
	}
	return strings.Join(parts, ",")
}

// grokKeepContinuationHistoryEnabled controls whether plain user continuation turns
// keep prior user/assistant dialogue in the outbound Grok input. Defaults to ON:
// collapsing to only the trailing user message relies on unreliable xAI server-side
// prompt_cache_key affinity and drops multi-turn context. Set SUB2API_GROK_KEEP_HISTORY=0
// (or false) to restore the legacy trailing-user-only behavior.
func grokKeepContinuationHistoryEnabled() bool {
	v := strings.TrimSpace(os.Getenv("SUB2API_GROK_KEEP_HISTORY"))
	if v == "" {
		return true
	}
	return !(v == "0" || strings.EqualFold(v, "false"))
}

type grokCollapseInputOpts struct {
	// forceTrailingUserOnly collapses plain user continuations to the latest user
	// message even when SUB2API_GROK_KEEP_HISTORY keeps full dialogue on HTTP paths.
	forceTrailingUserOnly bool
}

func collapseGrokResponsesInputForContinuation(payload map[string]any, input []any, observe *grokObserveSpan, opts ...grokCollapseInputOpts) []any {
	if len(input) == 0 {
		return input
	}
	var collapseOpts grokCollapseInputOpts
	if len(opts) > 0 {
		collapseOpts = opts[0]
	}
	_ = payload

	lastAssistantIdx := -1
	for i, rawItem := range input {
		if isGrokAssistantInputItem(rawItem) {
			lastAssistantIdx = i
		}
	}

	strategy := grokCollapseStrategyName(input)
	var collapsed []any
	preserveAssistants := false
	recallDetected := false
	switch {
	case !collapseOpts.forceTrailingUserOnly && grokUserMessageReferencesPriorContext(grokLatestUserMessageText(input)):
		// Recall turns need prior user/assistant answers in outbound input; cache alone is flaky.
		collapsed = keepGrokRecallDialogueInput(input)
		preserveAssistants = true
		recallDetected = true
	case isGrokTrailingUserContinuation(input):
		if !collapseOpts.forceTrailingUserOnly && grokKeepContinuationHistoryEnabled() {
			// Keep prior user/assistant dialogue so context survives across turns;
			// relying on xAI server-side prompt_cache_key affinity alone loses context.
			collapsed = keepGrokRecallDialogueInput(input)
			preserveAssistants = true
		} else {
			// Fresh user turn after tool rounds: only forward the latest user message.
			collapsed = keepTrailingGrokUserMessages(input)
		}
	case needsGrokTrailingToolContinuation(input):
		collapsed = keepGrokToolContinuationItemsSinceLastUser(input)
		collapsed = prependGrokUserRequestAnchor(input, collapsed)
	case lastAssistantIdx >= 0 && lastAssistantIdx+1 < len(input):
		collapsed = input[lastAssistantIdx+1:]
		collapsed = prependGrokUserRequestAnchor(input, collapsed)
	default:
		collapsed = keepTrailingGrokUserMessages(input)
	}
	if !preserveAssistants {
		collapsed = dropGrokAssistantInputItems(collapsed)
	} else {
		collapsed = sanitizeGrokInputAssistantItems(collapsed)
	}
	if len(collapsed) == 0 {
		collapsed = keepTrailingGrokContinuationInputItems(input)
	}
	droppedAssistantChars := grokDroppedAssistantTextLen(input, collapsed)
	observeGrokCollapseDecision(observe, strategy, input, collapsed, preserveAssistants, recallDetected, droppedAssistantChars)
	return collapsed
}

func keepGrokRecallDialogueInput(input []any) []any {
	if len(input) == 0 {
		return input
	}
	out := make([]any, 0, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			continue
		}
		role := strings.TrimSpace(firstNonEmptyString(item["role"]))
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		if role != "user" && role != "assistant" {
			continue
		}
		if itemType != "" && itemType != "message" {
			continue
		}
		out = append(out, rawItem)
	}
	return out
}

func prependGrokUserRequestAnchor(fullInput, collapsed []any) []any {
	if len(collapsed) == 0 || len(fullInput) == 0 {
		return collapsed
	}
	userAnchor := keepTrailingGrokUserMessages(fullInput)
	if len(userAnchor) == 0 {
		return collapsed
	}
	if grokCollapsedInputStartsWithUserItem(collapsed, userAnchor[0]) {
		return collapsed
	}
	out := make([]any, 0, len(userAnchor)+len(collapsed))
	out = append(out, userAnchor...)
	out = append(out, collapsed...)
	return out
}

func grokCollapsedInputEqual(before, after []any) bool {
	if len(before) != len(after) {
		return false
	}
	beforeJSON, err1 := json.Marshal(before)
	afterJSON, err2 := json.Marshal(after)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(beforeJSON) == string(afterJSON)
}

func grokCollapsedInputStartsWithUserItem(collapsed []any, userItem any) bool {
	if len(collapsed) == 0 || userItem == nil {
		return false
	}
	first, ok := collapsed[0].(map[string]any)
	if !ok || first == nil {
		return false
	}
	anchor, ok := userItem.(map[string]any)
	if !ok || anchor == nil {
		return false
	}
	firstText := strings.TrimSpace(extractGrokInputItemText(first))
	anchorText := strings.TrimSpace(extractGrokInputItemText(anchor))
	return firstText != "" && firstText == anchorText
}

func isGrokTrailingUserContinuation(input []any) bool {
	for i := len(input) - 1; i >= 0; i-- {
		item, ok := input[i].(map[string]any)
		if !ok || item == nil {
			continue
		}
		role := strings.TrimSpace(firstNonEmptyString(item["role"]))
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		if role == "user" {
			return true
		}
		if itemType == "function_call" || itemType == "function_call_output" || isCodexToolCallOutputItemType(itemType) {
			return false
		}
	}
	return false
}

func needsGrokTrailingToolContinuation(input []any) bool {
	for i := len(input) - 1; i >= 0; i-- {
		item, ok := input[i].(map[string]any)
		if !ok || item == nil {
			continue
		}
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		switch {
		case itemType == "function_call_output", isCodexToolCallOutputItemType(itemType):
			return true
		case itemType == "function_call", isCodexToolCallContextItemType(itemType):
			return true
		case strings.TrimSpace(firstNonEmptyString(item["role"])) == "user":
			return false
		}
	}
	return false
}

func keepTrailingGrokUserMessages(input []any) []any {
	if len(input) == 0 {
		return input
	}
	for i := len(input) - 1; i >= 0; i-- {
		item, ok := input[i].(map[string]any)
		if !ok || item == nil {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(item["role"])) == "user" {
			return []any{item}
		}
	}
	return nil
}

func isGrokToolContinuationItem(item map[string]any) bool {
	if item == nil {
		return false
	}
	itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
	switch {
	case itemType == "function_call", itemType == "function_call_output":
		return true
	case isCodexToolCallContextItemType(itemType), isCodexToolCallOutputItemType(itemType):
		return true
	default:
		return false
	}
}

func grokLastUserMessageIndex(input []any) int {
	lastUserIdx := -1
	for i, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(item["role"])) == "user" {
			lastUserIdx = i
		}
	}
	return lastUserIdx
}

// keepGrokToolContinuationItemsSinceLastUser retains every tool call/output pair
// after the latest user message so multi-step agent turns do not lose context.
func keepGrokToolContinuationItemsSinceLastUser(input []any) []any {
	if len(input) == 0 {
		return input
	}
	lastUserIdx := grokLastUserMessageIndex(input)
	start := 0
	if lastUserIdx >= 0 {
		start = lastUserIdx + 1
	}
	if start >= len(input) {
		return keepTrailingGrokToolContinuationItems(input)
	}
	filtered := make([]any, 0)
	for _, rawItem := range input[start:] {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			continue
		}
		if isGrokToolContinuationItem(item) {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		return keepTrailingGrokToolContinuationItems(input)
	}
	return filtered
}

func keepTrailingGrokToolContinuationItems(input []any) []any {
	if len(input) == 0 {
		return input
	}
	lastIdx := len(input) - 1
	lastItem, ok := input[lastIdx].(map[string]any)
	if !ok || lastItem == nil {
		return keepTrailingGrokUserMessages(input)
	}
	lastType := strings.TrimSpace(firstNonEmptyString(lastItem["type"]))
	switch {
	case lastType == "function_call_output", isCodexToolCallOutputItemType(lastType):
		start := lastIdx
		if start > 0 {
			if prev, ok := input[start-1].(map[string]any); ok && prev != nil {
				prevType := strings.TrimSpace(firstNonEmptyString(prev["type"]))
				if prevType == "function_call" || isCodexToolCallContextItemType(prevType) {
					start--
				}
			}
		}
		return input[start:]
	case lastType == "function_call", isCodexToolCallContextItemType(lastType):
		return input[lastIdx:]
	default:
		lastToolIdx := -1
		for i, rawItem := range input {
			item, ok := rawItem.(map[string]any)
			if !ok || item == nil {
				continue
			}
			itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
			if itemType == "function_call" || itemType == "function_call_output" ||
				isCodexToolCallContextItemType(itemType) || isCodexToolCallOutputItemType(itemType) {
				lastToolIdx = i
			}
		}
		if lastToolIdx >= 0 {
			return keepTrailingGrokToolContinuationItems(input[lastToolIdx:])
		}
		return keepTrailingGrokUserMessages(input)
	}
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
	"additional_tools":      {},
	"agent_message":         {},
	"item_reference":        {},
	"web_search_call":       {},
	"file_search_call":      {},
	"computer_call":         {},
	"code_interpreter_call": {},
	"image_generation_call": {},
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
		if key == "id" {
			continue
		}
		if key == "encrypted_content" {
			encrypted, ok := value.(string)
			if !ok || strings.TrimSpace(encrypted) == "" {
				continue
			}
			if _, err := xai.InspectGrokEncryptedContent(encrypted); err != nil {
				continue
			}
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

func finalizeGrokFunctionCallOutputStrings(input []any) []any {
	if len(input) == 0 {
		return input
	}
	for i, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item == nil {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(item["type"])) != "function_call_output" {
			continue
		}
		rawOutput, ok := item["output"]
		if !ok || rawOutput == nil {
			continue
		}
		if _, isString := rawOutput.(string); isString {
			continue
		}
		item["output"] = stringifyCodexContentText(rawOutput)
		input[i] = item
	}
	return input
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
	return patchGrokResponsesBodyWithOptions(body, upstreamModel, grokResponsesPatchOptions{})
}

func patchGrokResponsesBodyWithOptions(body []byte, upstreamModel string, opts grokResponsesPatchOptions) ([]byte, error) {
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
	if grokModelRejectsReasoningEffort(upstreamModel) {
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
	} else if opts.collapseForGrokBuildHTTPRelay {
		input := gjson.GetBytes(out, "input")
		if input.IsArray() && len(input.Array()) > 1 {
			if next, deleteErr := sjson.DeleteBytes(out, "instructions"); deleteErr == nil {
				out = next
			}
		}
	}
	span := grokObserveSpanFromPayload(opts.observe, body, upstreamModel)
	observeGrokPatchPipeline(span, "patch_begin", body, out, nil)

	out, err = sanitizeGrokResponsesInclude(out)
	if err != nil {
		return nil, err
	}
	beforeReplay := append([]byte(nil), out...)
	out = applyGrokReasoningReplayCacheObserved(out, upstreamModel, opts.requestHeaders, span)
	observeGrokPatchPipeline(span, "reasoning_replay", beforeReplay, out, nil)

	beforeEncrypted := append([]byte(nil), out...)
	beforeCodex, beforePreserved, beforeInvalid := countGrokEncryptedContentStats(beforeEncrypted)
	out = sanitizeGrokInputEncryptedContent(out)
	afterCodex, afterPreserved, afterInvalid := countGrokEncryptedContentStats(out)
	observeGrokPatchPipeline(span, "encrypted_sanitize", beforeEncrypted, out, map[string]string{
		"encrypted_codex_rejected": fmt.Sprintf("%d", beforeCodex-afterCodex),
		"encrypted_preserved":      fmt.Sprintf("%d", afterPreserved),
		"encrypted_dropped":        fmt.Sprintf("%d", beforeInvalid-afterInvalid+beforeCodex),
		"encrypted_before":         fmt.Sprintf("codex=%d preserved=%d invalid=%d", beforeCodex, beforePreserved, beforeInvalid),
		"encrypted_after":          fmt.Sprintf("codex=%d preserved=%d invalid=%d", afterCodex, afterPreserved, afterInvalid),
	})

	beforeInput := append([]byte(nil), out...)
	out, err = sanitizeGrokResponsesInputWithOptions(out, opts)
	if err != nil {
		return nil, err
	}
	observeGrokPatchPipeline(span, "input_sanitize", beforeInput, out, map[string]string{
		"force_incremental":            fmt.Sprintf("%v", opts.forceIncrementalInput),
		"collapse_http_relay":          fmt.Sprintf("%v", opts.collapseForGrokBuildHTTPRelay),
		"drop_replayed_assistant_msgs": fmt.Sprintf("%v", opts.dropReplayedAssistantMessages),
	})

	out, err = sanitizeGrokResponsesTools(out)
	if err != nil {
		return nil, err
	}
	observeGrokPatchPipeline(span, "patch_end", body, out, nil)
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
