package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// cursorWireFormat selects the client-facing protocol a cursor turn renders to.
type cursorWireFormat int

const (
	cursorWireResponses cursorWireFormat = iota
	cursorWireAnthropic
)

// cursorCompatSink renders canonical chat chunks into Responses SSE or
// Anthropic SSE on the client connection. Both formats share the
// ChatCompletions→Responses stream converter; Anthropic adds a second hop
// through the Responses→Anthropic event converter.
type cursorCompatSink struct {
	c            *gin.Context
	format       cursorWireFormat
	respState    *apicompat.ChatCompletionsToResponsesStreamState
	anthState    *apicompat.ResponsesEventToAnthropicState
	disconnected bool
}

func newCursorCompatSink(c *gin.Context, format cursorWireFormat, model string) *cursorCompatSink {
	s := &cursorCompatSink{
		c:         c,
		format:    format,
		respState: apicompat.NewChatCompletionsToResponsesStreamState(model),
	}
	if format == cursorWireAnthropic {
		s.anthState = apicompat.NewResponsesEventToAnthropicState()
		s.anthState.Model = model
	}
	return s
}

// responseID returns the Responses id synthesized for this turn ("resp_*").
func (s *cursorCompatSink) responseID() string {
	if s == nil || s.respState == nil {
		return ""
	}
	return s.respState.ResponseID
}

func (s *cursorCompatSink) emit(chunk *apicompat.ChatCompletionsChunk) {
	if s.disconnected || chunk == nil {
		return
	}
	s.writeResponsesEvents(apicompat.ChatCompletionsChunkToResponsesEvents(chunk, s.respState))
}

func (s *cursorCompatSink) finish() {
	s.writeResponsesEvents(apicompat.FinalizeChatCompletionsResponsesStream(s.respState))
	if s.format == cursorWireAnthropic && s.anthState != nil {
		s.writeAnthropicEvents(apicompat.FinalizeResponsesAnthropicStream(s.anthState))
	}
	if !s.disconnected {
		s.c.Writer.Flush()
	}
}

func (s *cursorCompatSink) writeResponsesEvents(events []apicompat.ResponsesStreamEvent) {
	for i := range events {
		if s.format == cursorWireAnthropic {
			s.writeAnthropicEvents(apicompat.ResponsesEventToAnthropicEvents(&events[i], s.anthState))
			continue
		}
		sse, err := apicompat.ResponsesEventToSSE(events[i])
		if err != nil {
			continue
		}
		s.write(sse)
	}
	if len(events) > 0 && !s.disconnected {
		s.c.Writer.Flush()
	}
}

func (s *cursorCompatSink) writeAnthropicEvents(events []apicompat.AnthropicStreamEvent) {
	for _, evt := range events {
		sse, err := apicompat.ResponsesAnthropicEventToSSE(evt)
		if err != nil {
			continue
		}
		s.write(sse)
	}
}

func (s *cursorCompatSink) write(sse string) {
	if s.disconnected {
		return
	}
	if _, err := fmt.Fprint(s.c.Writer, sse); err != nil {
		s.disconnected = true
		logger.L().Info("cursor compat stream: client disconnected, continuing to drain upstream for billing",
			zap.Error(err),
			zap.String("response_id", s.responseID()),
		)
	}
}

// writeCursorCompatSSEHeaders sets the SSE response headers for compat formats.
func writeCursorCompatSSEHeaders(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()
}

// forwardCursorResponses implements POST /v1/responses for cursor accounts.
// Cursor upstreams expose no OpenAI Responses endpoint — the sandbox Grok-Bot
// gateway (sendPrompt) is the only live transport for these session tokens, and
// InferenceService rejects them (401). The request is converted into a
// chat-shaped gateway turn and the resulting deltas are re-emitted as
// Responses SSE events without a Chat Completions wire round-trip.
func (s *OpenAIGatewayService) forwardCursorResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	originalModel string,
	reqStream bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	var resReq apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &resReq); err != nil {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "invalid responses request: "+err.Error())
		return nil, err
	}
	if originalModel == "" {
		originalModel = resReq.Model
	}
	return s.forwardCursorViaResponses(ctx, c, account, &resReq, cursorWireResponses, originalModel, "", reqStream, startTime)
}

// forwardCursorAnthropic implements POST /v1/messages for cursor accounts:
// Anthropic → Responses → gateway turn → Anthropic SSE events.
func (s *OpenAIGatewayService) forwardCursorAnthropic(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	var anthReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthReq); err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "invalid anthropic request: "+err.Error())
		return nil, err
	}
	resReq, err := apicompat.AnthropicToResponses(&anthReq)
	if err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "failed to convert anthropic request: "+err.Error())
		return nil, err
	}
	return s.forwardCursorViaResponses(ctx, c, account, resReq, cursorWireAnthropic, anthReq.Model, defaultMappedModel, resReq.Stream, startTime)
}

// forwardCursorViaResponses is the shared core for the /v1/responses and
// /v1/messages cursor paths: a Responses request is converted to a chat-shaped
// gateway turn, then rendered back in the caller's wire format.
func (s *OpenAIGatewayService) forwardCursorViaResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	resReq *apicompat.ResponsesRequest,
	format cursorWireFormat,
	originalModel string,
	defaultMappedModel string,
	clientStream bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	writeErr := func(status int, errType, msg string) {
		if format == cursorWireAnthropic {
			writeAnthropicError(c, status, errType, msg)
			return
		}
		writeChatCompletionsError(c, status, errType, msg)
	}

	if strings.TrimSpace(originalModel) == "" {
		writeErr(http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, errors.New("missing model in request")
	}

	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)

	ccReq, err := apicompat.ResponsesToChatCompletionsRequest(resReq)
	if err != nil {
		writeErr(http.StatusBadRequest, "invalid_request_error", "failed to convert request: "+err.Error())
		return nil, err
	}

	// Bridge through the OpenAI JSON shape into the cursor request type; both
	// follow the same field names so no fields are lost in this hop.
	raw, err := json.Marshal(ccReq)
	if err != nil {
		writeErr(http.StatusBadRequest, "invalid_request_error", "failed to convert request: "+err.Error())
		return nil, err
	}
	var chatReq cursor.OpenAIChatRequest
	if err := json.Unmarshal(raw, &chatReq); err != nil {
		writeErr(http.StatusBadRequest, "invalid_request_error", "failed to convert request: "+err.Error())
		return nil, err
	}
	chatReq.Model = upstreamModel
	chatReq.Stream = clientStream

	var reasoningEffort *string
	if resReq.Reasoning != nil {
		if effort := strings.TrimSpace(resReq.Reasoning.Effort); effort != "" {
			reasoningEffort = &effort
		}
	}
	var serviceTier *string
	if tier := normalizedOpenAIServiceTierValue(resReq.ServiceTier); tier != "" {
		serviceTier = &tier
	}

	cursorClient, token, err := s.cursorClientForAccount(ctx, account, writeErr)
	if err != nil {
		return nil, err
	}

	// Explicit inference transport keeps the legacy Connect-RPC path.
	transport := strings.ToLower(strings.TrimSpace(account.GetCredential("cursor_transport")))
	if transport == "inference" {
		return s.forwardCursorViaInferenceCompat(
			ctx, c, account, cursorClient, &chatReq, format,
			originalModel, billingModel, upstreamModel,
			reasoningEffort, serviceTier, clientStream, startTime, writeErr,
		)
	}

	conversationID := firstNonEmpty(
		strings.TrimSpace(resReq.PromptCacheKey),
		strings.TrimSpace(c.GetHeader("conversation_id")),
	)

	gwReq := &cursor.GatewayChatRequest{
		Messages:       chatReq.Messages,
		Model:          upstreamModel,
		ConversationID: conversationID,
		Tools:          chatReq.Tools,
		Stream:         clientStream,
	}

	deltas, result, err := s.runCursorGatewayTurn(ctx, c, account, cursorClient, token, gwReq, writeErr)
	if err != nil {
		return nil, err
	}

	if clientStream {
		return s.streamCursorGatewayCompat(
			ctx, c, deltas, result, account, format,
			originalModel, billingModel, upstreamModel,
			reasoningEffort, serviceTier, startTime,
		)
	}
	return s.bufferCursorGatewayCompat(
		ctx, c, deltas, result, account, format,
		originalModel, billingModel, upstreamModel,
		reasoningEffort, serviceTier, startTime,
	)
}

// streamCursorGatewayCompat drains a gateway turn and renders it as
// Responses or Anthropic SSE to the client.
func (s *OpenAIGatewayService) streamCursorGatewayCompat(
	ctx context.Context,
	c *gin.Context,
	deltas <-chan cursor.ChatDelta,
	result *cursor.GatewayChatResult,
	account *Account,
	format cursorWireFormat,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	writeCursorCompatSSEHeaders(c)

	// Compat formats let the Responses converter generate the "resp_*" id, so
	// chunks carry no chatcmpl id.
	sink := newCursorCompatSink(c, format, originalModel)
	usage, firstTokenMs := drainCursorGatewayDeltas(deltas, result, "", time.Now().Unix(), originalModel, startTime, sink.emit)
	sink.finish()

	_ = ctx
	_ = account

	return &OpenAIForwardResult{
		RequestID:        sink.responseID(),
		ResponseID:       sink.responseID(),
		BillingRequestID: uuid.NewString(),
		Model:            originalModel,
		BillingModel:     billingModel,
		UpstreamModel:    upstreamModel,
		Usage:            usage,
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ReasoningEffort:  reasoningEffort,
		ServiceTier:      serviceTier,
		ClientDisconnect: sink.disconnected,
		UpstreamEndpoint: cursorSandboxGatewayEndpoint,
	}, nil
}

// bufferCursorGatewayCompat drains a gateway turn into the Responses stream
// state, then writes the terminal response.completed payload (or its Anthropic
// conversion) as a single JSON document.
func (s *OpenAIGatewayService) bufferCursorGatewayCompat(
	ctx context.Context,
	c *gin.Context,
	deltas <-chan cursor.ChatDelta,
	result *cursor.GatewayChatResult,
	account *Account,
	format cursorWireFormat,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	writeErr := func(msg string) {
		if format == cursorWireAnthropic {
			writeAnthropicError(c, http.StatusBadGateway, "api_error", msg)
			return
		}
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", msg)
	}

	state := apicompat.NewChatCompletionsToResponsesStreamState(originalModel)
	usage, firstTokenMs := drainCursorGatewayDeltas(deltas, result, "", time.Now().Unix(), originalModel, startTime, func(chunk *apicompat.ChatCompletionsChunk) {
		apicompat.ChatCompletionsChunkToResponsesEvents(chunk, state)
	})

	var finalResp *apicompat.ResponsesResponse
	for _, evt := range apicompat.FinalizeChatCompletionsResponsesStream(state) {
		if evt.Type == "response.completed" && evt.Response != nil {
			finalResp = evt.Response
		}
	}
	if finalResp == nil {
		writeErr("Upstream stream ended without a terminal response event")
		return nil, errors.New("upstream stream ended without terminal event")
	}

	if format == cursorWireAnthropic {
		c.JSON(http.StatusOK, apicompat.ResponsesToAnthropic(finalResp, originalModel))
	} else {
		c.JSON(http.StatusOK, finalResp)
	}

	_ = ctx
	_ = account

	return &OpenAIForwardResult{
		RequestID:        state.ResponseID,
		ResponseID:       state.ResponseID,
		BillingRequestID: uuid.NewString(),
		Model:            originalModel,
		BillingModel:     billingModel,
		UpstreamModel:    upstreamModel,
		Usage:            usage,
		Stream:           false,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ReasoningEffort:  reasoningEffort,
		ServiceTier:      serviceTier,
		UpstreamEndpoint: cursorSandboxGatewayEndpoint,
	}, nil
}

// startCursorInferenceStream opens the legacy Connect-RPC InferenceService
// stream, retrying once with a force-refreshed token on 401.
func (s *OpenAIGatewayService) startCursorInferenceStream(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	cursorClient *cursor.Client,
	chatReq *cursor.OpenAIChatRequest,
	writeErr func(status int, errType, msg string),
) (*cursor.StreamReader, error) {
	protoReq, err := cursor.MapOpenAIRequestToProto(chatReq)
	if err != nil {
		writeErr(http.StatusBadRequest, "invalid_request_error", "failed to map request: "+err.Error())
		return nil, fmt.Errorf("map request to cursor proto: %w", err)
	}

	stream, err := cursorClient.StreamInference(ctx, protoReq)
	if err != nil {
		var upErr *cursor.UpstreamError
		if errors.As(err, &upErr) {
			if upErr.StatusCode == http.StatusUnauthorized && account.Type == AccountTypeOAuth && s.cursorTokenProvider != nil {
				logger.L().Info("cursor inference returned 401, attempting token refresh",
					zap.Int64("account_id", account.ID),
				)
				if refreshedToken, refErr := s.cursorTokenProvider.ForceRefresh(ctx, account); refErr == nil && refreshedToken != "" {
					cursorClient.SetToken(refreshedToken)
					stream, err = cursorClient.StreamInference(ctx, protoReq)
				}
			}
		}
	}
	if err != nil {
		var upErr *cursor.UpstreamError
		if errors.As(err, &upErr) {
			if s.shouldFailoverUpstreamError(upErr.StatusCode) {
				return nil, newOpenAIUpstreamFailoverError(
					upErr.StatusCode,
					upErr.Headers,
					[]byte(upErr.Body),
					upErr.Error(),
					false,
				)
			}
			writeErr(upErr.StatusCode, "upstream_error", upErr.Body)
			return nil, err
		}
		writeErr(http.StatusBadGateway, "upstream_error", err.Error())
		return nil, err
	}
	return stream, nil
}

// forwardCursorViaInferenceCompat serves Responses/Anthropic requests over the
// legacy InferenceService transport, converting proto frames to canonical chat
// chunks then to the target wire format.
func (s *OpenAIGatewayService) forwardCursorViaInferenceCompat(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	cursorClient *cursor.Client,
	chatReq *cursor.OpenAIChatRequest,
	format cursorWireFormat,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	clientStream bool,
	startTime time.Time,
	writeErr func(status int, errType, msg string),
) (*OpenAIForwardResult, error) {
	stream, err := s.startCursorInferenceStream(ctx, c, account, cursorClient, chatReq, writeErr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()

	createdTimestamp := time.Now().Unix()
	// The upstream-supplied response id is captured here but compat formats
	// present the converter-generated resp_* id to the client.
	upstreamRespID := ""
	sink := newCursorCompatSink(c, format, originalModel)

	var usage OpenAIUsage
	var firstTokenMs *int
	if clientStream {
		writeCursorCompatSSEHeaders(c)
		usage, firstTokenMs, err = drainCursorInferenceStream(stream, &upstreamRespID, createdTimestamp, originalModel, startTime, sink.emit)
	} else {
		usage, firstTokenMs, err = drainCursorInferenceStream(stream, &upstreamRespID, createdTimestamp, originalModel, startTime, func(chunk *apicompat.ChatCompletionsChunk) {
			apicompat.ChatCompletionsChunkToResponsesEvents(chunk, sink.respState)
		})
	}
	if err != nil {
		var upErr *cursor.UpstreamError
		if errors.As(err, &upErr) && s.shouldFailoverUpstreamError(upErr.StatusCode) && firstTokenMs == nil {
			return nil, newOpenAIUpstreamFailoverError(
				upErr.StatusCode,
				upErr.Headers,
				[]byte(upErr.Body),
				upErr.Error(),
				false,
			)
		}
		if !sink.disconnected {
			logger.L().Warn("cursor stream read error", zap.Error(err))
		}
	}

	// Terminal finish + usage chunks keep the converter's output/usage complete.
	finishReason := "stop"
	if len(sink.respState.ToolCalls) > 0 {
		finishReason = "tool_calls"
	}
	finishAndUsage := func(chunk *apicompat.ChatCompletionsChunk) {
		if clientStream {
			sink.emit(chunk)
			return
		}
		apicompat.ChatCompletionsChunkToResponsesEvents(chunk, sink.respState)
	}
	finishAndUsage(&apicompat.ChatCompletionsChunk{
		Object:  "chat.completion.chunk",
		Created: createdTimestamp,
		Model:   originalModel,
		Choices: []apicompat.ChatChunkChoice{{
			Index:        0,
			Delta:        apicompat.ChatDelta{},
			FinishReason: &finishReason,
		}},
	})
	finishAndUsage(&apicompat.ChatCompletionsChunk{
		Object:  "chat.completion.chunk",
		Created: createdTimestamp,
		Model:   originalModel,
		Choices: []apicompat.ChatChunkChoice{},
		Usage: &apicompat.ChatUsage{
			PromptTokens:     usage.InputTokens,
			CompletionTokens: usage.OutputTokens,
			TotalTokens:      usage.InputTokens + usage.OutputTokens,
			PromptTokensDetails: &apicompat.ChatTokenDetails{
				CachedTokens: usage.CacheReadInputTokens,
			},
		},
	})

	if clientStream {
		sink.finish()
		return &OpenAIForwardResult{
			RequestID:        sink.responseID(),
			ResponseID:       sink.responseID(),
			BillingRequestID: uuid.NewString(),
			Model:            originalModel,
			BillingModel:     billingModel,
			UpstreamModel:    upstreamModel,
			Usage:            usage,
			Stream:           true,
			Duration:         time.Since(startTime),
			FirstTokenMs:     firstTokenMs,
			ReasoningEffort:  reasoningEffort,
			ServiceTier:      serviceTier,
			ClientDisconnect: sink.disconnected,
			UpstreamEndpoint: cursorInferenceStreamEndpoint,
		}, nil
	}

	var finalResp *apicompat.ResponsesResponse
	for _, evt := range apicompat.FinalizeChatCompletionsResponsesStream(sink.respState) {
		if evt.Type == "response.completed" && evt.Response != nil {
			finalResp = evt.Response
		}
	}
	if finalResp == nil {
		if format == cursorWireAnthropic {
			writeAnthropicError(c, http.StatusBadGateway, "api_error", "Upstream stream ended without a terminal response event")
		} else {
			writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream stream ended without a terminal response event")
		}
		return nil, errors.New("upstream stream ended without terminal event")
	}
	if format == cursorWireAnthropic {
		c.JSON(http.StatusOK, apicompat.ResponsesToAnthropic(finalResp, originalModel))
	} else {
		c.JSON(http.StatusOK, finalResp)
	}

	return &OpenAIForwardResult{
		RequestID:        sink.responseID(),
		ResponseID:       sink.responseID(),
		BillingRequestID: uuid.NewString(),
		Model:            originalModel,
		BillingModel:     billingModel,
		UpstreamModel:    upstreamModel,
		Usage:            usage,
		Stream:           false,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ReasoningEffort:  reasoningEffort,
		ServiceTier:      serviceTier,
		UpstreamEndpoint: cursorInferenceStreamEndpoint,
	}, nil
}
