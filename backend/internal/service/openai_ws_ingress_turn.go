package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	coderws "github.com/coder/websocket"
)

func (sess *openAIWSIngressSession) runHTTPBridgeTurns(firstPayload openAIWSClientPayload) error {
	s := sess.svc
	ctx := sess.ctx
	c := sess.c
	account := sess.account
	token := sess.token
	hooks := sess.hooks
	logOpenAIWSModeInfo(
		"ingress_ws_http_bridge_start account_id=%d account_type=%s payload_bytes=%d threshold_bytes=%d has_session_hash=%v store_disabled=%v",
		account.ID,
		account.Type,
		firstPayload.payloadBytes,
		s.openAIWSHTTPBridgeThresholdBytes(),
		sess.sessionHash != "",
		sess.storeDisabled,
	)
	currentBridgePayload := firstPayload
	var bridgeReplayInput []json.RawMessage
	bridgeReplayInputExists := false
	for turn := 1; ; turn++ {
		if turn > 1 && hooks != nil && hooks.BeforeRequest != nil {
			if err := hooks.BeforeRequest(turn, currentBridgePayload.payloadRaw, currentBridgePayload.originalModel); err != nil {
				return err
			}
		}
		if hooks != nil && hooks.BeforeTurn != nil {
			if err := hooks.BeforeTurn(turn); err != nil {
				return err
			}
		}
		if sess.turnState != "" && c != nil && c.Request != nil {
			c.Request.Header.Set(openAIWSTurnStateHeader, sess.turnState)
		}
		bridgePayloadRaw := currentBridgePayload.payloadRaw
		bridgePayloadBytes := currentBridgePayload.payloadBytes
		needsBridgeReplay := currentBridgePayload.previousResponseID != "" || openAIWSRawPayloadHasToolCallOutput(currentBridgePayload.payloadRaw)
		turnReplayInput, turnReplayInputExists, replayInputErr := buildOpenAIWSReplayInputSequence(
			bridgeReplayInput,
			bridgeReplayInputExists,
			currentBridgePayload.payloadRaw,
			needsBridgeReplay,
		)
		if replayInputErr != nil {
			return fmt.Errorf("build websocket http bridge replay input: %w", replayInputErr)
		}
		if needsBridgeReplay && turnReplayInputExists {
			turnReplayInput = alignOpenAIReplayOutputTypesToCalls(turnReplayInput)
			updatedPayload, setInputErr := setOpenAIWSPayloadInputSequence(
				currentBridgePayload.payloadRaw,
				turnReplayInput,
				true,
			)
			if setInputErr != nil {
				return fmt.Errorf("set websocket http bridge replay input: %w", setInputErr)
			}
			bridgePayloadRaw = updatedPayload
			bridgePayloadBytes = len(updatedPayload)
			logOpenAIWSModeInfo(
				"ingress_ws_http_bridge_replay_input account_id=%d turn=%d input_items=%d previous_response_id_present=%v has_tool_output=%v",
				account.ID,
				turn,
				len(turnReplayInput),
				currentBridgePayload.previousResponseID != "",
				openAIWSRawPayloadHasToolCallOutput(currentBridgePayload.payloadRaw),
			)
		}
		billingRequestID := "ws-turn:" + generateRequestID()
		bridgeCtx := ctx
		if hooks != nil && hooks.UpstreamAccepted != nil {
			bridgeCtx = context.WithValue(ctx, usageSettlementAcceptedCallbackKey{}, &usageSettlementAcceptedCallback{fn: func() error {
				hooks.UpstreamAccepted(turn, billingRequestID, "")
				return nil
			}})
		}
		result, bridgeErr := s.proxyOpenAIWSHTTPBridgeTurn(
			bridgeCtx,
			c,
			account,
			token,
			bridgePayloadRaw,
			bridgePayloadBytes,
			currentBridgePayload.originalModel,
			currentBridgePayload.imageBillingModel,
			currentBridgePayload.imageSizeTier,
			currentBridgePayload.imageInputSize,
			turn,
			sess.writeClientMessage,
		)
		if result != nil {
			result.BillingRequestID = billingRequestID
		}
		if hooks != nil && hooks.AfterTurn != nil {
			hooks.AfterTurn(turn, result, bridgeErr)
		}
		if bridgeErr != nil {
			return bridgeErr
		}
		if result == nil {
			return errors.New("websocket http bridge turn result is nil")
		}
		bridgeReplayInput = cloneOpenAIWSRawMessages(turnReplayInput)
		bridgeReplayInputExists = turnReplayInputExists
		if result.wsReplayInputExists {
			bridgeReplayInput = append(bridgeReplayInput, cloneOpenAIWSRawMessages(result.wsReplayInput)...)
			bridgeReplayInputExists = true
		}
		if bridgeTurnState := strings.TrimSpace(result.ResponseHeaders.Get(openAIWSTurnStateHeader)); bridgeTurnState != "" {
			sess.turnState = bridgeTurnState
			if sess.stateStore != nil && sess.sessionHash != "" {
				sess.stateStore.BindSessionTurnState(sess.groupID, sess.sessionHash, bridgeTurnState, s.openAIWSSessionStickyTTL())
			}
		}
		responseID := strings.TrimSpace(result.RequestID)
		if responseID != "" && sess.stateStore != nil {
			ttl := s.openAIWSResponseStickyTTL()
			logOpenAIWSBindResponseAccountWarn(sess.groupID, account.ID, responseID, sess.stateStore.BindResponseAccount(ctx, sess.groupID, responseID, account.ID, ttl))
		}
		nextClientMessage, readErr := sess.readClientMessage()
		if readErr != nil {
			if isOpenAIWSClientDisconnectError(readErr) {
				closeStatus, closeReason := summarizeOpenAIWSReadCloseError(readErr)
				logOpenAIWSModeInfo(
					"ingress_ws_http_bridge_client_closed account_id=%d close_status=%s close_reason=%s",
					account.ID,
					closeStatus,
					truncateOpenAIWSLogValue(closeReason, openAIWSHeaderValueMaxLen),
				)
				return nil
			}
			return fmt.Errorf("read client websocket request: %w", readErr)
		}
		nextPayload, parseErr := sess.parseClientPayload(nextClientMessage)
		if parseErr != nil {
			return parseErr
		}
		currentBridgePayload = nextPayload
	}
}

func (sess *openAIWSIngressSession) hasCurrentOrReplayFunctionCallOutput(payload []byte) bool {
	if openAIWSRawPayloadHasToolCallOutput(payload) {
		return true
	}
	return sess.currentTurnReplayInputExists && openAIWSRawItemsHasFunctionCallOutput(sess.currentTurnReplayInput)
}

func (sess *openAIWSIngressSession) recoverIngressPrevResponseNotFound(relayErr error, turn int, connID string) bool {
	s := sess.svc
	account := sess.account
	if !isOpenAIWSIngressPreviousResponseNotFound(relayErr) {
		return false
	}
	if sess.turnPrevRecoveryTried || !s.openAIWSIngressPreviousResponseRecoveryEnabled() {
		return false
	}
	// 携带 function_call_output 的请求不能丢弃 previous_response_id：
	// 上游 API 需要 response chain 来匹配 tool_result 与之前的 tool_use，
	// 丢弃后会导致 "No tool call found for function call output" 400 错误。
	if sess.hasCurrentOrReplayFunctionCallOutput(sess.currentPayload) {
		return false
	}
	if sess.isStrictAffinityTurn(sess.currentPayload) {
		// Layer 2：严格亲和链路命中 previous_response_not_found 时，降级为“去掉 previous_response_id 后重放一次”。
		// 该错误说明续链锚点已失效，继续 strict fail-close 只会直接中断本轮请求。
		logOpenAIWSModeInfo(
			"ingress_ws_prev_response_recovery_layer2 account_id=%d turn=%d conn_id=%s store_disabled_conn_mode=%s action=drop_previous_response_id_retry",
			account.ID,
			turn,
			truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
			normalizeOpenAIWSLogValue(sess.storeDisabledConnMode),
		)
	}
	sess.turnPrevRecoveryTried = true
	updatedPayload, removed, dropErr := dropPreviousResponseIDFromRawPayload(sess.currentPayload)
	if dropErr != nil || !removed {
		reason := "not_removed"
		if dropErr != nil {
			reason = "drop_error"
		}
		logOpenAIWSModeInfo(
			"ingress_ws_prev_response_recovery_skip account_id=%d turn=%d conn_id=%s reason=%s",
			account.ID,
			turn,
			truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
			normalizeOpenAIWSLogValue(reason),
		)
		return false
	}
	updatedWithInput, setInputErr := setOpenAIWSPayloadInputSequence(
		updatedPayload,
		sess.currentTurnReplayInput,
		sess.currentTurnReplayInputExists,
	)
	if setInputErr != nil {
		logOpenAIWSModeInfo(
			"ingress_ws_prev_response_recovery_skip account_id=%d turn=%d conn_id=%s reason=set_full_input_error cause=%s",
			account.ID,
			turn,
			truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
			truncateOpenAIWSLogValue(setInputErr.Error(), openAIWSLogValueMaxLen),
		)
		return false
	}
	logOpenAIWSModeInfo(
		"ingress_ws_prev_response_recovery account_id=%d turn=%d conn_id=%s action=drop_previous_response_id retry=1",
		account.ID,
		turn,
		truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
	)
	sess.currentPayload = updatedWithInput
	sess.currentPayloadBytes = len(updatedWithInput)
	sess.resetSessionLease(true)
	sess.skipBeforeTurn = true
	return true
}

func (sess *openAIWSIngressSession) retryIngressTurn(relayErr error, turn int, connID string) bool {
	ctx := sess.ctx
	account := sess.account
	if !isOpenAIWSIngressTurnRetryable(relayErr) || sess.turnRetry >= 2 {
		return false
	}
	// 客户端/上游请求级 ctx 已结束时，DeadlineExceeded 不可重试；
	// 只有父 ctx 存活时的内部写/读超时（连接疑似僵死）才换连接重试。
	if ctx.Err() != nil {
		return false
	}
	if sess.isStrictAffinityTurn(sess.currentPayload) {
		logOpenAIWSModeInfo(
			"ingress_ws_turn_retry_skip account_id=%d turn=%d conn_id=%s reason=strict_affinity",
			account.ID,
			turn,
			truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
		)
		return false
	}
	sess.turnRetry++
	logOpenAIWSModeInfo(
		"ingress_ws_turn_retry account_id=%d turn=%d retry=%d reason=%s conn_id=%s",
		account.ID,
		turn,
		sess.turnRetry,
		truncateOpenAIWSLogValue(openAIWSIngressTurnRetryReason(relayErr), openAIWSLogValueMaxLen),
		truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
	)
	sess.resetSessionLease(true)
	// 重试强制拨新连接，避免从池里再捞到同样僵死的连接。
	sess.forceNewConnOnRetry = true
	sess.skipBeforeTurn = true
	return true
}

func (sess *openAIWSIngressSession) retryIngressIndexedCompatibility(relayErr error, turn int, connID string) bool {
	ctx := sess.ctx
	account := sess.account
	var retryErr *openAIWSIndexedCompatibilityRetryError
	if !errors.As(relayErr, &retryErr) || retryErr == nil || len(retryErr.body) == 0 {
		return false
	}
	if sess.turnIndexedCompatibilityRetryTried || ctx.Err() != nil {
		return false
	}
	sess.turnIndexedCompatibilityRetryTried = true
	sess.currentPayload = append([]byte(nil), retryErr.body...)
	sess.currentPayloadBytes = len(sess.currentPayload)
	logOpenAIWSModeInfo(
		"ingress_ws_indexed_compatibility_retry account_id=%d turn=%d conn_id=%s reason=%s payload_bytes=%d",
		account.ID,
		turn,
		truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
		normalizeOpenAIWSLogValue(retryErr.reason),
		sess.currentPayloadBytes,
	)
	sess.resetSessionLease(true)
	sess.forceNewConnOnRetry = true
	sess.skipBeforeTurn = true
	return true
}

func (sess *openAIWSIngressSession) runWSTurns(firstPayload openAIWSClientPayload) error {
	s := sess.svc
	ctx := sess.ctx
	c := sess.c
	account := sess.account
	token := sess.token
	hooks := sess.hooks
	isCodexCLI := sess.isCodexCLI
	wsHeaders, _, buildHdrErr := s.buildOpenAIWSHeaders(ctx, c, account, token, sess.wsDecision, isCodexCLI, sess.turnState, strings.TrimSpace(c.GetHeader(openAIWSTurnMetadataHeader)), firstPayload.promptCacheKey)
	if buildHdrErr != nil {
		return fmt.Errorf("build ws headers: %w", buildHdrErr)
	}
	sess.baseAcquireReq = openAIWSAcquireRequest{
		Account: account,
		WSURL:   sess.wsURL,
		Headers: wsHeaders,
		HeadersFactory: func(factoryCtx context.Context, headers http.Header) (http.Header, error) {
			return s.refreshOpenAIAgentIdentityHeaders(factoryCtx, account, headers)
		},
		ProxyURL: func() string {
			if account.ProxyID != nil && account.Proxy != nil {
				return account.Proxy.URL()
			}
			return ""
		}(),
		ForceNewConn: false,
	}
	pool := s.getOpenAIWSConnPool()
	if pool == nil {
		return errors.New("openai ws conn pool is nil")
	}
	sess.pool = pool

	logOpenAIWSModeInfo(
		"ingress_ws_protocol_confirm account_id=%d account_type=%s transport=%s ws_host=%s ws_path=%s ws_mode=%s store_disabled=%v has_session_hash=%v has_previous_response_id=%v",
		account.ID,
		account.Type,
		normalizeOpenAIWSLogValue(string(sess.wsDecision.Transport)),
		sess.wsHost,
		sess.wsPath,
		normalizeOpenAIWSLogValue(sess.ingressMode),
		sess.storeDisabled,
		sess.sessionHash != "",
		firstPayload.previousResponseID != "",
	)

	if sess.debugEnabled {
		logOpenAIWSModeDebug(
			"ingress_ws_start account_id=%d account_type=%s transport=%s ws_host=%s preferred_conn_id=%s has_session_hash=%v has_previous_response_id=%v store_disabled=%v",
			account.ID,
			account.Type,
			normalizeOpenAIWSLogValue(string(sess.wsDecision.Transport)),
			sess.wsHost,
			truncateOpenAIWSLogValue(sess.preferredConnID, openAIWSIDValueMaxLen),
			sess.sessionHash != "",
			firstPayload.previousResponseID != "",
			sess.storeDisabled,
		)
	}
	if firstPayload.previousResponseID != "" {
		firstPreviousResponseIDKind := ClassifyOpenAIPreviousResponseIDKind(firstPayload.previousResponseID)
		logOpenAIWSModeInfo(
			"ingress_ws_continuation_probe account_id=%d turn=%d previous_response_id=%s previous_response_id_kind=%s preferred_conn_id=%s session_hash=%s header_session_id=%s header_conversation_id=%s has_turn_state=%v turn_state_len=%d has_prompt_cache_key=%v store_disabled=%v",
			account.ID,
			1,
			truncateOpenAIWSLogValue(firstPayload.previousResponseID, openAIWSIDValueMaxLen),
			normalizeOpenAIWSLogValue(firstPreviousResponseIDKind),
			truncateOpenAIWSLogValue(sess.preferredConnID, openAIWSIDValueMaxLen),
			truncateOpenAIWSLogValue(sess.sessionHash, 12),
			openAIWSHeaderValueForLog(sess.baseAcquireReq.Headers, "session_id"),
			openAIWSHeaderValueForLog(sess.baseAcquireReq.Headers, "conversation_id"),
			sess.turnState != "",
			len(sess.turnState),
			firstPayload.promptCacheKey != "",
			sess.storeDisabled,
		)
	}

	sess.acquireTimeout = s.openAIWSAcquireTimeout()
	if sess.acquireTimeout <= 0 {
		sess.acquireTimeout = 30 * time.Second
	}

	sess.currentPayload = firstPayload.payloadRaw
	sess.currentOriginalModel = firstPayload.originalModel
	sess.currentImageBillingModel = firstPayload.imageBillingModel
	sess.currentImageSizeTier = firstPayload.imageSizeTier
	sess.currentImageInputSize = firstPayload.imageInputSize
	sess.currentPayloadBytes = firstPayload.payloadBytes

	defer sess.releaseSessionLease()
	sess.turn = 1
	sess.turnRetry = 0
	sess.turnPrevRecoveryTried = false
	sess.turnIndexedCompatibilityRetryTried = false
	sess.lastTurnFinishedAt = time.Time{}
	sess.lastTurnResponseID = ""
	sess.lastTurnPayload = []byte(nil)
	sess.lastTurnStrictState = nil // *openAIWSIngressPreviousTurnStrictState
	sess.lastTurnReplayInput = []json.RawMessage(nil)
	sess.lastTurnReplayInputExists = false
	sess.currentTurnReplayInput = []json.RawMessage(nil)
	sess.currentTurnReplayInputExists = false
	sess.skipBeforeTurn = false

	for {
		if sess.turn > 1 && !sess.skipBeforeTurn && hooks != nil && hooks.BeforeRequest != nil {
			if err := hooks.BeforeRequest(sess.turn, sess.currentPayload, sess.currentOriginalModel); err != nil {
				return err
			}
		}
		if !sess.skipBeforeTurn && hooks != nil && hooks.BeforeTurn != nil {
			if err := hooks.BeforeTurn(sess.turn); err != nil {
				return err
			}
		}
		sess.skipBeforeTurn = false
		currentPreviousResponseID := openAIWSPayloadStringFromRaw(sess.currentPayload, "previous_response_id")
		expectedPrev := strings.TrimSpace(sess.lastTurnResponseID)
		toolSignals := ToolContinuationSignals{
			HasFunctionCallOutput: openAIWSRawPayloadHasToolCallOutput(sess.currentPayload),
		}
		if toolSignals.HasFunctionCallOutput {
			var currentReqBody map[string]any
			if err := json.Unmarshal(sess.currentPayload, &currentReqBody); err == nil {
				toolSignals = AnalyzeToolContinuationSignals(currentReqBody)
			}
		}
		hasFunctionCallOutput := toolSignals.HasFunctionCallOutput
		// store=false + function_call_output 场景必须有续链锚点。
		// 若客户端未传 previous_response_id，优先回填上一轮响应 ID，避免上游报 call_id 无法关联。
		if shouldInferIngressFunctionCallOutputPreviousResponseID(
			sess.storeDisabled,
			sess.turn,
			toolSignals,
			currentPreviousResponseID,
			expectedPrev,
		) {
			updatedPayload, setPrevErr := setPreviousResponseIDToRawPayload(sess.currentPayload, expectedPrev)
			if setPrevErr != nil {
				logOpenAIWSModeInfo(
					"ingress_ws_function_call_output_prev_infer_skip account_id=%d turn=%d conn_id=%s reason=set_previous_response_id_error cause=%s expected_previous_response_id=%s",
					account.ID,
					sess.turn,
					truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
					truncateOpenAIWSLogValue(setPrevErr.Error(), openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(expectedPrev, openAIWSIDValueMaxLen),
				)
			} else {
				sess.currentPayload = updatedPayload
				sess.currentPayloadBytes = len(updatedPayload)
				currentPreviousResponseID = expectedPrev
				logOpenAIWSModeInfo(
					"ingress_ws_function_call_output_prev_infer account_id=%d turn=%d conn_id=%s action=set_previous_response_id previous_response_id=%s",
					account.ID,
					sess.turn,
					truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
					truncateOpenAIWSLogValue(expectedPrev, openAIWSIDValueMaxLen),
				)
			}
		}
		nextReplayInput, nextReplayInputExists, replayInputErr := buildOpenAIWSReplayInputSequence(
			sess.lastTurnReplayInput,
			sess.lastTurnReplayInputExists,
			sess.currentPayload,
			currentPreviousResponseID != "",
		)
		if replayInputErr == nil && nextReplayInputExists {
			nextReplayInput = alignOpenAIReplayOutputTypesToCalls(nextReplayInput)
		}
		if replayInputErr != nil {
			logOpenAIWSModeInfo(
				"ingress_ws_replay_input_skip account_id=%d turn=%d conn_id=%s reason=build_error cause=%s",
				account.ID,
				sess.turn,
				truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
				truncateOpenAIWSLogValue(replayInputErr.Error(), openAIWSLogValueMaxLen),
			)
			sess.currentTurnReplayInput = nil
			sess.currentTurnReplayInputExists = false
		} else {
			sess.currentTurnReplayInput = nextReplayInput
			sess.currentTurnReplayInputExists = nextReplayInputExists
		}
		replayHasFunctionCallOutput := sess.currentTurnReplayInputExists &&
			openAIWSRawItemsHasFunctionCallOutput(sess.currentTurnReplayInput)
		hasFunctionCallOutput = hasFunctionCallOutput || replayHasFunctionCallOutput
		if sess.storeDisabled && sess.turn > 1 && currentPreviousResponseID != "" {
			shouldKeepPreviousResponseID := false
			strictReason := ""
			var strictErr error
			if sess.lastTurnStrictState != nil {
				shouldKeepPreviousResponseID, strictReason, strictErr = shouldKeepIngressPreviousResponseIDWithStrictState(
					sess.lastTurnStrictState,
					sess.currentPayload,
					sess.lastTurnResponseID,
					hasFunctionCallOutput,
				)
			} else {
				shouldKeepPreviousResponseID, strictReason, strictErr = shouldKeepIngressPreviousResponseID(
					sess.lastTurnPayload,
					sess.currentPayload,
					sess.lastTurnResponseID,
					hasFunctionCallOutput,
				)
			}
			if strictErr != nil {
				logOpenAIWSModeInfo(
					"ingress_ws_prev_response_strict_eval account_id=%d turn=%d conn_id=%s action=keep_previous_response_id reason=%s cause=%s previous_response_id=%s expected_previous_response_id=%s has_function_call_output=%v",
					account.ID,
					sess.turn,
					truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
					normalizeOpenAIWSLogValue(strictReason),
					truncateOpenAIWSLogValue(strictErr.Error(), openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(currentPreviousResponseID, openAIWSIDValueMaxLen),
					truncateOpenAIWSLogValue(expectedPrev, openAIWSIDValueMaxLen),
					hasFunctionCallOutput,
				)
			} else if !shouldKeepPreviousResponseID {
				updatedPayload, removed, dropErr := dropPreviousResponseIDFromRawPayload(sess.currentPayload)
				if dropErr != nil || !removed {
					dropReason := "not_removed"
					if dropErr != nil {
						dropReason = "drop_error"
					}
					logOpenAIWSModeInfo(
						"ingress_ws_prev_response_strict_eval account_id=%d turn=%d conn_id=%s action=keep_previous_response_id reason=%s drop_reason=%s previous_response_id=%s expected_previous_response_id=%s has_function_call_output=%v",
						account.ID,
						sess.turn,
						truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
						normalizeOpenAIWSLogValue(strictReason),
						normalizeOpenAIWSLogValue(dropReason),
						truncateOpenAIWSLogValue(currentPreviousResponseID, openAIWSIDValueMaxLen),
						truncateOpenAIWSLogValue(expectedPrev, openAIWSIDValueMaxLen),
						hasFunctionCallOutput,
					)
				} else {
					updatedWithInput, setInputErr := setOpenAIWSPayloadInputSequence(
						updatedPayload,
						sess.currentTurnReplayInput,
						sess.currentTurnReplayInputExists,
					)
					if setInputErr != nil {
						logOpenAIWSModeInfo(
							"ingress_ws_prev_response_strict_eval account_id=%d turn=%d conn_id=%s action=keep_previous_response_id reason=%s drop_reason=set_full_input_error previous_response_id=%s expected_previous_response_id=%s cause=%s has_function_call_output=%v",
							account.ID,
							sess.turn,
							truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
							normalizeOpenAIWSLogValue(strictReason),
							truncateOpenAIWSLogValue(currentPreviousResponseID, openAIWSIDValueMaxLen),
							truncateOpenAIWSLogValue(expectedPrev, openAIWSIDValueMaxLen),
							truncateOpenAIWSLogValue(setInputErr.Error(), openAIWSLogValueMaxLen),
							hasFunctionCallOutput,
						)
					} else {
						sess.currentPayload = updatedWithInput
						sess.currentPayloadBytes = len(updatedWithInput)
						logOpenAIWSModeInfo(
							"ingress_ws_prev_response_strict_eval account_id=%d turn=%d conn_id=%s action=drop_previous_response_id_full_create reason=%s previous_response_id=%s expected_previous_response_id=%s has_function_call_output=%v",
							account.ID,
							sess.turn,
							truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
							normalizeOpenAIWSLogValue(strictReason),
							truncateOpenAIWSLogValue(currentPreviousResponseID, openAIWSIDValueMaxLen),
							truncateOpenAIWSLogValue(expectedPrev, openAIWSIDValueMaxLen),
							hasFunctionCallOutput,
						)
						currentPreviousResponseID = ""
					}
				}
			}
		}
		forcePreferredConn := sess.isStrictAffinityTurn(sess.currentPayload)
		if sess.sessionLease == nil {
			acquiredLease, acquireErr := sess.acquireTurnLease(sess.turn, sess.preferredConnID, forcePreferredConn)
			if acquireErr != nil {
				return fmt.Errorf("acquire upstream websocket: %w", acquireErr)
			}
			sess.sessionLease = acquiredLease
			sess.sessionConnID = strings.TrimSpace(sess.sessionLease.ConnID())
			if sess.storeDisabled {
				sess.pinSessionConn(sess.sessionConnID)
			} else {
				sess.unpinSessionConn(sess.sessionConnID)
			}
		}
		shouldPreflightPing := sess.turn > 1 && sess.sessionLease != nil && sess.sessionLease.SupportsIdlePingWithoutReader() && sess.turnRetry == 0
		if shouldPreflightPing && openAIWSIngressPreflightPingIdle > 0 && !sess.lastTurnFinishedAt.IsZero() {
			if time.Since(sess.lastTurnFinishedAt) < openAIWSIngressPreflightPingIdle {
				shouldPreflightPing = false
			}
		}
		if shouldPreflightPing {
			if pingErr := sess.sessionLease.PingWithTimeout(openAIWSConnHealthCheckTO); pingErr != nil {
				logOpenAIWSModeInfo(
					"ingress_ws_upstream_preflight_ping_fail account_id=%d turn=%d conn_id=%s cause=%s",
					account.ID,
					sess.turn,
					truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
					truncateOpenAIWSLogValue(pingErr.Error(), openAIWSLogValueMaxLen),
				)
				if forcePreferredConn {
					// 携带 function_call_output 的请求不能丢弃 previous_response_id：
					// 上游 API 需要 response chain 来匹配 tool_result 与之前的 tool_use，
					// 除非 replay input 已经包含与每个 tool_result 匹配的 tool_use 上下文。
					hasFCOutput := hasFunctionCallOutput
					hasReplayToolContext := hasFCOutput &&
						sess.currentTurnReplayInputExists &&
						openAIWSRawItemsHaveToolCallContextForOutputs(sess.currentTurnReplayInput)
					if !sess.turnPrevRecoveryTried && currentPreviousResponseID != "" && (!hasFCOutput || hasReplayToolContext) {
						updatedPayload, removed, dropErr := dropPreviousResponseIDFromRawPayload(sess.currentPayload)
						if dropErr != nil || !removed {
							reason := "not_removed"
							if dropErr != nil {
								reason = "drop_error"
							}
							logOpenAIWSModeInfo(
								"ingress_ws_preflight_ping_recovery_skip account_id=%d turn=%d conn_id=%s reason=%s previous_response_id=%s",
								account.ID,
								sess.turn,
								truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
								normalizeOpenAIWSLogValue(reason),
								truncateOpenAIWSLogValue(currentPreviousResponseID, openAIWSIDValueMaxLen),
							)
						} else {
							updatedWithInput, setInputErr := setOpenAIWSPayloadInputSequence(
								updatedPayload,
								sess.currentTurnReplayInput,
								sess.currentTurnReplayInputExists,
							)
							if setInputErr != nil {
								logOpenAIWSModeInfo(
									"ingress_ws_preflight_ping_recovery_skip account_id=%d turn=%d conn_id=%s reason=set_full_input_error previous_response_id=%s cause=%s",
									account.ID,
									sess.turn,
									truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
									truncateOpenAIWSLogValue(currentPreviousResponseID, openAIWSIDValueMaxLen),
									truncateOpenAIWSLogValue(setInputErr.Error(), openAIWSLogValueMaxLen),
								)
							} else {
								logOpenAIWSModeInfo(
									"ingress_ws_preflight_ping_recovery account_id=%d turn=%d conn_id=%s action=drop_previous_response_id_retry previous_response_id=%s has_function_call_output=%v has_replay_tool_context=%v",
									account.ID,
									sess.turn,
									truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
									truncateOpenAIWSLogValue(currentPreviousResponseID, openAIWSIDValueMaxLen),
									hasFCOutput,
									hasReplayToolContext,
								)
								sess.turnPrevRecoveryTried = true
								sess.currentPayload = updatedWithInput
								sess.currentPayloadBytes = len(updatedWithInput)
								sess.resetSessionLease(true)
								sess.skipBeforeTurn = true
								continue
							}
						}
					}
					if hasFCOutput && currentPreviousResponseID != "" {
						reason := "function_call_output_missing_replay_context"
						if hasReplayToolContext {
							reason = "function_call_output_replay_not_applied"
						}
						logOpenAIWSModeInfo(
							"ingress_ws_preflight_ping_recovery_skip account_id=%d turn=%d conn_id=%s reason=%s action=fail_close previous_response_id=%s has_replay_tool_context=%v",
							account.ID,
							sess.turn,
							truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
							reason,
							truncateOpenAIWSLogValue(currentPreviousResponseID, openAIWSIDValueMaxLen),
							hasReplayToolContext,
						)
					}
					sess.resetSessionLease(true)
					return NewOpenAIWSClientCloseError(
						coderws.StatusPolicyViolation,
						"upstream continuation connection is unavailable; please restart the conversation",
						pingErr,
					)
				}
				sess.resetSessionLease(true)

				acquiredLease, acquireErr := sess.acquireTurnLease(sess.turn, sess.preferredConnID, forcePreferredConn)
				if acquireErr != nil {
					return fmt.Errorf("acquire upstream websocket after preflight ping fail: %w", acquireErr)
				}
				sess.sessionLease = acquiredLease
				sess.sessionConnID = strings.TrimSpace(sess.sessionLease.ConnID())
				if sess.storeDisabled {
					sess.pinSessionConn(sess.sessionConnID)
				}
			}
		}
		connID := sess.sessionConnID
		if currentPreviousResponseID != "" {
			chainedFromLast := expectedPrev != "" && currentPreviousResponseID == expectedPrev
			currentPreviousResponseIDKind := ClassifyOpenAIPreviousResponseIDKind(currentPreviousResponseID)
			logOpenAIWSModeInfo(
				"ingress_ws_turn_chain account_id=%d turn=%d conn_id=%s previous_response_id=%s previous_response_id_kind=%s last_turn_response_id=%s chained_from_last=%v preferred_conn_id=%s header_session_id=%s header_conversation_id=%s has_turn_state=%v turn_state_len=%d has_prompt_cache_key=%v store_disabled=%v",
				account.ID,
				sess.turn,
				truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
				truncateOpenAIWSLogValue(currentPreviousResponseID, openAIWSIDValueMaxLen),
				normalizeOpenAIWSLogValue(currentPreviousResponseIDKind),
				truncateOpenAIWSLogValue(expectedPrev, openAIWSIDValueMaxLen),
				chainedFromLast,
				truncateOpenAIWSLogValue(sess.preferredConnID, openAIWSIDValueMaxLen),
				openAIWSHeaderValueForLog(sess.baseAcquireReq.Headers, "session_id"),
				openAIWSHeaderValueForLog(sess.baseAcquireReq.Headers, "conversation_id"),
				sess.turnState != "",
				len(sess.turnState),
				openAIWSPayloadStringFromRaw(sess.currentPayload, "prompt_cache_key") != "",
				sess.storeDisabled,
			)
		}

		result, relayErr := sess.sendAndRelay(sess.turn, sess.sessionLease, sess.currentPayload, sess.currentPayloadBytes, sess.currentOriginalModel, sess.currentImageBillingModel, sess.currentImageSizeTier, sess.currentImageInputSize, !sess.turnIndexedCompatibilityRetryTried)
		if relayErr != nil {
			sess.lastTurnClean = false
			if sess.retryIngressIndexedCompatibility(relayErr, sess.turn, connID) {
				continue
			}
			if sess.recoverIngressPrevResponseNotFound(relayErr, sess.turn, connID) {
				continue
			}
			if sess.retryIngressTurn(relayErr, sess.turn, connID) {
				continue
			}
			finalErr := relayErr
			if unwrapped := errors.Unwrap(relayErr); unwrapped != nil {
				finalErr = unwrapped
			}
			if hooks != nil && hooks.AfterTurn != nil {
				hooks.AfterTurn(sess.turn, nil, finalErr)
			}
			sess.sessionLease.MarkBroken()
			return finalErr
		}
		sess.turnRetry = 0
		sess.turnPrevRecoveryTried = false
		sess.turnIndexedCompatibilityRetryTried = false
		sess.lastTurnFinishedAt = time.Now()
		sess.lastTurnClean = true
		if hooks != nil && hooks.AfterTurn != nil {
			hooks.AfterTurn(sess.turn, result, nil)
		}
		if result == nil {
			return errors.New("websocket turn result is nil")
		}
		responseID := strings.TrimSpace(result.RequestID)
		sess.lastTurnResponseID = responseID
		sess.lastTurnPayload = cloneOpenAIWSPayloadBytes(sess.currentPayload)
		sess.lastTurnReplayInput = cloneOpenAIWSRawMessages(sess.currentTurnReplayInput)
		sess.lastTurnReplayInputExists = sess.currentTurnReplayInputExists
		if result.wsReplayInputExists {
			sess.lastTurnReplayInput = append(sess.lastTurnReplayInput, cloneOpenAIWSRawMessages(result.wsReplayInput)...)
			sess.lastTurnReplayInputExists = true
		}
		nextStrictState, strictStateErr := buildOpenAIWSIngressPreviousTurnStrictState(sess.currentPayload)
		if strictStateErr != nil {
			sess.lastTurnStrictState = nil
			logOpenAIWSModeInfo(
				"ingress_ws_prev_response_strict_state_skip account_id=%d turn=%d conn_id=%s reason=build_error cause=%s",
				account.ID,
				sess.turn,
				truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
				truncateOpenAIWSLogValue(strictStateErr.Error(), openAIWSLogValueMaxLen),
			)
		} else {
			sess.lastTurnStrictState = nextStrictState
		}

		if responseID != "" && sess.stateStore != nil {
			ttl := s.openAIWSResponseStickyTTL()
			logOpenAIWSBindResponseAccountWarn(sess.groupID, account.ID, responseID, sess.stateStore.BindResponseAccount(ctx, sess.groupID, responseID, account.ID, ttl))
			sess.stateStore.BindResponseConn(responseID, connID, ttl)
		}
		if sess.stateStore != nil && sess.storeDisabled && sess.sessionHash != "" {
			sess.stateStore.BindSessionConn(sess.groupID, sess.sessionHash, connID, s.openAIWSSessionStickyTTL())
		}
		if connID != "" {
			sess.preferredConnID = connID
		}

		nextClientMessage, readErr := sess.readClientMessage()
		if readErr != nil {
			if isOpenAIWSClientDisconnectError(readErr) {
				closeStatus, closeReason := summarizeOpenAIWSReadCloseError(readErr)
				logOpenAIWSModeInfo(
					"ingress_ws_client_closed account_id=%d conn_id=%s close_status=%s close_reason=%s",
					account.ID,
					truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
					closeStatus,
					truncateOpenAIWSLogValue(closeReason, openAIWSHeaderValueMaxLen),
				)
				return nil
			}
			return fmt.Errorf("read client websocket request: %w", readErr)
		}

		nextPayload, parseErr := sess.parseClientPayload(nextClientMessage)
		if parseErr != nil {
			return parseErr
		}
		if nextPayload.promptCacheKey != "" {
			// ingress 会话在整个客户端 WS 生命周期内复用同一上游连接；
			// prompt_cache_key 对握手头的更新仅在未来需要重新建连时生效。
			updatedHeaders, _, updHdrErr := s.buildOpenAIWSHeaders(ctx, c, account, token, sess.wsDecision, isCodexCLI, sess.turnState, strings.TrimSpace(c.GetHeader(openAIWSTurnMetadataHeader)), nextPayload.promptCacheKey)
			if updHdrErr != nil {
				logOpenAIWSModeInfo("ingress_ws_update_headers_failed account_id=%d err=%v", account.ID, updHdrErr)
			} else {
				sess.baseAcquireReq.Headers = updatedHeaders
			}
		}
		if nextPayload.previousResponseID != "" {
			expectedPrev := strings.TrimSpace(sess.lastTurnResponseID)
			chainedFromLast := expectedPrev != "" && nextPayload.previousResponseID == expectedPrev
			nextPreviousResponseIDKind := ClassifyOpenAIPreviousResponseIDKind(nextPayload.previousResponseID)
			logOpenAIWSModeInfo(
				"ingress_ws_next_turn_chain account_id=%d turn=%d next_turn=%d conn_id=%s previous_response_id=%s previous_response_id_kind=%s last_turn_response_id=%s chained_from_last=%v has_prompt_cache_key=%v store_disabled=%v",
				account.ID,
				sess.turn,
				sess.turn+1,
				truncateOpenAIWSLogValue(connID, openAIWSIDValueMaxLen),
				truncateOpenAIWSLogValue(nextPayload.previousResponseID, openAIWSIDValueMaxLen),
				normalizeOpenAIWSLogValue(nextPreviousResponseIDKind),
				truncateOpenAIWSLogValue(expectedPrev, openAIWSIDValueMaxLen),
				chainedFromLast,
				nextPayload.promptCacheKey != "",
				sess.storeDisabled,
			)
		}
		if sess.stateStore != nil && nextPayload.previousResponseID != "" {
			if stickyConnID, ok := sess.stateStore.GetResponseConn(nextPayload.previousResponseID); ok {
				if sess.sessionConnID != "" && stickyConnID != "" && stickyConnID != sess.sessionConnID {
					logOpenAIWSModeInfo(
						"ingress_ws_keep_session_conn account_id=%d turn=%d conn_id=%s sticky_conn_id=%s previous_response_id=%s",
						account.ID,
						sess.turn,
						truncateOpenAIWSLogValue(sess.sessionConnID, openAIWSIDValueMaxLen),
						truncateOpenAIWSLogValue(stickyConnID, openAIWSIDValueMaxLen),
						truncateOpenAIWSLogValue(nextPayload.previousResponseID, openAIWSIDValueMaxLen),
					)
				} else {
					sess.preferredConnID = stickyConnID
				}
			}
		}
		sess.currentPayload = nextPayload.payloadRaw
		sess.currentOriginalModel = nextPayload.originalModel
		sess.currentImageBillingModel = nextPayload.imageBillingModel
		sess.currentImageSizeTier = nextPayload.imageSizeTier
		sess.currentImageInputSize = nextPayload.imageInputSize
		sess.currentPayloadBytes = nextPayload.payloadBytes
		sess.storeDisabled = s.isOpenAIWSStoreDisabledInRequestRaw(sess.currentPayload, account)
		if !sess.storeDisabled {
			sess.unpinSessionConn(sess.sessionConnID)
		}
		sess.turn++
	}
}
