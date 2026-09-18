package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func (s *OpenAIGatewayService) forwardOpenAIParse(ex *pipeline.Exchange) error {
	st := openaiForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse request: empty request")
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

func (s *OpenAIGatewayService) forwardOpenAIRewrite(ex *pipeline.Exchange) error {
	st := openaiForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse request: empty request")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	apiKey := getAPIKeyFromContext(c)
	st.imageGenerationAllowed = GroupAllowsImageGeneration(nil)
	if apiKey != nil {
		st.imageGenerationAllowed = GroupAllowsImageGeneration(apiKey.Group)
	}
	st.codexImageGenerationBridgeEnabled = st.isCodexCLI &&
		!isOpenAIResponsesLiteHeader(c.GetHeader(responsesLiteHeader)) &&
		st.imageGenerationAllowed &&
		st.codexImageGenerationExplicitToolPolicy != codexImageGenerationExplicitToolPolicyStrip &&
		s.isCodexImageGenerationBridgeEnabled(ctx, account, apiKey)
	if st.isCodexCLI && st.codexImageGenerationExplicitToolPolicy == codexImageGenerationExplicitToolPolicyStrip {
		decoded, decodeErr := st.ensureReqBody()
		if decodeErr != nil {
			return decodeErr
		}
		if stripOpenAIImageGenerationTools(decoded) {
			st.markDecodedModified()
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Stripped /responses image_generation tool for Codex client by account policy")
		}
		st.imageIntent = IsImageGenerationIntentMap(openAIResponsesEndpoint, st.reqModel, decoded)
	} else {
		st.imageIntent = IsImageGenerationIntent(openAIResponsesEndpoint, st.reqModel, st.body)
	}
	if st.imageIntent && !st.imageGenerationAllowed {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"type": "permission_error", "message": ImageGenerationPermissionMessage()}})
		return errors.New("image generation disabled for group")
	}

	instructions := gjson.GetBytes(st.body, "instructions")
	instructionsEmpty := !instructions.Exists() || instructions.Type != gjson.String || strings.TrimSpace(instructions.String()) == ""
	if instructionsEmpty && !st.compatMessagesBridge {
		st.markPatchSet("instructions", defaultCodexSynthInstructions(st.reqModel))
	}

	st.billingModel = account.GetMappedModel(st.reqModel)
	if st.billingModel != st.reqModel {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Model mapping applied: %s -> %s (account: %s, isCodexCLI: %v)", st.reqModel, st.billingModel, account.Name, st.isCodexCLI)
		st.reqModel = st.billingModel
		st.markPatchSet("model", st.billingModel)
	}
	st.upstreamModel = st.billingModel
	st.isCompactRequest = isOpenAICompactUpstreamRequest(c)
	compactMapped := false
	if st.isCompactRequest {
		compactMappedModel := resolveOpenAICompactForwardModel(account, st.billingModel)
		if compactMappedModel != "" && compactMappedModel != st.billingModel {
			compactMapped = true
			st.upstreamModel = compactMappedModel
			st.reqModel = compactMappedModel
			st.markPatchSet("model", compactMappedModel)
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Compact model mapping applied: %s -> %s (account: %s, isCodexCLI: %v)", st.billingModel, compactMappedModel, account.Name, st.isCodexCLI)
		}
	}
	if !compactMapped {
		modelForNormalize := st.reqModel
		if modelForNormalize == "" {
			modelForNormalize = st.requestView.Model
		}
		st.upstreamModel = normalizeOpenAIModelForUpstream(account, modelForNormalize)
		if st.upstreamModel != "" && st.upstreamModel != modelForNormalize {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Upstream model resolved: %s -> %s (account: %s, type: %s, isCodexCLI: %v)", modelForNormalize, st.upstreamModel, account.Name, account.Type, st.isCodexCLI)
			st.reqModel = st.upstreamModel
			st.markPatchSet("model", st.upstreamModel)
		}
	}
	// Responses-Lite is a (slug, header, body) pair. Mapping a Lite model onto a
	// non-Lite upstream (e.g. sol → gpt-5.5) requires demoting body shape and
	// stripping the Lite header before the upstream call. Mutate via ensureReqBody
	// so earlier model/instructions patches stay on the same decoded map.
	if shouldDemoteOpenAIResponsesLite(isOpenAIResponsesLiteInbound(c, st.body), st.upstreamModel) {
		decoded, decodeErr := st.ensureReqBody()
		if decodeErr != nil {
			return decodeErr
		}
		if st.upstreamModel != "" {
			decoded["model"] = st.upstreamModel
			st.markDecodedModified()
		}
		if demoteOpenAIResponsesLiteRequestMap(decoded) {
			st.markDecodedModified()
		}
		markOpenAIResponsesLiteDemoted(c)
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Demoted Responses-Lite request for non-lite upstream model=%s (account: %s)", st.upstreamModel, account.Name)
	}
	if strings.TrimSpace(gjson.GetBytes(st.body, "reasoning.effort").String()) == "minimal" {
		st.markPatchSet("reasoning.effort", "none")
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Normalized reasoning.effort: minimal -> none (account: %s)", account.Name)
	}

	st.imageIntent = st.imageIntent || IsImageGenerationIntent(openAIResponsesEndpoint, st.reqModel, nil) || isOpenAIImageGenerationModel(st.upstreamModel)
	if st.imageIntent && !st.imageGenerationAllowed {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"type": "permission_error", "message": ImageGenerationPermissionMessage()}})
		return errors.New("image generation disabled for group")
	}

	if st.imageGenerationAllowed && !st.isCompactRequest && (st.codexImageGenerationBridgeEnabled || isOpenAIImageGenerationModel(st.requestView.Model) || openAIRequestBodyImageGenerationToolNeedsNormalization(st.body) || isOpenAIImageGenerationModel(st.upstreamModel)) {
		decoded, decodeErr := st.ensureReqBody()
		if decodeErr != nil {
			return decodeErr
		}
		if st.codexImageGenerationBridgeEnabled && ensureOpenAIResponsesImageGenerationTool(decoded) {
			st.markDecodedModified()
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Injected /responses image_generation tool for Codex client")
		}
		if st.codexImageGenerationBridgeEnabled && ensureOpenAIResponsesImageGenerationToolChoiceAuto(decoded) {
			st.markDecodedModified()
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Set /responses image_generation tool_choice=auto for Codex client")
		}
		if normalizeOpenAIResponsesImageGenerationTools(decoded) {
			st.markDecodedModified()
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Normalized /responses image_generation tool payload")
		}
		if normalizeOpenAIResponsesImageOnlyModel(decoded) {
			st.markDecodedModified()
			if model, ok := decoded["model"].(string); ok {
				st.upstreamModel = strings.TrimSpace(model)
			}
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Normalized /responses image-only model request inbound_model=%s image_model=%s upstream_model=%s", st.requestView.Model, st.billingModel, st.upstreamModel)
		}
		if err := validateOpenAIResponsesImageModel(decoded, st.upstreamModel); err != nil {
			setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": err.Error(), "param": "model"}})
			return err
		}
		if hasOpenAIImageGenerationTool(decoded) {
			st.imageIntent = true
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] /responses image_generation request inbound_model=%s mapped_model=%s account_type=%s", st.requestView.Model, st.upstreamModel, account.Type)
		}
		if st.codexImageGenerationBridgeEnabled && applyCodexImageGenerationBridgeInstructions(decoded) {
			st.markDecodedModified()
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Added Codex image_generation bridge instructions")
		}
	} else if st.imageGenerationAllowed && st.imageIntent && openAIRequestBodyHasImageGenerationDeclaration(st.body) {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] /responses image_generation request inbound_model=%s mapped_model=%s account_type=%s", st.requestView.Model, st.upstreamModel, account.Type)
	}

	if isCodexSparkModel(st.upstreamModel) && openAIRequestBodyMayContainImageInput(st.body) {
		decoded, decodeErr := st.ensureReqBody()
		if decodeErr != nil {
			return decodeErr
		}
		if err := validateCodexSparkInput(decoded, st.upstreamModel); err != nil {
			setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": err.Error(), "param": "input"}})
			return err
		}
	}

	if isCodexSparkModel(st.upstreamModel) && openAIRequestBodyHasImageGenerationDeclaration(st.body) {
		decoded, decodeErr := st.ensureReqBody()
		if decodeErr != nil {
			return decodeErr
		}
		if stripCodexSparkImageGenerationTools(decoded) {
			st.markDecodedModified()
		}
	}

	if account.Type == AccountTypeOAuth {
		decoded, decodeErr := st.ensureReqBody()
		if decodeErr != nil {
			return decodeErr
		}
		codexResult := codexTransformResult{}
		if st.compatMessagesBridge {
			codexResult = applyCodexOAuthTransformWithOptions(decoded, codexOAuthTransformOptions{IsCodexCLI: st.isCodexCLI, IsCompact: st.isCompactRequest, SkipDefaultInstructions: true, PreserveToolCallIDs: true})
			ensureCodexOAuthInstructionsField(decoded)
			st.markDecodedModified()
		} else {
			codexResult = applyCodexOAuthTransform(decoded, st.isCodexCLI, st.isCompactRequest)
		}
		if codexResult.Modified {
			st.markDecodedModified()
		}
		if !st.isCompactRequest && applyCodexClientMetadata(decoded, account) {
			st.markDecodedModified()
		}
		if !st.isCompactRequest {
			fingerprintIDs := resolveCodexFingerprintIDsFromContext(c, account)
			if applyCodexFingerprintClientMetadata(decoded, fingerprintIDs) {
				st.markDecodedModified()
			}
		}
		if codexResult.NormalizedModel != "" {
			st.upstreamModel = codexResult.NormalizedModel
		}
		if codexResult.PromptCacheKey != "" {
			st.promptCacheKey = codexResult.PromptCacheKey
		}
	}

	if !SupportsVerbosity(st.upstreamModel) && gjson.GetBytes(st.body, "text.verbosity").Exists() {
		st.markPatchDelete("text.verbosity")
	}

	if !st.isCodexCLI {
		maxOutputTokens := gjson.GetBytes(st.body, "max_output_tokens")
		if maxOutputTokens.Exists() {
			switch account.Platform {
			case PlatformOpenAI:
				st.markPatchDelete("max_output_tokens")
			case PlatformAnthropic:
				decoded, decodeErr := st.ensureReqBody()
				if decodeErr != nil {
					return decodeErr
				}
				delete(decoded, "max_output_tokens")
				if _, hasMaxTokens := decoded["max_tokens"]; !hasMaxTokens {
					decoded["max_tokens"] = maxOutputTokens.Value()
				}
				st.markDecodedModified()
			case PlatformGemini:
				st.markPatchDelete("max_output_tokens")
			default:
				st.markPatchDelete("max_output_tokens")
			}
		}
		if gjson.GetBytes(st.body, "max_completion_tokens").Exists() && (account.Type == AccountTypeAPIKey || account.Platform != PlatformOpenAI) {
			st.markPatchDelete("max_completion_tokens")
		}
		for _, unsupportedField := range []string{"prompt_cache_retention", "safety_identifier"} {
			if gjson.GetBytes(st.body, unsupportedField).Exists() {
				st.markPatchDelete(unsupportedField)
			}
		}
	}
	if st.wsDecision.Transport != OpenAIUpstreamTransportResponsesWebsocketV2 && gjson.GetBytes(st.body, "previous_response_id").Exists() {
		st.markPatchDelete("previous_response_id")
	}
	if openAIRequestBodyMayContainEmptyBase64InputImage(st.body) {
		decoded, decodeErr := st.ensureReqBody()
		if decodeErr != nil {
			return decodeErr
		}
		if sanitizeEmptyBase64InputImagesInOpenAIRequestBodyMap(decoded) {
			st.markDecodedModified()
		}
	}

	{
		rawTier := st.requestView.ServiceTier
		setTier, deleteTier, blockMsg := s.resolveOpenAIFastPolicyMutation(
			ctx, account, st.upstreamModel, rawTier, forceOpenAIFastFromContext(c),
		)
		if blockMsg != "" {
			blocked := &OpenAIFastBlockedError{Message: blockMsg}
			writeOpenAIFastPolicyBlockedResponse(c, blocked)
			return blocked
		}
		if deleteTier {
			st.markPatchDelete("service_tier")
		} else if setTier != "" && setTier != rawTier {
			st.markPatchSet("service_tier", setTier)
		}
	}

	if st.bodyModified {
		if st.requestView.HasPatches() {
			if patchedBody, patchErr := st.requestView.ApplyPatches(); patchErr == nil {
				st.body = patchedBody
				st.requestView = newOpenAIRequestView(st.body)
				st.reqBody = nil
				st.bodyModified = false
			}
		}
		if st.bodyModified {
			decoded, decodeErr := st.ensureReqBody()
			if decodeErr != nil {
				return decodeErr
			}
			var marshalErr error
			st.body, marshalErr = marshalOpenAIUpstreamJSON(decoded)
			if marshalErr != nil {
				return fmt.Errorf("serialize request body: %w", marshalErr)
			}
			st.requestView = newOpenAIRequestView(st.body)
		}
	}
	if st.imageIntent {
		var imageCfg OpenAIResponsesImageBillingConfig
		var imageCfgErr error
		if st.reqBody != nil {
			imageCfg, imageCfgErr = resolveOpenAIResponsesImageBillingConfigDetailed(st.reqBody, st.billingModel)
		} else {
			imageCfg, imageCfgErr = resolveOpenAIResponsesImageBillingConfigDetailedFromBody(st.body, st.billingModel)
		}
		if imageCfgErr != nil {
			setOpsUpstreamError(c, http.StatusBadRequest, imageCfgErr.Error(), "")
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": imageCfgErr.Error(), "param": "size"}})
			return imageCfgErr
		}
		st.imageBillingModel = imageCfg.Model
		st.imageSizeTier = imageCfg.SizeTier
		st.imageInputSize = imageCfg.InputSize
	}

	ex.UpstreamModel = st.upstreamModel
	ex.Inbound = st.body
	return nil
}

func (s *OpenAIGatewayService) forwardOpenAITransport(ex *pipeline.Exchange) error {
	st := openaiForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse request: empty request")
	}
	account := ex.Account
	var err error
	st.token, _, err = s.GetAccessToken(ex.Ctx, account)
	if err != nil {
		return err
	}

	// 命中 WS 时仅走 WebSocket Mode；不再自动回退 HTTP。
	if st.wsDecision.Transport == OpenAIUpstreamTransportResponsesWebsocketV2 {
		// WS 分支需要结构化 payload 与重连恢复，命中后再触发 full-map decode。
		wsReqBody, err := st.ensureReqBody()
		if err != nil {
			return err
		}
		_, hasPreviousResponseID := wsReqBody["previous_response_id"]
		logOpenAIWSModeDebug(
			"forward_start account_id=%d account_type=%s model=%s stream=%v has_previous_response_id=%v",
			account.ID,
			account.Type,
			st.upstreamModel,
			st.reqStream,
			hasPreviousResponseID,
		)
		maxAttempts := openAIWSReconnectRetryLimit + 1
		wsAttempts := 0
		var wsResult *OpenAIForwardResult
		var wsErr error
		wsLastFailureReason := ""
		agentTaskRecoveryTried := false
		wsPrevResponseRecoveryTried := false
		wsInvalidEncryptedContentRecoveryTried := false
		recoverPrevResponseNotFound := func(attempt int) bool {
			if wsPrevResponseRecoveryTried {
				return false
			}
			previousResponseID := openAIWSPayloadString(wsReqBody, "previous_response_id")
			if previousResponseID == "" {
				logOpenAIWSModeInfo(
					"reconnect_prev_response_recovery_skip account_id=%d attempt=%d reason=missing_previous_response_id previous_response_id_present=false",
					account.ID,
					attempt,
				)
				return false
			}
			if HasFunctionCallOutput(wsReqBody) {
				logOpenAIWSModeInfo(
					"reconnect_prev_response_recovery_skip account_id=%d attempt=%d reason=has_function_call_output previous_response_id_present=true",
					account.ID,
					attempt,
				)
				return false
			}
			delete(wsReqBody, "previous_response_id")
			wsPrevResponseRecoveryTried = true
			logOpenAIWSModeInfo(
				"reconnect_prev_response_recovery account_id=%d attempt=%d action=drop_previous_response_id retry=1 previous_response_id=%s previous_response_id_kind=%s",
				account.ID,
				attempt,
				truncateOpenAIWSLogValue(previousResponseID, openAIWSIDValueMaxLen),
				normalizeOpenAIWSLogValue(ClassifyOpenAIPreviousResponseIDKind(previousResponseID)),
			)
			return true
		}
		recoverInvalidEncryptedContent := func(attempt int) bool {
			if wsInvalidEncryptedContentRecoveryTried {
				return false
			}
			removedReasoningItems := trimOpenAIEncryptedReasoningItems(wsReqBody)
			if !removedReasoningItems {
				logOpenAIWSModeInfo(
					"reconnect_invalid_encrypted_content_recovery_skip account_id=%d attempt=%d reason=missing_encrypted_reasoning_items",
					account.ID,
					attempt,
				)
				return false
			}
			previousResponseID := openAIWSPayloadString(wsReqBody, "previous_response_id")
			hasFunctionCallOutput := HasFunctionCallOutput(wsReqBody)
			if previousResponseID != "" && !hasFunctionCallOutput {
				delete(wsReqBody, "previous_response_id")
			}
			wsInvalidEncryptedContentRecoveryTried = true
			logOpenAIWSModeInfo(
				"reconnect_invalid_encrypted_content_recovery account_id=%d attempt=%d action=drop_encrypted_reasoning_items retry=1 previous_response_id_present=%v previous_response_id=%s previous_response_id_kind=%s has_function_call_output=%v dropped_previous_response_id=%v",
				account.ID,
				attempt,
				previousResponseID != "",
				truncateOpenAIWSLogValue(previousResponseID, openAIWSIDValueMaxLen),
				normalizeOpenAIWSLogValue(ClassifyOpenAIPreviousResponseIDKind(previousResponseID)),
				hasFunctionCallOutput,
				previousResponseID != "" && !hasFunctionCallOutput,
			)
			return true
		}
		retryBudget := s.openAIWSRetryTotalBudget()
		retryStartedAt := time.Now()
	wsRetryLoop:
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			wsAttempts = attempt
			wsResult, wsErr = s.forwardOpenAIWSV2(
				ex.Ctx,
				st.c,
				account,
				wsReqBody,
				st.token,
				st.wsDecision,
				st.isCodexCLI,
				st.reqStream,
				st.originalModel,
				st.upstreamModel,
				st.startTime,
				attempt,
				wsLastFailureReason,
				&agentTaskRecoveryTried,
			)
			if wsErr == nil {
				break
			}
			if st.c != nil && st.c.Writer != nil && st.c.Writer.Written() {
				break
			}
			var taskRecoveredErr *agentIdentityTaskRecoveredError
			if errors.As(wsErr, &taskRecoveredErr) {
				continue
			}

			reason, retryable := classifyOpenAIWSReconnectReason(wsErr)
			if reason != "" {
				wsLastFailureReason = reason
			}
			// previous_response_not_found 说明续链锚点不可用：
			// 对非 function_call_output 场景，允许一次“去掉 previous_response_id 后重放”。
			if reason == "previous_response_not_found" && recoverPrevResponseNotFound(attempt) {
				continue
			}
			if reason == "invalid_encrypted_content" && recoverInvalidEncryptedContent(attempt) {
				continue
			}
			if retryable && attempt < maxAttempts {
				backoff := s.openAIWSRetryBackoff(attempt)
				if retryBudget > 0 && time.Since(retryStartedAt)+backoff > retryBudget {
					s.recordOpenAIWSRetryExhausted()
					logOpenAIWSModeInfo(
						"reconnect_budget_exhausted account_id=%d attempts=%d max_retries=%d reason=%s elapsed_ms=%d budget_ms=%d",
						account.ID,
						attempt,
						openAIWSReconnectRetryLimit,
						normalizeOpenAIWSLogValue(reason),
						time.Since(retryStartedAt).Milliseconds(),
						retryBudget.Milliseconds(),
					)
					break
				}
				s.recordOpenAIWSRetryAttempt(backoff)
				logOpenAIWSModeInfo(
					"reconnect_retry account_id=%d retry=%d max_retries=%d reason=%s backoff_ms=%d",
					account.ID,
					attempt,
					openAIWSReconnectRetryLimit,
					normalizeOpenAIWSLogValue(reason),
					backoff.Milliseconds(),
				)
				if backoff > 0 {
					timer := time.NewTimer(backoff)
					select {
					case <-ex.Ctx.Done():
						if !timer.Stop() {
							<-timer.C
						}
						wsErr = wrapOpenAIWSFallback("retry_backoff_canceled", ex.Ctx.Err())
						break wsRetryLoop
					case <-timer.C:
					}
				}
				continue
			}
			if retryable {
				s.recordOpenAIWSRetryExhausted()
				logOpenAIWSModeInfo(
					"reconnect_exhausted account_id=%d attempts=%d max_retries=%d reason=%s",
					account.ID,
					attempt,
					openAIWSReconnectRetryLimit,
					normalizeOpenAIWSLogValue(reason),
				)
			} else if reason != "" {
				s.recordOpenAIWSNonRetryableFastFallback()
				logOpenAIWSModeInfo(
					"reconnect_stop account_id=%d attempt=%d reason=%s",
					account.ID,
					attempt,
					normalizeOpenAIWSLogValue(reason),
				)
			}
			break
		}
		if wsErr == nil {
			wsFirstTokenMs := int64(0)
			hasFirstTokenMs := wsResult != nil && wsResult.FirstTokenMs != nil
			if hasFirstTokenMs {
				wsFirstTokenMs = int64(*wsResult.FirstTokenMs)
			}
			requestID := ""
			if wsResult != nil {
				requestID = strings.TrimSpace(wsResult.RequestID)
			}
			logOpenAIWSModeDebug(
				"forward_succeeded account_id=%d request_id=%s stream=%v has_first_token_ms=%v first_token_ms=%d ws_attempts=%d",
				account.ID,
				requestID,
				st.reqStream,
				hasFirstTokenMs,
				wsFirstTokenMs,
				wsAttempts,
			)
			wsResult.UpstreamModel = st.upstreamModel
			if wsResult.BillingModel == "" {
				wsResult.BillingModel = st.billingModel
			}
			if wsResult.ImageCount > 0 {
				wsResult.ImageSize = st.imageSizeTier
				wsResult.ImageInputSize = st.imageInputSize
				wsResult.BillingModel = st.imageBillingModel
			}
			st.result = wsResult
			st.skipRemaining = true
			return nil
		}
		s.writeOpenAIWSFallbackErrorResponse(st.c, account, wsErr)
		return wsErr
	}

	// Body-signal v2: client stays on streaming /responses, upstream uses legacy
	// /responses/compact JSON, then we bridge to Codex v2 SSE terminal events.
	upstreamBody := st.body
	upstreamStream := st.reqStream
	if st.bodySignalCompactV2 {
		prepared, prepErr := prepareOpenAIBodySignalV2UpstreamBody(st.body)
		if prepErr != nil {
			return prepErr
		}
		upstreamBody = prepared
		upstreamStream = false
		st.reqStream = true // client contract remains SSE
	}

	httpInvalidEncryptedContentRetryTried := false
	rejectedFieldRetryState := newOpenAIResponsesRejectedFieldRetryState(st.body)
	agentTaskRecoveryTried := false
	// compact v2 每轮重试都会 detach 出一个带软超时的 upstream context。用单一变量跟踪，
	// 下一轮开始时释放上一轮、函数返回时释放最后一轮，避免 defer 在循环内堆积 cancel/timer。
	var pendingCompactCancel context.CancelFunc
	defer func() {
		if pendingCompactCancel != nil {
			pendingCompactCancel()
		}
	}()
	for {
		// Build upstream request.
		// Body-signal v2 keeps the compact soft-timeout deadline but detaches from
		// client cancellation so upstream success can still be drained for billing.
		// Other paths still detach so client disconnect can finish upstream for billing.
		var upstreamReq *http.Request
		var err error
		var upstreamCtx context.Context
		var headerGuard *openAIFirstOutputHeaderGuard
		if st.bodySignalCompactV2 {
			// 释放上一轮重试残留的 detached context，最后一轮由函数返回时的 defer 兜底。
			if pendingCompactCancel != nil {
				pendingCompactCancel()
				pendingCompactCancel = nil
			}
			upstreamCtx, pendingCompactCancel = detachOpenAICompactUpstreamContext(ex.Ctx)
			upstreamReq, err = s.buildUpstreamRequest(upstreamCtx, st.c, account, upstreamBody, st.token, upstreamStream, st.promptCacheKey, st.isCodexCLI)
		} else {
			var releaseUpstreamCtx context.CancelFunc
			upstreamCtx, releaseUpstreamCtx = detachUpstreamContext(ex.Ctx)
			// Opt-in first-output budget also covers waiting for response headers so a
			// stalled HTTP/SSE attempt can fail over before any client bytes are written.
			if st.reqStream && account.Platform == PlatformOpenAI {
				if timeout := s.openAIFirstOutputTimeout(""); timeout > 0 {
					upstreamCtx, headerGuard = newOpenAIFirstOutputHeaderGuard(
						upstreamCtx, releaseUpstreamCtx, st.startTime.Add(timeout),
					)
				}
			}
			upstreamReq, err = s.buildUpstreamRequest(upstreamCtx, st.c, account, upstreamBody, st.token, upstreamStream, st.promptCacheKey, st.isCodexCLI)
			if headerGuard == nil {
				releaseUpstreamCtx()
			}
		}
		if err != nil {
			if headerGuard != nil {
				headerGuard.close()
			}
			return err
		}

		// Get proxy URL
		proxyURL := ""
		if account.ProxyID != nil && account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		}

		// Send request
		upstreamStart := time.Now()
		markFirstOutputConnectionStart(ex.Ctx)
		trace := &httptrace.ClientTrace{
			WroteRequest: func(httptrace.WroteRequestInfo) {
				markFirstOutputRequestWritten(ex.Ctx)
			},
		}
		upstreamReq = upstreamReq.WithContext(httptrace.WithClientTrace(upstreamReq.Context(), trace))
		var resp *http.Response
		if st.bodySignalCompactV2 {
			// Start downstream SSE+keepalive before headers arrive; legacy compact
			// often withholds response headers until compaction finishes.
			resp, err = s.doOpenAIUpstreamWithCompactV2Keepalive(upstreamCtx, st.c, upstreamReq, proxyURL, account)
		} else {
			resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		}
		SetOpsLatencyMs(st.c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if headerGuard != nil && headerGuard.stopHeaderWait() {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			headerGuard.close()
			return s.newOpenAIFirstOutputTimeoutError(
				ex.Ctx, st.c, account, st.startTime, st.originalModel, "",
				s.openAIFirstOutputTimeout(""), "response_headers", nil,
			)
		}
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			if headerGuard != nil {
				headerGuard.close()
			}
			// Soft-timeout / client cancel while waiting for unary compact.
			if st.bodySignalCompactV2 && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				return err
			}
			// Transport-level failure (proxy/DNS/TCP/TLS — no HTTP response). Convert to
			// a failover so the handler switches to a healthy account, and temporarily
			// unschedule the account on durable faults (e.g. rejected proxy credentials).
			return s.handleOpenAIUpstreamTransportError(ex.Ctx, st.c, account, err, false)
		}
		if headerGuard != nil {
			resp.Body = &openAIRequestContextReadCloser{ReadCloser: resp.Body, cleanup: headerGuard.close}
		}

		// Handle error response
		if resp.StatusCode >= 400 {
			respBody := s.readUpstreamErrorBody(resp)
			_ = resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(respBody))

			upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
			upstreamCode := extractUpstreamErrorCode(respBody)
			if !agentTaskRecoveryTried && s.isAgentIdentityAccount(ex.Ctx, account) && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, respBody) {
				markFirstOutputAttemptFailed(ex.Ctx)
				agentTaskRecoveryTried = true
				expectedTaskID := account.GetCredential("task_id")
				if err := s.recoverAgentIdentityTask(ex.Ctx, account, expectedTaskID); err != nil {
					return fmt.Errorf("agent identity task recovery failed: %w", err)
				}
				continue
			}
			respBody = s.redactAgentIdentitySensitiveBody(ex.Ctx, account, respBody)
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			if !httpInvalidEncryptedContentRetryTried && resp.StatusCode == http.StatusBadRequest && upstreamCode == "invalid_encrypted_content" {
				markFirstOutputAttemptFailed(ex.Ctx)
				decoded, decodeErr := st.ensureReqBody()
				if decodeErr != nil {
					return decodeErr
				}
				if trimOpenAIEncryptedReasoningItems(decoded) {
					st.body, err = marshalOpenAIUpstreamJSON(decoded)
					if err != nil {
						return fmt.Errorf("serialize invalid_encrypted_content retry st.body: %w", err)
					}
					upstreamBody = st.body
					if st.bodySignalCompactV2 {
						prepared, prepErr := prepareOpenAIBodySignalV2UpstreamBody(st.body)
						if prepErr != nil {
							return prepErr
						}
						upstreamBody = prepared
					}
					httpInvalidEncryptedContentRetryTried = true
					rejectedFieldRetryState.remember(st.body)
					logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Retrying non-WSv2 request once after invalid_encrypted_content (account: %s)", account.Name)
					continue
				}
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Skip non-WSv2 invalid_encrypted_content retry because encrypted reasoning items are missing (account: %s)", account.Name)
			}
			if retryBody, reason, changed, retryErr := normalizeOpenAIResponsesRejectedFieldRetryBody(resp.StatusCode, st.body, respBody); retryErr != nil {
				return fmt.Errorf("normalize rejected Responses field retry st.body: %w", retryErr)
			} else if changed && rejectedFieldRetryState.AllowForReason(retryBody, reason) {
				markFirstOutputAttemptFailed(ex.Ctx)
				st.body = retryBody
				st.requestView = newOpenAIRequestView(st.body)
				st.reqBody = nil
				upstreamBody = st.body
				if st.bodySignalCompactV2 {
					prepared, prepErr := prepareOpenAIBodySignalV2UpstreamBody(st.body)
					if prepErr != nil {
						return prepErr
					}
					upstreamBody = prepared
				}
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Retrying non-WSv2 request after %s (account: %s)", reason, account.Name)
				continue
			}
			if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
				s.dumpOpenAICapacityABHTTP(st.c, account, resp, "http_responses", firstNonEmpty(st.billingModel, st.upstreamModel, st.originalModel), upstreamMsg, respBody, false)
				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = redactAndTruncateUpstreamDetail(string(respBody), maxBytes)
				}
				appendOpsUpstreamError(st.c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					Kind:               "failover",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})

				s.handleFailoverSideEffects(ex.Ctx, resp, account, respBody, st.upstreamModel)
				return newOpenAIUpstreamFailoverError(
					resp.StatusCode,
					resp.Header,
					respBody,
					upstreamMsg,
					account.IsPoolMode() && (account.IsPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
				)
			}
			// Body-signal v2 may have already committed 200 text/event-stream headers
			// via keepalives from a prior soft-timeout attempt; JSON error bodies would
			// corrupt the stream, so terminate with a Responses response.failed event.
			if st.bodySignalCompactV2 && IsOpenAICompactV2SSEStarted(st.c) {
				return s.handleOpenAICompactV2UpstreamErrorSSE(ex.Ctx, resp, st.c, account, respBody, st.billingModel)
			}
			st.result, err = s.handleErrorResponse(ex.Ctx, resp, st.c, account, st.body, st.billingModel)
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		st.reasoningEffort = extractOpenAIReasoningEffortFromBody(st.body, firstNonEmpty(st.upstreamModel, st.billingModel, st.originalModel))
		// 国产模型默认 effort 补充：此处 st.reqModel 已被 mapping 重写为 st.billingModel（见
		// line 2510-2515 的 GetMappedModel + st.reqModel 赋值），可直接作为 mappedModel。
		st.reasoningEffort = ApplyThinkingEnabledFallback(st.reasoningEffort, st.body, st.reqModel)
		st.serviceTier = extractOpenAIServiceTierFromBody(st.body)
		// 上游接受后只保留计费需要的标量，避免响应处理期间继续保活完整 input/tools map。
		st.reqBody = nil

		// Handle normal response
		var usage *OpenAIUsage
		st.firstTokenMs = nil
		st.responseID = ""
		st.responseModel = ""
		st.imageCount = 0
		st.imageOutputSizes = nil
		if st.bodySignalCompactV2 {
			// Upstream is non-stream compact JSON; bridge to v2 SSE for the client.
			bridgedID, bridgedUsage, bridgeErr := s.handleOpenAIBodySignalV2LegacyCompactResponse(upstreamCtx, st.c, resp, st.originalModel)
			if bridgeErr != nil {
				isCancelOrTimeout := errors.Is(bridgeErr, context.Canceled) || errors.Is(bridgeErr, context.DeadlineExceeded)
				if !isCancelOrTimeout {
					writeOpenAICompactV2StreamError(st.c, bridgeErr.Error())
				}
				// Upstream compact already spent tokens when usage was parsed from the
				// JSON st.body. Downstream write/client disconnect must not drop billing
				// (mirrors image partial-error accounting in the handler).
				if bridgedUsage != nil {
					payloadBytes := int64(len(upstreamBody))
					clientCanceled := st.c != nil && st.c.Request != nil && st.c.Request.Context().Err() != nil
					st.result = &OpenAIForwardResult{
						RequestID:             resp.Header.Get("x-request-id"),
						ResponseID:            strings.TrimSpace(bridgedID),
						Usage:                 *bridgedUsage,
						Model:                 st.originalModel,
						BillingModel:          st.billingModel,
						UpstreamModel:         st.upstreamModel,
						ServiceTier:           st.serviceTier,
						ReasoningEffort:       st.reasoningEffort,
						Stream:                true,
						OpenAIWSMode:          false,
						Duration:              time.Since(st.startTime),
						Compact:               true,
						CompactPayloadBytes:   &payloadBytes,
						CompactClientCanceled: clientCanceled,
						ClientDisconnect:      clientCanceled,
					}
					return bridgeErr
				}
				return bridgeErr
			}
			st.responseID = strings.TrimSpace(bridgedID)
			usage = bridgedUsage
			// Treat bridge start as first-byte for health metrics when available.
			if IsOpenAICompactV2SSEStarted(st.c) {
				elapsed := int(time.Since(st.startTime).Milliseconds())
				if elapsed < 0 {
					elapsed = 0
				}
				st.firstTokenMs = &elapsed
			}
		} else if st.reqStream {
			streamResult, err := s.handleStreamingResponse(ex.Ctx, resp, st.c, account, st.startTime, st.originalModel, st.upstreamModel)
			if err != nil {
				return err
			}
			usage = streamResult.usage
			st.firstTokenMs = streamResult.firstTokenMs
			st.responseID = strings.TrimSpace(streamResult.responseID)
			st.responseModel = streamResult.responseModel
			st.imageCount = streamResult.imageCount
			st.imageOutputSizes = streamResult.imageOutputSizes
		} else {
			nonStreamResult, err := s.handleNonStreamingResponse(ex.Ctx, resp, st.c, account, st.originalModel, st.upstreamModel)
			if err != nil {
				return err
			}
			usage = nonStreamResult.usage
			st.responseID = strings.TrimSpace(nonStreamResult.responseID)
			st.responseModel = nonStreamResult.responseModel
			st.imageCount = nonStreamResult.imageCount
			st.imageOutputSizes = nonStreamResult.imageOutputSizes
			if st.isCompactRequest {
				MarkOpenAICompactTerminalCommitted(st.c)
			}
		}
		s.bindHTTPResponseAccount(ex.Ctx, st.c, account, st.responseID)

		// Extract and save Codex usage snapshot from response headers (for OAuth accounts).
		// 排除 spark 影子:其 codex_* 仅由 QueryUsage(/wham/usage bengalfox)更新(外审第7轮 P1)。
		if account.Type == AccountTypeOAuth && !account.IsShadow() {
			if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
				s.updateCodexUsageSnapshot(ex.Ctx, account.ID, snapshot)
			}
		}
		s.dumpOpenAICapacityABHTTP(st.c, account, resp, "http_responses", firstNonEmpty(st.originalModel, st.billingModel, st.upstreamModel), "", nil, true)

		if usage == nil {
			usage = &OpenAIUsage{}
		}

		forwardResult := &OpenAIForwardResult{
			RequestID:       resp.Header.Get("x-request-id"),
			ResponseID:      st.responseID,
			ResponseModel:   st.responseModel,
			Usage:           *usage,
			Model:           st.originalModel,
			BillingModel:    st.billingModel,
			UpstreamModel:   st.upstreamModel,
			ServiceTier:     st.serviceTier,
			ReasoningEffort: st.reasoningEffort,
			Stream:          st.reqStream,
			OpenAIWSMode:    false,
			Duration:        time.Since(st.startTime),
			FirstTokenMs:    st.firstTokenMs,
		}
		if st.isCompactRequest || st.bodySignalCompactV2 {
			payloadBytes := int64(len(upstreamBody))
			forwardResult.Compact = true
			forwardResult.CompactPayloadBytes = &payloadBytes
			// Client disconnect must use the inbound request context, not a
			// soft-timeout child context (which may be canceled by the gateway).
			forwardResult.CompactClientCanceled = st.c != nil && st.c.Request != nil && st.c.Request.Context().Err() != nil
		}
		if st.imageCount > 0 {
			forwardResult.ImageCount = st.imageCount
			forwardResult.ImageSize = st.imageSizeTier
			forwardResult.ImageInputSize = st.imageInputSize
			forwardResult.ImageOutputSizes = st.imageOutputSizes
			forwardResult.BillingModel = st.imageBillingModel
		}
		st.result = forwardResult
		return nil
	}

}

func (s *OpenAIGatewayService) forwardOpenAIRelay(ex *pipeline.Exchange) error {
	st := openaiForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse request: empty request")
	}
	if st.skipRemaining || st.result != nil {
		return nil
	}
	return nil
}

func (s *OpenAIGatewayService) forwardOpenAISettle(ex *pipeline.Exchange) error {
	st := openaiForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse request: empty request")
	}
	if st.skipRemaining {
		return nil
	}
	if st.result == nil {
		return fmt.Errorf("upstream request failed: empty response")
	}
	ex.UpstreamModel = st.upstreamModel
	return nil
}
