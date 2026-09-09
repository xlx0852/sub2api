package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	cursorInferenceStreamEndpoint = "/aiserver.v1.InferenceService/Stream"
	cursorSandboxGatewayEndpoint  = "sand-gateway/sendPrompt"
)

// CursorAccessTokenProvider handles token caching and refresh for Cursor accounts.
type CursorAccessTokenProvider interface {
	GetAccessToken(ctx context.Context, account *Account) (string, error)
	ForceRefresh(ctx context.Context, account *Account) (string, error)
}

// forwardCursorChatCompletions translates an incoming OpenAI Chat Completions
// request into Cursor Sandbox Gateway chat (EnsureSandBox → sendPrompt →
// /events SSE or transcript poll), with optional InferenceService fallback
// when credentials explicitly request it.
func (s *OpenAIGatewayService) forwardCursorChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	originalModel := gjson.GetBytes(body, "model").String()
	if originalModel == "" {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, errors.New("missing model in request")
	}
	clientStream := gjson.GetBytes(body, "stream").Bool()

	billingModel := resolveOpenAIForwardModel(account, originalModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	reasoningEffort := extractOpenAIReasoningEffortFromBody(body, firstNonEmpty(upstreamModel, billingModel, originalModel))
	serviceTier := extractOpenAIServiceTierFromBody(body)

	var chatReq cursor.OpenAIChatRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "failed to parse request body: "+err.Error())
		return nil, fmt.Errorf("unmarshal chat completions request: %w", err)
	}
	chatReq.Model = upstreamModel

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		writeChatCompletionsError(c, http.StatusUnauthorized, "invalid_request_error", "failed to get cursor credentials: "+err.Error())
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		writeChatCompletionsError(c, http.StatusUnauthorized, "invalid_request_error", "cursor access token or api key is missing")
		return nil, fmt.Errorf("account %d missing cursor token", account.ID)
	}

	machineID := strings.TrimSpace(account.GetCredential("machine_id"))
	if machineID == "" {
		machineID = uuid.NewString()
	}

	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		baseURL = cursor.DefaultBackendURL
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	cursorClient := cursor.NewClient(token, machineID, createCursorHTTPClient(proxyURL), baseURL)

	// Explicit inference transport keeps the legacy Connect-RPC path (tests / advanced).
	transport := strings.ToLower(strings.TrimSpace(account.GetCredential("cursor_transport")))
	if transport == "inference" {
		return s.forwardCursorViaInference(
			ctx, c, account, cursorClient, &chatReq,
			originalModel, billingModel, upstreamModel,
			reasoningEffort, serviceTier, clientStream, startTime,
		)
	}

	conversationID := firstNonEmpty(
		gjson.GetBytes(body, "conversation_id").String(),
		strings.TrimSpace(c.GetHeader("conversation_id")),
		gjson.GetBytes(body, "user").String(),
	)

	gwReq := &cursor.GatewayChatRequest{
		Messages:       chatReq.Messages,
		Model:          upstreamModel,
		ConversationID: conversationID,
		Tools:          chatReq.Tools,
		Stream:         clientStream,
	}

	deltas, result, err := cursorClient.ChatViaGateway(ctx, gwReq)
	if err != nil {
		// One force-refresh + retry on auth failure.
		var upErr *cursor.UpstreamError
		if errors.As(err, &upErr) && upErr.StatusCode == http.StatusUnauthorized &&
			account.Type == AccountTypeOAuth && s.cursorTokenProvider != nil {
			logger.L().Info("cursor sandbox returned 401, attempting token refresh",
				zap.Int64("account_id", account.ID),
			)
			if refreshed, refErr := s.cursorTokenProvider.ForceRefresh(ctx, account); refErr == nil && refreshed != "" {
				cursor.InvalidateSandboxCache(token)
				cursorClient.SetToken(refreshed)
				deltas, result, err = cursorClient.ChatViaGateway(ctx, gwReq)
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
			writeChatCompletionsError(c, upErr.StatusCode, "upstream_error", upErr.Body)
			return nil, err
		}
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", err.Error())
		return nil, err
	}

	if clientStream {
		return s.streamCursorGatewayChat(
			ctx, c, deltas, result,
			account, originalModel, billingModel, upstreamModel,
			reasoningEffort, serviceTier, startTime,
		)
	}
	return s.bufferCursorGatewayChat(
		ctx, c, deltas, result,
		account, originalModel, billingModel, upstreamModel,
		reasoningEffort, serviceTier, startTime,
	)
}

func (s *OpenAIGatewayService) streamCursorGatewayChat(
	ctx context.Context,
	c *gin.Context,
	deltas <-chan cursor.ChatDelta,
	result *cursor.GatewayChatResult,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	var usage OpenAIUsage
	var firstTokenMs *int
	clientDisconnected := false
	hasToolCalls := false
	createdTimestamp := time.Now().Unix()
	responseID := "chatcmpl-" + uuid.NewString()
	finishReason := "stop"
	emittedFinish := false

	writeChunk := func(chunk *apicompat.ChatCompletionsChunk) {
		if clientDisconnected {
			return
		}
		b, err := json.Marshal(chunk)
		if err != nil {
			return
		}
		if _, werr := fmt.Fprintf(c.Writer, "data: %s\n\n", b); werr != nil {
			clientDisconnected = true
			logger.L().Debug("cursor gateway stream: client disconnected",
				zap.Error(werr),
				zap.String("response_id", responseID),
			)
		} else {
			c.Writer.Flush()
		}
	}

	for d := range deltas {
		if firstTokenMs == nil && (d.Text != "" || d.Reasoning != "" || d.ToolName != "") {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		if d.Reasoning != "" {
			text := d.Reasoning
			writeChunk(&apicompat.ChatCompletionsChunk{
				ID:      responseID,
				Object:  "chat.completion.chunk",
				Created: createdTimestamp,
				Model:   originalModel,
				Choices: []apicompat.ChatChunkChoice{{
					Index: 0,
					Delta: apicompat.ChatDelta{ReasoningContent: &text},
				}},
			})
		}
		if d.Text != "" {
			text := d.Text
			writeChunk(&apicompat.ChatCompletionsChunk{
				ID:      responseID,
				Object:  "chat.completion.chunk",
				Created: createdTimestamp,
				Model:   originalModel,
				Choices: []apicompat.ChatChunkChoice{{
					Index: 0,
					Delta: apicompat.ChatDelta{Content: &text},
				}},
			})
		}
		if d.ToolName != "" || d.ToolCallID != "" || d.ToolArgs != "" {
			hasToolCalls = true
			idx := d.ToolIndex
			writeChunk(&apicompat.ChatCompletionsChunk{
				ID:      responseID,
				Object:  "chat.completion.chunk",
				Created: createdTimestamp,
				Model:   originalModel,
				Choices: []apicompat.ChatChunkChoice{{
					Index: 0,
					Delta: apicompat.ChatDelta{
						ToolCalls: []apicompat.ChatToolCall{{
							Index: &idx,
							ID:    d.ToolCallID,
							Type:  "function",
							Function: apicompat.ChatFunctionCall{
								Name:      d.ToolName,
								Arguments: d.ToolArgs,
							},
						}},
					},
				}},
			})
		}
		if d.FinishReason != "" {
			finishReason = d.FinishReason
		}
		if d.Usage != nil {
			usage.InputTokens = d.Usage.InputTokens
			usage.OutputTokens = d.Usage.OutputTokens
		}
		if d.Done && !emittedFinish {
			emittedFinish = true
			if hasToolCalls && finishReason == "stop" {
				finishReason = "tool_calls"
			}
			writeChunk(&apicompat.ChatCompletionsChunk{
				ID:      responseID,
				Object:  "chat.completion.chunk",
				Created: createdTimestamp,
				Model:   originalModel,
				Choices: []apicompat.ChatChunkChoice{{
					Index:        0,
					Delta:        apicompat.ChatDelta{},
					FinishReason: &finishReason,
				}},
			})
		}
	}

	if result != nil {
		if usage.InputTokens == 0 {
			usage.InputTokens = result.Usage.InputTokens
		}
		if usage.OutputTokens == 0 {
			usage.OutputTokens = result.Usage.OutputTokens
		}
		if firstTokenMs == nil && !result.FirstTokenAt.IsZero() {
			ms := int(result.FirstTokenAt.Sub(startTime).Milliseconds())
			if ms < 0 {
				ms = int(time.Since(startTime).Milliseconds())
			}
			firstTokenMs = &ms
		}
		if hasToolCalls || len(result.ToolCalls) > 0 {
			finishReason = "tool_calls"
		} else if result.FinishReason != "" {
			finishReason = result.FinishReason
		}
	}

	if !emittedFinish {
		if hasToolCalls {
			finishReason = "tool_calls"
		}
		writeChunk(&apicompat.ChatCompletionsChunk{
			ID:      responseID,
			Object:  "chat.completion.chunk",
			Created: createdTimestamp,
			Model:   originalModel,
			Choices: []apicompat.ChatChunkChoice{{
				Index:        0,
				Delta:        apicompat.ChatDelta{},
				FinishReason: &finishReason,
			}},
		})
	}

	writeChunk(&apicompat.ChatCompletionsChunk{
		ID:      responseID,
		Object:  "chat.completion.chunk",
		Created: createdTimestamp,
		Model:   originalModel,
		Choices: []apicompat.ChatChunkChoice{},
		Usage: &apicompat.ChatUsage{
			PromptTokens:     usage.InputTokens,
			CompletionTokens: usage.OutputTokens,
			TotalTokens:      usage.InputTokens + usage.OutputTokens,
		},
	})

	if !clientDisconnected {
		_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}

	_ = ctx
	_ = account

	return &OpenAIForwardResult{
		RequestID:        responseID,
		ResponseID:       responseID,
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
		ClientDisconnect: clientDisconnected,
		UpstreamEndpoint: cursorSandboxGatewayEndpoint,
	}, nil
}

func (s *OpenAIGatewayService) bufferCursorGatewayChat(
	ctx context.Context,
	c *gin.Context,
	deltas <-chan cursor.ChatDelta,
	result *cursor.GatewayChatResult,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	// Drain stream so the gateway worker finishes aggregation into result.
	for range deltas {
	}

	if result == nil {
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "empty gateway result")
		return nil, errors.New("empty gateway result")
	}
	if result.Text == "" && len(result.ToolCalls) == 0 {
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "timed out waiting for gateway reply")
		return nil, errors.New("timed out waiting for gateway reply")
	}

	var firstTokenMs *int
	if !result.FirstTokenAt.IsZero() {
		ms := int(result.FirstTokenAt.Sub(startTime).Milliseconds())
		if ms < 0 {
			ms = int(time.Since(startTime).Milliseconds())
		}
		firstTokenMs = &ms
	} else {
		ms := int(time.Since(startTime).Milliseconds())
		firstTokenMs = &ms
	}

	usage := OpenAIUsage{
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
	}

	finishReason := result.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}
	var chatToolCalls []apicompat.ChatToolCall
	if len(result.ToolCalls) > 0 {
		finishReason = "tool_calls"
		chatToolCalls = make([]apicompat.ChatToolCall, 0, len(result.ToolCalls))
		for _, tc := range result.ToolCalls {
			chatToolCalls = append(chatToolCalls, apicompat.ChatToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: apicompat.ChatFunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}

	rawContent, _ := json.Marshal(result.Text)
	responseID := "chatcmpl-" + uuid.NewString()
	createdTimestamp := time.Now().Unix()
	chatResp := apicompat.ChatCompletionsResponse{
		ID:      responseID,
		Object:  "chat.completion",
		Created: createdTimestamp,
		Model:   originalModel,
		Choices: []apicompat.ChatChoice{{
			Index: 0,
			Message: apicompat.ChatMessage{
				Role:             "assistant",
				Content:          rawContent,
				ReasoningContent: result.Reasoning,
				ToolCalls:        chatToolCalls,
			},
			FinishReason: finishReason,
		}},
		Usage: &apicompat.ChatUsage{
			PromptTokens:     usage.InputTokens,
			CompletionTokens: usage.OutputTokens,
			TotalTokens:      usage.InputTokens + usage.OutputTokens,
		},
	}
	c.JSON(http.StatusOK, chatResp)

	_ = ctx
	_ = account

	return &OpenAIForwardResult{
		RequestID:        responseID,
		ResponseID:       responseID,
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

// forwardCursorViaInference is the legacy Connect-RPC InferenceService.Stream path.
func (s *OpenAIGatewayService) forwardCursorViaInference(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	cursorClient *cursor.Client,
	chatReq *cursor.OpenAIChatRequest,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	clientStream bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	protoReq, err := cursor.MapOpenAIRequestToProto(chatReq)
	if err != nil {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "failed to map request: "+err.Error())
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
			writeChatCompletionsError(c, upErr.StatusCode, "upstream_error", upErr.Body)
			return nil, err
		}
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", err.Error())
		return nil, err
	}

	if clientStream {
		return s.streamCursorChatCompletions(
			ctx, c, stream, account,
			originalModel, billingModel, upstreamModel,
			reasoningEffort, serviceTier, startTime,
		)
	}
	return s.bufferCursorChatCompletions(
		ctx, c, stream, account,
		originalModel, billingModel, upstreamModel,
		reasoningEffort, serviceTier, startTime,
	)
}

func createCursorHTTPClient(proxyURL string) *http.Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if proxyURL != "" {
		if parsed, err := url.Parse(proxyURL); err == nil {
			_ = proxyutil.ConfigureTransportProxy(transport, parsed)
		}
	}
	return &http.Client{
		Transport: transport,
	}
}

func (s *OpenAIGatewayService) streamCursorChatCompletions(
	ctx context.Context,
	c *gin.Context,
	stream *cursor.StreamReader,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	defer func() { _ = stream.Close() }()

	c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	var usage OpenAIUsage
	var firstTokenMs *int
	clientDisconnected := false
	hasToolCalls := false
	createdTimestamp := time.Now().Unix()
	responseID := "chatcmpl-" + uuid.NewString()

	writeChunk := func(chunk *apicompat.ChatCompletionsChunk) {
		if clientDisconnected {
			return
		}
		b, err := json.Marshal(chunk)
		if err != nil {
			return
		}
		if _, werr := fmt.Fprintf(c.Writer, "data: %s\n\n", b); werr != nil {
			clientDisconnected = true
			logger.L().Debug("cursor chat_completions stream: client disconnected, draining upstream",
				zap.Error(werr),
				zap.String("response_id", responseID),
			)
		} else {
			c.Writer.Flush()
		}
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if clientDisconnected {
				break
			}
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
			logger.L().Warn("cursor stream read error", zap.Error(err))
			break
		}

		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}

		if respInfo := msg.GetResponseInfo(); respInfo != nil {
			if respInfo.Id != "" {
				responseID = respInfo.Id
			}
		}

		if extUsage := msg.GetExtendedUsage(); extUsage != nil {
			usage.InputTokens = int(extUsage.InputTokens)
			usage.OutputTokens = int(extUsage.OutputTokens)
			usage.CacheReadInputTokens = int(extUsage.CacheReadTokens)
			usage.CacheCreationInputTokens = int(extUsage.CacheWriteTokens)
		} else if u := msg.GetUsage(); u != nil && usage.InputTokens == 0 && usage.OutputTokens == 0 {
			usage.InputTokens = int(u.PromptTokens)
			usage.OutputTokens = int(u.CompletionTokens)
		}

		if thinking := msg.GetThinkingPart(); thinking != nil && thinking.Text != "" {
			text := thinking.Text
			writeChunk(&apicompat.ChatCompletionsChunk{
				ID:      responseID,
				Object:  "chat.completion.chunk",
				Created: createdTimestamp,
				Model:   originalModel,
				Choices: []apicompat.ChatChunkChoice{
					{
						Index: 0,
						Delta: apicompat.ChatDelta{
							ReasoningContent: &text,
						},
					},
				},
			})
		}

		if textPart := msg.GetTextPart(); textPart != nil && textPart.Text != "" {
			text := textPart.Text
			writeChunk(&apicompat.ChatCompletionsChunk{
				ID:      responseID,
				Object:  "chat.completion.chunk",
				Created: createdTimestamp,
				Model:   originalModel,
				Choices: []apicompat.ChatChunkChoice{
					{
						Index: 0,
						Delta: apicompat.ChatDelta{
							Content: &text,
						},
					},
				},
			})
		}

		if toolPart := msg.GetToolCallPart(); toolPart != nil {
			hasToolCalls = true
			toolIdx := 0
			if toolPart.ToolIndex != nil {
				toolIdx = int(*toolPart.ToolIndex)
			}
			writeChunk(&apicompat.ChatCompletionsChunk{
				ID:      responseID,
				Object:  "chat.completion.chunk",
				Created: createdTimestamp,
				Model:   originalModel,
				Choices: []apicompat.ChatChunkChoice{
					{
						Index: 0,
						Delta: apicompat.ChatDelta{
							ToolCalls: []apicompat.ChatToolCall{
								{
									Index: &toolIdx,
									ID:    toolPart.ToolCallId,
									Type:  "function",
									Function: apicompat.ChatFunctionCall{
										Name:      toolPart.ToolName,
										Arguments: toolPart.Args,
									},
								},
							},
						},
					},
				},
			})
		}
	}

	finishReason := "stop"
	if hasToolCalls {
		finishReason = "tool_calls"
	}

	writeChunk(&apicompat.ChatCompletionsChunk{
		ID:      responseID,
		Object:  "chat.completion.chunk",
		Created: createdTimestamp,
		Model:   originalModel,
		Choices: []apicompat.ChatChunkChoice{
			{
				Index:        0,
				Delta:        apicompat.ChatDelta{},
				FinishReason: &finishReason,
			},
		},
	})

	writeChunk(&apicompat.ChatCompletionsChunk{
		ID:      responseID,
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

	if !clientDisconnected {
		_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}

	_ = ctx
	_ = account

	return &OpenAIForwardResult{
		RequestID:        responseID,
		ResponseID:       responseID,
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
		ClientDisconnect: clientDisconnected,
		UpstreamEndpoint: cursorInferenceStreamEndpoint,
	}, nil
}

func (s *OpenAIGatewayService) bufferCursorChatCompletions(
	ctx context.Context,
	c *gin.Context,
	stream *cursor.StreamReader,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	defer func() { _ = stream.Close() }()

	var contentBuilder strings.Builder
	var reasoningBuilder strings.Builder
	type toolCallState struct {
		id   string
		name string
		args strings.Builder
	}
	var toolCallsState []*toolCallState
	toolCallIndexMap := make(map[string]int)

	var usage OpenAIUsage
	var firstTokenMs *int
	createdTimestamp := time.Now().Unix()
	responseID := "chatcmpl-" + uuid.NewString()

	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
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
				writeChatCompletionsError(c, upErr.StatusCode, "upstream_error", upErr.Body)
				return nil, err
			}
			writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", err.Error())
			return nil, err
		}

		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}

		if respInfo := msg.GetResponseInfo(); respInfo != nil {
			if respInfo.Id != "" {
				responseID = respInfo.Id
			}
		}

		if extUsage := msg.GetExtendedUsage(); extUsage != nil {
			usage.InputTokens = int(extUsage.InputTokens)
			usage.OutputTokens = int(extUsage.OutputTokens)
			usage.CacheReadInputTokens = int(extUsage.CacheReadTokens)
			usage.CacheCreationInputTokens = int(extUsage.CacheWriteTokens)
		} else if u := msg.GetUsage(); u != nil && usage.InputTokens == 0 && usage.OutputTokens == 0 {
			usage.InputTokens = int(u.PromptTokens)
			usage.OutputTokens = int(u.CompletionTokens)
		}

		if thinking := msg.GetThinkingPart(); thinking != nil {
			reasoningBuilder.WriteString(thinking.Text)
		}

		if textPart := msg.GetTextPart(); textPart != nil {
			contentBuilder.WriteString(textPart.Text)
		}

		if toolPart := msg.GetToolCallPart(); toolPart != nil {
			callKey := toolPart.ToolCallId
			if callKey == "" {
				callKey = fmt.Sprintf("call_%d", len(toolCallsState))
			}
			idx, exists := toolCallIndexMap[callKey]
			if !exists {
				idx = len(toolCallsState)
				toolCallIndexMap[callKey] = idx
				toolCallsState = append(toolCallsState, &toolCallState{
					id:   callKey,
					name: toolPart.ToolName,
				})
			}
			if toolPart.ToolName != "" {
				toolCallsState[idx].name = toolPart.ToolName
			}
			toolCallsState[idx].args.WriteString(toolPart.Args)
		}
	}

	var chatToolCalls []apicompat.ChatToolCall
	finishReason := "stop"
	if len(toolCallsState) > 0 {
		finishReason = "tool_calls"
		chatToolCalls = make([]apicompat.ChatToolCall, 0, len(toolCallsState))
		for _, tc := range toolCallsState {
			chatToolCalls = append(chatToolCalls, apicompat.ChatToolCall{
				ID:   tc.id,
				Type: "function",
				Function: apicompat.ChatFunctionCall{
					Name:      tc.name,
					Arguments: tc.args.String(),
				},
			})
		}
	}

	rawContent, _ := json.Marshal(contentBuilder.String())
	chatResp := apicompat.ChatCompletionsResponse{
		ID:      responseID,
		Object:  "chat.completion",
		Created: createdTimestamp,
		Model:   originalModel,
		Choices: []apicompat.ChatChoice{
			{
				Index: 0,
				Message: apicompat.ChatMessage{
					Role:             "assistant",
					Content:          rawContent,
					ReasoningContent: reasoningBuilder.String(),
					ToolCalls:        chatToolCalls,
				},
				FinishReason: finishReason,
			},
		},
		Usage: &apicompat.ChatUsage{
			PromptTokens:     usage.InputTokens,
			CompletionTokens: usage.OutputTokens,
			TotalTokens:      usage.InputTokens + usage.OutputTokens,
			PromptTokensDetails: &apicompat.ChatTokenDetails{
				CachedTokens: usage.CacheReadInputTokens,
			},
		},
	}

	c.JSON(http.StatusOK, chatResp)

	_ = ctx
	_ = account

	return &OpenAIForwardResult{
		RequestID:        responseID,
		ResponseID:       responseID,
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
