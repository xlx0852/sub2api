package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

func (s *OpenAIGatewayService) forwardGrokChatParse(ex *pipeline.Exchange) error {
	st := grokChatForwardStateFrom(ex)
	if st == nil || st.chatReq == nil {
		return fmt.Errorf("parse grok chat completions request: empty state")
	}
	ex.RequestedModel = st.originalModel
	ex.Inbound = st.body
	return nil
}

func (s *OpenAIGatewayService) forwardGrokChatRewrite(ex *pipeline.Exchange) error {
	st := grokChatForwardStateFrom(ex)
	if st == nil || st.chatReq == nil {
		return fmt.Errorf("rewrite grok chat completions request: empty state")
	}
	account := ex.Account
	responsesReq, err := apicompat.ChatCompletionsToResponses(st.chatReq)
	if err != nil {
		return fmt.Errorf("convert grok chat completions to responses: %w", err)
	}
	responsesReq.Model = st.upstreamModel
	responsesReq.Stream = true
	responsesReq.Include = nil
	responsesReq.Store = nil

	responsesBody, err := json.Marshal(responsesReq)
	if err != nil {
		return fmt.Errorf("marshal grok responses bridge request: %w", err)
	}
	responsesBody, err = patchGrokResponsesBody(responsesBody, st.upstreamModel)
	if err != nil {
		return fmt.Errorf("patch grok responses bridge request: %w", err)
	}
	intentBody, err := grokChatCacheIntentBody(st.body)
	if err != nil {
		return fmt.Errorf("normalize grok responses bridge tool intent: %w", err)
	}
	responsesBody, err = applyGrokResponsesCacheIdentity(responsesBody, intentBody, st.cacheIdentity, true)
	if err != nil {
		return fmt.Errorf("apply grok responses bridge cache identity: %w", err)
	}

	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ex.Ctx, account, st.upstreamModel, responsesBody, forceOpenAIFastFromContext(st.c))
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			MarkOpsClientBusinessLimited(st.c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			writeChatCompletionsError(st.c, http.StatusForbidden, "permission_error", blocked.Message)
		}
		return policyErr
	}
	responsesBody = updatedBody
	responsesBody, err = s.optimizeGrokPayload(account, responsesBody, grokPayloadResponses)
	if err != nil {
		if handleGrokPayloadOptimizationError(st.c, grokPayloadChat, err) {
			return err
		}
		return fmt.Errorf("optimize Grok Responses bridge payload: %w", err)
	}
	st.responsesBody = responsesBody
	ex.UpstreamModel = st.upstreamModel
	ex.Outbound = responsesBody
	return nil
}

func (s *OpenAIGatewayService) forwardGrokChatTransport(ex *pipeline.Exchange) error {
	st := grokChatForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("transport grok chat completions request: empty state")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	token, _, credential, err := s.getGrokAccessTokenWithDescriptor(ctx, account)
	if err != nil {
		return fmt.Errorf("get grok access token: %w", err)
	}
	st.token = token
	st.credential = credential
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := buildGrokResponsesRequest(upstreamCtx, c, account, st.responsesBody, token, st.cacheIdentity)
	releaseUpstreamCtx()
	if err != nil {
		return fmt.Errorf("build grok responses bridge request: %w", err)
	}
	SetActualOpenAIUpstreamEndpoint(c, grokChatResponsesEndpoint)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	st.resp = resp

	if resp.StatusCode >= http.StatusBadRequest {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
		}
		attribution := s.attributeGrokUnauthorized(ctx, c, account, resp.StatusCode, credential)
		retryDisabled := grokUpstreamExplicitlyDisablesRetry(resp.Header)
		if retryDisabled {
			result, handleErr := s.handleChatCompletionsErrorResponse(resp, c, account, st.billingModel)
			st.result = result
			return handleErr
		}
		opsEvent := OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
			Kind:               "failover",
			Message:            upstreamMsg,
		}
		applyGrokUnauthorizedAttribution(&opsEvent, attribution)
		appendOpsUpstreamError(c, opsEvent)
		grokSameAccountRetry := s.isGrokSameAccountRetry(c, resp.StatusCode)
		penalizeAccount := shouldPenalizeGrokUnauthorized(resp.StatusCode, attribution)
		if !grokSameAccountRetry && penalizeAccount {
			s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return &UpstreamFailoverError{
				StatusCode:                resp.StatusCode,
				ResponseBody:              respBody,
				ResponseHeaders:           resp.Header.Clone(),
				RetryableOnSameAccount:    grokSameAccountRetry || (account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)),
				GrokSameAccountRetry:      grokSameAccountRetry,
				GrokAccountPenaltyApplied: !grokSameAccountRetry && penalizeAccount,
				GrokStaleCredential:       !penalizeAccount,
			}
		}
		result, handleErr := s.handleChatCompletionsErrorResponse(resp, c, account, st.billingModel)
		st.result = result
		return handleErr
	}

	s.updateGrokUsageSnapshot(ctx, account, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
	return nil
}

func (s *OpenAIGatewayService) forwardGrokChatRelay(ex *pipeline.Exchange) error {
	st := grokChatForwardStateFrom(ex)
	if st == nil {
		return nil
	}
	if st.result != nil {
		return nil
	}
	if st.resp == nil {
		return fmt.Errorf("relay grok chat completions request: empty response")
	}
	account := ex.Account
	var result *OpenAIForwardResult
	var err error
	if st.clientStream {
		result, err = s.handleChatStreamingResponse(st.resp, st.c, account, st.originalModel, st.billingModel, st.upstreamModel, st.startTime, len(st.body))
	} else {
		result, err = s.handleChatBufferedStreamingResponse(st.resp, st.c, account, st.originalModel, st.billingModel, st.upstreamModel, st.startTime)
	}
	if result != nil {
		result.UpstreamEndpoint = grokChatResponsesEndpoint
		result.ResponseHeaders = st.resp.Header.Clone()
		if result.RequestID == "" {
			result.RequestID = firstNonEmpty(st.resp.Header.Get("x-request-id"), st.resp.Header.Get("xai-request-id"))
		}
		result.ReasoningEffort = extractOpenAIReasoningEffortFromBody(st.body, firstNonEmpty(st.upstreamModel, st.billingModel, st.originalModel))
	}
	st.result = result
	return err
}

func (s *OpenAIGatewayService) forwardGrokChatSettle(ex *pipeline.Exchange) error {
	return nil
}
