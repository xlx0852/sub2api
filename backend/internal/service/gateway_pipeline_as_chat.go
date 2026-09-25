package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

func (s *GatewayService) forwardClaudeAsChatParse(ex *pipeline.Exchange) error {
	st := claudeAsChatStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse chat completions request: empty state")
	}
	var ccReq apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(st.body, &ccReq); err != nil {
		return fmt.Errorf("parse chat completions request: %w", err)
	}
	st.originalModel = ccReq.Model
	st.clientStream = ccReq.Stream
	st.includeUsage = ccReq.StreamOptions != nil && ccReq.StreamOptions.IncludeUsage

	responsesReq, err := apicompat.ChatCompletionsToResponses(&ccReq)
	if err != nil {
		return fmt.Errorf("convert chat completions to responses: %w", err)
	}
	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(responsesReq)
	if err != nil {
		return fmt.Errorf("convert responses to anthropic: %w", err)
	}
	anthropicReq.Stream = true
	st.reqStream = true
	st.anthropicReq = anthropicReq
	ex.RequestedModel = st.originalModel
	ex.Inbound = st.body
	return nil
}

func (s *GatewayService) forwardClaudeAsChatRewrite(ex *pipeline.Exchange) error {
	st := claudeAsChatStateFrom(ex)
	if st == nil || st.anthropicReq == nil {
		return fmt.Errorf("rewrite chat completions request: empty state")
	}
	account := ex.Account
	st.mappedModel = mapClaudeCompatUpstreamModel(account, st.originalModel)
	st.anthropicReq.Model = st.mappedModel
	ex.UpstreamModel = st.mappedModel

	logger.L().Debug("gateway forward_as_chat_completions: model mapping applied",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", st.originalModel),
		zap.String("mapped_model", st.mappedModel),
		zap.Bool("client_stream", st.clientStream),
	)

	anthropicBody, err := json.Marshal(st.anthropicReq)
	if err != nil {
		return fmt.Errorf("marshal anthropic request: %w", err)
	}

	st.shouldMimic = false
	st.anthropicBody = enforceCacheControlLimit(anthropicBody)
	ex.Outbound = st.anthropicBody
	return nil
}

func (s *GatewayService) forwardClaudeAsChatTransport(ex *pipeline.Exchange) error {
	st := claudeAsChatStateFrom(ex)
	if st == nil {
		return fmt.Errorf("transport chat completions request: empty state")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}
	st.token = token
	st.tokenType = tokenType
	if account.ProxyID != nil && account.Proxy != nil {
		st.proxyURL = account.Proxy.URL()
	}

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, st.reqStream)
	upstreamReq, _, err := s.buildUpstreamRequest(upstreamCtx, c, account, st.anthropicBody, st.token, st.tokenType, st.mappedModel, st.reqStream, st.shouldMimic)
	releaseUpstreamCtx()
	if err != nil {
		return fmt.Errorf("build upstream request: %w", err)
	}

	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, st.proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		writeGatewayCCError(c, http.StatusBadGateway, "server_error", "Upstream request failed")
		return fmt.Errorf("upstream request failed: %s", safeErr)
	}
	st.resp = resp

	if resp.StatusCode >= 400 {
		respBody, _ := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			if s.rateLimitService != nil {
				s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, st.mappedModel)
			}
			return &UpstreamFailoverError{
				StatusCode:   resp.StatusCode,
				ResponseBody: respBody,
			}
		}

		writeGatewayCCError(c, mapUpstreamStatusCode(resp.StatusCode), "server_error", upstreamMsg)
		return fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}
	return nil
}

func (s *GatewayService) forwardClaudeAsChatRelay(ex *pipeline.Exchange) error {
	st := claudeAsChatStateFrom(ex)
	if st == nil || st.resp == nil {
		return fmt.Errorf("relay chat completions request: empty response")
	}
	st.reasoningEffort = extractCCReasoningEffortFromBody(st.body)
	st.reasoningEffort = ApplyThinkingEnabledFallback(st.reasoningEffort, st.body, st.mappedModel)

	var result *ForwardResult
	var err error
	if st.clientStream {
		result, err = s.handleCCStreamingFromAnthropic(st.resp, st.c, st.originalModel, st.mappedModel, st.reasoningEffort, st.startTime, st.includeUsage, st.sink)
	} else {
		result, err = s.handleCCBufferedFromAnthropic(st.resp, st.c, st.originalModel, st.mappedModel, st.reasoningEffort, st.startTime)
	}
	st.result = result
	return err
}

func (s *GatewayService) forwardClaudeAsChatSettle(ex *pipeline.Exchange) error {
	return nil
}
