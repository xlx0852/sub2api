package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

func (s *OpenAIGatewayService) forwardResponsesChatFallbackParse(ex *pipeline.Exchange) error {
	st := responsesChatFallbackStateFrom(ex)
	if st == nil || st.c == nil {
		return fmt.Errorf("parse request: empty request")
	}
	body := st.body
	if len(body) == 0 {
		body = ex.Inbound
	}
	flattenedBody, err := flattenOpenAIResponsesNamespaces(st.c, body)
	if err != nil {
		writeOpenAIResponsesFallbackError(st.c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return fmt.Errorf("flatten responses namespace tools: %w", err)
	}
	st.body = flattenedBody

	var responsesReq apicompat.ResponsesRequest
	if err := json.Unmarshal(st.body, &responsesReq); err != nil {
		writeOpenAIResponsesFallbackError(st.c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return fmt.Errorf("parse responses request: %w", err)
	}
	st.originalModel = strings.TrimSpace(responsesReq.Model)
	if st.originalModel == "" {
		writeOpenAIResponsesFallbackError(st.c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return fmt.Errorf("missing model in request")
	}
	st.responsesReq = &responsesReq
	st.clientStream = responsesReq.Stream
	st.serviceTier = extractOpenAIServiceTierFromBody(st.body)
	ex.RequestedModel = st.originalModel
	ex.Inbound = st.body
	return nil
}

func (s *OpenAIGatewayService) forwardResponsesChatFallbackRewrite(ex *pipeline.Exchange) error {
	st := responsesChatFallbackStateFrom(ex)
	if st == nil || st.responsesReq == nil {
		return fmt.Errorf("parse request: empty request")
	}
	account := ex.Account

	effectiveTools, err := apicompat.EffectiveResponsesTools(st.responsesReq)
	if err != nil {
		writeOpenAIResponsesFallbackError(st.c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return fmt.Errorf("resolve responses tools: %w", err)
	}
	st.customTools = apicompat.CustomToolNames(effectiveTools)

	chatReq, err := apicompat.ResponsesToChatCompletionsRequest(st.responsesReq)
	if err != nil {
		writeOpenAIResponsesFallbackError(st.c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return fmt.Errorf("convert responses to chat completions: %w", err)
	}

	st.billingModel = resolveOpenAIForwardModel(account, st.originalModel, "")
	st.upstreamModel = normalizeOpenAIModelForUpstream(account, st.billingModel)
	st.reasoningEffort = extractOpenAIReasoningEffortFromBody(st.body, firstNonEmpty(st.upstreamModel, st.billingModel, st.originalModel))
	st.reasoningEffort = ApplyThinkingEnabledFallback(st.reasoningEffort, st.body, st.billingModel)
	chatReq.Model = st.upstreamModel
	if st.clientStream {
		chatReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}
	}

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		return fmt.Errorf("marshal chat completions fallback request: %w", err)
	}
	chatBody, err = s.applyOpenAIFastPolicyToBody(ex.Ctx, account, st.upstreamModel, chatBody, forceOpenAIFastFromContext(st.c))
	if err != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(err, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(st.c, blocked)
		}
		return err
	}
	if account.Platform == PlatformKimi {
		chatBody, err = normalizeKimiToolMessageLinks(chatBody)
		if err != nil {
			return fmt.Errorf("normalize Kimi tool messages: %w", err)
		}
	}
	if st.serviceTier == nil {
		st.serviceTier = extractOpenAIServiceTierFromBody(chatBody)
	}
	st.chatBody = chatBody
	ex.UpstreamModel = st.upstreamModel
	ex.Inbound = chatBody

	logger.L().Debug("openai responses: forwarding via raw chat completions",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", st.originalModel),
		zap.String("billing_model", st.billingModel),
		zap.String("upstream_model", st.upstreamModel),
		zap.Bool("stream", st.clientStream),
	)
	return nil
}

func (s *OpenAIGatewayService) forwardResponsesChatFallbackTransport(ex *pipeline.Exchange) error {
	st := responsesChatFallbackStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse request: empty request")
	}
	account := ex.Account
	ctx := ex.Ctx

	apiKey, targetURL, err := s.resolveCCFallbackTarget(ctx, account)
	if err != nil {
		return err
	}
	resp, err := s.sendCCUpstreamRequestWithKimiRefresh(ctx, st.c, account, targetURL, st.chatBody, st.clientStream, apiKey, account.GetOpenAIUserAgent(), "")
	if err != nil {
		return err
	}
	st.resp = resp

	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, st.c, account, resp, respBody, upstreamMsg, st.upstreamModel); foErr != nil {
			return foErr
		}
		st.result, err = s.handleErrorResponse(ctx, resp, st.c, account, st.chatBody, st.billingModel)
		return err
	}
	return nil
}

func (s *OpenAIGatewayService) forwardResponsesChatFallbackRelay(ex *pipeline.Exchange) error {
	st := responsesChatFallbackStateFrom(ex)
	if st == nil || st.resp == nil {
		return fmt.Errorf("upstream request failed: empty response")
	}
	var err error
	if st.clientStream {
		st.result, err = s.streamChatCompletionsAsResponses(st.c, st.resp, st.originalModel, st.customTools, st.billingModel, st.upstreamModel, st.reasoningEffort, st.serviceTier, st.startTime)
		return err
	}
	st.result, err = s.bufferChatCompletionsAsResponses(st.c, st.resp, st.originalModel, st.customTools, st.billingModel, st.upstreamModel, st.reasoningEffort, st.serviceTier, st.startTime)
	return err
}

func (s *OpenAIGatewayService) forwardResponsesChatFallbackSettle(ex *pipeline.Exchange) error {
	st := responsesChatFallbackStateFrom(ex)
	if st == nil || st.result == nil {
		return fmt.Errorf("upstream request failed: empty response")
	}
	ex.UpstreamModel = st.upstreamModel
	return nil
}
