package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const devinConnectEndpoint = "/exa.api_server_pb.ApiServerService/GetChatMessage"

func (s *OpenAIGatewayService) devinClientForAccount(ctx context.Context, account *Account, writeErr func(int, string, string)) (*devin.Client, error) {
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		writeErr(http.StatusUnauthorized, "invalid_request_error", "failed to get Devin credentials: "+err.Error())
		return nil, err
	}
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	model := strings.TrimSpace(account.GetCredential("model"))
	if model == "" {
		model = strings.TrimSpace(account.GetCredential("model_uid"))
	}
	if model == "" {
		model = "glm-5-2"
	}
	client, err := devin.NewClient(baseURL, token, model, createCursorHTTPClient(proxyURLForAccount(account)))
	if err != nil {
		writeErr(http.StatusUnauthorized, "invalid_request_error", err.Error())
		return nil, err
	}
	return client, nil
}

func proxyURLForAccount(account *Account) string {
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		return account.Proxy.URL()
	}
	return ""
}

func (s *OpenAIGatewayService) forwardDevinChatCompletions(ctx context.Context, c *gin.Context, account *Account, body []byte, defaultMappedModel string) (*OpenAIForwardResult, error) {
	start := time.Now()
	var req apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "invalid Devin chat request: "+err.Error())
		return nil, err
	}
	if strings.TrimSpace(req.Model) == "" {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, errors.New("missing Devin model")
	}
	billingModel := resolveOpenAIForwardModel(account, req.Model, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	originalModel := req.Model
	req.Model = upstreamModel
	client, err := s.devinClientForAccount(ctx, account, func(status int, typ, msg string) { writeChatCompletionsError(c, status, typ, msg) })
	if err != nil {
		return nil, err
	}
	protoReq, err := client.BuildRequest(&req)
	if err != nil {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	stream, err := client.Stream(ctx, protoReq)
	if err != nil {
		writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", err.Error())
		return nil, err
	}
	return s.renderDevinChatStream(c, stream, account, originalModel, billingModel, upstreamModel, req.Stream, start)
}

func (s *OpenAIGatewayService) renderDevinChatStream(c *gin.Context, stream *devin.Stream, account *Account, originalModel, billingModel, upstreamModel string, clientStream bool, start time.Time) (*OpenAIForwardResult, error) {
	responseID := "chatcmpl-" + uuid.NewString()
	created := time.Now().Unix()
	var text strings.Builder
	var reasoning strings.Builder
	var toolCalls []apicompat.ChatToolCall
	var usage OpenAIUsage
	finishReason := "stop"
	var firstToken *int
	toolIndexes := map[string]int{}

	clientDisconnected := false
	if clientStream {
		c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.Flush()
	}
	writeChunk := func(chunk *apicompat.ChatCompletionsChunk) {
		if !clientStream || clientDisconnected || chunk == nil {
			return
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
			clientDisconnected = true
			return
		}
		c.Writer.Flush()
	}
	for {
		response, recvErr := stream.Recv()
		if recvErr != nil {
			if !errors.Is(recvErr, io.EOF) {
				if clientStream && !clientDisconnected {
					_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
					c.Writer.Flush()
				}
				return nil, recvErr
			}
			break
		}
		delta := devin.DecodeResponse(response, len(toolCalls))
		if firstToken == nil && (delta.Text != "" || delta.Reasoning != "" || delta.ToolName != "") {
			ms := int(time.Since(start).Milliseconds())
			firstToken = &ms
		}
		if delta.Text != "" {
			text.WriteString(delta.Text)
			v := delta.Text
			writeChunk(&apicompat.ChatCompletionsChunk{ID: responseID, Object: "chat.completion.chunk", Created: created, Model: originalModel, Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{Content: &v}}}})
		}
		if delta.Reasoning != "" {
			reasoning.WriteString(delta.Reasoning)
			v := delta.Reasoning
			writeChunk(&apicompat.ChatCompletionsChunk{ID: responseID, Object: "chat.completion.chunk", Created: created, Model: originalModel, Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{ReasoningContent: &v}}}})
		}
		if delta.ToolName != "" || delta.ToolCallID != "" || delta.ToolArgs != "" {
			idx, ok := toolIndexes[delta.ToolCallID]
			if !ok {
				idx = len(toolCalls)
				toolIndexes[delta.ToolCallID] = idx
				toolCalls = append(toolCalls, apicompat.ChatToolCall{ID: delta.ToolCallID, Type: "function", Function: apicompat.ChatFunctionCall{Name: delta.ToolName}})
			}
			if delta.ToolName != "" {
				toolCalls[idx].Function.Name = delta.ToolName
			}
			toolCalls[idx].Function.Arguments += delta.ToolArgs
			index := idx
			writeChunk(&apicompat.ChatCompletionsChunk{ID: responseID, Object: "chat.completion.chunk", Created: created, Model: originalModel, Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{ToolCalls: []apicompat.ChatToolCall{{Index: &index, ID: delta.ToolCallID, Type: "function", Function: apicompat.ChatFunctionCall{Name: delta.ToolName, Arguments: delta.ToolArgs}}}}}}})
			finishReason = "tool_calls"
		}
		if delta.Usage != nil {
			usage.InputTokens, usage.OutputTokens = delta.Usage.InputTokens, delta.Usage.OutputTokens
			usage.CacheReadInputTokens, usage.CacheCreationInputTokens = delta.Usage.CacheReadTokens, delta.Usage.CacheWriteTokens
		}
		if delta.FinishReason != "" {
			finishReason = delta.FinishReason
		}
	}
	if clientStream {
		finish := finishReason
		writeChunk(&apicompat.ChatCompletionsChunk{ID: responseID, Object: "chat.completion.chunk", Created: created, Model: originalModel, Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{}, FinishReason: &finish}}})
		writeChunk(&apicompat.ChatCompletionsChunk{ID: responseID, Object: "chat.completion.chunk", Created: created, Model: originalModel, Choices: []apicompat.ChatChunkChoice{}, Usage: &apicompat.ChatUsage{PromptTokens: usage.InputTokens, CompletionTokens: usage.OutputTokens, TotalTokens: usage.InputTokens + usage.OutputTokens}})
		if !clientDisconnected {
			_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
			c.Writer.Flush()
		}
	} else {
		message := apicompat.ChatMessage{Role: "assistant"}
		if text.Len() > 0 {
			message.Content = json.RawMessage(strconv.Quote(text.String()))
		}
		message.ReasoningContent = reasoning.String()
		message.ToolCalls = toolCalls
		c.JSON(http.StatusOK, apicompat.ChatCompletionsResponse{ID: responseID, Object: "chat.completion", Created: created, Model: originalModel, Choices: []apicompat.ChatChoice{{Index: 0, Message: message, FinishReason: finishReason}}, Usage: &apicompat.ChatUsage{PromptTokens: usage.InputTokens, CompletionTokens: usage.OutputTokens, TotalTokens: usage.InputTokens + usage.OutputTokens}})
	}
	_ = account
	return &OpenAIForwardResult{RequestID: responseID, ResponseID: responseID, BillingRequestID: uuid.NewString(), Model: originalModel, BillingModel: billingModel, UpstreamModel: upstreamModel, Usage: usage, Stream: clientStream, Duration: time.Since(start), FirstTokenMs: firstToken, UpstreamEndpoint: devinConnectEndpoint}, nil
}

func (s *OpenAIGatewayService) forwardDevinResponses(ctx context.Context, c *gin.Context, account *Account, body []byte, originalModel string, reqStream bool, start time.Time) (*OpenAIForwardResult, error) {
	var resReq apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &resReq); err != nil {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	chatReq, err := apicompat.ResponsesToChatCompletionsRequest(&resReq)
	if err != nil {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	chatReq.Model = originalModel
	return s.forwardDevinCompat(ctx, c, account, chatReq, cursorWireResponses, originalModel, reqStream, start)
}

func (s *OpenAIGatewayService) forwardDevinAnthropic(ctx context.Context, c *gin.Context, account *Account, body []byte, defaultMappedModel string) (*OpenAIForwardResult, error) {
	start := time.Now()
	var anthReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthReq); err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	resReq, err := apicompat.AnthropicToResponses(&anthReq)
	if err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	chatReq, err := apicompat.ResponsesToChatCompletionsRequest(resReq)
	if err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	chatReq.Model = resolveOpenAIForwardModel(account, anthReq.Model, defaultMappedModel)
	return s.forwardDevinCompat(ctx, c, account, chatReq, cursorWireAnthropic, anthReq.Model, resReq.Stream, start)
}

func (s *OpenAIGatewayService) forwardDevinCompat(ctx context.Context, c *gin.Context, account *Account, req *apicompat.ChatCompletionsRequest, format cursorWireFormat, originalModel string, clientStream bool, start time.Time) (*OpenAIForwardResult, error) {
	billingModel := resolveOpenAIForwardModel(account, originalModel, req.Model)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	req.Model = upstreamModel
	writeErr := func(status int, typ, msg string) {
		if format == cursorWireAnthropic {
			writeAnthropicError(c, status, typ, msg)
		} else {
			writeChatCompletionsError(c, status, typ, msg)
		}
	}
	client, err := s.devinClientForAccount(ctx, account, writeErr)
	if err != nil {
		return nil, err
	}
	protoReq, err := client.BuildRequest(req)
	if err != nil {
		writeErr(http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, err
	}
	stream, err := client.Stream(ctx, protoReq)
	if err != nil {
		writeErr(http.StatusBadGateway, "upstream_error", err.Error())
		return nil, err
	}
	return renderDevinCompatStream(stream, c, account, format, originalModel, billingModel, upstreamModel, clientStream, start)
}

func renderDevinCompatStream(stream *devin.Stream, c *gin.Context, account *Account, format cursorWireFormat, originalModel, billingModel, upstreamModel string, clientStream bool, start time.Time) (*OpenAIForwardResult, error) {
	state := apicompat.NewChatCompletionsToResponsesStreamState(originalModel)
	var sink *cursorCompatSink
	if clientStream {
		sink = newCursorCompatSink(c, format, originalModel)
		writeCursorCompatSSEHeaders(c)
	}
	created := time.Now().Unix()
	responseID := "chatcmpl-" + uuid.NewString()
	finish := "stop"
	var usage OpenAIUsage
	var firstToken *int
	toolIndexes := map[string]int{}
	emit := func(chunk *apicompat.ChatCompletionsChunk) {
		if clientStream {
			sink.emit(chunk)
		} else {
			apicompat.ChatCompletionsChunkToResponsesEvents(chunk, state)
		}
	}
	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if clientStream {
				sink.finish()
			}
			return nil, err
		}
		d := devin.DecodeResponse(resp, len(toolIndexes))
		if firstToken == nil && (d.Text != "" || d.Reasoning != "" || d.ToolName != "") {
			ms := int(time.Since(start).Milliseconds())
			firstToken = &ms
		}
		if d.Text != "" {
			v := d.Text
			emit(&apicompat.ChatCompletionsChunk{ID: responseID, Object: "chat.completion.chunk", Created: created, Model: originalModel, Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{Content: &v}}}})
		}
		if d.Reasoning != "" {
			v := d.Reasoning
			emit(&apicompat.ChatCompletionsChunk{ID: responseID, Object: "chat.completion.chunk", Created: created, Model: originalModel, Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{ReasoningContent: &v}}}})
		}
		if d.ToolName != "" || d.ToolCallID != "" || d.ToolArgs != "" {
			idx, ok := toolIndexes[d.ToolCallID]
			if !ok {
				idx = len(toolIndexes)
				toolIndexes[d.ToolCallID] = idx
			}
			finish = "tool_calls"
			i := idx
			emit(&apicompat.ChatCompletionsChunk{ID: responseID, Object: "chat.completion.chunk", Created: created, Model: originalModel, Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{ToolCalls: []apicompat.ChatToolCall{{Index: &i, ID: d.ToolCallID, Type: "function", Function: apicompat.ChatFunctionCall{Name: d.ToolName, Arguments: d.ToolArgs}}}}}}})
		}
		if d.Usage != nil {
			usage = OpenAIUsage{InputTokens: d.Usage.InputTokens, OutputTokens: d.Usage.OutputTokens, CacheReadInputTokens: d.Usage.CacheReadTokens, CacheCreationInputTokens: d.Usage.CacheWriteTokens}
		}
		if d.FinishReason != "" {
			finish = d.FinishReason
		}
	}
	f := finish
	emit(&apicompat.ChatCompletionsChunk{
		ID: responseID, Object: "chat.completion.chunk", Created: created, Model: originalModel,
		Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: apicompat.ChatDelta{}, FinishReason: &f}},
	})
	emit(&apicompat.ChatCompletionsChunk{
		ID: responseID, Object: "chat.completion.chunk", Created: created, Model: originalModel,
		Choices: []apicompat.ChatChunkChoice{},
		Usage:   &apicompat.ChatUsage{PromptTokens: usage.InputTokens, CompletionTokens: usage.OutputTokens, TotalTokens: usage.InputTokens + usage.OutputTokens},
	})
	if clientStream {
		sink.finish()
	} else {
		var final *apicompat.ResponsesResponse
		for _, evt := range apicompat.FinalizeChatCompletionsResponsesStream(state) {
			if evt.Type == "response.completed" {
				final = evt.Response
			}
		}
		if final == nil {
			return nil, errors.New("Devin stream ended without terminal response")
		}
		if format == cursorWireAnthropic {
			c.JSON(http.StatusOK, apicompat.ResponsesToAnthropic(final, originalModel))
		} else {
			c.JSON(http.StatusOK, final)
		}
	}
	_ = account
	return &OpenAIForwardResult{RequestID: responseID, ResponseID: responseID, BillingRequestID: uuid.NewString(), Model: originalModel, BillingModel: billingModel, UpstreamModel: upstreamModel, Usage: usage, Stream: clientStream, Duration: time.Since(start), FirstTokenMs: firstToken, UpstreamEndpoint: devinConnectEndpoint}, nil
}
