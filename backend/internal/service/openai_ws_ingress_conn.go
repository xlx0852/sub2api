package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	coderws "github.com/coder/websocket"
)

// rotateAgedUpstreamConnBeforeTurn proactively replaces a leased upstream
// Responses WebSocket before OpenAI's 60-minute lifetime limit. Strict
// continuations are converted to a replayable full create when their replay
// context is complete; otherwise we keep the connection and let the existing
// error-triggered recovery path decide safely.
func (sess *openAIWSIngressSession) rotateAgedUpstreamConnBeforeTurn() bool {
	if sess == nil || sess.sessionLease == nil || sess.turn <= 1 {
		return false
	}
	if sess.sessionLease.ConnAge(time.Now()) < openAIWSConnRotationAge {
		return false
	}
	if sess.isStrictAffinityTurn(sess.currentPayload) {
		withoutPrevious, removed, dropErr := dropPreviousResponseIDFromRawPayload(sess.currentPayload)
		if dropErr != nil || !removed {
			return false
		}
		withReplay, setInputErr := setOpenAIWSPayloadInputSequence(
			withoutPrevious,
			sess.currentTurnReplayInput,
			sess.currentTurnReplayInputExists,
		)
		if setInputErr != nil {
			return false
		}
		sess.currentPayload = withReplay
		sess.currentPayloadBytes = len(withReplay)
	}
	oldConnID := sess.sessionConnID
	sess.resetSessionLease(true)
	sess.forceNewConnOnRetry = true
	logOpenAIWSModeInfo(
		"ingress_ws_proactive_rotation account_id=%d turn=%d old_conn_id=%s age_minutes=%d",
		sess.account.ID,
		sess.turn,
		truncateOpenAIWSLogValue(oldConnID, openAIWSIDValueMaxLen),
		int(openAIWSConnRotationAge/time.Minute),
	)
	return true
}

func (sess *openAIWSIngressSession) acquireTurnLease(turn int, preferred string, forcePreferredConn bool) (*openAIWSConnLease, error) {
	s := sess.svc
	ctx := sess.ctx
	account := sess.account
	pool := sess.pool
	req := cloneOpenAIWSAcquireRequest(sess.baseAcquireReq)
	req.PreferredConnID = strings.TrimSpace(preferred)
	req.ForcePreferredConn = forcePreferredConn
	// dedicated 模式下每次获取均新建连接，避免跨会话复用残留上下文。
	req.ForceNewConn = sess.dedicatedMode || sess.forceNewConnOnRetry
	// Codex 自己会在同一个下游 WS 内复用会话。首次只领取未承载请求的
	// 预连接，防止把其他 Codex 会话的上游连接重新分配给当前会话。
	req.RequirePristine = sess.codexSessionExclusive() && !req.ForceNewConn && req.PreferredConnID == "" &&
		strings.TrimSpace(openAIWSPayloadStringFromRaw(sess.currentPayload, "previous_response_id")) == ""
	sess.forceNewConnOnRetry = false
	acquireCtx, acquireCancel := context.WithTimeout(ctx, sess.acquireTimeout)
	lease, acquireErr := pool.Acquire(acquireCtx, req)
	acquireCancel()
	var dialErr *openAIWSDialError
	if acquireErr != nil && s.isAgentIdentityAccount(ctx, account) && errors.As(acquireErr, &dialErr) && isAgentIdentityTaskInvalidWSDialError(dialErr) && !sess.agentTaskRecoveryTried {
		sess.agentTaskRecoveryTried = true
		if recoveryErr := s.recoverAgentIdentityTask(ctx, account, account.GetCredential("task_id")); recoveryErr != nil {
			return nil, fmt.Errorf("agent identity task recovery failed: %w", recoveryErr)
		}
		return sess.acquireTurnLease(turn, preferred, forcePreferredConn)
	}
	if acquireErr != nil {
		dialStatus, dialClass, dialCloseStatus, dialCloseReason, dialRespServer, dialRespVia, dialRespCFRay, dialRespReqID := summarizeOpenAIWSDialError(acquireErr)
		logOpenAIWSModeInfo(
			"ingress_ws_upstream_acquire_fail account_id=%d turn=%d reason=%s dial_status=%d dial_class=%s dial_close_status=%s dial_close_reason=%s dial_resp_server=%s dial_resp_via=%s dial_resp_cf_ray=%s dial_resp_x_request_id=%s cause=%s preferred_conn_id=%s force_preferred_conn=%v require_pristine=%v ws_host=%s ws_path=%s proxy_enabled=%v",
			account.ID,
			turn,
			normalizeOpenAIWSLogValue(classifyOpenAIWSAcquireError(acquireErr)),
			dialStatus,
			dialClass,
			dialCloseStatus,
			truncateOpenAIWSLogValue(dialCloseReason, openAIWSHeaderValueMaxLen),
			dialRespServer,
			dialRespVia,
			dialRespCFRay,
			dialRespReqID,
			truncateOpenAIWSLogValue(acquireErr.Error(), openAIWSLogValueMaxLen),
			truncateOpenAIWSLogValue(preferred, openAIWSIDValueMaxLen),
			forcePreferredConn,
			req.RequirePristine,
			sess.wsHost,
			sess.wsPath,
			account.ProxyID != nil && account.Proxy != nil,
		)
		var dialErr *openAIWSDialError
		if errors.As(acquireErr, &dialErr) && dialErr != nil && dialErr.StatusCode == http.StatusTooManyRequests {
			s.persistOpenAIWSRateLimitSignal(ctx, account, dialErr.ResponseHeaders, nil, "rate_limit_exceeded", "rate_limit_error", strings.TrimSpace(acquireErr.Error()))
			return nil, &UpstreamFailoverError{
				StatusCode:      http.StatusTooManyRequests,
				ResponseHeaders: cloneHeader(dialErr.ResponseHeaders),
			}
		}
		if errors.Is(acquireErr, errOpenAIWSPreferredConnUnavailable) {
			return nil, NewOpenAIWSClientCloseError(
				coderws.StatusPolicyViolation,
				"upstream continuation connection is unavailable; please restart the conversation",
				acquireErr,
			)
		}
		if errors.Is(acquireErr, context.DeadlineExceeded) || errors.Is(acquireErr, errOpenAIWSConnQueueFull) {
			return nil, NewOpenAIWSClientCloseError(
				coderws.StatusTryAgainLater,
				"upstream websocket is busy, please retry later",
				acquireErr,
			)
		}
		return nil, acquireErr
	}
	connID := strings.TrimSpace(lease.ConnID())
	if handshakeTurnState := strings.TrimSpace(lease.HandshakeHeader(openAIWSTurnStateHeader)); handshakeTurnState != "" {
		sess.turnState = handshakeTurnState
		if sess.stateStore != nil && sess.sessionHash != "" {
			sess.stateStore.BindSessionTurnState(sess.groupID, sess.sessionHash, handshakeTurnState, s.openAIWSSessionStickyTTL())
		}
		updatedHeaders := cloneHeader(sess.baseAcquireReq.Headers)
		if updatedHeaders == nil {
			updatedHeaders = make(http.Header)
		}
		updatedHeaders.Set(openAIWSTurnStateHeader, handshakeTurnState)
		sess.baseAcquireReq.Headers = updatedHeaders
	}
	logOpenAIWSModeInfo(
		"ingress_ws_upstream_connected account_id=%d turn=%d conn_id=%s conn_reused=%v conn_age_ms=%d request_count=%d pristine=%v session_owned=%v conn_pick_ms=%d queue_wait_ms=%d preferred_conn_id=%s",
		account.ID,
		turn,
		truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
		lease.Reused(),
		lease.ConnAge(time.Now()).Milliseconds(),
		lease.RequestCount(),
		lease.IsPristine(),
		sess.codexSessionExclusive(),
		lease.ConnPickDuration().Milliseconds(),
		lease.QueueWaitDuration().Milliseconds(),
		truncateOpenAIWSLogValue(preferred, openAIWSIDValueMaxLen),
	)
	return lease, nil
}

func (sess *openAIWSIngressSession) turnConnReused(lease *openAIWSConnLease) bool {
	if lease == nil {
		return false
	}
	connID := strings.TrimSpace(lease.ConnID())
	return lease.Reused() || (sess.lastCompletedConnID != "" && connID == sess.lastCompletedConnID)
}

func (sess *openAIWSIngressSession) sendAndRelay(turn int, lease *openAIWSConnLease, payload []byte, payloadBytes int, originalModel string, imageBillingModel string, imageSizeTier string, imageInputSize string, allowIndexedCompatibilityRetry bool) (*OpenAIForwardResult, error) {
	s := sess.svc
	ctx := sess.ctx
	c := sess.c
	account := sess.account
	if lease == nil {
		return nil, errors.New("upstream websocket lease is nil")
	}
	turnStart := time.Now()
	firstOutputTracker := NewFirstOutputStageTracker(turnStart)
	firstOutputTracker.transition(firstOutputStageConnect, turnStart, false)
	billingRequestID := "ws-turn:" + generateRequestID()
	acceptedPersisted := false
	wroteDownstream := false
	if err := lease.WriteJSONWithContextTimeout(ctx, json.RawMessage(payload), s.openAIWSWriteTimeout()); err != nil {
		return nil, wrapOpenAIWSIngressTurnError(
			"write_upstream",
			fmt.Errorf("write upstream websocket request: %w", err),
			false,
		)
	}
	firstOutputTracker.transition(firstOutputStageUpstream, time.Now(), false)
	if sess.debugEnabled {
		logOpenAIWSModeDebug(
			"ingress_ws_turn_request_sent account_id=%d turn=%d conn_id=%s payload_bytes=%d",
			account.ID,
			turn,
			truncateOpenAIWSLogValue(lease.ConnID(), openAIWSIDValueMaxLen),
			payloadBytes,
		)
	}

	responseID := ""
	usage := OpenAIUsage{}
	imageCounter := newOpenAIImageOutputCounter()
	var firstTokenMs *int
	firstOutputPending := false
	reqStream := openAIWSPayloadBoolFromRaw(payload, "stream", true)
	turnPreviousResponseID := openAIWSPayloadStringFromRaw(payload, "previous_response_id")
	turnPreviousResponseIDKind := ClassifyOpenAIPreviousResponseIDKind(turnPreviousResponseID)
	turnPromptCacheKey := openAIWSPayloadStringFromRaw(payload, "prompt_cache_key")
	turnStoreDisabled := s.isOpenAIWSStoreDisabledInRequestRaw(payload, account)
	turnHasFunctionCallOutput := openAIWSRawPayloadHasToolCallOutput(payload)
	eventCount := 0
	tokenEventCount := 0
	terminalEventCount := 0
	replayCollector := &openAIWSToolCallReplayCollector{}
	firstEventType := ""
	lastEventType := ""
	needModelReplace := false
	clientDisconnected := false
	mappedModel := ""
	// upstreamResponseModel 记录上游响应声明的模型（终止帧优先），写入用量记录用于路由审计。
	upstreamResponseModel := ""
	var mappedModelBytes []byte
	if originalModel != "" {
		mappedModel = normalizeOpenAIModelForUpstream(account, account.GetMappedModel(originalModel))
		needModelReplace = mappedModel != "" && mappedModel != originalModel
		if needModelReplace {
			mappedModelBytes = []byte(mappedModel)
		}
	}
	for {
		upstreamMessage, readErr := lease.ReadMessageWithContextTimeout(ctx, s.openAIWSReadTimeout())
		if readErr != nil {
			lease.MarkBroken()
			return nil, wrapOpenAIWSIngressTurnError(
				"read_upstream",
				fmt.Errorf("read upstream websocket event: %w", readErr),
				wroteDownstream,
			)
		}

		eventType, eventResponseID, _ := parseOpenAIWSEventEnvelope(upstreamMessage)
		if responseID == "" && eventResponseID != "" {
			responseID = eventResponseID
		}
		if eventType != "" {
			eventCount++
			if firstEventType == "" {
				firstEventType = eventType
			}
			lastEventType = eventType
		}
		if eventType == "error" {
			errCodeRaw, errTypeRaw, errMsgRaw := parseOpenAIWSErrorEventFields(upstreamMessage)
			s.persistOpenAIWSRateLimitSignal(ctx, account, lease.HandshakeHeaders(), upstreamMessage, errCodeRaw, errTypeRaw, errMsgRaw)
			if allowIndexedCompatibilityRetry && !wroteDownstream {
				retryBody, reason, changed, repairErr := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, payload, upstreamMessage)
				if repairErr != nil {
					lease.MarkBroken()
					return nil, wrapOpenAIWSIngressTurnError(
						"indexed_compatibility_repair",
						repairErr,
						false,
					)
				}
				if changed {
					lease.MarkBroken()
					return nil, newOpenAIWSIndexedCompatibilityRetryError(retryBody, reason)
				}
			}
			fallbackReason, _ := classifyOpenAIWSErrorEventFromRaw(errCodeRaw, errTypeRaw, errMsgRaw)
			recoverableConnectionLimit := fallbackReason == openAIWSIngressStageConnectionLimitReached &&
				!wroteDownstream &&
				!sess.turnConnectionLimitRecoveryTried &&
				sess.canRecoverIngressConnectionLimit(payload)
			if recoverableConnectionLimit {
				lease.MarkBroken()
				errMsg := strings.TrimSpace(errMsgRaw)
				if errMsg == "" {
					errMsg = "upstream websocket connection limit reached"
				}
				return nil, wrapOpenAIWSIngressTurnError(
					openAIWSIngressStageConnectionLimitReached,
					errors.New(errMsg),
					false,
				)
			}
			errCode, errType, errMessage := summarizeOpenAIWSErrorEventFieldsFromRaw(errCodeRaw, errTypeRaw, errMsgRaw)
			recoverablePrevNotFound := fallbackReason == openAIWSIngressStagePreviousResponseNotFound &&
				turnPreviousResponseID != "" &&
				!turnHasFunctionCallOutput &&
				s.openAIWSIngressPreviousResponseRecoveryEnabled() &&
				!wroteDownstream
			if recoverablePrevNotFound {
				// 可恢复场景使用非 error 关键字日志，避免被 LegacyPrintf 误判为 ERROR 级别。
				logOpenAIWSModeInfo(
					"ingress_ws_prev_response_recoverable account_id=%d turn=%d conn_id=%s idx=%d reason=%s code=%s type=%s message=%s previous_response_id=%s previous_response_id_kind=%s response_id=%s store_disabled=%v has_prompt_cache_key=%v",
					account.ID,
					turn,
					truncateOpenAIWSLogValue(lease.ConnID(), openAIWSIDValueMaxLen),
					eventCount,
					truncateOpenAIWSLogValue(fallbackReason, openAIWSLogValueMaxLen),
					errCode,
					errType,
					errMessage,
					truncateOpenAIWSLogValue(turnPreviousResponseID, openAIWSIDValueMaxLen),
					normalizeOpenAIWSLogValue(turnPreviousResponseIDKind),
					truncateOpenAIWSLogValue(responseID, openAIWSIDValueMaxLen),
					turnStoreDisabled,
					turnPromptCacheKey != "",
				)
			} else {
				logOpenAIWSModeInfo(
					"ingress_ws_error_event account_id=%d turn=%d conn_id=%s idx=%d fallback_reason=%s err_code=%s err_type=%s err_message=%s previous_response_id=%s previous_response_id_kind=%s response_id=%s store_disabled=%v has_prompt_cache_key=%v",
					account.ID,
					turn,
					truncateOpenAIWSLogValue(lease.ConnID(), openAIWSIDValueMaxLen),
					eventCount,
					truncateOpenAIWSLogValue(fallbackReason, openAIWSLogValueMaxLen),
					errCode,
					errType,
					errMessage,
					truncateOpenAIWSLogValue(turnPreviousResponseID, openAIWSIDValueMaxLen),
					normalizeOpenAIWSLogValue(turnPreviousResponseIDKind),
					truncateOpenAIWSLogValue(responseID, openAIWSIDValueMaxLen),
					turnStoreDisabled,
					turnPromptCacheKey != "",
				)
			}
			// previous_response_not_found 在 ingress 模式支持单次恢复重试：
			// 不把该 error 直接下发客户端，而是由上层去掉 previous_response_id 后重放当前 turn。
			if recoverablePrevNotFound {
				lease.MarkBroken()
				errMsg := strings.TrimSpace(errMsgRaw)
				if errMsg == "" {
					errMsg = "previous response not found"
				}
				return nil, wrapOpenAIWSIngressTurnError(
					openAIWSIngressStagePreviousResponseNotFound,
					errors.New(errMsg),
					false,
				)
			}
			if !wroteDownstream && isOpenAIWSRateLimitError(errCodeRaw, errTypeRaw, errMsgRaw) {
				lease.MarkBroken()
				return nil, &UpstreamFailoverError{
					StatusCode:      http.StatusTooManyRequests,
					ResponseBody:    append([]byte(nil), upstreamMessage...),
					ResponseHeaders: cloneHeader(lease.HandshakeHeaders()),
				}
			}
			s.markOpenAIStreamTerminalError(c, account, false, responseID, upstreamMessage, errMsgRaw)
		}
		if eventType != "" && eventType != "error" && !acceptedPersisted {
			acceptedPersisted = true
			if sess.hooks != nil && sess.hooks.UpstreamAccepted != nil {
				sess.hooks.UpstreamAccepted(turn, billingRequestID, eventResponseID)
			}
		}
		isTokenEvent := isOpenAIWSTokenEvent(eventType)
		if isTokenEvent {
			tokenEventCount++
		}
		isTerminalEvent := isOpenAIWSTerminalEvent(eventType)
		if isTerminalEvent {
			terminalEventCount++
		}
		if firstTokenMs == nil && isTokenEvent {
			ms := int(time.Since(turnStart).Milliseconds())
			firstTokenMs = &ms
			firstOutputTracker.transition(firstOutputStageWrite, time.Now(), false)
			firstOutputPending = true
		}
		if openAIWSEventShouldParseUsage(eventType) {
			parseOpenAIWSResponseUsageFromCompletedEvent(upstreamMessage, &usage)
		}
		imageCounter.AddSSEData(upstreamMessage)

		// 在改写回客户端模型之前先捕获上游声明的模型。
		if model := extractOpenAIResponseModelFromJSONBytes(upstreamMessage); model != "" {
			if isTerminalEvent || upstreamResponseModel == "" {
				upstreamResponseModel = model
			}
		}

		if eventType == "response.failed" {
			if hit, code, msg := detectOpenAICyberPolicy(upstreamMessage); hit {
				MarkOpsCyberPolicy(c, CyberPolicyMark{
					Code:           code,
					Message:        msg,
					Body:           truncateString(string(upstreamMessage), 4096),
					UpstreamStatus: http.StatusOK,
					UpstreamInTok:  usage.InputTokens,
					UpstreamOutTok: usage.OutputTokens,
				})
			}
			s.markOpenAIStreamTerminalError(c, account, false, responseID, upstreamMessage, extractOpenAISSEErrorMessage(upstreamMessage))
		}

		if !clientDisconnected {
			if needModelReplace && len(mappedModelBytes) > 0 && openAIWSEventMayContainModel(eventType) && bytes.Contains(upstreamMessage, mappedModelBytes) {
				upstreamMessage = replaceOpenAIWSMessageModel(upstreamMessage, mappedModel, originalModel)
			}
			if openAIWSEventMayContainToolCalls(eventType) && openAIWSMessageLikelyContainsToolCalls(upstreamMessage) {
				if corrected, changed := s.toolCorrector.CorrectToolCallsInSSEBytes(upstreamMessage); changed {
					upstreamMessage = corrected
				}
			}
			replayCollector.AddEvent(eventType, upstreamMessage)
			if err := sess.writeClientMessage(upstreamMessage); err != nil {
				if isOpenAIWSClientDisconnectError(err) {
					clientDisconnected = true
					closeStatus, closeReason := summarizeOpenAIWSReadCloseError(err)
					logOpenAIWSModeInfo(
						"ingress_ws_client_disconnected_drain account_id=%d turn=%d conn_id=%s close_status=%s close_reason=%s",
						account.ID,
						turn,
						truncateOpenAIWSLogValue(lease.ConnID(), openAIWSIDValueMaxLen),
						closeStatus,
						truncateOpenAIWSLogValue(closeReason, openAIWSHeaderValueMaxLen),
					)
				} else {
					return nil, wrapOpenAIWSIngressTurnError(
						"write_client",
						fmt.Errorf("write client websocket event: %w", err),
						wroteDownstream,
					)
				}
			} else {
				wroteDownstream = true
				if firstOutputPending {
					firstOutputTracker.finish(time.Now())
					firstOutputPending = false
				}
			}
		}
		if isTerminalEvent {
			// 客户端已断连时，上游连接的 session 状态不可信，标记 broken 避免回池复用。
			if clientDisconnected {
				lease.MarkBroken()
			}
			firstTokenMsValue := -1
			if firstTokenMs != nil {
				firstTokenMsValue = *firstTokenMs
			}
			if sess.debugEnabled {
				logOpenAIWSModeDebug(
					"ingress_ws_turn_completed account_id=%d turn=%d conn_id=%s response_id=%s duration_ms=%d events=%d token_events=%d terminal_events=%d first_event=%s last_event=%s first_token_ms=%d client_disconnected=%v",
					account.ID,
					turn,
					truncateOpenAIWSLogValue(lease.ConnID(), openAIWSIDValueMaxLen),
					truncateOpenAIWSLogValue(responseID, openAIWSIDValueMaxLen),
					time.Since(turnStart).Milliseconds(),
					eventCount,
					tokenEventCount,
					terminalEventCount,
					truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
					firstTokenMsValue,
					clientDisconnected,
				)
			}
			imageCount := imageCounter.Count()
			// lease.Reused reports whether this lease came from an already-idle pool
			// connection. An ingress session keeps the same lease across turns, so
			// later turns must also be recorded as reused when they run on the
			// connection that completed the previous turn.
			reused := sess.turnConnReused(lease)
			connPickMs := int(lease.ConnPickDuration().Milliseconds())
			queueWaitMs := int(lease.QueueWaitDuration().Milliseconds())
			payloadBytesValue := int64(payloadBytes)
			eventCountValue := eventCount
			result := &OpenAIForwardResult{
				RequestID:         responseID,
				BillingRequestID:  billingRequestID,
				Usage:             usage,
				Model:             originalModel,
				UpstreamModel:     mappedModel,
				ResponseModel:     upstreamResponseModel,
				ServiceTier:       extractOpenAIServiceTierFromBody(payload),
				ReasoningEffort:   ApplyThinkingEnabledFallback(extractOpenAIReasoningEffortFromBody(payload, firstNonEmpty(mappedModel, originalModel)), payload, mappedModel),
				Stream:            reqStream,
				OpenAIWSMode:      true,
				ResponseHeaders:   lease.HandshakeHeaders(),
				Duration:          time.Since(turnStart),
				FirstTokenMs:      firstTokenMs,
				FirstOutputStages: firstOutputTracker.Snapshot(),
				WSPayloadBytes:    &payloadBytesValue,
				WSEventCount:      &eventCountValue,
				WSConnReused:      &reused,
				WSConnPickMs:      &connPickMs,
				WSQueueWaitMs:     &queueWaitMs,
			}
			if replayInput := normalizeOpenAIReplayItemsForAccount(account, replayCollector.Items()); len(replayInput) > 0 {
				result.wsReplayInput = replayInput
				result.wsReplayInputExists = true
			}
			if imageCount > 0 {
				result.ImageCount = imageCount
				result.ImageSize = imageSizeTier
				result.ImageInputSize = imageInputSize
				result.ImageOutputSizes = imageCounter.Sizes()
				result.BillingModel = imageBillingModel
			}
			return result, nil
		}
	}
}

func (sess *openAIWSIngressSession) unpinSessionConn(connID string) {
	account := sess.account
	pool := sess.pool
	connID = strings.TrimSpace(connID)
	if connID == "" || sess.pinnedSessionConnID != connID {
		return
	}
	pool.UnpinConn(account.ID, connID)
	sess.pinnedSessionConnID = ""
}

func (sess *openAIWSIngressSession) pinSessionConn(connID string) {
	account := sess.account
	pool := sess.pool
	if !sess.storeDisabled {
		return
	}
	connID = strings.TrimSpace(connID)
	if connID == "" || sess.pinnedSessionConnID == connID {
		return
	}
	if sess.pinnedSessionConnID != "" {
		pool.UnpinConn(account.ID, sess.pinnedSessionConnID)
		sess.pinnedSessionConnID = ""
	}
	if pool.PinConn(account.ID, connID) {
		sess.pinnedSessionConnID = connID
	}
}

func (sess *openAIWSIngressSession) releaseSessionLease() {
	account := sess.account
	if sess.sessionLease == nil {
		return
	}
	retire := sess.codexSessionExclusive()
	if !sess.lastTurnClean {
		sess.sessionLease.MarkBroken()
	}
	sess.unpinSessionConn(sess.sessionConnID)
	if retire {
		retireReason := "session_end"
		if !sess.lastTurnClean {
			retireReason = "broken"
		}
		logOpenAIWSModeInfo(
			"ingress_ws_upstream_retired account_id=%d conn_id=%s reason=%s clean=%v conn_age_ms=%d request_count=%d",
			account.ID,
			truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
			retireReason,
			sess.lastTurnClean,
			sess.sessionLease.ConnAge(time.Now()).Milliseconds(),
			sess.sessionLease.RequestCount(),
		)
		sess.sessionLease.Retire()
	} else {
		sess.sessionLease.Release()
	}
	if sess.debugEnabled {
		logOpenAIWSModeDebug(
			"ingress_ws_upstream_released account_id=%d conn_id=%s",
			account.ID,
			truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
		)
	}
}

func (sess *openAIWSIngressSession) resetSessionLease(markBroken bool) {
	if sess.sessionLease == nil {
		return
	}
	if markBroken {
		sess.sessionLease.MarkBroken()
	}
	sess.releaseSessionLease()
	sess.sessionLease = nil
	sess.sessionConnID = ""
	sess.preferredConnID = ""
}
