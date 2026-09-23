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
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

func (s *OpenAIGatewayService) forwardOpenAIChatParse(ex *pipeline.Exchange) error {
	st := openaiChatForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse chat completions request: empty state")
	}
	account := ex.Account

	var chatReq apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(st.body, &chatReq); err != nil {
		return fmt.Errorf("parse chat completions request: %w", err)
	}
	st.originalModel = chatReq.Model
	st.clientStream = chatReq.Stream
	st.billingModel = resolveOpenAIForwardModel(account, st.originalModel, st.defaultMappedModel)
	st.upstreamModel = normalizeOpenAIModelForUpstream(account, st.billingModel)

	st.promptCacheKey = strings.TrimSpace(st.promptCacheKey)
	compatPromptCacheInjected := false
	if st.promptCacheKey == "" && account.Type == AccountTypeOAuth && shouldAutoInjectPromptCacheKeyForCompat(st.upstreamModel) {
		st.promptCacheKey = deriveCompatPromptCacheKey(&chatReq, st.upstreamModel)
		compatPromptCacheInjected = st.promptCacheKey != ""
	}

	st.isResponsesShape = !gjson.GetBytes(st.body, "messages").Exists() && gjson.GetBytes(st.body, "input").Exists()

	var (
		responsesReq  *apicompat.ResponsesRequest
		responsesBody []byte
		err           error
	)
	if st.isResponsesShape {
		responsesBody, err = sjson.SetBytes(st.body, "model", st.upstreamModel)
		if err != nil {
			return fmt.Errorf("rewrite model in responses-shape body: %w", err)
		}
		for _, field := range responsesUnsupportedFields {
			if stripped, derr := sjson.DeleteBytes(responsesBody, field); derr == nil {
				responsesBody = stripped
			}
		}
		responsesBody, normalizedServiceTier, err := normalizeResponsesBodyServiceTier(responsesBody)
		if err != nil {
			return fmt.Errorf("normalize service_tier in responses-shape body: %w", err)
		}
		responsesReq = &apicompat.ResponsesRequest{
			Model:       st.upstreamModel,
			ServiceTier: normalizedServiceTier,
		}
		if effort := gjson.GetBytes(responsesBody, "reasoning.effort").String(); effort != "" {
			responsesReq.Reasoning = &apicompat.ResponsesReasoning{Effort: effort}
		}
	} else {
		responsesReq, err = apicompat.ChatCompletionsToResponses(&chatReq)
		if err != nil {
			return fmt.Errorf("convert chat completions to responses: %w", err)
		}
		responsesReq.Model = st.upstreamModel
		normalizeResponsesRequestServiceTier(responsesReq)
		responsesBody, err = json.Marshal(responsesReq)
		if err != nil {
			return fmt.Errorf("marshal responses request: %w", err)
		}
	}
	st.responsesReq = responsesReq
	st.responsesBody = responsesBody

	logFields := []zap.Field{
		zap.Int64("account_id", account.ID),
		zap.String("original_model", st.originalModel),
		zap.String("billing_model", st.billingModel),
		zap.String("upstream_model", st.upstreamModel),
		zap.Bool("stream", st.clientStream),
		zap.Bool("responses_shape", st.isResponsesShape),
	}
	if compatPromptCacheInjected {
		logFields = append(logFields,
			zap.Bool("compat_prompt_cache_key_injected", true),
			zap.String("compat_prompt_cache_key_sha256", hashSensitiveValueForLog(st.promptCacheKey)),
		)
	}
	logger.L().Debug("openai chat_completions: model mapping applied", logFields...)
	ex.RequestedModel = st.originalModel
	ex.UpstreamModel = st.upstreamModel
	ex.Inbound = st.body
	ex.Outbound = st.responsesBody
	return nil
}

func (s *OpenAIGatewayService) forwardOpenAIChatRewrite(ex *pipeline.Exchange) error {
	st := openaiChatForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("rewrite chat completions request: empty state")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	if account.Type == AccountTypeOAuth {
		var reqBody map[string]any
		if err := json.Unmarshal(st.responsesBody, &reqBody); err != nil {
			return fmt.Errorf("unmarshal for codex transform: %w", err)
		}
		codexResult := applyCodexOAuthTransformWithOptions(reqBody, codexOAuthTransformOptions{
			SkipDefaultInstructions: !st.isResponsesShape,
		})
		if !st.isResponsesShape {
			ensureCodexOAuthInstructionsField(reqBody)
		}
		if codexResult.NormalizedModel != "" {
			st.upstreamModel = codexResult.NormalizedModel
		}
		if codexResult.PromptCacheKey != "" {
			st.promptCacheKey = codexResult.PromptCacheKey
		} else if st.promptCacheKey != "" {
			reqBody["prompt_cache_key"] = st.promptCacheKey
		}
		remarshaled, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("remarshal after codex transform: %w", err)
		}
		st.responsesBody = remarshaled
	}

	if account.Type == AccountTypeAPIKey {
		if trimmedKey := strings.TrimSpace(st.promptCacheKey); trimmedKey != "" {
			var reqBody map[string]any
			if err := json.Unmarshal(st.responsesBody, &reqBody); err != nil {
				return fmt.Errorf("unmarshal for prompt cache key injection: %w", err)
			}
			if existing, ok := reqBody["prompt_cache_key"].(string); !ok || strings.TrimSpace(existing) == "" {
				reqBody["prompt_cache_key"] = trimmedKey
				remarshaled, err := json.Marshal(reqBody)
				if err != nil {
					return fmt.Errorf("remarshal after prompt cache key injection: %w", err)
				}
				st.responsesBody = remarshaled
			}
		}
	}

	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, st.upstreamModel, st.responsesBody, forceOpenAIFastFromContext(c))
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			writeChatCompletionsError(c, http.StatusForbidden, "permission_error", blocked.Message)
		}
		return policyErr
	}
	st.responsesBody = updatedBody
	ex.UpstreamModel = st.upstreamModel
	ex.Outbound = st.responsesBody
	return nil
}

func (s *OpenAIGatewayService) forwardOpenAIChatTransport(ex *pipeline.Exchange) error {
	st := openaiChatForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("transport chat completions request: empty state")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := s.buildUpstreamRequest(upstreamCtx, c, account, st.responsesBody, token, true, st.promptCacheKey, false)
	releaseUpstreamCtx()
	if err != nil {
		return fmt.Errorf("build upstream request: %w", err)
	}

	if st.promptCacheKey != "" {
		apiKeyID := getAPIKeyIDFromContext(c)
		upstreamReq.Header.Set("session_id", generateSessionUUID(isolateOpenAISessionID(apiKeyID, st.promptCacheKey)))
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	st.resp = resp

	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if !agentIdentityTaskRecoveryWasTried(ctx) && s.isAgentIdentityAccount(ctx, account) && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, respBody) {
			expectedTaskID := account.GetCredential("task_id")
			if err := s.recoverAgentIdentityTask(ctx, account, expectedTaskID); err != nil {
				return fmt.Errorf("agent identity task recovery failed: %w", err)
			}
			result, recErr := s.ForwardAsChatCompletions(markAgentIdentityTaskRecoveryTried(ctx), c, account, st.body, st.promptCacheKey, st.defaultMappedModel)
			st.result = result
			st.skipRemaining = true
			return recErr
		}
		if account.Type == AccountTypeAPIKey &&
			openai_compat.ResolveResponsesSupport(account.Extra) == openai_compat.ResponsesSupportUnknown &&
			!isResponsesEndpointSupportedByStatus(resp.StatusCode) {
			logger.L().Info("openai chat_completions: /responses unsupported, falling back to raw chat completions",
				zap.Int64("account_id", account.ID),
				zap.Int("upstream_status", resp.StatusCode),
				zap.String("upstream_message", upstreamMsg),
			)
			result, fallbackErr := s.forwardAsRawChatCompletions(ctx, c, account, st.body, st.defaultMappedModel)
			st.result = result
			st.skipRemaining = true
			return fallbackErr
		}
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, st.upstreamModel); foErr != nil {
			return foErr
		}
		result, handleErr := s.handleChatCompletionsErrorResponse(resp, c, account, st.billingModel)
		st.result = result
		st.skipRemaining = true
		return handleErr
	}
	return nil
}

func (s *OpenAIGatewayService) forwardOpenAIChatRelay(ex *pipeline.Exchange) error {
	st := openaiChatForwardStateFrom(ex)
	if st == nil || st.skipRemaining {
		return nil
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	var result *OpenAIForwardResult
	var handleErr error
	if st.clientStream {
		result, handleErr = s.handleChatStreamingResponse(st.resp, c, account, st.originalModel, st.billingModel, st.upstreamModel, st.startTime, len(st.body))
	} else {
		result, handleErr = s.handleChatBufferedStreamingResponse(st.resp, c, account, st.originalModel, st.billingModel, st.upstreamModel, st.startTime)
	}

	if GetOpsCyberPolicy(c) != nil {
		if handleErr == nil {
			handleErr = errOpenAICyberPolicyForwarded
		}
		return handleErr
	}

	if handleErr == nil && result != nil {
		// 请求体声明的档位只作为回退：转发层可能已按上游报告的实际档位覆盖
		// （Fast mode 被降级时上游回 service_tier=default）。
		if tier := extractOpenAIServiceTierFromBody(st.responsesBody); tier != nil {
			result.ServiceTier = billedServiceTier(tier, result.ServiceTier)
		}
		if st.responsesReq != nil && st.responsesReq.Reasoning != nil && st.responsesReq.Reasoning.Effort != "" {
			re := st.responsesReq.Reasoning.Effort
			result.ReasoningEffort = &re
		}
	}

	if handleErr == nil && account.Type == AccountTypeOAuth && !account.IsShadow() {
		if snapshot := ParseCodexRateLimitHeaders(st.resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	}
	if handleErr == nil && st.resp != nil {
		s.dumpOpenAICapacityABHTTP(c, account, st.resp, "http_chat", firstNonEmpty(st.originalModel, st.billingModel, st.upstreamModel), "", nil, true)
	}

	st.result = result
	return handleErr
}

func (s *OpenAIGatewayService) forwardOpenAIChatSettle(ex *pipeline.Exchange) error {
	return nil
}
