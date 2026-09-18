package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"go.uber.org/zap"
)

func (s *OpenAIGatewayService) forwardOpenAIMessagesParse(ex *pipeline.Exchange) error {
	st := openaiMessagesForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse anthropic request: empty state")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(st.body, &anthropicReq); err != nil {
		return fmt.Errorf("parse anthropic request: %w", err)
	}
	anthropicDigestReq := cloneAnthropicRequestForDigest(&anthropicReq)
	st.originalModel = anthropicReq.Model
	applyOpenAICompatModelNormalization(&anthropicReq)
	st.normalizedModel = anthropicReq.Model
	st.clientStream = anthropicReq.Stream

	st.billingModel = resolveOpenAIForwardModel(account, st.normalizedModel, st.defaultMappedModel)
	st.upstreamModel = normalizeOpenAIModelForUpstream(account, st.billingModel)
	st.promptCacheKey = strings.TrimSpace(st.promptCacheKey)
	st.apiKeyID = getAPIKeyIDFromContext(c)
	compatPromptCacheInjected := false
	if st.promptCacheKey == "" && shouldAutoInjectPromptCacheKeyForCompat(st.upstreamModel) {
		st.promptCacheKey = promptCacheKeyFromAnthropicMetadataSession(&anthropicReq)
		if st.promptCacheKey == "" {
			st.promptCacheKey = deriveAnthropicCacheControlPromptCacheKey(&anthropicReq)
		}
		if st.promptCacheKey == "" {
			st.anthropicDigestChain = buildOpenAICompatAnthropicDigestChain(anthropicDigestReq)
			if reusedKey, matchedChain := s.findOpenAICompatAnthropicDigestPromptCacheKey(account, st.apiKeyID, st.anthropicDigestChain); reusedKey != "" {
				st.promptCacheKey = reusedKey
				st.anthropicMatchedDigestChain = matchedChain
			} else {
				st.promptCacheKey = promptCacheKeyFromAnthropicDigest(st.anthropicDigestChain)
			}
		}
		compatPromptCacheInjected = st.promptCacheKey != ""
	}
	compatReplayTrimmed := false
	st.compatReplayGuardEnabled = shouldAutoInjectPromptCacheKeyForCompat(st.upstreamModel)
	st.compatContinuationEnabled = openAICompatContinuationEnabled(account, st.upstreamModel)
	if st.compatContinuationEnabled {
		st.previousResponseID = s.getOpenAICompatSessionResponseID(ctx, c, account, st.promptCacheKey)
	}
	compatContinuationDisabled := st.compatContinuationEnabled &&
		s.isOpenAICompatSessionContinuationDisabled(ctx, c, account, st.promptCacheKey)
	if st.compatReplayGuardEnabled && account.Type != AccountTypeOAuth && st.previousResponseID == "" && !compatContinuationDisabled {
		compatReplayTrimmed = applyAnthropicCompatFullReplayGuard(&anthropicReq)
	}

	responsesReq, err := apicompat.AnthropicToResponses(&anthropicReq)
	if err != nil {
		return fmt.Errorf("convert anthropic to responses: %w", err)
	}
	responsesReq.Stream = true
	st.isStream = true
	if containsBetaToken(c.GetHeader("anthropic-beta"), claude.BetaFastMode) {
		responsesReq.ServiceTier = "priority"
	}
	responsesReq.Model = st.upstreamModel
	if st.previousResponseID != "" {
		responsesReq.PreviousResponseID = st.previousResponseID
		trimAnthropicCompatResponsesInputToLatestTurn(responsesReq)
	}
	if st.compatReplayGuardEnabled && account.Type != AccountTypeOAuth {
		appendOpenAICompatClaudeCodeTodoGuard(responsesReq)
	}
	st.responsesReq = responsesReq

	logFields := []zap.Field{
		zap.Int64("account_id", account.ID),
		zap.String("original_model", st.originalModel),
		zap.String("normalized_model", st.normalizedModel),
		zap.String("billing_model", st.billingModel),
		zap.String("upstream_model", st.upstreamModel),
		zap.Bool("stream", st.isStream),
	}
	if compatPromptCacheInjected {
		logFields = append(logFields,
			zap.Bool("compat_prompt_cache_key_injected", true),
			zap.String("compat_prompt_cache_key_sha256", hashSensitiveValueForLog(st.promptCacheKey)),
		)
	}
	if compatReplayTrimmed {
		logFields = append(logFields,
			zap.Bool("compat_full_replay_trimmed", true),
			zap.Int("compat_messages_after_trim", len(anthropicReq.Messages)),
		)
	}
	if st.previousResponseID != "" {
		logFields = append(logFields,
			zap.Bool("compat_previous_response_id_attached", true),
			zap.String("compat_previous_response_id", truncateOpenAIWSLogValue(st.previousResponseID, openAIWSIDValueMaxLen)),
		)
	}
	if st.compatTurnState != "" {
		logFields = append(logFields, zap.Bool("compat_turn_state_attached", true))
	}
	logger.L().Debug("openai messages: model mapping applied", logFields...)

	responsesBody, err := json.Marshal(responsesReq)
	if err != nil {
		return fmt.Errorf("marshal responses request: %w", err)
	}
	st.responsesBody = responsesBody
	ex.RequestedModel = st.originalModel
	ex.UpstreamModel = st.upstreamModel
	ex.Inbound = st.body
	ex.Outbound = st.responsesBody
	return nil
}

func (s *OpenAIGatewayService) forwardOpenAIMessagesRewrite(ex *pipeline.Exchange) error {
	st := openaiMessagesForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("rewrite anthropic request: empty state")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	if account.Type == AccountTypeOAuth && account.Platform != PlatformGrok {
		var reqBody map[string]any
		if err := json.Unmarshal(st.responsesBody, &reqBody); err != nil {
			return fmt.Errorf("unmarshal for codex transform: %w", err)
		}
		codexResult := applyCodexOAuthTransformWithOptions(reqBody, codexOAuthTransformOptions{
			SkipDefaultInstructions: true,
			PreserveToolCallIDs:     true,
		})
		forcedTemplateText := ""
		if s.cfg != nil {
			forcedTemplateText = s.cfg.Gateway.ForcedCodexInstructionsTemplate
		}
		templateUpstreamModel := st.upstreamModel
		if codexResult.NormalizedModel != "" {
			templateUpstreamModel = codexResult.NormalizedModel
		}
		existingInstructions, _ := reqBody["instructions"].(string)
		if strings.TrimSpace(existingInstructions) == "" {
			existingInstructions = extractPromptLikeInstructionsFromInput(reqBody)
		}
		if _, err := applyForcedCodexInstructionsTemplate(reqBody, forcedTemplateText, forcedCodexInstructionsTemplateData{
			ExistingInstructions: strings.TrimSpace(existingInstructions),
			OriginalModel:        st.originalModel,
			NormalizedModel:      st.normalizedModel,
			BillingModel:         st.billingModel,
			UpstreamModel:        templateUpstreamModel,
		}); err != nil {
			return err
		}
		ensureCodexOAuthInstructionsField(reqBody)
		if shouldAutoInjectPromptCacheKeyForCompat(st.upstreamModel) {
			appendOpenAICompatClaudeCodeTodoGuardToRequestBody(reqBody)
		}
		if codexResult.NormalizedModel != "" {
			st.upstreamModel = codexResult.NormalizedModel
		}
		if codexResult.PromptCacheKey != "" {
			st.promptCacheKey = codexResult.PromptCacheKey
		}
		delete(reqBody, "prompt_cache_key")
		if shouldAutoInjectPromptCacheKeyForCompat(st.upstreamModel) {
			st.compatTurnState = s.getOpenAICompatSessionTurnState(ctx, c, account, st.promptCacheKey)
		}
		st.isStream = true
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
				updated, err := json.Marshal(reqBody)
				if err != nil {
					return fmt.Errorf("remarshal after prompt cache key injection: %w", err)
				}
				st.responsesBody = updated
			}
		}
	}

	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, st.upstreamModel, st.responsesBody, forceOpenAIFastFromContext(c))
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			writeAnthropicError(c, http.StatusForbidden, "forbidden_error", blocked.Message)
		}
		return policyErr
	}
	st.responsesBody = updatedBody
	if account.Platform == PlatformGrok {
		st.grokCacheIdentity = resolveGrokCacheIdentity(c, st.responsesBody, st.promptCacheKey, st.upstreamModel)
		patchedBody, patchErr := patchGrokResponsesBody(st.responsesBody, st.upstreamModel)
		if patchErr != nil {
			return patchErr
		}
		st.responsesBody, patchErr = applyGrokResponsesCacheIdentity(patchedBody, st.responsesBody, st.grokCacheIdentity, account.IsGrokOAuth())
		if patchErr != nil {
			return fmt.Errorf("apply grok prompt cache identity: %w", patchErr)
		}
	}
	ex.UpstreamModel = st.upstreamModel
	ex.Outbound = st.responsesBody
	return nil
}

func (s *OpenAIGatewayService) forwardOpenAIMessagesTransport(ex *pipeline.Exchange) error {
	st := openaiMessagesForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("transport anthropic request: empty state")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	if account.Type == AccountTypeOAuth && account.Platform != PlatformGrok {
		setOpenAICompatMessagesBridgeContext(c, true)
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	var upstreamReq *http.Request
	if account.Platform == PlatformGrok {
		upstreamReq, err = buildGrokResponsesRequest(upstreamCtx, c, account, st.responsesBody, token, st.grokCacheIdentity)
	} else {
		upstreamReq, err = s.buildUpstreamRequest(upstreamCtx, c, account, st.responsesBody, token, st.isStream, st.promptCacheKey, false)
	}
	releaseUpstreamCtx()
	if err != nil {
		return fmt.Errorf("build upstream request: %w", err)
	}

	if account.Platform != PlatformGrok && st.promptCacheKey != "" {
		isolatedSessionID := generateSessionUUID(isolateOpenAISessionID(st.apiKeyID, st.promptCacheKey))
		upstreamReq.Header.Set("session_id", isolatedSessionID)
		if upstreamReq.Header.Get("conversation_id") != "" {
			upstreamReq.Header.Set("conversation_id", isolatedSessionID)
		}
	}
	if account.Type == AccountTypeOAuth && account.Platform != PlatformGrok {
		s.codexIdentity.ensureHeaders(upstreamReq.Header)
		s.codexIdentity.enforceHeaders(upstreamReq.Header)
		logger.L().Debug("openai messages: upstream identity restored",
			zap.Int64("account_id", account.ID),
			zap.String("upstream_model", st.upstreamModel),
			zap.Bool("compat_identity_restored", true),
		)
	}
	if account.Type == AccountTypeOAuth && st.promptCacheKey != "" && strings.TrimSpace(c.GetHeader("conversation_id")) == "" {
		upstreamReq.Header.Del("conversation_id")
	}
	if st.compatTurnState != "" && upstreamReq.Header.Get("x-codex-turn-state") == "" {
		upstreamReq.Header.Set("x-codex-turn-state", st.compatTurnState)
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
			result, recErr := s.ForwardAsAnthropic(markAgentIdentityTaskRecoveryTried(ctx), c, account, st.body, st.promptCacheKey, st.defaultMappedModel)
			st.result = result
			st.skipRemaining = true
			return recErr
		}
		if account.Platform == PlatformGrok {
			s.updateGrokUsageSnapshot(ctx, account, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
			s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		}

		if st.previousResponseID != "" && (isOpenAICompatPreviousResponseNotFound(resp.StatusCode, upstreamMsg, respBody) || isOpenAICompatPreviousResponseUnsupported(resp.StatusCode, upstreamMsg, respBody)) {
			if isOpenAICompatPreviousResponseUnsupported(resp.StatusCode, upstreamMsg, respBody) {
				s.disableOpenAICompatSessionContinuation(ctx, c, account, st.promptCacheKey)
			} else {
				s.deleteOpenAICompatSessionResponseID(ctx, c, account, st.promptCacheKey)
			}
			logger.L().Info("openai messages: previous_response_id unavailable, retrying without continuation",
				zap.Int64("account_id", account.ID),
				zap.String("previous_response_id", truncateOpenAIWSLogValue(st.previousResponseID, openAIWSIDValueMaxLen)),
				zap.String("upstream_model", st.upstreamModel),
			)
			result, retryErr := s.ForwardAsAnthropic(ctx, c, account, st.body, st.promptCacheKey, st.defaultMappedModel)
			st.result = result
			st.skipRemaining = true
			return retryErr
		}
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, st.upstreamModel); foErr != nil {
			return foErr
		}
		result, handleErr := s.handleAnthropicErrorResponse(resp, c, account, st.billingModel)
		st.result = result
		st.skipRemaining = true
		return handleErr
	}

	if account.Type == AccountTypeOAuth && st.promptCacheKey != "" {
		if turnState := strings.TrimSpace(resp.Header.Get("x-codex-turn-state")); turnState != "" {
			s.bindOpenAICompatSessionTurnState(ctx, c, account, st.promptCacheKey, turnState)
		}
	}
	return nil
}

func (s *OpenAIGatewayService) forwardOpenAIMessagesRelay(ex *pipeline.Exchange) error {
	st := openaiMessagesForwardStateFrom(ex)
	if st == nil || st.skipRemaining {
		return nil
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	var result *OpenAIForwardResult
	var handleErr error
	if st.clientStream {
		result, handleErr = s.handleAnthropicStreamingResponse(st.resp, c, account, st.originalModel, st.billingModel, st.upstreamModel, st.startTime)
	} else {
		result, handleErr = s.handleAnthropicBufferedStreamingResponse(st.resp, c, account, st.originalModel, st.billingModel, st.upstreamModel, st.startTime)
	}

	if GetOpsCyberPolicy(c) != nil {
		if handleErr == nil {
			handleErr = errOpenAICyberPolicyForwarded
		}
		return handleErr
	}

	if handleErr == nil && result != nil {
		if st.compatContinuationEnabled && st.promptCacheKey != "" && result.ResponseID != "" {
			s.bindOpenAICompatSessionResponseID(ctx, c, account, st.promptCacheKey, result.ResponseID)
		}
		if st.promptCacheKey != "" && st.anthropicDigestChain != "" {
			s.bindOpenAICompatAnthropicDigestPromptCacheKey(account, st.apiKeyID, st.anthropicDigestChain, st.promptCacheKey, st.anthropicMatchedDigestChain)
		}
		if tier := extractOpenAIServiceTierFromBody(st.responsesBody); tier != nil {
			result.ServiceTier = tier
		}
		if st.responsesReq != nil && st.responsesReq.Reasoning != nil && st.responsesReq.Reasoning.Effort != "" {
			re := st.responsesReq.Reasoning.Effort
			result.ReasoningEffort = &re
		}
	}

	if handleErr == nil && account.Type == AccountTypeOAuth && !account.IsShadow() {
		if account.Platform == PlatformGrok {
			s.updateGrokUsageSnapshot(ctx, account, xai.ParseQuotaHeaders(st.resp.Header, st.resp.StatusCode))
		} else if snapshot := ParseCodexRateLimitHeaders(st.resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	}
	if handleErr == nil && st.resp != nil && account.Platform == PlatformOpenAI {
		s.dumpOpenAICapacityABHTTP(st.c, account, st.resp, "http_messages", firstNonEmpty(st.originalModel, st.billingModel, st.upstreamModel), "", nil, true)
	}

	st.result = result
	return handleErr
}

func (s *OpenAIGatewayService) forwardOpenAIMessagesSettle(ex *pipeline.Exchange) error {
	return nil
}
