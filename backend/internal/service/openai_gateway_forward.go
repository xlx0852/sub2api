package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Forward forwards request to OpenAI API.
// Grok/Kimi and the main OpenAI/Codex HTTP path run dedicated pipelines.
// Codex restriction, passthrough, and WSv1 rejection stay at the entrance.
func (s *OpenAIGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	restrictionResult := s.detectCodexClientRestriction(c, account, body)
	apiKeyID := getAPIKeyIDFromContext(c)
	logCodexCLIOnlyDetection(ctx, c, account, apiKeyID, restrictionResult, body)
	if restrictionResult.Enabled && !restrictionResult.Matched {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"type":    "forbidden_error",
				"message": CodexClientRestrictionMessage(restrictionResult),
			},
		})
		return nil, errors.New("codex_cli_only restriction: only codex official clients are allowed")
	}

	normalizedBody, normalized, err := normalizeOpenAICodexCompactReasoningEffortForAccount(c, account, body)
	if err != nil {
		return nil, err
	}
	if normalized {
		body = normalizedBody
	}

	// Resolve WS transport early so namespace flatten can skip when the request
	// will actually ride WSv2 (egress relays events without HTTP restore).
	earlyWSDecision := s.getOpenAIWSProtocolResolver().Resolve(account)
	earlyWSDecision = resolveOpenAIWSDecisionByClientTransport(earlyWSDecision, GetOpenAIClientTransport(c), false)
	if IsOpenAICompactRequest(c) {
		earlyWSDecision = OpenAIWSProtocolDecision{
			Transport: OpenAIUpstreamTransportHTTPSSE,
			Reason:    "compact_request_forces_http",
		}
	}
	earlyPassthroughEnabled := account.IsOpenAIPassthroughEnabled()
	if shouldFlattenOpenAIResponsesNamespaces(account, earlyWSDecision.Transport, earlyPassthroughEnabled) {
		body, err = flattenOpenAIResponsesNamespaces(c, body)
		if err != nil {
			setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
				"type": "invalid_request_error", "message": err.Error(), "param": "tools",
			}})
			return nil, err
		}
	}

	originalBody := body
	requestView := newOpenAIRequestView(body)
	reqModel, reqStream, promptCacheKey := requestView.Model, requestView.Stream, requestView.PromptCacheKey
	originalModel := reqModel

	if account.Platform == PlatformGrok {
		return s.forwardGrokResponses(ctx, c, account, body, originalModel, reqStream, startTime)
	}
	if account.Platform == PlatformKimi {
		return s.forwardResponsesViaRawChatCompletions(ctx, c, account, body)
	}
	// OpenCode Go 的 DeepSeek 等默认模型是原生 Chat Completions；
	// 不能把 Responses 体直接打到 /responses，否则可能返回空响应或协议错误。
	if IsOpenCodeGoAccount(account) && openCodeGoUsesChatCompletions(reqModel) {
		return s.forwardResponsesViaRawChatCompletions(ctx, c, account, body)
	}
	// External bridge：仅在 extra 明确 force_responses / 探测支持=true 时走原生 Responses。
	// Unknown 不能默认透传：Cursor 等 sidecar 往往只有 /v1/chat/completions，
	// 误走 /v1/responses 会 404；Devin 需显式 force_responses。
	externalBridgeResponses := account.IsExternalBridge() &&
		openai_compat.ResolveResponsesSupport(account.Extra) == openai_compat.ResponsesSupportYes
	if account.IsExternalBridge() && !externalBridgeResponses {
		return s.forwardResponsesViaRawChatCompletions(ctx, c, account, body)
	}
	if account.Type == AccountTypeAPIKey && !account.IsExternalBridge() && !openai_compat.ShouldUseResponsesAPI(account.Extra) {
		return s.forwardResponsesViaRawChatCompletions(ctx, c, account, body)
	}
	if account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey {
		sanitizedBody, changed, sanitizeErr := sanitizeOpenAIResponsesInputItemIDs(body)
		if sanitizeErr != nil {
			return nil, fmt.Errorf("sanitize OpenAI Responses input item IDs: %w", sanitizeErr)
		}
		if changed {
			body = sanitizedBody
			originalBody = sanitizedBody
			requestView = newOpenAIRequestView(sanitizedBody)
			reqModel, reqStream, promptCacheKey = requestView.Model, requestView.Stream, requestView.PromptCacheKey
			originalModel = reqModel
		}
	}

	compatMessagesBridge := isOpenAICompatMessagesBridgeBody(body)
	setOpenAICompatMessagesBridgeContext(c, compatMessagesBridge)

	isCodexCLI := openai.IsCodexOfficialClientByHeaders(c.GetHeader("User-Agent"), c.GetHeader("originator")) || (s.cfg != nil && s.cfg.Gateway.ForceCodexCLI)
	codexImageGenerationExplicitToolPolicy := codexImageGenerationExplicitToolPolicyAllow
	if isCodexCLI {
		codexImageGenerationExplicitToolPolicy = account.CodexImageGenerationExplicitToolPolicy()
	}
	wsDecision := s.getOpenAIWSProtocolResolver().Resolve(account)
	clientTransport := GetOpenAIClientTransport(c)
	// 仅允许 WS 入站请求走 WS 上游，避免出现 HTTP -> WS 协议混用。
	wsDecision = resolveOpenAIWSDecisionByClientTransport(wsDecision, clientTransport, false)
	if c != nil {
		c.Set("openai_ws_transport_decision", string(wsDecision.Transport))
		c.Set("openai_ws_transport_reason", wsDecision.Reason)
	}
	if wsDecision.Transport == OpenAIUpstreamTransportResponsesWebsocketV2 {
		logOpenAIWSModeDebug(
			"selected account_id=%d account_type=%s transport=%s reason=%s model=%s stream=%v",
			account.ID,
			account.Type,
			normalizeOpenAIWSLogValue(string(wsDecision.Transport)),
			normalizeOpenAIWSLogValue(wsDecision.Reason),
			reqModel,
			reqStream,
		)
	}
	// 当前仅支持 WSv2；WSv1 命中时直接返回错误，避免出现“配置可开但行为不确定”。
	if wsDecision.Transport == OpenAIUpstreamTransportResponsesWebsocket {
		if c != nil {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"type":    "invalid_request_error",
					"message": "OpenAI WSv1 is temporarily unsupported. Please enable responses_websockets_v2.",
				},
			})
		}
		return nil, errors.New("openai ws v1 is temporarily unsupported; use ws v2")
	}
	bodySignalCompactV2 := IsOpenAIBodySignalCompactV2(c)
	// Compact (legacy or body-signal v2) must not ride WSv2; keep HTTP compact contract.
	if IsOpenAICompactRequest(c) {
		wsDecision = OpenAIWSProtocolDecision{
			Transport: OpenAIUpstreamTransportHTTPSSE,
			Reason:    "compact_request_forces_http",
		}
		if c != nil {
			c.Set("openai_ws_transport_decision", string(wsDecision.Transport))
			c.Set("openai_ws_transport_reason", wsDecision.Reason)
		}
	}

	// External bridge 仅在明确支持 Responses 时强制透传：bridge 本身就是 OpenAI 兼容上游，
	// 再走 Codex/WS pipeline 既无收益，也会因 private host / 协议改写丢掉 usage。
	passthroughEnabled := account.IsOpenAIPassthroughEnabled() || externalBridgeResponses
	// Body-signal v2 needs the compact SSE bridge; skip generic passthrough.
	if passthroughEnabled && !bodySignalCompactV2 {
		if isCodexCLI && codexImageGenerationExplicitToolPolicy == codexImageGenerationExplicitToolPolicyStrip {
			strippedBody, changed, stripErr := stripOpenAIImageGenerationToolsFromRawPayload(body)
			if stripErr != nil {
				return nil, stripErr
			}
			if changed {
				body = strippedBody
				originalBody = strippedBody
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Stripped /responses image_generation tool for Codex client by account policy")
			}
		}
		// 透传分支只需要轻量提取字段，避免热路径全量 Unmarshal。
		mappedModel := account.GetMappedModel(reqModel)
		reasoningEffort := extractOpenAIReasoningEffortFromBody(body, mappedModel)
		// 国产模型默认 effort 补充：也要用 mappedModel 判定是否是 passback-required 上游。
		reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, body, mappedModel)
		return s.forwardOpenAIPassthrough(ctx, c, account, originalBody, reqModel, reasoningEffort, reqStream, startTime)
	}

	st := &openaiForwardState{
		c:                                      c,
		startTime:                              startTime,
		body:                                   body,
		originalBody:                           originalBody,
		requestView:                            requestView,
		reqModel:                               reqModel,
		reqStream:                              reqStream,
		promptCacheKey:                         promptCacheKey,
		originalModel:                          originalModel,
		isCodexCLI:                             isCodexCLI,
		codexImageGenerationExplicitToolPolicy: codexImageGenerationExplicitToolPolicy,
		wsDecision:                             wsDecision,
		bodySignalCompactV2:                    bodySignalCompactV2,
		compatMessagesBridge:                   compatMessagesBridge,
	}
	ex := &pipeline.Exchange{
		Ctx:            ctx,
		Platform:       account.Platform,
		RequestedModel: originalModel,
		Inbound:        body,
		Account:        account,
	}
	ex.Set(openaiForwardStateKey, st)
	return s.finishOpenAIForward(st, s.openaiForwardPipeline().Run(ex))
}

func (s *OpenAIGatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string, isStream bool, promptCacheKey string, isCodexCLI bool) (*http.Request, error) {
	// Determine target URL based on account type
	var targetURL string
	switch account.Type {
	case AccountTypeOAuth:
		// OAuth accounts use ChatGPT internal API
		targetURL = chatgptCodexURL
	case AccountTypeAPIKey:
		// API Key accounts use Platform API or custom base URL
		baseURL := account.GetOpenAIBaseURL()
		if baseURL == "" {
			targetURL = openaiPlatformAPIURL
		} else {
			var validatedURL string
			var err error
			if account.IsExternalBridge() {
				validatedURL, err = validateExternalBridgeBaseURL(baseURL)
			} else {
				validatedURL, err = s.validateUpstreamBaseURL(baseURL)
			}
			if err != nil {
				return nil, err
			}
			targetURL = buildOpenAIResponsesURL(validatedURL)
		}
	default:
		targetURL = openaiPlatformAPIURL
	}
	suffix, suffixErr := openAIResponsesRequestPathSuffix(c)
	if suffixErr != nil {
		return nil, fmt.Errorf("invalid responses path: %w", suffixErr)
	}
	targetURL, suffixErr = appendOpenAIResponsesRequestPathSuffix(targetURL, suffix)
	if suffixErr != nil {
		return nil, fmt.Errorf("invalid responses path: %w", suffixErr)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	if account.IsExternalBridge() {
		req = req.WithContext(domain.WithHTTPUpstreamPrivateHostsAllowed(req.Context()))
	}

	// Build authentication for this request. Agent Identity signs a fresh
	// assertion here; OAuth/PAT/API-key keep their existing Bearer behavior.
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build openai authentication headers: %w", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Set headers specific to OAuth accounts (ChatGPT internal API)
	if account.Type == AccountTypeOAuth {
		// Required: set Host for ChatGPT API (must use req.Host, not Header.Set)
		req.Host = "chatgpt.com"
		if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, req.Header, account); err != nil {
			return nil, fmt.Errorf("resolve chatgpt account headers: %w", err)
		}
	}

	// Whitelist passthrough headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiAllowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}
	// 设备证明只对 ChatGPT 内部接口（OAuth）有意义：API-key 账号打的是 api.openai.com，
	// 把 ChatGPT.app 的设备证明发过去既无意义也多余。
	if account.Type != AccountTypeOAuth {
		req.Header.Del(openaiAttestationHeader)
	}
	// Lite demotion rewrites body shape for non-lite upstream models; the matching
	// internal header must not ride along or ChatGPT rejects the model.
	if isOpenAIResponsesLiteDemoted(c) {
		stripOpenAIResponsesLiteHeader(req.Header)
	}
	if account.Type == AccountTypeOAuth {
		compatMessagesBridge := isOpenAICompatMessagesBridgeContext(c) || isOpenAICompatMessagesBridgeBody(body)
		// 清除客户端透传的 session 头，后续用隔离后的值重新设置，防止跨用户会话碰撞。
		// 兼容旧 session_id/conversation_id 与官方 0.144 session-id/thread-id。
		clientConversationID := firstNonEmptyHeader(req.Header, "conversation_id", "thread-id", "thread_id")
		req.Header.Del("conversation_id")
		req.Header.Del("session_id")
		req.Header.Del("session-id")
		req.Header.Del("thread-id")
		req.Header.Del("thread_id")

		if compatMessagesBridge {
			req.Header.Del("OpenAI-Beta")
			req.Header.Del("originator")
		} else {
			// Codex 官方自 #5892 起已移除 HTTP OpenAI-Beta=responses=experimental。
			// 仅透传客户端自带值；不要再强制注入旧 beta，降低非官方指纹。
			req.Header.Set("originator", resolveOpenAIUpstreamOriginator(c, isCodexCLI))
		}
		apiKeyID := getAPIKeyIDFromContext(c)
		// 不再发送 version 头：实测（142 条全量抓包，App/TUI/exec 三个入口）真实客户端
		// 一律不发它。此前按「官方 provider 固定带 version」的注释补齐，是一个纯增量特征。
		if isOpenAICompactUpstreamRequest(c) {
			req.Header.Set("accept", "application/json")
			compactSession := resolveOpenAICompactSessionID(c)
			isolatedCompact := isolateOpenAISessionID(apiKeyID, compactSession)
			setOpenAIIsolatedSessionHeaders(req.Header, isolatedCompact, true)
			// 隔离必须同时覆盖会话元数据，否则上游仍能从 x-codex-turn-metadata 读到原始 thread。
			alignCodexTurnMetadataWithIsolatedSession(req.Header, isolatedCompact, isolatedCompact,
				func(raw string) string { return isolateOpenAISessionID(apiKeyID, raw) })
		} else {
			req.Header.Set("accept", "text/event-stream")
		}
		if promptCacheKey != "" {
			isolated := isolateOpenAISessionID(apiKeyID, promptCacheKey)
			withConversation := !compatMessagesBridge || clientConversationID != ""
			setOpenAIIsolatedSessionHeaders(req.Header, isolated, withConversation)
			alignCodexTurnMetadataWithIsolatedSession(req.Header, isolated, isolated,
				func(raw string) string { return isolateOpenAISessionID(apiKeyID, raw) })
		}
		// x-codex-routing-hint：官方客户端（App / TUI / exec）在 /responses 上
		// 100% 都会发送它，且实测与 body.model 严格相等（均为客户端请求的模型，
		// 而非实际服务的模型）。网关此前从不发，是一条稳定的缺失特征。
		// 这里取**出站 body 的 model**，保证与 body 自洽（网关可能已改写模型）。
		if model := strings.TrimSpace(gjson.GetBytes(body, "model").String()); model != "" {
			req.Header.Set("x-codex-routing-hint", "model="+model)
		}
	} else if isOpenAIResponsesCompactPath(c) {
		// compact 上游是 unary JSON 协议：API-key 账号也显式声明 Accept，
		// 避免 OpenAI 兼容网关按 SSE 返回（#3777 期望行为 4）。
		req.Header.Set("accept", "application/json")
	}

	// Apply custom User-Agent if configured
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
	}

	// 若开启 ForceCodexCLI，则强制将上游 User-Agent 伪装为 Codex CLI。
	// 用于网关未透传/改写 User-Agent 时，仍能命中 Codex 侧识别逻辑。
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", codexCLIUserAgent)
	}

	// 请求体压缩：官方客户端走 Codex 后端时一律对 /responses 请求体做 zstd 压缩
	// （codex-rs：enable_request_compression && auth.uses_codex_backend() && provider.is_openai()，
	// 无尺寸阈值）。compact 走 unary JSON 协议，不在此列。
	if account.Type == AccountTypeOAuth && !isOpenAICompactUpstreamRequest(c) {
		if compressed, enc := compressOpenAIUpstreamBodyZstd(body); enc != "" {
			req.Body = io.NopCloser(bytes.NewReader(compressed))
			req.ContentLength = int64(len(compressed))
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(compressed)), nil
			}
			req.Header.Set(openAIRequestCompressionHeader, enc)
		}
	}

	// 浏览器型 UA 兜底：仅 OAuth（ChatGPT 内部接口）账号生效，若最终 user-agent 仍为浏览器
	// （Chrome/Firefox/Safari/Edge 等），替换为后台配置的 Codex UA，避免 Cloudflare 触发 JS 质询。
	s.overrideBrowserUserAgent(ctx, account, req)
	if account.Type == AccountTypeOAuth && !isOpenAICompactUpstreamRequest(c) {
		applyCodexFingerprintHeaders(req.Header, resolveCodexFingerprintIDsFromContext(c, account))
	}

	// 终态收口：originator 必须与最终 User-Agent 首段配套且为官方身份，否则上游 404（issue #3901）。
	if account.Type == AccountTypeOAuth {
		s.codexIdentity.enforceHeadersWithUA(req.Header, s.codexIdentityCandidateUA(c, account))
		// OpenCode 官方主机：UA 强制收敛为 opencode 客户端形态（CF 按 UA 拦，
		// 403 会被计成凭据类故障），并带上官方要求的会话头。
		// seed 取客户端会话标识，保证同一会话每轮同值（否则上游 prompt cache 持续 miss）。
		applyOpenCodeUpstreamHeaders(req.Header, account, targetURL, openCodeSessionSeedForRequest(req.Header, body))
		s.applyOpenAICodexTurnStateReuseForModel(req.Header, account, strings.TrimSpace(gjson.GetBytes(body, "model").String()))
	}

	// Ensure required headers exist
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}

	// 账号级请求头覆写（仅 openai api_key 账号启用时生效；OAuth 路径 no-op）
	account.ApplyHeaderOverrides(req.Header)

	return req, nil
}

// overrideBrowserUserAgent 检查请求的最终 user-agent，若为浏览器 UA 则替换为后台配置的 Codex UA。
// 用于规避 Cloudflare 对浏览器型 UA 在 ChatGPT 内部接口上的访问质询。
// 影响范围严格限定：仅 OAuth（Codex/ChatGPT 内部接口）账号生效；API Key 等其他账号原样透传。
// 仅在识别为浏览器（Mozilla/...）时改写，其他 CLI/工具 UA 不动。
func (s *OpenAIGatewayService) overrideBrowserUserAgent(ctx context.Context, account *Account, req *http.Request) {
	if req == nil || account == nil {
		return
	}
	if account.Type != AccountTypeOAuth {
		return
	}
	currentUA := req.Header.Get("user-agent")
	if !openai.IsBrowserUserAgent(currentUA) {
		return
	}
	codexUA := DefaultOpenAICodexUserAgent
	if s != nil && s.settingService != nil {
		if v := strings.TrimSpace(s.settingService.GetOpenAICodexUserAgent(ctx)); v != "" {
			codexUA = v
		}
	}
	req.Header.Set("user-agent", codexUA)
}
