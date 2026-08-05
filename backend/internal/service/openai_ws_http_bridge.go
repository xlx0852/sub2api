package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIWSClientReadLimitBytesDefault     int64 = 64 * 1024 * 1024
	openAIWSHTTPBridgeThresholdBytesDefault int64 = 15 * 1024 * 1024
	openAIWSHTTPBridgeErrorBodyLimitBytes         = 64 * 1024
	// Hard caps for WS→HTTP bridge streams. Idle timeout alone is not enough
	// when an upstream keeps emitting token events without ever completing.
	openAIWSHTTPBridgeMaxTokenEventsDefault = 80_000
	openAIWSHTTPBridgeMaxDurationDefault    = 8 * time.Minute
)

// ResolveOpenAIWSClientFirstMessageTimeout returns the effective client ingress deadline.
func ResolveOpenAIWSClientFirstMessageTimeout(cfg *config.Config) time.Duration {
	seconds := config.DefaultOpenAIWSClientFirstMessageTimeoutSeconds
	if cfg != nil && cfg.Gateway.OpenAIWS.ClientFirstMessageTimeoutSeconds > 0 {
		seconds = cfg.Gateway.OpenAIWS.ClientFirstMessageTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func ResolveOpenAIWSClientReadLimitBytes(cfg *config.Config) int64 {
	if cfg == nil || cfg.Gateway.OpenAIWS.ClientReadLimitBytes <= 0 {
		return openAIWSClientReadLimitBytesDefault
	}
	return cfg.Gateway.OpenAIWS.ClientReadLimitBytes
}

func (s *OpenAIGatewayService) openAIWSHTTPBridgeEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled
}

func (s *OpenAIGatewayService) openAIWSHTTPBridgeThresholdBytes() int64 {
	if s == nil || s.cfg == nil || s.cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes <= 0 {
		return openAIWSHTTPBridgeThresholdBytesDefault
	}
	return s.cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes
}

func (s *OpenAIGatewayService) shouldBridgeOpenAIWSHTTP(account *Account, payloadBytes int, previousResponseID string) bool {
	if !s.openAIWSHTTPBridgeEnabled() {
		return false
	}
	if strings.TrimSpace(previousResponseID) != "" {
		return false
	}
	threshold := s.openAIWSHTTPBridgeThresholdBytes()
	return threshold > 0 && int64(payloadBytes) >= threshold
}

func prepareOpenAIWSHTTPBridgeBody(payload []byte) ([]byte, error) {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("response.create payload must be a JSON object")
	}
	delete(body, "type")
	delete(body, "generate")
	delete(body, "previous_response_id")
	ensureOpenAIWSBridgeInputArguments(body)
	body["stream"] = true
	return json.Marshal(body)
}

// ensureOpenAIWSBridgeInputArguments 兜底多轮会话重放时 function_call 系列项的
// arguments 字段。上游 OpenAI 对每个 tool 调用项都要求 arguments（缺失或空会
// 返回 400 "Missing required parameter: 'input[N].arguments'"）。重放路径会把
// 上一轮从 response.output_item.done 抓到的 function_call 项原样注入下一轮
// input，若该事件抓到的是分片中间态（arguments 尚未到齐），就会原样转发触发
// 400。此处对缺失或空字符串的 arguments 统一补 "{}"，对象形态（新版 codex）
// 保持原样。
func ensureOpenAIWSBridgeInputArguments(body map[string]any) {
	rawInput, ok := body["input"]
	if !ok {
		return
	}
	items, ok := rawInput.([]any)
	if !ok {
		return
	}
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := item["type"].(string)
		if !isCodexToolCallContextItemType(typ) {
			continue
		}
		args, has := item["arguments"]
		if !has {
			item["arguments"] = "{}"
			continue
		}
		if s, ok := args.(string); ok && strings.TrimSpace(s) == "" {
			item["arguments"] = "{}"
		}
	}
}

type openAIWSToolCallReplayCollector struct {
	items []json.RawMessage
	seen  map[string]struct{}
}

func (c *openAIWSToolCallReplayCollector) AddEvent(eventType string, message []byte) {
	switch strings.TrimSpace(eventType) {
	case "response.output_item.done":
		c.addItem(gjson.GetBytes(message, "item"))
	case "response.completed", "response.done":
		output := gjson.GetBytes(message, "response.output")
		if !output.IsArray() {
			return
		}
		for _, item := range output.Array() {
			c.addItem(item)
		}
	}
}

func (c *openAIWSToolCallReplayCollector) Items() []json.RawMessage {
	return cloneOpenAIWSRawMessages(c.items)
}

// addItem keeps prior-turn output items that must be re-injected into the next
// HTTP bridge request when the bridge cannot rely on server-side response state.
func (c *openAIWSToolCallReplayCollector) addItem(item gjson.Result) {
	if !item.Exists() || item.Type != gjson.JSON {
		return
	}
	raw := strings.TrimSpace(item.Raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return
	}
	itemType := strings.TrimSpace(item.Get("type").String())
	switch {
	case isCodexToolCallContextItemType(itemType):
		// keep as-is
	case itemType == "message" || (itemType == "" && strings.EqualFold(item.Get("role").String(), "assistant")):
		// Always normalize so assistant content parts become output_text.
		// If normalization fails (empty/unusable content), drop the item rather
		// than replaying the original shape (which previously caused 400s).
		normalized := normalizeOpenAIWSReplayMessageItem(item)
		if len(normalized) == 0 {
			return
		}
		raw = string(normalized)
	case itemType == "reasoning":
		normalized := normalizeOpenAIWSReplayReasoningItem(item)
		if len(normalized) == 0 {
			return
		}
		raw = string(normalized)
	default:
		return
	}
	key := strings.TrimSpace(item.Get("id").String())
	if key == "" {
		key = strings.TrimSpace(item.Get("call_id").String())
	}
	if key == "" {
		key = raw
	}
	if c.seen == nil {
		c.seen = make(map[string]struct{})
	}
	if _, ok := c.seen[key]; ok {
		return
	}
	c.seen[key] = struct{}{}
	c.items = append(c.items, json.RawMessage(raw))
}

func openAIWSReplayContentPartType(role string) string {
	// OpenAI Responses input items: assistant/tool content parts must use
	// output_text/refusal; user/system use input_text. Replaying assistant
	// turns with input_text yields 400:
	//   Invalid value: 'input_text'. Supported values are: 'output_text' and 'refusal'.
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "tool":
		return "output_text"
	default:
		return "input_text"
	}
}

func normalizeOpenAIWSReplayMessageItem(item gjson.Result) []byte {
	role := strings.TrimSpace(item.Get("role").String())
	if role == "" {
		role = "assistant"
	}
	partType := openAIWSReplayContentPartType(role)
	textParts := make([]map[string]any, 0, 2)
	content := item.Get("content")
	if content.IsArray() {
		for _, part := range content.Array() {
			text := strings.TrimSpace(firstNonEmpty(
				part.Get("text").String(),
				part.Get("output_text").String(),
				part.Get("input_text").String(),
			))
			if text == "" {
				continue
			}
			textParts = append(textParts, map[string]any{
				"type": partType,
				"text": text,
			})
		}
	} else if content.Type == gjson.String {
		if text := strings.TrimSpace(content.String()); text != "" {
			textParts = append(textParts, map[string]any{
				"type": partType,
				"text": text,
			})
		}
	}
	if len(textParts) == 0 {
		return nil
	}
	encoded, err := json.Marshal(map[string]any{
		"type":    "message",
		"role":    role,
		"content": textParts,
	})
	if err != nil {
		return nil
	}
	return encoded
}

func normalizeOpenAIWSReplayReasoningItem(item gjson.Result) []byte {
	texts := make([]string, 0, 2)
	summary := item.Get("summary")
	if summary.IsArray() {
		for _, part := range summary.Array() {
			text := strings.TrimSpace(firstNonEmpty(part.Get("text").String(), part.Get("summary_text").String()))
			if text != "" {
				texts = append(texts, text)
			}
		}
	}
	if len(texts) == 0 {
		return nil
	}
	// Reasoning is always re-emitted as an assistant note for input replay.
	encoded, err := json.Marshal(map[string]any{
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{{
			"type": openAIWSReplayContentPartType("assistant"),
			"text": "[reasoning] " + strings.Join(texts, "\n"),
		}},
	})
	if err != nil {
		return nil
	}
	return encoded
}

func buildOpenAIWSHTTPBridgeErrorEvent(statusCode int, message string) []byte {
	message = strings.TrimSpace(message)
	if message == "" {
		message = http.StatusText(statusCode)
	}
	if message == "" {
		message = "upstream request failed"
	}
	event := map[string]any{
		"type":   "error",
		"status": statusCode,
		"error": map[string]any{
			"type":    "upstream_error",
			"message": message,
		},
	}
	body, err := json.Marshal(event)
	if err != nil {
		return []byte(`{"type":"error","error":{"type":"upstream_error","message":"upstream request failed"}}`)
	}
	return body
}

func openAIWSHTTPBridgeUpstreamContext(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	detachedCtx, releaseDetachedCtx := detachUpstreamContext(ctx)
	deadlineCtx, cancelDeadline := context.WithDeadline(detachedCtx, deadline)
	return deadlineCtx, func() {
		cancelDeadline()
		releaseDetachedCtx()
	}
}

func (s *OpenAIGatewayService) proxyOpenAIWSHTTPBridgeTurn(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	payload []byte,
	payloadBytes int,
	originalModel string,
	imageBillingModel string,
	imageSizeTier string,
	imageInputSize string,
	turn int,
	writeClientMessage func([]byte) error,
) (*OpenAIForwardResult, error) {
	if s == nil {
		return nil, errors.New("service is nil")
	}
	if s.httpUpstream == nil {
		return nil, errors.New("openai http upstream is nil")
	}
	if account == nil {
		return nil, errors.New("account is nil")
	}
	if writeClientMessage == nil {
		return nil, errors.New("client websocket writer is nil")
	}

	body, err := prepareOpenAIWSHTTPBridgeBody(payload)
	if err != nil {
		return nil, fmt.Errorf("prepare http bridge body: %w", err)
	}
	// HTTP bridge talks to ChatGPT over HTTP SSE, which does not accept native
	// Codex namespace tools. Reuse the Forward-path flatten/restore so WS clients
	// on http_bridge stay compatible with multi-turn namespace tool calls.
	if account.Type == AccountTypeOAuth {
		body, err = flattenOpenAIResponsesNamespaces(c, body)
		if err != nil {
			return nil, fmt.Errorf("flatten http bridge namespace tools: %w", err)
		}
	}

	turnStart := time.Now()
	maxDuration := openAIWSHTTPBridgeMaxDurationDefault
	streamDeadline := turnStart.Add(maxDuration)
	upstreamCtx, releaseUpstreamCtx := openAIWSHTTPBridgeUpstreamContext(ctx, streamDeadline)
	defer releaseUpstreamCtx()
	upstreamReq, err := s.buildUpstreamRequestOpenAIPassthrough(upstreamCtx, c, account, body, token)
	if err != nil {
		return nil, err
	}
	if account.Platform != PlatformGrok && isOpenAIResponsesLiteWebSocketPayload(payload) {
		upstreamReq.Header.Set(responsesLiteHeader, "true")
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	if c != nil {
		c.Set("openai_passthrough", true)
		c.Set("openai_ws_http_bridge", true)
	}

	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		statusCode := http.StatusBadGateway
		clientMessage := "Upstream request failed"
		if errors.Is(err, context.DeadlineExceeded) {
			statusCode = http.StatusGatewayTimeout
			clientMessage = "Upstream response timed out"
		}
		_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(statusCode, clientMessage))
		return nil, fmt.Errorf("upstream http bridge request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, openAIWSHTTPBridgeErrorBodyLimitBytes))
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if upstreamMsg == "" {
			upstreamMsg = http.StatusText(resp.StatusCode)
		}
		_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(resp.StatusCode, upstreamMsg))
		return nil, fmt.Errorf("upstream http bridge error: status=%d message=%s", resp.StatusCode, upstreamMsg)
	}

	logOpenAIWSModeInfo(
		"ingress_ws_http_bridge_upstream_ok account_id=%d turn=%d status=%d payload_bytes=%d header_ms=%d",
		account.ID,
		turn,
		resp.StatusCode,
		payloadBytes,
		time.Since(turnStart).Milliseconds(),
	)

	responseID := ""
	usage := OpenAIUsage{}
	imageCounter := newOpenAIImageOutputCounter()
	var firstTokenMs *int
	reqStream := openAIWSPayloadBoolFromRaw(body, "stream", true)
	eventCount := 0
	tokenEventCount := 0
	terminalEventCount := 0
	replayCollector := &openAIWSToolCallReplayCollector{}
	firstEventType := ""
	lastEventType := ""
	sawDone := false
	wroteDownstream := false
	clientDisconnected := false
	mappedModel := ""
	needModelReplace := false
	var mappedModelBytes []byte
	if originalModel != "" {
		mappedModel = normalizeOpenAIModelForUpstream(account, account.GetMappedModel(originalModel))
		needModelReplace = mappedModel != "" && mappedModel != originalModel
		if needModelReplace {
			mappedModelBytes = []byte(mappedModel)
		}
	}

	resultWithUsage := func() *OpenAIForwardResult {
		imageCount := imageCounter.Count()
		payloadBytesValue := int64(payloadBytes)
		eventCountValue := eventCount
		result := &OpenAIForwardResult{
			RequestID:       responseID,
			Usage:           usage,
			Model:           originalModel,
			UpstreamModel:   mappedModel,
			ServiceTier:     extractOpenAIServiceTierFromBody(body),
			ReasoningEffort: ApplyThinkingEnabledFallback(extractOpenAIReasoningEffortFromBody(body, firstNonEmpty(mappedModel, originalModel)), body, mappedModel),
			Stream:          reqStream,
			OpenAIWSMode:    true,
			ResponseHeaders: cloneHeader(resp.Header),
			Duration:        time.Since(turnStart),
			FirstTokenMs:    firstTokenMs,
			WSPayloadBytes:  &payloadBytesValue,
			WSEventCount:    &eventCountValue,
		}
		if replayInput := replayCollector.Items(); len(replayInput) > 0 {
			result.wsReplayInput = replayInput
			result.wsReplayInputExists = true
		}
		if imageCount > 0 {
			result.ImageCount = imageCount
			result.ImageSize = imageSizeTier
			result.ImageInputSize = imageInputSize
			result.ImageOutputSizes = imageCounter.Sizes()
			result.BillingModel = imageBillingModel
		}
		return result
	}

	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	// Prevent WS bridge turns from hanging forever when upstream accepts the
	// request but never emits another SSE line.
	if streamInterval <= 0 {
		streamInterval = 90 * time.Second
	}

	type streamLine struct {
		line string
		err  error
		done bool
	}
	// Cancelable reader: early returns (idle/max-duration/max-token) must stop the
	// scanner goroutine. Otherwise it can block forever on a full channel after
	// resp.Body is closed by defer, leaking the goroutine and scanner buffer.
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	lines := make(chan streamLine, 16)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(resp.Body)
		scanBuf := getSSEScannerBuf64K()
		scanner.Buffer(scanBuf[:0], maxLineSize)
		defer putSSEScannerBuf64K(scanBuf)
		send := func(item streamLine) bool {
			select {
			case lines <- item:
				return true
			case <-readCtx.Done():
				return false
			}
		}
		for scanner.Scan() {
			if !send(streamLine{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = send(streamLine{err: err, done: true})
			return
		}
		_ = send(streamLine{done: true})
	}()

	maxTokenEvents := openAIWSHTTPBridgeMaxTokenEventsDefault

	// Reuse one timer instead of time.After in the hot loop to avoid leaking
	// timers under high event rates.
	var waitTimer *time.Timer
	stopWaitTimer := func() {
		if waitTimer == nil {
			return
		}
		if !waitTimer.Stop() {
			select {
			case <-waitTimer.C:
			default:
			}
		}
	}
	defer stopWaitTimer()
	armWaitTimer := func(d time.Duration) <-chan time.Time {
		if d <= 0 {
			d = time.Millisecond
		}
		if waitTimer == nil {
			waitTimer = time.NewTimer(d)
			return waitTimer.C
		}
		stopWaitTimer()
		waitTimer.Reset(d)
		return waitTimer.C
	}
	abortMaxDuration := func() (*OpenAIForwardResult, error) {
		msg := fmt.Sprintf("upstream stream exceeded max duration %s", maxDuration)
		_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(http.StatusGatewayTimeout, msg))
		logOpenAIWSModeInfo(
			"ingress_ws_http_bridge_stream_max_duration account_id=%d turn=%d max_duration=%s events=%d token_events=%d first_event=%s last_event=%s",
			account.ID,
			turn,
			maxDuration.String(),
			eventCount,
			tokenEventCount,
			truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
			truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
		)
		return resultWithUsage(), errors.New(msg)
	}

	lastRead := time.Now()
	for {
		var item streamLine
		wait := streamInterval
		if remaining := time.Until(streamDeadline); remaining > 0 && (wait <= 0 || remaining < wait) {
			wait = remaining
		}
		if wait <= 0 {
			return abortMaxDuration()
		}
		select {
		case item = <-lines:
		case <-armWaitTimer(wait):
			if time.Now().After(streamDeadline) {
				return abortMaxDuration()
			}
			if streamInterval > 0 && time.Since(lastRead) >= streamInterval {
				msg := fmt.Sprintf("upstream stream idle for %s", streamInterval)
				_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(http.StatusGatewayTimeout, msg))
				logOpenAIWSModeInfo(
					"ingress_ws_http_bridge_stream_idle_timeout account_id=%d turn=%d interval=%s events=%d first_event=%s last_event=%s",
					account.ID,
					turn,
					streamInterval.String(),
					eventCount,
					truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
				)
				return resultWithUsage(), errors.New(msg)
			}
			continue
		case <-ctx.Done():
			return resultWithUsage(), ctx.Err()
		}
		if item.err != nil {
			return resultWithUsage(), fmt.Errorf("read upstream http bridge stream: %w", item.err)
		}
		if item.done && item.line == "" {
			break
		}
		lastRead = time.Now()

		data, ok := extractOpenAISSEDataLine(item.line)
		if !ok {
			continue
		}
		trimmedData := strings.TrimSpace(data)
		if trimmedData == "" {
			continue
		}
		if trimmedData == "[DONE]" {
			sawDone = true
			continue
		}

		upstreamMessage := []byte(trimmedData)
		eventType, eventResponseID, _ := parseOpenAIWSEventEnvelope(upstreamMessage)
		if responseID == "" && eventResponseID != "" {
			responseID = eventResponseID
		}
		if eventType != "" {
			eventCount++
			if firstEventType == "" {
				firstEventType = eventType
			}
			lastEventType = eventType
		}
		if isOpenAIWSTokenEvent(eventType) {
			tokenEventCount++
			if firstTokenMs == nil {
				ms := int(time.Since(turnStart).Milliseconds())
				firstTokenMs = &ms
			}
			if tokenEventCount > maxTokenEvents {
				msg := fmt.Sprintf("upstream stream exceeded max token events %d", maxTokenEvents)
				_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(http.StatusGatewayTimeout, msg))
				logOpenAIWSModeInfo(
					"ingress_ws_http_bridge_stream_max_token_events account_id=%d turn=%d max_token_events=%d events=%d token_events=%d first_event=%s last_event=%s",
					account.ID,
					turn,
					maxTokenEvents,
					eventCount,
					tokenEventCount,
					truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
				)
				return resultWithUsage(), errors.New(msg)
			}
		}
		if time.Now().After(streamDeadline) {
			msg := fmt.Sprintf("upstream stream exceeded max duration %s", maxDuration)
			_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(http.StatusGatewayTimeout, msg))
			logOpenAIWSModeInfo(
				"ingress_ws_http_bridge_stream_max_duration account_id=%d turn=%d max_duration=%s events=%d token_events=%d first_event=%s last_event=%s",
				account.ID,
				turn,
				maxDuration.String(),
				eventCount,
				tokenEventCount,
				truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
				truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
			)
			return resultWithUsage(), errors.New(msg)
		}
		if openAIWSEventShouldParseUsage(eventType) {
			parseOpenAIWSResponseUsageFromCompletedEvent(upstreamMessage, &usage)
		}
		imageCounter.AddSSEData(upstreamMessage)

		if needModelReplace && len(mappedModelBytes) > 0 && openAIWSEventMayContainModel(eventType) && strings.Contains(trimmedData, mappedModel) {
			upstreamMessage = replaceOpenAIWSMessageModel(upstreamMessage, mappedModel, originalModel)
		}
		if s.toolCorrector != nil && openAIWSEventMayContainToolCalls(eventType) && openAIWSMessageLikelyContainsToolCalls(upstreamMessage) {
			if corrected, changed := s.toolCorrector.CorrectToolCallsInSSEBytes(upstreamMessage); changed {
				upstreamMessage = corrected
			}
		}
		replayCollector.AddEvent(eventType, upstreamMessage)

		restoredMessage, restoreErr := restoreOpenAIResponsesNamespacePayload(c, upstreamMessage)
		if restoreErr != nil {
			return resultWithUsage(), fmt.Errorf("restore http bridge namespace response: %w", restoreErr)
		}
		upstreamMessage = restoredMessage

		if !clientDisconnected {
			if err := writeClientMessage(upstreamMessage); err != nil {
				if isOpenAIWSClientDisconnectError(err) {
					clientDisconnected = true
					closeStatus, closeReason := summarizeOpenAIWSReadCloseError(err)
					logOpenAIWSModeInfo(
						"ingress_ws_http_bridge_client_disconnected_drain account_id=%d turn=%d close_status=%s close_reason=%s",
						account.ID,
						turn,
						closeStatus,
						truncateOpenAIWSLogValue(closeReason, openAIWSHeaderValueMaxLen),
					)
				} else {
					return nil, wrapOpenAIWSIngressTurnError(
						"write_client",
						fmt.Errorf("write client websocket event: %w", err),
						wroteDownstream,
					)
				}
			} else {
				wroteDownstream = true
			}
		}

		if eventType == "error" {
			errCodeRaw, errTypeRaw, errMsgRaw := parseOpenAIWSErrorEventFields(upstreamMessage)
			s.persistOpenAIWSRateLimitSignal(ctx, account, resp.Header, upstreamMessage, errCodeRaw, errTypeRaw, errMsgRaw)
			errMessage := strings.TrimSpace(errMsgRaw)
			if errMessage == "" {
				errMessage = "upstream error event"
			}
			return resultWithUsage(), errors.New(errMessage)
		}
		if isOpenAIWSTerminalEvent(eventType) {
			terminalEventCount++
			firstTokenMsValue := -1
			if firstTokenMs != nil {
				firstTokenMsValue = *firstTokenMs
			}
			logOpenAIWSModeInfo(
				"ingress_ws_http_bridge_turn_completed account_id=%d turn=%d response_id=%s payload_bytes=%d duration_ms=%d events=%d token_events=%d terminal_events=%d first_event=%s last_event=%s first_token_ms=%d client_disconnected=%v",
				account.ID,
				turn,
				truncateOpenAIWSLogValue(responseID, openAIWSIDValueMaxLen),
				payloadBytes,
				time.Since(turnStart).Milliseconds(),
				eventCount,
				tokenEventCount,
				terminalEventCount,
				truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
				truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
				firstTokenMsValue,
				clientDisconnected,
			)
			return resultWithUsage(), nil
		}
	}
	if sawDone && eventCount > 0 {
		return resultWithUsage(), nil
	}
	return resultWithUsage(), errors.New("upstream http bridge stream ended before terminal event")
}
