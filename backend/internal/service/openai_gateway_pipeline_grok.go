package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

func (s *OpenAIGatewayService) forwardGrokParse(ex *pipeline.Exchange) error {
	st := grokForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse request: empty request")
	}
	account := ex.Account
	if account.Type != AccountTypeOAuth && account.Type != AccountTypeAPIKey {
		return fmt.Errorf("grok account type %s is not supported by responses forwarding", account.Type)
	}
	if len(st.body) == 0 {
		st.body = ex.Inbound
	}
	if st.originalModel == "" {
		st.originalModel = ex.RequestedModel
	}
	ex.RequestedModel = st.originalModel
	ex.Inbound = st.body
	return nil
}

func (s *OpenAIGatewayService) forwardGrokRewrite(ex *pipeline.Exchange) error {
	st := grokForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse request: empty request")
	}
	account := ex.Account

	st.upstreamModel = account.GetMappedModel(st.originalModel)
	if strings.TrimSpace(st.upstreamModel) == "" {
		st.upstreamModel = xai.CurrentDefaultChatModel()
	}
	st.cacheIdentity = resolveGrokCacheIdentity(st.c, st.body, "", st.upstreamModel)
	patchedBody, translation, err := patchGrokResponsesBodyWithTranslation(st.body, st.upstreamModel)
	if err != nil {
		return err
	}
	setGrokCodexTranslation(st.c, translation)
	patchedBody, err = applyGrokResponsesCacheIdentity(patchedBody, st.body, st.cacheIdentity, account.IsGrokOAuth())
	if err != nil {
		return fmt.Errorf("apply grok prompt cache identity: %w", err)
	}
	patchedBody, err = s.optimizeGrokPayload(account, patchedBody, grokPayloadResponses)
	if err != nil {
		if handleGrokPayloadOptimizationError(st.c, grokPayloadResponses, err) {
			return err
		}
		return fmt.Errorf("optimize Grok Responses payload: %w", err)
	}

	// Grok 原生 Responses 在 Forward 的通用 OpenAI Fast 处理之前会提前分流到这里，
	// 因此必须在最终 Grok payload 上单独应用同一套策略。这同时覆盖客户端
	// 显式 service_tier、管理员 Fast policy 和 API Key 的强制 Fast 开关。
	patchedBody, err = s.applyOpenAIFastPolicyToBody(
		ex.Ctx,
		account,
		st.upstreamModel,
		patchedBody,
		forceOpenAIFastFromContext(st.c),
	)
	if err != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(err, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(st.c, blocked)
		}
		return err
	}
	st.patchedBody = patchedBody
	st.serviceTier = extractOpenAIServiceTierFromBody(patchedBody)
	ex.UpstreamModel = st.upstreamModel
	ex.Inbound = patchedBody
	return nil
}

func (s *OpenAIGatewayService) forwardGrokTransport(ex *pipeline.Exchange) error {
	st := grokForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse request: empty request")
	}
	account := ex.Account
	ctx := ex.Ctx

	token, _, credential, err := s.getGrokAccessTokenWithDescriptor(ctx, account)
	if err != nil {
		return err
	}
	st.token = token
	st.credential = credential

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	upstreamReq, err := buildGrokResponsesRequest(upstreamCtx, st.c, account, st.patchedBody, st.token, st.cacheIdentity)
	if err != nil {
		return err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(st.c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return s.handleOpenAIUpstreamTransportError(ctx, st.c, account, err, false)
	}
	st.resp = resp

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		s.updateGrokUsageSnapshot(ctx, account, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
		}
		attribution := s.attributeGrokUnauthorized(ctx, st.c, account, resp.StatusCode, credential)
		retryDisabled := grokUpstreamExplicitlyDisablesRetry(resp.Header)
		if retryDisabled {
			st.result, err = s.handleErrorResponse(ctx, resp, st.c, account, st.patchedBody, st.upstreamModel)
			return err
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
		appendOpsUpstreamError(st.c, opsEvent)
		grokSameAccountRetry := s.isGrokSameAccountRetry(st.c, resp.StatusCode)
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
		st.result, err = s.handleErrorResponse(ctx, resp, st.c, account, st.patchedBody, st.upstreamModel)
		return err
	}

	s.updateGrokUsageSnapshot(ctx, account, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
	return nil
}

func (s *OpenAIGatewayService) forwardGrokRelay(ex *pipeline.Exchange) error {
	st := grokForwardStateFrom(ex)
	if st == nil || st.resp == nil {
		return fmt.Errorf("upstream request failed: empty response")
	}
	account := ex.Account
	ctx := ex.Ctx

	if st.reqStream {
		streamResult, err := s.handleStreamingResponse(ctx, st.resp, st.c, account, st.startTime, st.originalModel, st.upstreamModel)
		if err != nil {
			return err
		}
		st.usage = streamResult.usage
		st.firstTokenMs = streamResult.firstTokenMs
		st.responseID = strings.TrimSpace(streamResult.responseID)
		return nil
	}

	nonStreamResult, err := s.handleNonStreamingResponse(ctx, st.resp, st.c, account, st.originalModel, st.upstreamModel)
	if err != nil {
		return err
	}
	st.usage = nonStreamResult.usage
	st.responseID = strings.TrimSpace(nonStreamResult.responseID)
	return nil
}

func (s *OpenAIGatewayService) forwardGrokSettle(ex *pipeline.Exchange) error {
	st := grokForwardStateFrom(ex)
	if st == nil || st.resp == nil {
		return fmt.Errorf("upstream request failed: empty response")
	}
	if st.usage == nil {
		st.usage = &OpenAIUsage{}
	}
	reasoningEffort := extractOpenAIReasoningEffortFromBody(st.patchedBody, st.originalModel)
	st.result = &OpenAIForwardResult{
		RequestID:       firstNonEmpty(st.resp.Header.Get("x-request-id"), st.resp.Header.Get("xai-request-id")),
		ResponseID:      st.responseID,
		Usage:           *st.usage,
		Model:           st.originalModel,
		UpstreamModel:   st.upstreamModel,
		ReasoningEffort: reasoningEffort,
		ServiceTier:     st.serviceTier,
		Stream:          st.reqStream,
		OpenAIWSMode:    false,
		ResponseHeaders: st.resp.Header.Clone(),
		Duration:        time.Since(st.startTime),
		FirstTokenMs:    st.firstTokenMs,
	}
	ex.UpstreamModel = st.upstreamModel
	return nil
}
