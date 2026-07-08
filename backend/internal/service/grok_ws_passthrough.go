package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	grokWSRequestIDMapperGinKey      = "grok_ws_request_id_mapper"
	grokWSUpstreamRequestPayloadGinKey = "grok_ws_upstream_request_payload"
)

func (s *OpenAIGatewayService) resolveGrokXAIWSPassthroughMode(account *Account) string {
	defaultMode := GrokXAIWSPassthroughModeOff
	if s != nil && s.cfg != nil {
		defaultMode = s.cfg.Gateway.OpenAIWS.GrokXAIPassthroughMode
	}
	return account.ResolveGrokXAIWSPassthroughMode(defaultMode)
}

func isGrokXAIWSPassthroughEnabled(mode string) bool {
	switch normalizeGrokXAIWSPassthroughMode(mode) {
	case GrokXAIWSPassthroughModeAuto, GrokXAIWSPassthroughModeForce:
		return true
	default:
		return false
	}
}

func buildGrokResponsesWebSocketURL(account *Account) (string, error) {
	if account == nil || !account.IsGrok() {
		return "", fmt.Errorf("grok account is required")
	}
	return xai.BuildResponsesWebSocketURL(account.GetGrokBaseURL())
}

func grokResponsesWebSocketUsesInferenceAPI(account *Account) bool {
	return account != nil && account.IsGrokOAuth() && xai.IsCLIChatProxyBaseURL(account.GetGrokBaseURL())
}

// shouldUseGrokWSNativeResponseChain enables api.x.ai Responses WS semantics:
// keep previous_response_id, send incremental input, and store responses server-side.
// This path must not inherit OpenAI store=false replay / HTTP relay collapse behavior.
func shouldUseGrokWSNativeResponseChain(account *Account) bool {
	return grokResponsesWebSocketUsesInferenceAPI(account)
}

func buildGrokResponsesWebSocketHeaders(c *gin.Context, account *Account, token string, payload []byte, sessionID string) http.Header {
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	headers.Set("Accept", "application/json")

	if grokResponsesWebSocketUsesInferenceAPI(account) {
		// api.x.ai Responses WS accepts OAuth bearer directly; avoid forwarding Codex
		// client headers that can pollute the upstream handshake.
		return headers
	}

	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "sub2api-grok-ws/1.0")

	modelID := strings.TrimSpace(gjson.GetBytes(payload, "model").String())
	if account != nil && (xai.IsCLIChatProxyBaseURL(account.GetGrokBaseURL()) || xai.IsGrokCLIModel(modelID)) {
		xai.SetGrokCLIRequestHeaders(headers, modelID)
	}
	promptCacheKey := strings.TrimSpace(gjson.GetBytes(payload, "prompt_cache_key").String())
	if promptCacheKey == "" {
		promptCacheKey = strings.TrimSpace(sessionID)
	}
	if c != nil && c.Request != nil {
		xai.ForwardGrokCLIRequestHeaders(headers, c.Request.Header, promptCacheKey)
	} else if promptCacheKey != "" {
		headers.Set("x-grok-conv-id", promptCacheKey)
	}
	return headers
}

func prepareGrokWebSocketUpstreamPayload(payload []byte, upstreamModel string, account *Account, requestHeaders http.Header, observe *grokObserveSpan) ([]byte, error) {
	wsPayload, err := buildGrokResponsesWebSocketPayload(payload, account)
	if err != nil {
		return nil, err
	}
	patchOpts := grokResponsesPatchOptions{
		requestHeaders: requestHeaders,
		observe:        grokObserveSpanFromPayload(observe, wsPayload, upstreamModel),
	}
	if shouldUseGrokWSNativeResponseChain(account) {
		patchOpts.forceIncrementalInput = true
	}
	return patchGrokResponsesBodyWithOptions(wsPayload, upstreamModel, patchOpts)
}

// collapseGrokWSIngressInput performs ingress-time input collapse for Grok WS native
// chain. It keeps only incremental continuation items instead of forwarding full replay
// history, including on large first-turn payloads that would otherwise force HTTP bridge.
func collapseGrokWSIngressInput(payload []byte, largePayloadThreshold int64) ([]byte, bool, error) {
	if len(payload) == 0 || !json.Valid(payload) {
		return payload, false, nil
	}
	inputResult := gjson.GetBytes(payload, "input")
	if !inputResult.IsArray() || len(inputResult.Array()) <= 1 {
		return payload, false, nil
	}

	previousResponseID := strings.TrimSpace(gjson.GetBytes(payload, "previous_response_id").String())
	forceIncremental := previousResponseID != "" ||
		(largePayloadThreshold > 0 && int64(len(payload)) >= largePayloadThreshold)
	if !forceIncremental {
		return payload, false, nil
	}

	var payloadMap map[string]any
	if err := json.Unmarshal(payload, &payloadMap); err != nil {
		return nil, false, err
	}
	input, ok := payloadMap["input"].([]any)
	if !ok || len(input) <= 1 {
		return payload, false, nil
	}
	observe := &grokObserveSpan{
		PromptCacheKey:     strings.TrimSpace(firstNonEmptyString(payloadMap["prompt_cache_key"])),
		PreviousResponseID: strings.TrimSpace(firstNonEmptyString(payloadMap["previous_response_id"])),
		Transport:          "ws",
	}
	collapsed := collapseGrokResponsesInputForContinuation(
		payloadMap,
		input,
		observe,
		grokCollapseInputOpts{forceTrailingUserOnly: true},
	)
	if len(collapsed) == 0 || grokCollapsedInputEqual(input, collapsed) {
		return payload, false, nil
	}
	cacheKeyBefore := strings.TrimSpace(firstNonEmptyString(payloadMap["prompt_cache_key"]))
	if applyGrokCollapsedInputPromptCacheKeyRotation(payloadMap, input, collapsed) {
		cacheKeyAfter := strings.TrimSpace(firstNonEmptyString(payloadMap["prompt_cache_key"]))
		observeGrokEvent(observe, "cache_key_rotation", map[string]string{
			"rotation_reason":         "ws_ingress_isolated_user_turn",
			"cache_key_before":        cacheKeyBefore,
			"cache_key_after":         cacheKeyAfter,
			"dropped_assistant_chars": fmt.Sprintf("%d", grokDroppedAssistantTextLen(input, collapsed)),
		})
		logOpenAIWSModeInfo(
			"grok_prompt_cache_key_rotated reason=ws_ingress_isolated_user_turn prompt_cache_key=%s previous_response_id_stripped=true",
			truncateOpenAIWSLogValue(cacheKeyAfter, 96),
		)
	}
	payloadMap["input"] = collapsed
	out, err := json.Marshal(payloadMap)
	if err != nil {
		return nil, false, err
	}
	observeGrokEvent(observe, "ws_ingress_collapse", map[string]string{
		"collapse_reason":    grokWSIngressCollapseReason(payload, largePayloadThreshold),
		"collapse_changed":   "true",
		"input_items_before": fmt.Sprintf("%d", len(input)),
		"input_items_after":  fmt.Sprintf("%d", len(collapsed)),
		"inbound_bytes":      fmt.Sprintf("%d", len(payload)),
		"outbound_bytes":     fmt.Sprintf("%d", len(out)),
	})
	return out, true, nil
}

func grokWSIngressCollapseReason(payload []byte, largePayloadThreshold int64) string {
	if strings.TrimSpace(gjson.GetBytes(payload, "previous_response_id").String()) != "" {
		return "previous_response_id"
	}
	if largePayloadThreshold > 0 && int64(len(payload)) >= largePayloadThreshold {
		return "large_payload"
	}
	return "incremental"
}

func setGrokWSRequestIDMapper(c *gin.Context, mapper *grokWSRequestIDMapper) {
	if c == nil || mapper == nil {
		return
	}
	c.Set(grokWSRequestIDMapperGinKey, mapper)
}

func getGrokWSRequestIDMapper(c *gin.Context) *grokWSRequestIDMapper {
	if c == nil {
		return nil
	}
	value, ok := c.Get(grokWSRequestIDMapperGinKey)
	if !ok || value == nil {
		return nil
	}
	mapper, ok := value.(*grokWSRequestIDMapper)
	if !ok {
		return nil
	}
	return mapper
}

func setGrokWSUpstreamRequestPayload(c *gin.Context, payload []byte) {
	if c == nil || len(payload) == 0 {
		return
	}
	c.Set(grokWSUpstreamRequestPayloadGinKey, append([]byte(nil), payload...))
}

func getGrokWSUpstreamRequestPayload(c *gin.Context) []byte {
	if c == nil {
		return nil
	}
	value, ok := c.Get(grokWSUpstreamRequestPayloadGinKey)
	if !ok || value == nil {
		return nil
	}
	payload, ok := value.([]byte)
	if !ok || len(payload) == 0 {
		return nil
	}
	return payload
}

func shouldUseGrokHTTPBridgeIDChain(account *Account) bool {
	// Grok Build HTTP uses prompt_cache_key for conversation affinity; upstream
	// rejects synthetic previous_response_id values from the HTTP bridge.
	return false
}

func isGrokWSAutoDialFallbackEligible(err error) bool {
	if err == nil {
		return false
	}
	switch classifyOpenAIWSAcquireError(err) {
	case "dial_failed", "upgrade_required", "upstream_5xx":
		return true
	}
	var dialErr *openAIWSDialError
	if errors.As(err, &dialErr) && dialErr != nil {
		switch dialErr.StatusCode {
		case http.StatusMethodNotAllowed, http.StatusBadGateway, http.StatusNotImplemented:
			return true
		}
	}
	return false
}

func buildGrokResponsesWebSocketPayload(payload []byte, account *Account) ([]byte, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty grok websocket payload")
	}
	updated, err := sjson.SetBytes(payload, "type", "response.create")
	if err != nil {
		return nil, err
	}
	if updated, err = sjson.DeleteBytes(updated, "stream"); err != nil {
		return nil, err
	}
	if updated, err = sjson.DeleteBytes(updated, "stream_options"); err != nil {
		return nil, err
	}
	if updated, err = sjson.DeleteBytes(updated, "background"); err != nil {
		return nil, err
	}
	switch {
	case shouldUseGrokWSNativeResponseChain(account):
		// api.x.ai previous_response_id chain requires server-side storage even when Codex sends store=false.
		updated, err = sjson.SetBytes(updated, "store", true)
	case !gjson.GetBytes(updated, "store").Exists():
		updated, err = sjson.SetBytes(updated, "store", true)
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(gjson.GetBytes(updated, "previous_response_id").String()) != "" {
		if next, deleteErr := sjson.DeleteBytes(updated, "instructions"); deleteErr == nil {
			updated = next
		}
	}
	return updated, nil
}

func shouldBridgeGrokWSPayload(payload []byte) bool {
	if openAIWSRawPayloadHasToolCallOutput(payload) || grokPayloadHasToolContinuationInTrailingInput(payload) {
		return false
	}
	return grokPayloadContainsImageInput(payload)
}

func grokPayloadContainsImageInput(payload []byte) bool {
	return grokPayloadHasTrailingImageInput(payload)
}

func grokVisionDescriptionChars(payload []byte) int {
	marker := grokVisionTurnUserTextMarker
	text := string(payload)
	idx := strings.Index(text, marker)
	if idx < 0 {
		return 0
	}
	rest := text[idx+len(marker):]
	for _, suffix := range []string{
		`\\n\\nUser request:`,
		"\n\nUser request:",
		`\\n\\nUsing ONLY the image analysis above`,
		"\n\nUsing ONLY the image analysis above",
	} {
		if end := strings.Index(rest, suffix); end >= 0 {
			return len(strings.TrimSpace(rest[:end]))
		}
	}
	return len(strings.TrimSpace(rest))
}

func resolveGrokWSIngressUpstreamModel(account *Account, originalModel string) string {
	upstreamModel := ""
	if account != nil && strings.TrimSpace(originalModel) != "" {
		upstreamModel = normalizeOpenAIModelForUpstream(account, account.GetMappedModel(originalModel))
	}
	if strings.TrimSpace(upstreamModel) == "" {
		upstreamModel = "grok-4.3"
	}
	return upstreamModel
}

func shouldPreprocessGrokComposerImagesAtWSIngress(account *Account, upstreamModel string, payload []byte) bool {
	return shouldUseGrokWSNativeResponseChain(account) && shouldPreprocessGrokComposerImages(upstreamModel, payload)
}

func (s *OpenAIGatewayService) rewriteGrokComposerWSIngressImages(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	payload []byte,
	upstreamModel string,
) ([]byte, *OpenAIUsage, error) {
	if !shouldPreprocessGrokComposerImagesAtWSIngress(account, upstreamModel, payload) {
		return payload, nil, nil
	}
	proxyURL := ""
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	preprocessResult, err := s.preprocessGrokComposerImages(ctx, c, account, payload, token, proxyURL)
	if err != nil {
		return nil, nil, err
	}
	rewritten, err := rewriteGrokComposerBodyWithVisionDescription(payload, preprocessResult.Description)
	if err != nil {
		return nil, nil, err
	}
	var usage *OpenAIUsage
	if preprocessResult.Usage.InputTokens > 0 || preprocessResult.Usage.OutputTokens > 0 {
		usageCopy := preprocessResult.Usage
		usage = &usageCopy
	}
	return rewritten, usage, nil
}

func jsonValueHasType(value any, targetType string) bool {
	switch typed := value.(type) {
	case map[string]any:
		if rawType, ok := typed["type"].(string); ok && strings.EqualFold(strings.TrimSpace(rawType), targetType) {
			return true
		}
		for _, child := range typed {
			if jsonValueHasType(child, targetType) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if jsonValueHasType(child, targetType) {
				return true
			}
		}
	}
	return false
}
