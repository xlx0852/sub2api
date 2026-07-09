package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const defaultOpenAICompactV2KeepaliveInterval = 10 * time.Second

// prepareOpenAIBodySignalV2UpstreamBody converts a Codex remote compaction v2
// /responses body into a legacy /responses/compact JSON body:
// - keeps compact schema fields
// - strips stream/store/prompt_cache_key
// - removes compaction_trigger input items
func prepareOpenAIBodySignalV2UpstreamBody(body []byte) ([]byte, error) {
	normalized, _, err := normalizeOpenAICompactRequestBody(body)
	if err != nil {
		return nil, err
	}
	stripped, err := stripCompactionTriggerFromOpenAIInput(normalized)
	if err != nil {
		return nil, err
	}
	return stripped, nil
}

func stripCompactionTriggerFromOpenAIInput(body []byte) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || !input.IsArray() {
		return body, nil
	}
	kept := make([]json.RawMessage, 0, len(input.Array()))
	removed := false
	for _, item := range input.Array() {
		if item.Get("type").String() == "compaction_trigger" {
			removed = true
			continue
		}
		kept = append(kept, json.RawMessage(item.Raw))
	}
	if !removed {
		return body, nil
	}
	raw, err := json.Marshal(kept)
	if err != nil {
		return nil, fmt.Errorf("marshal compact input without trigger: %w", err)
	}
	next, err := sjson.SetRawBytes(body, "input", raw)
	if err != nil {
		return nil, fmt.Errorf("set compact input without trigger: %w", err)
	}
	return next, nil
}

func (s *OpenAIGatewayService) openAICompactV2KeepaliveInterval() time.Duration {
	if s != nil && s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		return time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}
	return defaultOpenAICompactV2KeepaliveInterval
}

func detachOpenAICompactUpstreamContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.Background(), func() {}
	}
	base := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(base, deadline)
	}
	return base, func() {}
}

// ensureOpenAICompactV2SSEStarted writes Responses SSE headers once for body-signal v2.
func ensureOpenAICompactV2SSEStarted(c *gin.Context, upstreamHeader http.Header, responseHeaderFilter *responseheaders.CompiledHeaderFilter) error {
	if c == nil || c.Writer == nil {
		return errors.New("missing response writer")
	}
	if IsOpenAICompactV2SSEStarted(c) {
		return nil
	}
	if responseHeaderFilter != nil && upstreamHeader != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), upstreamHeader, responseHeaderFilter)
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	if upstreamHeader != nil {
		if v := strings.TrimSpace(upstreamHeader.Get("x-request-id")); v != "" {
			c.Header("x-request-id", v)
		}
	}
	// Commit headers so keepalives can flow before the compact JSON is ready.
	c.Status(http.StatusOK)
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	MarkOpenAICompactV2SSEStarted(c)
	return nil
}

func writeOpenAIResponsesSSEData(c *gin.Context, payload any) error {
	if c == nil || c.Writer == nil {
		return errors.New("missing response writer")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", raw); err != nil {
		return err
	}
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func writeOpenAICompactV2Keepalive(c *gin.Context) error {
	// Codex applies stream_idle_timeout per parsed SSE *event*
	// (timeout(idle_timeout, stream.next()) in codex-rs sse/responses.rs); SSE
	// comment frames are swallowed by the eventsource parser and never reset
	// that timer. Send a well-formed data event with an unknown type instead:
	// codex-rs ignores unhandled kinds (trace-log only), while the event still
	// resets the client idle timer and keeps intermediaries warm.
	if c == nil || c.Writer == nil {
		return errors.New("missing response writer")
	}
	if _, err := io.WriteString(c.Writer, "data: {\"type\":\"keepalive\"}\n\n"); err != nil {
		return err
	}
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

// doOpenAIUpstreamWithCompactV2Keepalive starts the client SSE stream and emits
// keepalives while waiting for httpUpstream.Do. Legacy /responses/compact is
// unary JSON: headers often arrive only after compaction finishes, so keepalives
// must run during Do (not only while draining the body). The request context
// should observe compact soft-timeout (do not detach it).
func (s *OpenAIGatewayService) doOpenAIUpstreamWithCompactV2Keepalive(
	ctx context.Context,
	c *gin.Context,
	req *http.Request,
	proxyURL string,
	account *Account,
) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("missing upstream request")
	}
	if account == nil {
		return nil, errors.New("missing account")
	}
	if err := ensureOpenAICompactV2SSEStarted(c, nil, s.responseHeaderFilter); err != nil {
		return nil, err
	}

	type doResult struct {
		resp *http.Response
		err  error
	}
	done := make(chan doResult, 1)
	go func() {
		resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
		done <- doResult{resp: resp, err: err}
	}()

	keepaliveInterval := s.openAICompactV2KeepaliveInterval()
	var ticker *time.Ticker
	if keepaliveInterval > 0 {
		ticker = time.NewTicker(keepaliveInterval)
		defer ticker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if ticker != nil {
		keepaliveCh = ticker.C
	}

	for {
		select {
		case <-ctx.Done():
			// Soft-timeout / client cancel: drain late Do results so response
			// bodies are not leaked if the request completes after we return.
			go func() {
				select {
				case res := <-done:
					if res.resp != nil && res.resp.Body != nil {
						_ = res.resp.Body.Close()
					}
				case <-time.After(2 * time.Minute):
				}
			}()
			return nil, ctx.Err()
		case res := <-done:
			return res.resp, res.err
		case <-keepaliveCh:
			if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
				continue
			}
			if err := writeOpenAICompactV2Keepalive(c); err != nil {
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] compact v2 keepalive write failed: %v", err)
			}
		}
	}
}

// readOpenAIUpstreamBodyWithCompactV2Keepalive drains the upstream body while
// emitting compact-only SSE keepalives on the client connection.
func (s *OpenAIGatewayService) readOpenAIUpstreamBodyWithCompactV2Keepalive(
	ctx context.Context,
	c *gin.Context,
	resp *http.Response,
) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, errors.New("missing upstream response body")
	}
	if err := ensureOpenAICompactV2SSEStarted(c, resp.Header, s.responseHeaderFilter); err != nil {
		return nil, err
	}

	type readResult struct {
		body []byte
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		// Pass nil as too-large writer: openAITooLargeError writes JSON via
		// c.JSON which would corrupt the already-committed text/event-stream
		// response. The caller handles the error via the compact SSE error path.
		body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, nil)
		done <- readResult{body: body, err: err}
	}()

	keepaliveInterval := s.openAICompactV2KeepaliveInterval()
	var ticker *time.Ticker
	if keepaliveInterval > 0 {
		ticker = time.NewTicker(keepaliveInterval)
		defer ticker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if ticker != nil {
		keepaliveCh = ticker.C
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-done:
			return res.body, res.err
		case <-keepaliveCh:
			if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
				// Client gone: keep draining upstream for billing/usage where possible.
				continue
			}
			if err := writeOpenAICompactV2Keepalive(c); err != nil {
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] compact v2 keepalive write failed: %v", err)
			}
		}
	}
}

func extractOpenAICompactionItemFromLegacyCompactJSON(body []byte) (json.RawMessage, string, error) {
	if len(body) == 0 {
		return nil, "", errors.New("empty compact response")
	}
	responseID := strings.TrimSpace(gjson.GetBytes(body, "id").String())
	if responseID == "" {
		responseID = "compact_" + uuid.NewString()
	}

	output := gjson.GetBytes(body, "output")
	if !output.Exists() || !output.IsArray() {
		return nil, responseID, errors.New("compact response missing output array")
	}

	var fallback json.RawMessage
	sawCompactionType := false
	sawCompactionMissingEncrypted := false
	for _, item := range output.Array() {
		itemType := item.Get("type").String()
		enc := item.Get("encrypted_content")
		hasEncryptedContent := enc.Type == gjson.String && strings.TrimSpace(enc.String()) != ""
		if itemType == "compaction" {
			// Codex's ResponseItem::Compaction requires a non-optional string
			// encrypted_content; items missing it fail serde silently and Codex
			// then fatals with "expected exactly one compaction output item, got 0".
			// Treat such items as invalid instead of forwarding them.
			sawCompactionType = true
			if hasEncryptedContent {
				return json.RawMessage(item.Raw), responseID, nil
			}
			sawCompactionMissingEncrypted = true
			continue
		}
		if hasEncryptedContent {
			fallback = json.RawMessage(item.Raw)
		}
	}
	if len(fallback) > 0 {
		// Normalize fallback into a compaction item when upstream omitted type.
		if gjson.GetBytes(fallback, "type").String() == "" {
			patched, err := sjson.SetBytes(fallback, "type", "compaction")
			if err == nil {
				fallback = patched
			}
		}
		return fallback, responseID, nil
	}
	if sawCompactionMissingEncrypted {
		return nil, responseID, errors.New("compact response compaction item missing encrypted_content")
	}
	if sawCompactionType {
		return nil, responseID, errors.New("compact response compaction item invalid")
	}
	return nil, responseID, errors.New("compact response missing compaction output item")
}

func buildOpenAICompactV2CompletedEvent(responseID, model string, usage *OpenAIUsage) map[string]any {
	if strings.TrimSpace(responseID) == "" {
		responseID = "compact_" + uuid.NewString()
	}
	response := map[string]any{
		"id":     responseID,
		"object": "response",
		"status": "completed",
		"output": []any{},
	}
	if strings.TrimSpace(model) != "" {
		response["model"] = model
	}
	if usage != nil {
		response["usage"] = map[string]any{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
			"total_tokens":  usage.InputTokens + usage.OutputTokens,
		}
	}
	return map[string]any{
		"type":     "response.completed",
		"response": response,
	}
}

// writeOpenAIBodySignalV2BridgeFromLegacyJSON converts legacy /responses/compact
// JSON into the Codex remote compaction v2 SSE terminal sequence.
func (s *OpenAIGatewayService) writeOpenAIBodySignalV2BridgeFromLegacyJSON(
	c *gin.Context,
	resp *http.Response,
	body []byte,
	clientModel string,
) (responseID string, usage *OpenAIUsage, err error) {
	if err := ensureOpenAICompactV2SSEStarted(c, nilHeaderOr(resp), s.responseHeaderFilter); err != nil {
		return "", nil, err
	}
	MarkOpenAICompactBridgeUsed(c)

	compactionItem, responseID, err := extractOpenAICompactionItemFromLegacyCompactJSON(body)
	if err != nil {
		// Terminal response.failed is written by the caller via
		// writeOpenAICompactV2StreamError (single write site).
		return responseID, nil, err
	}

	usageVal, usageOK := extractOpenAIUsageFromJSONBytes(body)
	if usageOK {
		usage = &usageVal
	} else {
		usage = &OpenAIUsage{}
	}

	// Codex collect_compaction_output requires exactly one compaction OutputItemDone
	// followed by Completed.
	if err := writeOpenAIResponsesSSEData(c, map[string]any{
		"type": "response.output_item.done",
		"item": json.RawMessage(compactionItem),
	}); err != nil {
		return responseID, usage, err
	}

	model := strings.TrimSpace(clientModel)
	if model == "" {
		model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	}
	if err := writeOpenAIResponsesSSEData(c, buildOpenAICompactV2CompletedEvent(responseID, model, usage)); err != nil {
		return responseID, usage, err
	}

	MarkOpenAICompactTerminalCommitted(c)
	return responseID, usage, nil
}

func nilHeaderOr(resp *http.Response) http.Header {
	if resp == nil {
		return nil
	}
	return resp.Header
}

// handleOpenAIBodySignalV2LegacyCompactResponse reads legacy compact JSON (with
// keepalives) and bridges it to the v2 SSE contract.
func (s *OpenAIGatewayService) handleOpenAIBodySignalV2LegacyCompactResponse(
	ctx context.Context,
	c *gin.Context,
	resp *http.Response,
	clientModel string,
) (responseID string, usage *OpenAIUsage, err error) {
	body, err := s.readOpenAIUpstreamBodyWithCompactV2Keepalive(ctx, c, resp)
	if err != nil {
		return "", nil, err
	}
	if resp != nil && resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("upstream compact status %d", resp.StatusCode)
	}
	// Detect accidental SSE from upstream and reduce to final JSON when possible.
	if bodyHasSSEFraming(body) || (resp != nil && isEventStreamResponse(resp.Header)) {
		if finalResponse, ok := extractCodexFinalResponse(string(body)); ok {
			body = finalResponse
		}
	}
	return s.writeOpenAIBodySignalV2BridgeFromLegacyJSON(c, resp, body, clientModel)
}

// writeOpenAICompactV2FailedEvent emits the Responses terminal failure event.
// Codex's SSE parser has no handler for a bare {"type":"error"} event; only the
// response.failed family terminates the stream with a surfaced error.
func writeOpenAICompactV2FailedEvent(c *gin.Context, code, message string) error {
	return writeOpenAIResponsesSSEData(c, map[string]any{
		"type": "response.failed",
		"response": map[string]any{
			"id":     "compact_" + uuid.NewString(),
			"object": "response",
			"status": "failed",
			"output": []any{},
			"error": map[string]any{
				"code":    code,
				"message": message,
			},
		},
	})
}

// writeOpenAICompactV2StreamError best-effort notifies the client on a started SSE stream.
func writeOpenAICompactV2StreamError(c *gin.Context, message string) {
	if c == nil || !IsOpenAICompactV2SSEStarted(c) || IsOpenAICompactTerminalCommitted(c) {
		return
	}
	if strings.TrimSpace(message) == "" {
		message = "compact upstream failed"
	}
	if err := writeOpenAICompactV2FailedEvent(c, "compact_v2_failed", message); err != nil {
		return
	}
	// response.failed is terminal for Codex: block soft-timeout retry and stop the
	// handler's generic SSE fallback from appending duplicate terminal events.
	MarkOpenAICompactTerminalCommitted(c)
	MarkResponseCommitted(c)
}

// handleOpenAICompactV2UpstreamErrorSSE mirrors handleErrorResponse for body-signal
// v2 requests whose downstream SSE stream has already started (keepalives from a
// previous soft-timeout attempt committed 200 text/event-stream headers): JSON
// error bodies would corrupt the stream, so terminate with response.failed instead.
func (s *OpenAIGatewayService) handleOpenAICompactV2UpstreamErrorSSE(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	respBody []byte,
	reqModel string,
) error {
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(respBody), maxBytes)
	}

	// cyber_policy 硬阻断：标记供 handler 风控/邮件使用，不冷却账号，直接终止流。
	if hit, code, cyberMsg := detectOpenAICyberPolicy(respBody); hit {
		MarkOpsCyberPolicy(c, CyberPolicyMark{
			Code:           code,
			Message:        cyberMsg,
			Body:           truncateString(string(respBody), 4096),
			UpstreamStatus: resp.StatusCode,
		})
		setOpsUpstreamError(c, resp.StatusCode, cyberMsg, truncateString(string(respBody), 2048))
		msg := cyberMsg
		if msg == "" {
			msg = "Upstream request blocked by policy"
		}
		writeOpenAICompactV2StreamError(c, msg)
		if cyberMsg == "" {
			return fmt.Errorf("openai cyber_policy: %d", resp.StatusCode)
		}
		return fmt.Errorf("openai cyber_policy: %s", cyberMsg)
	}

	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  resp.Header.Get("x-request-id"),
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
		writeOpenAICompactV2StreamError(c, "Upstream gateway error")
		return fmt.Errorf("upstream error: %d (not in custom error codes)", resp.StatusCode)
	}

	shouldDisable := s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, reqModel)
	kind := "http_error"
	if shouldDisable {
		kind = "failover"
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	if shouldDisable {
		// Keepalive frames are non-terminal, so the handler may still switch to
		// another account safely (CanRetryOpenAICompactAfterForwardError).
		return &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           respBody,
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	errMsg := "Upstream request failed"
	switch resp.StatusCode {
	case 401:
		errMsg = "Upstream authentication failed, please contact administrator"
	case 402:
		errMsg = "Upstream payment required: insufficient balance or billing issue"
	case 403:
		errMsg = "Upstream access forbidden, please contact administrator"
	case 429:
		errMsg = "Upstream rate limit exceeded, please retry later"
	}
	if isOpenAIContextWindowError(upstreamMsg, respBody) && upstreamMsg != "" {
		errMsg = upstreamMsg
	}
	writeOpenAICompactV2StreamError(c, errMsg)
	if upstreamMsg == "" {
		return fmt.Errorf("upstream error: %d", resp.StatusCode)
	}
	return fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
}
