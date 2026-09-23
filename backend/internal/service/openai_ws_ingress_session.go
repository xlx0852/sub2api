package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func applyOpenAIWSPayloadMutation(current []byte, path string, value any) ([]byte, error) {
	next, err := sjson.SetBytes(current, path, value)
	if err == nil {
		return next, nil
	}

	// 仅在确实需要修改 payload 且 sjson 失败时，退回 map 路径确保兼容性。
	payload := make(map[string]any)
	if unmarshalErr := json.Unmarshal(current, &payload); unmarshalErr != nil {
		return nil, err
	}
	switch path {
	case "type", "model":
		payload[path] = value
	case "client_metadata." + openAIWSTurnMetadataHeader:
		setOpenAIWSTurnMetadata(payload, fmt.Sprintf("%v", value))
	default:
		return nil, err
	}
	rebuilt, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return nil, marshalErr
	}
	return rebuilt, nil
}

func (sess *openAIWSIngressSession) parseClientPayload(raw []byte) (openAIWSClientPayload, error) {
	s := sess.svc
	ctx := sess.ctx
	c := sess.c
	account := sess.account
	clientConn := sess.clientConn
	isCodexCLI := sess.isCodexCLI
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "empty websocket request payload", nil)
	}
	if !gjson.ValidBytes(trimmed) {
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", errors.New("invalid json"))
	}

	values := gjson.GetManyBytes(trimmed, "type", "model", "prompt_cache_key", "previous_response_id")
	eventType := strings.TrimSpace(values[0].String())
	normalized := trimmed
	switch eventType {
	case "":
		eventType = "response.create"
		next, setErr := applyOpenAIWSPayloadMutation(normalized, "type", eventType)
		if setErr != nil {
			return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", setErr)
		}
		normalized = next
	case "response.create":
	case "response.append":
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"response.append is not supported in ws v2; use response.create with previous_response_id",
			nil,
		)
	default:
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			fmt.Sprintf("unsupported websocket request type: %s", eventType),
			nil,
		)
	}

	originalModel := strings.TrimSpace(values[1].String())
	modelMissing := originalModel == ""
	if originalModel == "" {
		// 入站 WS 长会话里，部分客户端只在第一轮 response.create 上声明
		// model，后续 turn 复用同一 session-level model。为避免因省略
		// model 直接断开用户连接，这里回落到上一轮已通过校验的客户端模型，
		// 并在下方写回上游 payload，保证账号模型映射/fast policy/图片权限
		// 仍按同一模型执行。
		originalModel = sess.ingressSessionOriginalModel
		if originalModel == "" {
			return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(
				coderws.StatusPolicyViolation,
				"model is required in response.create payload",
				nil,
			)
		}
	}
	promptCacheKey := strings.TrimSpace(values[2].String())
	previousResponseID := strings.TrimSpace(values[3].String())
	previousResponseIDKind := ClassifyOpenAIPreviousResponseIDKind(previousResponseID)
	if previousResponseID != "" && previousResponseIDKind == OpenAIPreviousResponseIDKindMessageID {
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"previous_response_id must be a response.id (resp_*), not a message id",
			nil,
		)
	}
	if turnMetadata := strings.TrimSpace(c.GetHeader(openAIWSTurnMetadataHeader)); turnMetadata != "" {
		next, setErr := applyOpenAIWSPayloadMutation(normalized, "client_metadata."+openAIWSTurnMetadataHeader, turnMetadata)
		if setErr != nil {
			return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", setErr)
		}
		normalized = next
	}
	if sess.fingerprintIDs != nil {
		// 派生维度（请求内轮次 / 跨请求过载 epoch）变化时刷新快照，保证头（从 context
		// 重新解析）/ 体 / 后续 turn 使用同一套会话身份。
		attempt := CodexFingerprintAttempt(c)
		epoch := openAICodexOverloadSessionEpoch(account.ID, requestClientSessionID(c))
		if attempt != sess.fingerprintAttempt || epoch != sess.fingerprintEpoch {
			sess.fingerprintIDs = resolveCodexFingerprintIDsFromContext(c, account)
			sess.fingerprintAttempt = attempt
			sess.fingerprintEpoch = epoch
			sess.fingerprintTurn = 0
		}
	}
	if sess.fingerprintIDs != nil {
		turnIDs := sess.fingerprintIDs
		if sess.fingerprintTurn > 0 {
			turnIDs = nextCodexFingerprintTurnIDs(sess.fingerprintIDs)
		}
		normalized = applyCodexFingerprintJSONBytes(normalized, turnIDs)
		sess.fingerprintTurn++
	}
	apiKey := getAPIKeyFromContext(c)
	imageGenerationAllowed := GroupAllowsImageGeneration(apiKeyGroup(apiKey))
	codexImageGenerationExplicitToolPolicy := codexImageGenerationExplicitToolPolicyAllow
	if isCodexCLI {
		codexImageGenerationExplicitToolPolicy = account.CodexImageGenerationExplicitToolPolicy()
	}
	codexBridgeEnabled := isCodexCLI &&
		!isOpenAIResponsesLiteWebSocketPayload(normalized) &&
		imageGenerationAllowed &&
		codexImageGenerationExplicitToolPolicy != codexImageGenerationExplicitToolPolicyStrip &&
		s.isCodexImageGenerationBridgeEnabled(ctx, account, apiKey)
	if codexBridgeEnabled {
		payloadMap := make(map[string]any)
		if err := json.Unmarshal(normalized, &payloadMap); err != nil {
			return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", err)
		}
		bridgeModified := false
		if ensureOpenAIResponsesImageGenerationTool(payloadMap) {
			bridgeModified = true
			logOpenAIWSModeInfo("ingress_ws_codex_image_tool_injected account_id=%d", account.ID)
		}
		if ensureOpenAIResponsesImageGenerationToolChoiceAuto(payloadMap) {
			bridgeModified = true
			logOpenAIWSModeInfo("ingress_ws_codex_image_tool_choice_auto account_id=%d", account.ID)
		}
		if normalizeOpenAIResponsesImageGenerationTools(payloadMap) {
			bridgeModified = true
		}
		if applyCodexImageGenerationBridgeInstructions(payloadMap) {
			bridgeModified = true
			logOpenAIWSModeInfo("ingress_ws_codex_image_bridge_instructions_added account_id=%d", account.ID)
		}
		if bridgeModified {
			rebuilt, marshalErr := json.Marshal(payloadMap)
			if marshalErr != nil {
				return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", marshalErr)
			}
			normalized = rebuilt
		}
	}
	upstreamModel := normalizeOpenAIModelForUpstream(account, account.GetMappedModel(originalModel))
	if modelMissing || upstreamModel != originalModel {
		next, setErr := applyOpenAIWSPayloadMutation(normalized, "model", upstreamModel)
		if setErr != nil {
			return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", setErr)
		}
		normalized = next
	}
	forceStandardLite := account != nil && account.IsExternalBridge()
	if demotedBody, demoted, demoteErr := applyOpenAIResponsesLiteForUpstream(c, normalized, upstreamModel, forceStandardLite); demoteErr != nil {
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", demoteErr)
	} else if demoted {
		normalized = demotedBody
		logOpenAIWSModeInfo(
			"ingress_ws_responses_lite_demoted account_id=%d original_model=%s upstream_model=%s force_standard=%v",
			account.ID,
			truncateOpenAIWSLogValue(originalModel, openAIWSLogValueMaxLen),
			truncateOpenAIWSLogValue(upstreamModel, openAIWSLogValueMaxLen),
			forceStandardLite,
		)
	}
	if account != nil && account.IsExternalBridge() {
		downgradedBody, downgradeErr := downgradeExternalBridgeResponsesTools(c, normalized, account.ExternalBridgeKind() == "devin")
		if downgradeErr != nil {
			return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", downgradeErr)
		}
		normalized = downgradedBody
	}
	if isCodexCLI && codexImageGenerationExplicitToolPolicy == codexImageGenerationExplicitToolPolicyStrip {
		if stripped, changed, stripErr := stripOpenAIImageGenerationToolsFromRawPayload(normalized); stripErr != nil {
			return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", stripErr)
		} else if changed {
			normalized = stripped
			logOpenAIWSModeInfo("ingress_ws_codex_image_tool_stripped_by_policy account_id=%d", account.ID)
		}
	}
	if stripped, changed, stripErr := stripCodexSparkImageGenerationToolFromRawPayload(normalized, upstreamModel); stripErr != nil {
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", stripErr)
	} else if changed {
		normalized = stripped
		logOpenAIWSModeInfo("ingress_ws_codex_spark_image_tool_stripped account_id=%d", account.ID)
	}
	imageIntent := IsImageGenerationIntent(openAIResponsesEndpoint, originalModel, normalized)
	if imageIntent && !imageGenerationAllowed {
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, ImageGenerationPermissionMessage(), nil)
	}
	imageBillingModel := ""
	imageSizeTier := ""
	imageInputSize := ""
	if imageIntent {
		var imageCfgErr error
		imageCfg, imageCfgErr := resolveOpenAIResponsesImageBillingConfigDetailedFromBody(normalized, originalModel)
		if imageCfgErr != nil {
			return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, imageCfgErr.Error(), imageCfgErr)
		}
		imageBillingModel = imageCfg.Model
		imageSizeTier = imageCfg.SizeTier
		imageInputSize = imageCfg.InputSize
	}

	// Apply OpenAI Fast Policy on the response.create frame using the same
	// evaluator/normalize/scope rules as the HTTP entrypoints. This is the
	// single integration point for all WS ingress turns (first + follow-up
	// frames flow through here).
	//
	// Model fallback: first turn still requires model at the handler layer；
	// follow-up response.create frames may omit it and then reuse
	// sess.ingressSessionOriginalModel. We always write a concrete upstream model
	// before evaluating policy, so whitelist / filter behavior remains stable.
	policyApplied, blocked, policyErr := s.applyOpenAIFastPolicyToWSResponseCreate(ctx, account, upstreamModel, normalized, forceOpenAIFastFromContext(c))
	if policyErr != nil {
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", policyErr)
	}
	if blocked != nil {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
		// Send a Realtime-style error event to the client first, then
		// signal the handler to close the connection with PolicyViolation.
		// We intentionally do NOT forward this frame upstream.
		//
		// coder/websocket@v1.8.14 Conn.Write is synchronous and flushes
		// the underlying bufio writer before returning (write.go:42 →
		// 307-311), and the subsequent close handshake re-acquires the
		// same writeFrameMu, so the error event is guaranteed to reach
		// the kernel send buffer before any close frame is queued.
		eventBytes := buildOpenAIFastPolicyBlockedWSEvent(blocked)
		if eventBytes != nil {
			writeCtx, cancel := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
			_ = clientConn.Write(writeCtx, coderws.MessageText, eventBytes)
			cancel()
		}
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			blocked.Message,
			blocked,
		)
	}
	normalized = policyApplied
	// 与 HTTP OAuth 路径对齐：clamp 超长 call_id；OAuth 额外 strip max_output_tokens。
	if sanitized, sanitizeChanged, sanitizeErr := sanitizeOpenAIWSResponseCreateFrame(normalized, account); sanitizeErr != nil {
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", sanitizeErr)
	} else if sanitizeChanged {
		normalized = sanitized
	}
	sess.ingressSessionOriginalModel = originalModel

	return openAIWSClientPayload{
		payloadRaw:         normalized,
		rawForHash:         trimmed,
		promptCacheKey:     promptCacheKey,
		previousResponseID: previousResponseID,
		originalModel:      originalModel,
		imageBillingModel:  imageBillingModel,
		imageSizeTier:      imageSizeTier,
		imageInputSize:     imageInputSize,
		payloadBytes:       len(normalized),
	}, nil
}

func (sess *openAIWSIngressSession) writeClientMessage(message []byte) error {
	s := sess.svc
	ctx := sess.ctx
	clientConn := sess.clientConn
	writeCtx, cancel := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
	defer cancel()
	return clientConn.Write(writeCtx, coderws.MessageText, message)
}

func (sess *openAIWSIngressSession) readClientMessage() ([]byte, error) {
	s := sess.svc
	ctx := sess.ctx
	account := sess.account
	clientConn := sess.clientConn
	readCtx := ctx
	idleTimeout := s.openAIWSIngressInterTurnIdleTimeout()
	cancelRead := func() {}
	if idleTimeout > 0 {
		readCtx, cancelRead = context.WithTimeout(ctx, idleTimeout)
	}
	msgType, payload, readErr := clientConn.Read(readCtx)
	cancelRead()
	if readErr != nil {
		if idleTimeout > 0 && errors.Is(readErr, context.DeadlineExceeded) && ctx.Err() == nil {
			logOpenAIWSModeInfo("ingress_ws_inter_turn_idle_timeout account_id=%d timeout_seconds=%d", account.ID, int(idleTimeout.Seconds()))
			return nil, NewOpenAIWSClientCloseError(
				coderws.StatusNormalClosure,
				"websocket idle timeout",
				readErr,
			)
		}
		return nil, readErr
	}
	if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
		return nil, NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			fmt.Sprintf("unsupported websocket client message type: %s", msgType.String()),
			nil,
		)
	}
	return payload, nil
}

func (s *OpenAIGatewayService) newOpenAIWSIngressSession(
	ctx context.Context,
	c *gin.Context,
	clientConn *coderws.Conn,
	account *Account,
	token string,
	hooks *OpenAIWSIngressHooks,
	wsDecision OpenAIWSProtocolDecision,
	ingressMode string,
	forceHTTPBridge bool,
	dedicatedMode bool,
	isCodexCLI bool,
	debugEnabled bool,
	wsURL string,
	wsHost string,
	wsPath string,
) *openAIWSIngressSession {
	return &openAIWSIngressSession{
		svc:                   s,
		ctx:                   ctx,
		c:                     c,
		clientConn:            clientConn,
		account:               account,
		token:                 token,
		hooks:                 hooks,
		wsDecision:            wsDecision,
		ingressMode:           ingressMode,
		forceHTTPBridge:       forceHTTPBridge,
		dedicatedMode:         dedicatedMode,
		isCodexCLI:            isCodexCLI,
		debugEnabled:          debugEnabled,
		wsURL:                 wsURL,
		wsHost:                wsHost,
		wsPath:                wsPath,
		fingerprintIDs:        resolveCodexFingerprintIDsFromContext(c, account),
		stateStore:            s.getOpenAIWSStateStore(),
		groupID:               getOpenAIGroupIDFromContext(c),
		storeDisabledConnMode: s.openAIWSStoreDisabledConnMode(),
		turnState:             "",
	}
}

func (sess *openAIWSIngressSession) run(firstClientMessage []byte) error {
	s := sess.svc
	account := sess.account
	firstPayload, err := sess.parseClientPayload(firstClientMessage)
	if err != nil {
		return err
	}
	sess.refreshRouteState(firstPayload)
	if sess.forceHTTPBridge || s.shouldBridgeOpenAIWSHTTP(account, firstPayload.payloadBytes, firstPayload.previousResponseID) {
		return sess.runHTTPBridgeTurns(firstPayload)
	}
	return sess.runWSTurns(firstPayload)
}
