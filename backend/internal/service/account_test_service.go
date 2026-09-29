package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/modelcatalog"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// sseDataPrefix matches SSE data lines with optional whitespace after colon.
// Some upstream APIs return non-standard "data:" without space (should be "data: ").
var sseDataPrefix = regexp.MustCompile(`^data:\s*`)

const (
	testClaudeAPIURL   = "https://api.anthropic.com/v1/messages?beta=true"
	chatgptCodexAPIURL = "https://chatgpt.com/backend-api/codex/responses"
)

// TestEvent represents a SSE event for account testing
type TestEvent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Model    string `json:"model,omitempty"`
	Status   string `json:"status,omitempty"`
	Code     string `json:"code,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Data     any    `json:"data,omitempty"`
	Success  bool   `json:"success,omitempty"`
	Error    string `json:"error,omitempty"`
}

const (
	defaultGeminiTextTestPrompt  = "hi"
	defaultGeminiImageTestPrompt = "Generate a cute orange cat astronaut sticker on a clean pastel background."
	defaultOpenAIImageTestPrompt = "Generate a cute orange cat astronaut sticker on a clean pastel background."
)

// isOpenAIImageModel checks if the model is an OpenAI image generation model (e.g. gpt-image-2).
func isOpenAIImageModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(model), "gpt-image-")
}

// AccountTestService handles account testing operations
type AccountTestService struct {
	mirasimRegistry           *MirasimRelayRegistry
	accountRepo               AccountRepository
	geminiTokenProvider       *GeminiTokenProvider
	grokTokenProvider         *GrokTokenProvider
	kimiTokenProvider         *KimiTokenProvider
	antigravityGatewayService *AntigravityGatewayService
	httpUpstream              HTTPUpstream
	mirasimMetadata           MirasimMetadataQuerier
	cfg                       *config.Config
	tlsFPProfileService       *TLSFingerprintProfileService
	agentIdentityTaskMu       sync.Mutex
	agentIdentityWS           agentIdentityWSConnectionInvalidator
	codexIdentity             *CodexIdentity
	// openAIGatewayService 提供 Codex manifest 取回（刷新感知 token、身份头、代理）。
	// 仅在 ProvideAccountTestService 中注入；nil 时 OpenAI OAuth 同步报 configuration。
	openAIGatewayService *OpenAIGatewayService
}

// NewAccountTestService creates a new AccountTestService
func NewAccountTestService(
	accountRepo AccountRepository,
	geminiTokenProvider *GeminiTokenProvider,
	grokTokenProvider *GrokTokenProvider,
	kimiTokenProvider *KimiTokenProvider,
	antigravityGatewayService *AntigravityGatewayService,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
	tlsFPProfileService *TLSFingerprintProfileService,
	registries ...*MirasimRelayRegistry,
) *AccountTestService {
	registry := NewMirasimRelayRegistry()
	if len(registries) > 0 && registries[0] != nil {
		registry = registries[0]
	}
	return &AccountTestService{
		mirasimRegistry:           registry,
		accountRepo:               accountRepo,
		geminiTokenProvider:       geminiTokenProvider,
		grokTokenProvider:         grokTokenProvider,
		kimiTokenProvider:         kimiTokenProvider,
		antigravityGatewayService: antigravityGatewayService,
		httpUpstream:              httpUpstream,
		cfg:                       cfg,
		tlsFPProfileService:       tlsFPProfileService,
	}
}

func (s *AccountTestService) validateUpstreamBaseURL(raw string) (string, error) {
	return validateUpstreamBaseURLWithConfig(s.cfg, raw)
}

// SetMirasimMetadataService 注入 mirasim 只读元数据客户端（roster / limits）。
// 形态对标 AccountUsageService.SetMirasimMetadataService：不动构造函数签名。
func (s *AccountTestService) SetMirasimMetadataService(service MirasimMetadataQuerier) {
	if s != nil {
		s.mirasimMetadata = service
	}
}

// generateSessionString generates a Claude Code style session string.
// The output format is determined by the UA version in claude.DefaultHeaders,
// ensuring consistency between the user_id format and the UA sent to upstream.
func generateSessionString() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	hex64 := hex.EncodeToString(b)
	sessionUUID := uuid.New().String()
	uaVersion := ExtractCLIVersion(claude.DefaultHeaders["User-Agent"])
	return FormatMetadataUserID(hex64, "", sessionUUID, uaVersion), nil
}

// accountTestMaxTokens 是账号测试报文的输出预算。
//
// 推理模型会先把预算花在 thinking 上再产出答案正文，预算过小会得到
// 「只有 thinking、答案正文为空」的响应（stop_reason=max_tokens）。
// 这种响应**不是**协议或连通性故障，必须在测试输出里显式说明，
// 否则空响应会被读成「测试通过」（见 finishClaudeTestStream）。
const accountTestMaxTokens = 1024

// accountTestNeutralSystemPrompt 是发给**非 Claude 模型**的 system 提示词。
// 实测样本（deepseek-v4.1-flash，max_tokens=2048，每组 5 次）用的是这句原文，
// 改字会作废该测量，故保留原样。
const accountTestNeutralSystemPrompt = "You are a helpful assistant. Answer the user's question directly."

// createTestPayload creates a test request payload.
//
// 请求体形态对齐 Claude Code（因为 Anthropic 协议账号的主体就是 Claude 上游），
// 但 **system 身份按模型区分**：给非 Claude 模型发 "You are Claude Code" 是在测试里
// 伪造身份，会让「连通性 / 降智」判定测到的不是真实模型行为（见 testPayloadSystemPrompt）。
func createTestPayload(modelID string) (map[string]any, error) {
	sessionID, err := generateSessionString()
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"model": modelID,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": reasoningProbePrompt,
						"cache_control": map[string]string{
							"type": "ephemeral",
						},
					},
				},
			},
		},
		"system": []map[string]any{
			{
				"type": "text",
				"text": testPayloadSystemPrompt(modelID),
				"cache_control": map[string]string{
					"type": "ephemeral",
				},
			},
		},
		"metadata": map[string]string{
			"user_id": sessionID,
		},
		"max_tokens":  accountTestMaxTokens,
		"temperature": 1,
		"stream":      true,
	}, nil
}

// testPayloadSystemPrompt 返回测试请求体里的 system 提示词。
//
// 只有 Claude 模型才注入 Claude Code 身份。理由：账号测试的用途是判断
// 「上游是否真的在服务它声称的模型 / 是否降智」，把一个不属于该模型的
// 身份提示词塞进去，测到的就不是真实模型行为。
//
// 已实测（deepseek-v4.1-flash，max_tokens=2048，每组 5 次）：
// cc 5/5 截断、neutral 4/5 截断 +1/5 答对、无 system 5/5 截断。
// 即该提示词**不是**截断的主因（预算被推理吃光才是，见 accountTestMaxTokens），
// 所以这里只按"身份是否分对"来区分，不把它当性能开关。
func testPayloadSystemPrompt(modelID string) string {
	if isClaudeTestModel(modelID) {
		return claudeCodeSystemPrompt
	}
	return accountTestNeutralSystemPrompt
}

// isClaudeTestModel 判断测试模型是否是 Claude 家族。
// 兼容 "anthropic/claude-sonnet-4" 这类带 vendor 前缀的写法。
func isClaudeTestModel(modelID string) bool {
	name := strings.ToLower(strings.TrimSpace(modelID))
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return strings.HasPrefix(name, "claude")
}

// TestAccountConnection tests an account's connection by sending a test request
// All account types use full Claude Code client characteristics, only auth header differs
// modelID is optional - if empty, defaults to claude.CurrentDefaultTestModel()
// mode is optional - "compact" routes OpenAI accounts to the /responses/compact probe path
// TestAccountConnection streams a connectivity test and, for text answers, appends the
// reasoning-probe verdict so an operator immediately sees whether the upstream still reasons
// like the model it claims to be.
func (s *AccountTestService) TestAccountConnection(c *gin.Context, accountID int64, modelID string, prompt string, mode string) error {
	tee := newSSETeeWriter(c)
	err := s.streamAccountConnection(c, accountID, modelID, prompt, mode)
	s.emitReasoningProbeVerdict(c, tee.recorded())
	return err
}

// sseTeeWriter forwards every SSE byte to the client while keeping a copy for judging.
type sseTeeWriter struct {
	gin.ResponseWriter
	buffer *bytes.Buffer
}

func newSSETeeWriter(c *gin.Context) *sseTeeWriter {
	tee := &sseTeeWriter{ResponseWriter: c.Writer, buffer: &bytes.Buffer{}}
	c.Writer = tee
	return tee
}

func (w *sseTeeWriter) Write(data []byte) (int, error) {
	_, _ = w.buffer.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *sseTeeWriter) recorded() string { return w.buffer.String() }

// emitReasoningProbeVerdict appends the probe judgement to the stream. Image tests and
// non-text payloads are skipped: their "answer" is a URL or base64 blob, not a number.
func (s *AccountTestService) emitReasoningProbeVerdict(c *gin.Context, body string) {
	answer, errMsg := parseTestSSEOutput(body)
	answer = strings.TrimSpace(answer)
	if errMsg != "" {
		return
	}
	if answer == "" {
		// 不能静默返回：空答案与「已判定为正确」在界面上同形，会让人把
		// 「没拿到答案」读成「测试通过」。这里明确说明本样本未判定。
		s.sendEvent(c, TestEvent{Type: "content", Text: "\n" + probeNoAnswerVerdict()})
		return
	}
	if looksLikeBinaryOrImageAnswer(answer) {
		return
	}
	expected := s.expectedProbeAnswer()
	s.sendEvent(c, TestEvent{Type: "content", Text: "\n" + formatProbeVerdictWith(judgeReasoningProbeWith(answer, expected), expected)})
}

// looksLikeBinaryOrImageAnswer guards the judge against payloads that are not a text answer.
func looksLikeBinaryOrImageAnswer(answer string) bool {
	lowered := strings.ToLower(answer)
	for _, marker := range []string{"data:image", "base64,", "http://", "https://"} {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return len(answer) > 8192
}

func (s *AccountTestService) streamAccountConnection(c *gin.Context, accountID int64, modelID string, prompt string, mode string) error {
	ctx := c.Request.Context()

	// Get account
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return s.sendErrorAndEnd(c, "Account not found")
	}
	if err := rejectRetiredSupplyAccount(account); err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	if account.IsExternalBridge() {
		testModelID := strings.TrimSpace(modelID)
		if testModelID == "" {
			testModelID = strings.TrimSpace(account.GetCredential("model"))
		}
		if testModelID == "" {
			testModelID = openai.CurrentDefaultTestModel()
		}
		testModelID = account.GetMappedModel(testModelID)
		baseURL, err := validateExternalBridgeBaseURL(account.GetOpenAIBaseURL())
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid external bridge base URL: %s", err.Error()))
		}
		token := account.GetOpenAIApiKey()
		if token == "" {
			return s.sendErrorAndEnd(c, "External bridge API key is missing")
		}
		return s.testOpenAIChatCompletionsConnection(c, account, testModelID, prompt, baseURL, token)
	}

	// Route to platform-specific test method
	if account.IsOpenAI() {
		return s.testOpenAIAccountConnection(c, account, modelID, prompt, normalizeAccountTestMode(mode))
	}

	if account.IsGemini() {
		return s.testGeminiAccountConnection(c, account, modelID, prompt)
	}

	if account.Platform == PlatformGrok {
		return s.testGrokAccountConnection(c, account, modelID)
	}
	if account.Platform == PlatformKimi {
		if s.kimiTokenProvider == nil {
			return s.sendErrorAndEnd(c, "Kimi token provider not configured")
		}
		token, err := s.kimiTokenProvider.GetAccessToken(ctx, account)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to get Kimi access token: %s", err.Error()))
		}
		if strings.TrimSpace(modelID) == "" {
			modelID = modelcatalog.KimiDefaultTestModel()
		}
		modelID = normalizeKimiUpstreamModel(account.GetMappedModel(modelID))
		baseURL := strings.TrimSpace(account.GetCredential("base_url"))
		if baseURL == "" {
			baseURL = "https://api.kimi.com/coding"
		}
		return s.testOpenAIChatCompletionsConnection(c, account, modelID, prompt, baseURL, token)
	}

	if account.Platform == PlatformAntigravity {
		return s.routeAntigravityTest(c, account, modelID, prompt)
	}

	return s.testClaudeAccountConnection(c, account, modelID)
}

// testClaudeAccountConnection tests an Anthropic Claude account's connection
func (s *AccountTestService) testClaudeAccountConnection(c *gin.Context, account *Account, modelID string) error {
	ctx := c.Request.Context()

	// Determine the model to use
	testModelID := modelID
	if testModelID == "" {
		testModelID = claude.CurrentDefaultTestModel()
	}

	// API Key 账号测试连接时也需要应用通配符模型映射。
	if account.Type == "apikey" {
		testModelID = account.GetMappedModel(testModelID)
	}

	if account.Type == AccountTypeServiceAccount {
		return s.sendErrorAndEnd(c, ErrRetiredVertexServiceAccount.Error())
	}

	// Determine authentication method and API URL
	var authToken string
	var apiURL string

	// Mirasim relay 账号没有 api_key：URL 与鉴权都由 relay 与签名头决定。
	isMirasimRelay := isMirasimRelayAccount(account)

	if account.IsOAuth() {
		apiURL = testClaudeAPIURL
		authToken = account.GetCredential("access_token")
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No access token available")
		}
	} else if isMirasimRelay {
		relayCfg, cfgErr := mirasimRelayConfigForAccount(account)
		if cfgErr != nil {
			return s.sendErrorAndEnd(c, cfgErr.Error())
		}
		normalizedBaseURL, err := s.validateUpstreamBaseURL(relayCfg.RelayURL)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
		}
		// relay 的模型端点不带 beta query：签名与密封的 AAD 只覆盖 path，带 query 会判无效。
		apiURL = strings.TrimSuffix(normalizedBaseURL, "/") + "/v1/messages"
	} else if account.Type == "apikey" {
		authToken = account.GetCredential("api_key")
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No API key available")
		}

		baseURL := account.GetBaseURL()
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
		}
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
		}
		apiURL = strings.TrimSuffix(normalizedBaseURL, "/") + "/v1/messages?beta=true"
	} else {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported account type: %s", account.Type))
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Create Claude Code style payload (same for all account types)
	payload, err := createTestPayload(testModelID)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create test payload")
	}
	payloadBytes, _ := json.Marshal(payload)

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}

	// Set common headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	// Apply Claude Code client headers
	for key, value := range claude.DefaultHeaders {
		req.Header.Set(key, value)
	}

	// Set authentication header
	if account.IsOAuth() {
		req.Header.Set("anthropic-beta", claude.DefaultBetaHeader)
		req.Header.Set("Authorization", "Bearer "+authToken)
	} else if !isMirasimRelay {
		req.Header.Set("anthropic-beta", claude.APIKeyBetaHeader)
		setAnthropicAPIKeyAuthHeader(req.Header, account, authToken)
	}

	// 账号级请求头覆写：测试请求与真实转发保持一致的最终头
	account.ApplyHeaderOverrides(req.Header)

	// Mirasim relay：换成 ticket + 签名密封头（与网关路径同一注入函数，
	// 保证测试请求与真实转发完全同构）。
	if isMirasimRelay {
		if err := applyMirasimRelayAuthToRequest(ctx, s.httpUpstream, s.accountRepo, account, req, payloadBytes, s.mirasimRegistry); err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
		}
	}

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body))

		// 403 表示账号被上游封禁，标记为 error 状态
		// Mirasim relay 例外：relay 的 403 多为风控/设备身份问题，永久禁用会误杀。
		if resp.StatusCode == http.StatusForbidden && !isMirasimRelay {
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}

		return s.sendErrorAndEnd(c, errMsg)
	}

	// Process SSE stream
	return s.processClaudeStream(c, resp.Body)
}

func (s *AccountTestService) testOpenAIAccountConnection(c *gin.Context, account *Account, modelID string, prompt string, mode string) error {
	ctx := c.Request.Context()
	mode = normalizeAccountTestMode(mode)

	// Default to openai.CurrentDefaultTestModel() for OpenAI testing
	testModelID := modelID
	if testModelID == "" {
		testModelID = openai.CurrentDefaultTestModel()
	}
	if account.IsExternalBridge() {
		// Scheduled diagnostics use generic OpenAI probe models. A sidecar may
		// expose a narrower catalog, so select its configured model when the
		// requested probe is outside the account mapping.
		bridgeModel := strings.TrimSpace(account.GetCredential("model"))
		if bridgeModel == "" {
			for _, mapped := range account.GetModelMapping() {
				if strings.TrimSpace(mapped) != "" {
					bridgeModel = strings.TrimSpace(mapped)
					break
				}
			}
		}
		if bridgeModel != "" && (strings.TrimSpace(modelID) == "" || !account.IsModelSupported(modelID)) {
			testModelID = bridgeModel
		}
	}

	// Align test routing with gateway behavior: OpenAI accounts apply normal
	// account model mapping, and compact mode applies compact-only mapping on top.
	testModelID = account.GetMappedModel(testModelID)
	if mode == AccountTestModeCompact {
		testModelID = resolveOpenAICompactForwardModel(account, testModelID)
		return s.testOpenAICompactConnection(c, account, testModelID)
	}

	// Route to image generation test if an image model is selected
	if isOpenAIImageModel(testModelID) {
		imagePrompt := strings.TrimSpace(prompt)
		if imagePrompt == "" {
			imagePrompt = defaultOpenAIImageTestPrompt
		}
		if account.Type == "apikey" {
			return s.testOpenAIImageAPIKey(c, ctx, account, testModelID, imagePrompt)
		}
		return s.testOpenAIImageOAuth(c, ctx, account, testModelID, imagePrompt)
	}

	credentialAccount := account
	if account.IsCredentialShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return s.sendErrorAndEnd(c, err.Error())
		}
		credentialAccount = resolved
	}

	// Determine authentication method and API URL
	var authToken string
	var apiURL string
	var isOAuth bool

	if credentialAccount.IsOAuth() {
		isOAuth = true
		// Agent Identity signs each request and does not retain the OAuth token.
		if !credentialAccount.IsOpenAIAgentIdentity() {
			authToken = credentialAccount.GetOpenAIAccessToken()
		}
		if authToken == "" && !credentialAccount.IsOpenAIAgentIdentity() {
			return s.sendErrorAndEnd(c, "No access token available")
		}

		// OAuth uses ChatGPT internal API
		apiURL = chatgptCodexAPIURL
	} else if credentialAccount.Type == "apikey" {
		// API Key - use Platform API
		authToken = credentialAccount.GetOpenAIApiKey()
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No API key available")
		}

		baseURL := credentialAccount.GetOpenAIBaseURL()
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		var normalizedBaseURL string
		var err error
		if credentialAccount.IsExternalBridge() {
			normalizedBaseURL, err = validateExternalBridgeBaseURL(baseURL)
		} else {
			normalizedBaseURL, err = s.validateUpstreamBaseURL(baseURL)
		}
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
		}
		if IsOpenCodeGoAccount(credentialAccount) && openCodeGoUsesChatCompletions(testModelID) {
			return s.testOpenAIChatCompletionsConnection(c, credentialAccount, testModelID, prompt, normalizedBaseURL, authToken)
		}
		if !openai_compat.ShouldUseResponsesAPI(account.Extra) {
			return s.testOpenAIChatCompletionsConnection(c, account, testModelID, prompt, normalizedBaseURL, authToken)
		}
		apiURL = buildOpenAIResponsesURL(normalizedBaseURL)
	} else {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported account type: %s", account.Type))
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Create OpenAI Responses API payload
	payload := createOpenAIDiagnosticPayload(testModelID, isOAuth)
	payloadBytes, _ := json.Marshal(payload)

	// Send test_start event once. A task-invalid Agent Identity response may
	// restart this probe after registering a replacement task.
	if !agentIdentityTaskRecoveryWasTried(ctx) {
		s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	if account.IsExternalBridge() {
		req = req.WithContext(domain.WithHTTPUpstreamPrivateHostsAllowed(req.Context()))
	}

	// Set common headers
	req.Header.Set("Content-Type", "application/json")
	if credentialAccount.IsOpenAIAgentIdentity() {
		authHeaders, authErr := buildAgentIdentityAuthenticationHeaders(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, credentialAccount)
		if authErr != nil {
			return s.sendErrorAndEnd(c, "Failed to build Agent Identity authentication")
		}
		for key, values := range authHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	// Set OAuth-specific headers for ChatGPT internal API
	if isOAuth {
		req.Host = "chatgpt.com"
		req.Header.Set("accept", "text/event-stream")
		// 对齐 Codex 0.144 HTTP：不再注入已废弃的 responses=experimental。
		req.Header.Set("Originator", "codex_cli_rs")
		if customUA := strings.TrimSpace(credentialAccount.GetOpenAIUserAgent()); customUA != "" {
			req.Header.Set("User-Agent", customUA)
		} else {
			req.Header.Set("User-Agent", codexCLIUserAgent)
		}
		setOpenAIChatGPTAccountHeaders(req.Header, credentialAccount)
		// 与真实转发一致：账号级自定义 UA 同样作为管理员显式配置传入。
		s.codexIdentity.enforceHeadersWithUA(req.Header, credentialAccount.GetOpenAIUserAgent())
	}

	// 账号级请求头覆写：测试请求与真实转发保持一致的最终头
	// OpenCode 官方主机：必须带 x-opencode-session（缺则 400 MissingSessionID），
	// 且 UA 必须是 opencode 客户端形态（Cloudflare 1010 会把编程库 UA 拦成 403，
	// 该 403 在本仓会被计成凭据类故障，可能把健康账号停掉）。
	// 放在 ApplyHeaderOverrides 之前：管理员显式配置的请求头仍拥有最终决定权。
	applyOpenCodeUpstreamHeaders(req.Header, credentialAccount, apiURL, "")
	credentialAccount.ApplyHeaderOverrides(req.Header)

	// Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if isOAuth && s.accountRepo != nil {
		if updates, err := extractOpenAICodexProbeUpdates(resp); err == nil && len(updates) > 0 {
			_ = s.accountRepo.UpdateExtra(ctx, account.ID, updates)
			mergeAccountExtra(account, updates)
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		body = redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, credentialAccount, body)
		if !agentIdentityTaskRecoveryWasTried(ctx) && credentialAccount.IsOpenAIAgentIdentity() && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, body) {
			expectedTaskID := credentialAccount.GetCredential("task_id")
			if err := ensureAgentIdentityTaskForAccount(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, credentialAccount, expectedTaskID); err != nil {
				return s.sendErrorAndEnd(c, fmt.Sprintf("Agent Identity task recovery failed: %s", err.Error()))
			}
			c.Request = c.Request.WithContext(markAgentIdentityTaskRecoveryTried(ctx))
			return s.testOpenAIAccountConnection(c, account, modelID, prompt, mode)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			s.reconcileOpenAI429State(ctx, account, resp.Header, body)
		}
		// 401 Unauthorized: 标记账号为永久错误
		if resp.StatusCode == http.StatusUnauthorized && s.accountRepo != nil {
			errMsg := fmt.Sprintf("Authentication failed (401): %s", string(body))
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	// Process SSE stream
	return s.processOpenAIStream(c, resp.Body)
}

// testGrokAccountConnection tests a Grok OAuth or API Key account through xAI's Responses API.
func (s *AccountTestService) testGrokAccountConnection(c *gin.Context, account *Account, modelID string) error {
	ctx := c.Request.Context()

	if s.httpUpstream == nil {
		return s.sendErrorAndEnd(c, "HTTP upstream not configured")
	}

	testModelID := strings.TrimSpace(modelID)
	if testModelID == "" {
		testModelID = xai.CurrentDefaultChatModel()
	}
	if mapped := strings.TrimSpace(account.GetMappedModel(testModelID)); mapped != "" {
		testModelID = mapped
	}

	var authToken string
	switch account.Type {
	case AccountTypeOAuth:
		if s.grokTokenProvider == nil {
			return s.sendErrorAndEnd(c, "Grok token provider not configured")
		}
		var err error
		// 手动测试不走生产请求路径：刷新失败时不写 temp_unschedulable，
		// 便于管理员探测冷却/限流账号（#4598）。
		authToken, err = s.grokTokenProvider.GetAccessTokenForManualTest(ctx, account)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to get Grok access token: %s", err.Error()))
		}
	case AccountTypeAPIKey:
		authToken = strings.TrimSpace(account.GetCredential("api_key"))
		if authToken == "" {
			return s.sendErrorAndEnd(c, "Grok API key not found in credentials")
		}
	default:
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported Grok account type: %s", account.Type))
	}

	apiURL, err := xai.BuildResponsesURL(account.GetGrokChatBaseURL())
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid Grok base URL: %s", err.Error()))
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	payloadBytes, err := json.Marshal(map[string]any{
		"model":  testModelID,
		"input":  "hi",
		"stream": true,
	})
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create Grok test payload")
	}

	if !agentIdentityTaskRecoveryWasTried(ctx) {
		s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create Grok request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileGrok))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("User-Agent", "grok-proxy/1.0")
	if xai.IsCLIChatProxyBaseURL(account.GetGrokChatBaseURL()) {
		xai.ApplyGrokCLIChatHeaders(req)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Grok Responses API request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if snapshot := xai.ParseQuotaHeaders(resp.Header, resp.StatusCode); snapshot != nil && s.accountRepo != nil {
		_ = s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
			grokQuotaSnapshotExtraKey: snapshot,
		})
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return s.sendErrorAndEnd(c, fmt.Sprintf("Grok Responses API returned %d: %s", resp.StatusCode, string(body)))
	}

	return s.processOpenAIStream(c, resp.Body)
}

// testOpenAIChatCompletionsConnection tests an OpenAI-compatible APIKey account
// through the raw /v1/chat/completions endpoint.
func (s *AccountTestService) testOpenAIChatCompletionsConnection(
	c *gin.Context,
	account *Account,
	testModelID string,
	prompt string,
	normalizedBaseURL string,
	authToken string,
) error {
	ctx := c.Request.Context()
	apiURL := buildOpenAIChatCompletionsURL(normalizedBaseURL)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	payload := createOpenAIChatCompletionsTestPayload(testModelID, prompt)
	payloadBytes, _ := json.Marshal(payload)

	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	s.sendEvent(c, TestEvent{Type: "status", Text: "正在通过 /v1/chat/completions 测试连接"})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create Chat Completions request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+authToken)
	if account.Platform == PlatformKimi {
		applyKimiCodingHeaders(req.Header, account)
	}

	// 账号级请求头覆写：测试请求与真实转发保持一致的最终头
	// OpenCode 官方主机：必须带 x-opencode-session（缺则 400 MissingSessionID），
	// 且 UA 必须是 opencode 客户端形态（Cloudflare 1010 会把编程库 UA 拦成 403，
	// 该 403 在本仓会被计成凭据类故障，可能把健康账号停掉）。
	// 放在 ApplyHeaderOverrides 之前：管理员显式配置的请求头仍拥有最终决定权。
	applyOpenCodeUpstreamHeaders(req.Header, account, apiURL, "")
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Chat Completions API (/v1/chat/completions) request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusTooManyRequests {
			s.reconcileOpenAI429State(ctx, account, resp.Header, body)
		}
		if resp.StatusCode == http.StatusUnauthorized && s.accountRepo != nil {
			errMsg := fmt.Sprintf("Chat Completions authentication failed (401): %s", string(body))
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("Chat Completions API (/v1/chat/completions) returned %d: %s", resp.StatusCode, string(body)))
	}

	return s.processOpenAIChatCompletionsStream(c, resp.Body)
}

// testOpenAICompactConnection probes /responses/compact and persists the
// resulting capability state on the account.
func (s *AccountTestService) testOpenAICompactConnection(c *gin.Context, account *Account, testModelID string) error {
	ctx := c.Request.Context()
	credentialAccount := account
	if account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return s.sendErrorAndEnd(c, "Failed to resolve account credentials")
		}
		credentialAccount = resolved
	}

	authToken := ""
	apiURL := ""
	isOAuth := false

	switch {
	case credentialAccount.IsOAuth():
		isOAuth = true
		if !credentialAccount.IsOpenAIAgentIdentity() {
			authToken = credentialAccount.GetOpenAIAccessToken()
		}
		if authToken == "" && !credentialAccount.IsOpenAIAgentIdentity() {
			return s.sendErrorAndEnd(c, "No access token available")
		}
		apiURL = chatgptCodexAPIURL + "/compact"
	case account.Type == AccountTypeAPIKey:
		authToken = account.GetOpenAIApiKey()
		if authToken == "" {
			return s.sendErrorAndEnd(c, "No API key available")
		}
		baseURL := account.GetOpenAIBaseURL()
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
		}
		nextURL, suffixErr := appendOpenAIResponsesRequestPathSuffix(buildOpenAIResponsesURL(normalizedBaseURL), "/compact")
		if suffixErr != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid responses path: %s", suffixErr.Error()))
		}
		apiURL = nextURL
	default:
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported account type: %s", account.Type))
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	payloadBytes, _ := json.Marshal(createOpenAICompactProbePayload(testModelID))
	if !agentIdentityTaskRecoveryWasTried(ctx) {
		s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if credentialAccount.IsOpenAIAgentIdentity() {
		authHeaders, authErr := buildAgentIdentityAuthenticationHeaders(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, credentialAccount)
		if authErr != nil {
			return s.sendErrorAndEnd(c, "Failed to build Agent Identity authentication")
		}
		for key, values := range authHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	// 对齐 Codex 0.144 HTTP：不再注入已废弃的 responses=experimental。
	req.Header.Set("Originator", "codex_cli_rs")
	req.Header.Set("User-Agent", codexCLIUserAgent)
	// 不发 version 头：实测真实客户端（App/TUI/exec）在所有 Codex 端点上都不发它。
	probeSessionID := compactProbeSessionID(account.ID)
	req.Header.Set("Session_ID", probeSessionID)
	req.Header.Set("Conversation_ID", probeSessionID)

	if isOAuth {
		req.Host = "chatgpt.com"
		setOpenAIChatGPTAccountHeaders(req.Header, credentialAccount)
	}

	// 账号级请求头覆写：测试请求与真实转发保持一致的最终头
	// OpenCode 官方主机：必须带 x-opencode-session（缺则 400 MissingSessionID），
	// 且 UA 必须是 opencode 客户端形态（Cloudflare 1010 会把编程库 UA 拦成 403，
	// 该 403 在本仓会被计成凭据类故障，可能把健康账号停掉）。
	// 放在 ApplyHeaderOverrides 之前：管理员显式配置的请求头仍拥有最终决定权。
	applyOpenCodeUpstreamHeaders(req.Header, account, apiURL, "")
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		if s.accountRepo != nil {
			updates := buildOpenAICompactProbeExtraUpdates(nil, nil, err, time.Now())
			_ = s.accountRepo.UpdateExtra(ctx, account.ID, updates)
			mergeAccountExtra(account, updates)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	body = redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, credentialAccount, body)
	if !agentIdentityTaskRecoveryWasTried(ctx) && credentialAccount.IsOpenAIAgentIdentity() && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, body) {
		expectedTaskID := credentialAccount.GetCredential("task_id")
		if err := ensureAgentIdentityTaskForAccount(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, credentialAccount, expectedTaskID); err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Agent Identity task recovery failed: %s", err.Error()))
		}
		c.Request = c.Request.WithContext(markAgentIdentityTaskRecoveryTried(ctx))
		return s.testOpenAICompactConnection(c, account, testModelID)
	}

	if s.accountRepo != nil {
		updates := buildOpenAICompactProbeExtraUpdates(resp, body, nil, time.Now())
		if codexUpdates, err := extractOpenAICodexProbeUpdates(resp); err == nil && len(codexUpdates) > 0 {
			updates = mergeExtraUpdates(updates, codexUpdates)
		}
		if len(updates) > 0 {
			_ = s.accountRepo.UpdateExtra(ctx, account.ID, updates)
			mergeAccountExtra(account, updates)
		}
		// 探测如返回 429,主动同步限流状态,避免后续短时间内继续选中。
		if resp.StatusCode == http.StatusTooManyRequests {
			s.reconcileOpenAI429State(ctx, account, resp.Header, body)
		}
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized && s.accountRepo != nil {
			errMsg := fmt.Sprintf("Authentication failed (401): %s", string(body))
			_ = s.accountRepo.SetError(ctx, account.ID, errMsg)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	s.sendEvent(c, TestEvent{Type: "content", Text: "Compact probe succeeded"})
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

func (s *AccountTestService) reconcileOpenAI429State(ctx context.Context, account *Account, headers http.Header, body []byte) {
	if s == nil || s.accountRepo == nil || account == nil {
		return
	}

	persistOpenAI429PlanType(ctx, s.accountRepo, account, body)

	// Prefer header snapshot (x-codex-*); body-only plan-limited still yields a wait window.
	var resetAt *time.Time
	if calculated := calculateOpenAI429ResetTime(headers); calculated != nil {
		resetAt = calculated
	} else if unixTs := parseOpenAIRateLimitResetTime(body); unixTs != nil {
		t := time.Unix(*unixTs, 0)
		resetAt = &t
	}
	if resetAt == nil {
		return
	}

	if err := s.accountRepo.SetRateLimited(ctx, account.ID, *resetAt); err != nil {
		return
	}

	now := time.Now()
	account.RateLimitedAt = &now
	account.RateLimitResetAt = resetAt

	// 跑满时同步额度窗口为 100%，避免 UI 只看到上游英文报错而看不到“满了”。
	if updates := buildOpenAIPlanLimitedUsageExtraUpdates(headers, body, *resetAt, now); len(updates) > 0 {
		_ = s.accountRepo.UpdateExtra(ctx, account.ID, updates)
		mergeAccountExtra(account, updates)
	}

	if account.Status == StatusError {
		if err := s.accountRepo.ClearError(ctx, account.ID); err != nil {
			return
		}
		account.Status = StatusActive
		account.ErrorMessage = ""
	}
}

// buildOpenAIPlanLimitedUsageExtraUpdates marks the primary usage window as full
// when ChatGPT reports plan-limited / usage-limit-reached without precise percent headers.
func buildOpenAIPlanLimitedUsageExtraUpdates(headers http.Header, body []byte, resetAt, now time.Time) map[string]any {
	if snapshot := ParseCodexRateLimitHeaders(headers); snapshot != nil {
		return buildCodexUsageExtraUpdates(snapshot, now)
	}

	msg := strings.TrimSpace(string(body))
	var parsed map[string]any
	if json.Unmarshal(body, &parsed) == nil {
		msg = extractOpenAIErrorMessage(parsed)
	}
	if !isOpenAIPlanLimitedMessage(msg) {
		// usage_limit_reached without percent headers still means the window is exhausted.
		if !bytes.Contains(bytes.ToLower(body), []byte("usage_limit_reached")) &&
			!bytes.Contains(bytes.ToLower(body), []byte("rate_limit_exceeded")) {
			return nil
		}
	}

	resetAfter := int(resetAt.Sub(now).Seconds())
	if resetAfter < 1 {
		resetAfter = 1
	}
	updates := map[string]any{
		"codex_5h_used_percent":             100.0,
		"codex_5h_reset_after_seconds":      resetAfter,
		"codex_5h_reset_at":                 resetAt.Format(time.RFC3339),
		"codex_usage_updated_at":            now.Format(time.RFC3339),
		"codex_primary_used_percent":        100.0,
		"codex_primary_reset_after_seconds": resetAfter,
	}
	return updates
}

// testGeminiAccountConnection tests a Gemini account's connection
func (s *AccountTestService) testGeminiAccountConnection(c *gin.Context, account *Account, modelID string, prompt string) error {
	ctx := c.Request.Context()

	// Determine the model to use
	testModelID := modelID
	if testModelID == "" {
		testModelID = GeminiDefaultTestModel()
	}

	// For static upstream credentials with model mapping, map the model
	if account.Type == AccountTypeAPIKey {
		mapping := account.GetModelMapping()
		if len(mapping) > 0 {
			if mappedModel, exists := mapping[testModelID]; exists {
				testModelID = mappedModel
			}
		}
	}
	if account.Type == AccountTypeServiceAccount {
		return s.sendErrorAndEnd(c, ErrRetiredVertexServiceAccount.Error())
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Create test payload (Gemini format)
	payload := createGeminiTestPayload(testModelID, prompt)

	// Build request based on account type
	var req *http.Request
	var err error

	switch account.Type {
	case AccountTypeAPIKey:
		req, err = s.buildGeminiAPIKeyRequest(ctx, account, testModelID, payload)
	case AccountTypeOAuth:
		req, err = s.buildGeminiOAuthRequest(ctx, account, testModelID, payload)
	default:
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported account type: %s", account.Type))
	}

	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to build request: %s", err.Error()))
	}

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	// Get proxy and execute request
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	// Process SSE stream
	return s.processGeminiStream(c, resp.Body)
}

// routeAntigravityTest 路由 Antigravity 账号的测试请求。
// APIKey 类型走原生协议（与 gateway_handler 路由一致），OAuth/Upstream 走 CRS 中转。
func (s *AccountTestService) routeAntigravityTest(c *gin.Context, account *Account, modelID string, prompt string) error {
	if account.Type == AccountTypeAPIKey {
		if strings.HasPrefix(modelID, "gemini-") {
			return s.testGeminiAccountConnection(c, account, modelID, prompt)
		}
		return s.testClaudeAccountConnection(c, account, modelID)
	}
	return s.testAntigravityAccountConnection(c, account, modelID)
}

// testAntigravityAccountConnection tests an Antigravity account's connection
// 支持 Claude 和 Gemini 两种协议，使用非流式请求
func (s *AccountTestService) testAntigravityAccountConnection(c *gin.Context, account *Account, modelID string) error {
	ctx := c.Request.Context()

	// 默认模型：Claude 使用 claude-sonnet-4-5，Gemini 使用 gemini-3-pro-preview
	testModelID := modelID
	if testModelID == "" {
		testModelID = "claude-sonnet-4-5"
	}

	if s.antigravityGatewayService == nil {
		return s.sendErrorAndEnd(c, "Antigravity gateway service not configured")
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Send test_start event
	s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})

	// 调用 AntigravityGatewayService.TestConnection（复用协议转换逻辑）
	result, err := s.antigravityGatewayService.TestConnection(ctx, account, testModelID)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}

	// 发送响应内容
	if result.Text != "" {
		s.sendEvent(c, TestEvent{Type: "content", Text: result.Text})
	}

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

// buildGeminiAPIKeyRequest builds request for Gemini API Key accounts
func (s *AccountTestService) buildGeminiAPIKeyRequest(ctx context.Context, account *Account, modelID string, payload []byte) (*http.Request, error) {
	apiKey := account.GetCredential("api_key")
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("no API key available")
	}

	baseURL := account.GetCredential("base_url")
	if baseURL == "" {
		baseURL = GeminiAIStudioBaseURL
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	// Use streamGenerateContent for real-time feedback
	fullURL, err := buildGeminiAIStudioModelURL(normalizedBaseURL, modelID, "streamGenerateContent", true)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	return req, nil
}

// buildGeminiOAuthRequest builds request for Gemini OAuth accounts
func (s *AccountTestService) buildGeminiOAuthRequest(ctx context.Context, account *Account, modelID string, payload []byte) (*http.Request, error) {
	if s.geminiTokenProvider == nil {
		return nil, fmt.Errorf("gemini token provider not configured")
	}

	// Get access token (auto-refreshes if needed)
	accessToken, err := s.geminiTokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	if projectID == "" {
		// AI Studio OAuth mode (no project_id): call generativelanguage API directly with Bearer token.
		baseURL := account.GetCredential("base_url")
		if strings.TrimSpace(baseURL) == "" {
			baseURL = GeminiAIStudioBaseURL
		}
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		fullURL, err := buildGeminiAIStudioModelURL(normalizedBaseURL, modelID, "streamGenerateContent", true)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)
		return req, nil
	}

	// Code Assist mode (with project_id)
	return s.buildCodeAssistRequest(ctx, accessToken, projectID, modelID, payload)
}

func (s *AccountTestService) buildCodeAssistRequest(ctx context.Context, accessToken, projectID, modelID string, payload []byte) (*http.Request, error) {
	var inner map[string]any
	if err := json.Unmarshal(payload, &inner); err != nil {
		return nil, err
	}

	wrapped := map[string]any{
		"model":   modelID,
		"project": projectID,
		"request": inner,
	}
	wrappedBytes, _ := json.Marshal(wrapped)

	normalizedBaseURL, err := s.validateUpstreamBaseURL(GeminiCliBaseURL)
	if err != nil {
		return nil, err
	}
	fullURL := fmt.Sprintf("%s/v1internal:streamGenerateContent?alt=sse", normalizedBaseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewReader(wrappedBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", GeminiCLIUserAgent)

	return req, nil
}

// createGeminiTestPayload creates a minimal test payload for Gemini API.
// Image models use the image-generation path so the frontend can preview the returned image.
func createGeminiTestPayload(modelID string, prompt string) []byte {
	if isImageGenerationModel(modelID) {
		imagePrompt := strings.TrimSpace(prompt)
		if imagePrompt == "" {
			imagePrompt = defaultGeminiImageTestPrompt
		}

		payload := map[string]any{
			"contents": []map[string]any{
				{
					"role": "user",
					"parts": []map[string]any{
						{"text": imagePrompt},
					},
				},
			},
			"generationConfig": map[string]any{
				"responseModalities": []string{"TEXT", "IMAGE"},
				"imageConfig": map[string]any{
					"aspectRatio": "1:1",
				},
			},
		}
		bytes, _ := json.Marshal(payload)
		return bytes
	}

	textPrompt := strings.TrimSpace(prompt)
	if textPrompt == "" {
		textPrompt = reasoningProbePrompt
	}

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": textPrompt},
				},
			},
		},
		"systemInstruction": map[string]any{
			"parts": []map[string]any{
				{"text": "You are a helpful AI assistant."},
			},
		},
	}
	bytes, _ := json.Marshal(payload)
	return bytes
}

// processGeminiStream processes SSE stream from Gemini API
func (s *AccountTestService) processGeminiStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
				return nil
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Stream read error: %s", err.Error()))
		}

		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonStr := strings.TrimPrefix(line, "data: ")
		if jsonStr == "[DONE]" {
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
			return nil
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		// Support two Gemini response formats:
		// - AI Studio: {"candidates": [...]}
		// - Gemini CLI: {"response": {"candidates": [...]}}
		if resp, ok := data["response"].(map[string]any); ok && resp != nil {
			data = resp
		}
		if candidates, ok := data["candidates"].([]any); ok && len(candidates) > 0 {
			if candidate, ok := candidates[0].(map[string]any); ok {
				// Extract content first (before checking completion)
				if content, ok := candidate["content"].(map[string]any); ok {
					if parts, ok := content["parts"].([]any); ok {
						for _, part := range parts {
							if partMap, ok := part.(map[string]any); ok {
								if text, ok := partMap["text"].(string); ok && text != "" {
									s.sendEvent(c, TestEvent{Type: "content", Text: text})
								}
								if inlineData, ok := partMap["inlineData"].(map[string]any); ok {
									mimeType, _ := inlineData["mimeType"].(string)
									data, _ := inlineData["data"].(string)
									if strings.HasPrefix(strings.ToLower(mimeType), "image/") && data != "" {
										s.sendEvent(c, TestEvent{
											Type:     "image",
											ImageURL: fmt.Sprintf("data:%s;base64,%s", mimeType, data),
											MimeType: mimeType,
										})
									}
								}
							}
						}
					}
				}

				// Check for completion after extracting content
				if finishReason, ok := candidate["finishReason"].(string); ok && finishReason != "" {
					s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
					return nil
				}
			}
		}

		// Handle errors
		if errData, ok := data["error"].(map[string]any); ok {
			errorMsg := "Unknown error"
			if msg, ok := errData["message"].(string); ok {
				errorMsg = msg
			}
			return s.sendErrorAndEnd(c, errorMsg)
		}
	}
}

// createOpenAITestPayload creates a test payload for OpenAI Responses API
// createOpenAITestPayload keeps the trivial "hi" probe: the usage/quota probe relies on it
// being cheap and answering inside its 15s budget.
func createOpenAITestPayload(modelID string, isOAuth bool) map[string]any {
	return createOpenAIResponsesPayload(modelID, isOAuth, "hi")
}

// createOpenAIDiagnosticPayload uses the reasoning probe so an account test also reveals
// whether the upstream still reasons like the model it claims to be.
func createOpenAIDiagnosticPayload(modelID string, isOAuth bool) map[string]any {
	return createOpenAIResponsesPayload(modelID, isOAuth, reasoningProbePrompt)
}

func createOpenAIResponsesPayload(modelID string, isOAuth bool, prompt string) map[string]any {
	payload := map[string]any{
		"model": modelID,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": prompt,
					},
				},
			},
		},
		"stream": true,
	}

	// OAuth accounts using ChatGPT internal API require store: false
	if isOAuth {
		payload["store"] = false
	}

	// All accounts require instructions for Responses API
	payload["instructions"] = openai.DefaultInstructions

	return payload
}

func createOpenAIChatCompletionsTestPayload(modelID string, prompt string) map[string]any {
	testPrompt := strings.TrimSpace(prompt)
	if testPrompt == "" {
		testPrompt = reasoningProbePrompt
	}

	return map[string]any{
		"model": modelID,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": testPrompt,
			},
		},
		"stream": true,
	}
}

// processClaudeStream processes the SSE stream from Claude API.
//
// 除转发答案正文外，这里还跟踪三件事实：是否真的产出过正文、是否出现过 thinking、
// 以及上游给出的 stop_reason。原因是「上游没产出答案」与「测试通过」在旧实现里
// 完全同形（都只发 test_complete），而推理模型在预算不足时恰好稳定产生前者。
func (s *AccountTestService) processClaudeStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)
	state := claudeTestStreamState{}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.finishClaudeTestStream(c, state)
				return nil
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Stream read error: %s", err.Error()))
		}

		line = strings.TrimSpace(line)
		if line == "" || !sseDataPrefix.MatchString(line) {
			continue
		}

		jsonStr := sseDataPrefix.ReplaceAllString(line, "")
		if jsonStr == "[DONE]" {
			s.finishClaudeTestStream(c, state)
			return nil
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		eventType, _ := data["type"].(string)

		switch eventType {
		case "content_block_delta":
			if delta, ok := data["delta"].(map[string]any); ok {
				if text, ok := delta["text"].(string); ok && text != "" {
					state.sawText = true
					s.sendEvent(c, TestEvent{Type: "content", Text: text})
				}
				// thinking 增量只用来报进度，绝不并入 content：判决函数只解析 content，
				// 推理文本里出现的数字会污染「降智检测」的答案。
				if thinking, ok := delta["thinking"].(string); ok && thinking != "" {
					state.sawThinking = true
					if !state.thinkingAnnounced {
						state.thinkingAnnounced = true
						s.sendEvent(c, TestEvent{Type: "status", Text: "模型正在推理（已收到 thinking 增量，答案正文尚未开始）…"})
					}
				}
			}
		case "message_delta":
			if delta, ok := data["delta"].(map[string]any); ok {
				if reason, ok := delta["stop_reason"].(string); ok && reason != "" {
					state.stopReason = reason
				}
			}
		case "message_stop":
			s.finishClaudeTestStream(c, state)
			return nil
		case "error":
			errorMsg := "Unknown error"
			if errData, ok := data["error"].(map[string]any); ok {
				if msg, ok := errData["message"].(string); ok {
					errorMsg = msg
				}
			}
			return s.sendErrorAndEnd(c, errorMsg)
		}
	}
}

// claudeTestStreamState 记录一次 Claude 测试流的可判定事实。
// stopReason 取上游 message_delta.delta.stop_reason；max_tokens 表示输出被预算截断。
type claudeTestStreamState struct {
	sawText           bool
	sawThinking       bool
	thinkingAnnounced bool
	stopReason        string
}

// finishClaudeTestStream 收尾一次 Claude 测试流。
//
// 连通性成功与「答案正文被截断」必须可区分：截断时补一条显式说明，
// 这样空响应不会被读成「测试完成 = 一切正常」。注意这里**不**发 error 事件，
// 因为截断不是账号故障，发 error 会牵连账号健康判定。
func (s *AccountTestService) finishClaudeTestStream(c *gin.Context, state claudeTestStreamState) {
	if !state.sawText {
		switch {
		case state.stopReason == "max_tokens":
			s.sendEvent(c, TestEvent{Type: "status", Text: fmt.Sprintf(
				"上游在 max_tokens=%d 处截断：预算被推理(thinking)耗尽，未产出答案正文（非协议故障，账号连通性正常）。"+
					"推理模型请换非推理模型复测，或提高测试预算。", accountTestMaxTokens)})
		case state.sawThinking:
			s.sendEvent(c, TestEvent{Type: "status", Text: "上游只返回了 thinking，未产出答案正文（本样本无法判定）"})
		case state.stopReason != "":
			s.sendEvent(c, TestEvent{Type: "status", Text: fmt.Sprintf(
				"上游未返回任何内容块，stop_reason=%s（本样本无法判定）", state.stopReason)})
		default:
			s.sendEvent(c, TestEvent{Type: "status", Text: "上游未返回任何内容块（本样本无法判定）"})
		}
	}
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
}

// processOpenAIChatCompletionsStream processes SSE chunks from the
// OpenAI-compatible Chat Completions API.
func (s *AccountTestService) processOpenAIChatCompletionsStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)
	seenJSON := false
	seenFinish := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if seenFinish {
					s.sendEvent(c, TestEvent{Type: "status", Text: "已通过 /v1/chat/completions 验证"})
					s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
					return nil
				}
				if seenJSON {
					return s.sendErrorAndEnd(c, "Chat Completions stream from /v1/chat/completions ended before [DONE]")
				}
				return s.sendErrorAndEnd(c, "Invalid Chat Completions response from /v1/chat/completions: expected SSE JSON data")
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Chat Completions stream read error from /v1/chat/completions: %s", err.Error()))
		}

		line = strings.TrimSpace(line)
		if line == "" || !sseDataPrefix.MatchString(line) {
			continue
		}

		jsonStr := sseDataPrefix.ReplaceAllString(line, "")
		if jsonStr == "[DONE]" {
			s.sendEvent(c, TestEvent{Type: "status", Text: "已通过 /v1/chat/completions 验证"})
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
			return nil
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			return s.sendErrorAndEnd(c, "Invalid Chat Completions response from /v1/chat/completions: expected JSON data")
		}
		seenJSON = true

		if errData, ok := data["error"].(map[string]any); ok {
			errorMsg := "Chat Completions API (/v1/chat/completions) returned an error"
			if msg, ok := errData["message"].(string); ok && msg != "" {
				errorMsg = msg
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Chat Completions API (/v1/chat/completions) error: %s", errorMsg))
		}

		choices, ok := data["choices"].([]any)
		if !ok {
			continue
		}
		for _, choiceValue := range choices {
			choice, ok := choiceValue.(map[string]any)
			if !ok {
				continue
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				if text, ok := delta["content"].(string); ok && text != "" {
					s.sendEvent(c, TestEvent{Type: "content", Text: text})
				}
			}
			if message, ok := choice["message"].(map[string]any); ok {
				if text, ok := message["content"].(string); ok && text != "" {
					s.sendEvent(c, TestEvent{Type: "content", Text: text})
				}
			}
			if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" {
				seenFinish = true
			}
		}
	}
}

// processOpenAIStream processes the SSE stream from OpenAI Responses API
func (s *AccountTestService) processOpenAIStream(c *gin.Context, body io.Reader) error {
	reader := bufio.NewReader(body)
	seenCompleted := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if seenCompleted {
					s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
					return nil
				}
				return s.sendErrorAndEnd(c, "Stream ended before response.completed")
			}
			return s.sendErrorAndEnd(c, fmt.Sprintf("Stream read error: %s", err.Error()))
		}

		line = strings.TrimSpace(line)
		if line == "" || !sseDataPrefix.MatchString(line) {
			continue
		}

		jsonStr := sseDataPrefix.ReplaceAllString(line, "")
		if jsonStr == "[DONE]" {
			if seenCompleted {
				s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
				return nil
			}
			return s.sendErrorAndEnd(c, "Stream ended before response.completed")
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		eventType, _ := data["type"].(string)

		switch eventType {
		case "response.output_text.delta":
			// OpenAI Responses API uses "delta" field for text content
			if delta, ok := data["delta"].(string); ok && delta != "" {
				s.sendEvent(c, TestEvent{Type: "content", Text: delta})
			}
		case "response.completed", "response.done":
			s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
			return nil
		case "response.failed":
			errorMsg := "OpenAI response failed"
			if responseData, ok := data["response"].(map[string]any); ok {
				if errData, ok := responseData["error"].(map[string]any); ok {
					if msg, ok := errData["message"].(string); ok && msg != "" {
						errorMsg = msg
					}
				}
			}
			return s.sendErrorAndEnd(c, errorMsg)
		case "error":
			errorMsg := "Unknown error"
			if errData, ok := data["error"].(map[string]any); ok {
				if msg, ok := errData["message"].(string); ok {
					errorMsg = msg
				}
			}
			return s.sendErrorAndEnd(c, errorMsg)
		}
	}
}

// testOpenAIImageAPIKey tests OpenAI image generation using an API Key account.
func (s *AccountTestService) testOpenAIImageAPIKey(c *gin.Context, ctx context.Context, account *Account, modelID, prompt string) error {
	authToken := account.GetOpenAIApiKey()
	if authToken == "" {
		return s.sendErrorAndEnd(c, "No API key available")
	}

	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
	}
	apiURL := buildOpenAIImagesURL(normalizedBaseURL, openAIImagesGenerationsEndpoint)

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	s.sendEvent(c, TestEvent{Type: "test_start", Model: modelID})

	payload := map[string]any{
		"model":           modelID,
		"prompt":          prompt,
		"n":               1,
		"response_format": "b64_json",
	}
	payloadBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	// 账号级请求头覆写：测试请求与真实转发保持一致的最终头
	// OpenCode 官方主机：必须带 x-opencode-session（缺则 400 MissingSessionID），
	// 且 UA 必须是 opencode 客户端形态（Cloudflare 1010 会把编程库 UA 拦成 403，
	// 该 403 在本仓会被计成凭据类故障，可能把健康账号停掉）。
	// 放在 ApplyHeaderOverrides 之前：管理员显式配置的请求头仍拥有最终决定权。
	applyOpenCodeUpstreamHeaders(req.Header, account, apiURL, "")
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to read response: %s", err.Error()))
	}

	if resp.StatusCode != http.StatusOK {
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	// Parse {"data": [{"b64_json": "...", "revised_prompt": "..."}]}
	var result struct {
		Data []struct {
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to parse response: %s", err.Error()))
	}

	if len(result.Data) == 0 {
		return s.sendErrorAndEnd(c, "No images returned from API")
	}

	for _, item := range result.Data {
		if item.RevisedPrompt != "" {
			s.sendEvent(c, TestEvent{Type: "content", Text: item.RevisedPrompt})
		}
		if item.B64JSON != "" {
			s.sendEvent(c, TestEvent{
				Type:     "image",
				ImageURL: "data:image/png;base64," + item.B64JSON,
				MimeType: "image/png",
			})
		}
	}

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

// testOpenAIImageOAuth tests OpenAI image generation using an OAuth account via Codex /responses API.
func (s *AccountTestService) testOpenAIImageOAuth(c *gin.Context, ctx context.Context, account *Account, modelID, prompt string) error {
	credentialAccount := account
	if account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return s.sendErrorAndEnd(c, "Failed to resolve account credentials")
		}
		credentialAccount = resolved
	}
	authToken := ""
	if !credentialAccount.IsOpenAIAgentIdentity() {
		authToken = credentialAccount.GetOpenAIAccessToken()
	}
	if authToken == "" && !credentialAccount.IsOpenAIAgentIdentity() {
		return s.sendErrorAndEnd(c, "No access token available")
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	s.sendEvent(c, TestEvent{Type: "test_start", Model: modelID})
	s.sendEvent(c, TestEvent{Type: "content", Text: "Calling Codex /responses image tool...\n"})

	parsed := &OpenAIImagesRequest{
		Endpoint: openAIImagesGenerationsEndpoint,
		Model:    strings.TrimSpace(modelID),
		Prompt:   prompt,
	}
	applyOpenAIImagesDefaults(parsed)

	responsesBody, err := buildOpenAIImagesResponsesRequest(parsed, parsed.Model)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to build image request: %s", err.Error()))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptCodexAPIURL, bytes.NewReader(responsesBody))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Host = "chatgpt.com"
	if credentialAccount.IsOpenAIAgentIdentity() {
		authHeaders, authErr := buildAgentIdentityAuthenticationHeaders(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, credentialAccount)
		if authErr != nil {
			return s.sendErrorAndEnd(c, "Failed to build Agent Identity authentication")
		}
		for key, values := range authHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	// 对齐 Codex 0.144 HTTP：不再注入已废弃的 responses=experimental。
	req.Header.Set("originator", "opencode")
	if customUA := strings.TrimSpace(credentialAccount.GetOpenAIUserAgent()); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	} else {
		req.Header.Set("User-Agent", codexCLIUserAgent)
	}
	setOpenAIChatGPTAccountHeaders(req.Header, credentialAccount)
	// 与真实转发一致：originator 与最终 User-Agent 首段配套（原 opencode 与 Codex UA 错配会 404，issue #3901）。
	s.codexIdentity.enforceHeadersWithUA(req.Header, credentialAccount.GetOpenAIUserAgent())

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Responses API request failed: %s", err.Error()))
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		body = redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, credentialAccount, body)
		message := strings.TrimSpace(extractUpstreamErrorMessage(body))
		if message == "" {
			message = fmt.Sprintf("Responses API returned %d", resp.StatusCode)
		}
		return s.sendErrorAndEnd(c, message)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to read image response: %s", err.Error()))
	}
	body = redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, credentialAccount, body)

	results, _, _, _, _, err := collectOpenAIImagesFromResponsesBody(body)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to parse image response: %s", err.Error()))
	}
	if len(results) == 0 {
		return s.sendErrorAndEnd(c, "No images returned from responses API")
	}

	for _, item := range results {
		if item.RevisedPrompt != "" {
			s.sendEvent(c, TestEvent{Type: "content", Text: item.RevisedPrompt})
		}
		mimeType := openAIImageOutputMIMEType(item.OutputFormat)
		s.sendEvent(c, TestEvent{
			Type:     "image",
			ImageURL: "data:" + mimeType + ";base64," + item.Result,
			MimeType: mimeType,
		})
	}

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

func (s *AccountTestService) sendEvent(c *gin.Context, event TestEvent) {
	eventJSON, _ := json.Marshal(event)
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", eventJSON); err != nil {
		log.Printf("failed to write SSE event: %v", err)
		return
	}
	c.Writer.Flush()
}

// sendErrorAndEnd sends an error event and ends the stream
func (s *AccountTestService) sendErrorAndEnd(c *gin.Context, errorMsg string) error {
	log.Printf("Account test error: %s", errorMsg)
	s.sendEvent(c, TestEvent{Type: "error", Error: errorMsg})
	return fmt.Errorf("%s", errorMsg)
}

// RunTestBackground executes an account test in-memory (no real HTTP client),
// capturing SSE output via httptest.NewRecorder, then parses the result.
func (s *AccountTestService) RunTestBackground(ctx context.Context, accountID int64, modelID string) (*ScheduledTestResult, error) {
	startedAt := time.Now()

	w := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(w)
	ginCtx.Request = (&http.Request{}).WithContext(ctx)

	testErr := s.TestAccountConnection(ginCtx, accountID, modelID, "", AccountTestModeDefault)

	finishedAt := time.Now()
	body := w.Body.String()
	responseText, errMsg := parseTestSSEOutput(body)

	status := "success"
	if testErr != nil || errMsg != "" {
		status = "failed"
		if errMsg == "" && testErr != nil {
			errMsg = testErr.Error()
		}
		// The reasoning probe is much slower than the old greeting. If the upstream already
		// delivered content and only ran out of budget, the account is connected: report the
		// truncation in the answer instead of failing the connectivity verdict (and leaving
		// the demote/quota-wall path to interpret a timeout as an upstream problem).
		if strings.TrimSpace(responseText) != "" && isProbeStreamTimeout(errMsg) {
			status = "success"
			errMsg = ""
			responseText = strings.TrimSpace(responseText) + "\n" + probeTruncatedVerdict()
		}
	}

	return &ScheduledTestResult{
		Status:       status,
		ResponseText: responseText,
		ErrorMessage: errMsg,
		LatencyMs:    finishedAt.Sub(startedAt).Milliseconds(),
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
	}, nil
}

// parseTestSSEOutput extracts response text and error message from captured SSE output.
func parseTestSSEOutput(body string) (responseText, errMsg string) {
	var texts []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")
		var event TestEvent
		if err := json.Unmarshal([]byte(jsonStr), &event); err != nil {
			continue
		}
		switch event.Type {
		case "content":
			if event.Text != "" {
				texts = append(texts, event.Text)
			}
		case "error":
			errMsg = event.Error
		}
	}
	responseText = strings.Join(texts, "")
	return
}
