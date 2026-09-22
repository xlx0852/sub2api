package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newOverloadRotationTestService 构造只带 Gateway 配置的最小服务实例。
func newOverloadRotationTestService(disableRotation bool) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.DisableCodexOverloadIdentityRotation = disableRotation
	// 票据复用是清票路径的前置条件；默认在测试中开启，贴近生产默认值。
	cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	return &OpenAIGatewayService{cfg: cfg}
}

// setOverloadBurstStateForTest 直接改写爆发追踪表，用于构造窗口内外场景。
func setOverloadBurstStateForTest(accountID int64, st codexOverloadBurstState) {
	openAICodexOverloadBurst.mu.Lock()
	defer openAICodexOverloadBurst.mu.Unlock()
	if openAICodexOverloadBurst.state == nil {
		openAICodexOverloadBurst.state = map[int64]codexOverloadBurstState{}
	}
	openAICodexOverloadBurst.state[accountID] = st
}

// clearOverloadBurstStateForTest 清除某账号的爆发追踪，避免用例间串扰。
func clearOverloadBurstStateForTest(accountID int64) {
	openAICodexOverloadBurst.mu.Lock()
	defer openAICodexOverloadBurst.mu.Unlock()
	delete(openAICodexOverloadBurst.state, accountID)
}

// reusableTurnStateToken 生成一个能通过 isReusableOpenAICodexTurnState 校验的票。
func reusableTurnStateToken() string {
	return "gAAAAA" + strings.Repeat("x", openAICodexTurnStateMinLen)
}

// --- resolveConvergedThreadID: attempt 维度 ---

// attempt=0 必须与历史实现逐字节一致：派生种子不含轮次后缀。
// 这条防回归保证上线后首次尝试的出站 thread-id 不发生任何变化。
func TestResolveConvergedThreadID_AttemptZeroIsLegacySeed(t *testing.T) {
	account := newTestOAuthAccount(7, nil)
	legacy := deriveStableUUIDv4("codex:thread-id:v1:7:session-aaa")
	assert.Equal(t, legacy, resolveConvergedThreadID(account, "session-aaa", 0, 0))
	// 负值按首次尝试处理，避免调用方传错时产生第三套种子。
	assert.Equal(t, legacy, resolveConvergedThreadID(account, "session-aaa", -3, 0))
}

func TestResolveConvergedThreadID_AttemptRotatesThread(t *testing.T) {
	account := newTestOAuthAccount(7, nil)
	base := resolveConvergedThreadID(account, "session-aaa", 0, 0)
	for attempt := 1; attempt <= 3; attempt++ {
		got := resolveConvergedThreadID(account, "session-aaa", attempt, 0)
		assert.NotEqual(t, base, got, "attempt=%d 应轮换 thread_id", attempt)
		// 同一 attempt 内必须稳定（头改写与体改写需一致）。
		assert.Equal(t, got, resolveConvergedThreadID(account, "session-aaa", attempt, 0))
	}
	// 不同 attempt 之间也要不同，否则重试仍会撞回同一后端。
	assert.NotEqual(t,
		resolveConvergedThreadID(account, "session-aaa", 1, 0),
		resolveConvergedThreadID(account, "session-aaa", 2, 0))
}

// --- BumpCodexFingerprintAttempt + 上下文缓存 ---

func newFingerprintTestContext(clientSessionID string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req.Header.Set("session-id", clientSessionID)
	c.Request = req
	return c
}

func TestBumpCodexFingerprintAttempt_RotatesContextIDs(t *testing.T) {
	account := newTestOAuthAccount(9, map[string]any{codexFingerprintModeExtraKey: "session"})
	c := newFingerprintTestContext("client-session-aaa")

	assert.Equal(t, 0, CodexFingerprintAttempt(c))
	first := resolveCodexFingerprintIDsFromContext(c, account)
	require.NotNil(t, first)
	// 同一轮次内必须复用同一实例，保证头/体的 turn_id 等随机字段一致。
	assert.Same(t, first, resolveCodexFingerprintIDsFromContext(c, account))

	attempt := BumpCodexFingerprintAttempt(c)
	assert.Equal(t, 1, attempt)

	second := resolveCodexFingerprintIDsFromContext(c, account)
	require.NotNil(t, second)
	assert.NotSame(t, first, second, "轮换后必须重新解析，不能复用失败尝试的指纹")
	assert.NotEqual(t, first.threadID, second.threadID, "thread_id 必须轮换")
	// session_id 是账号级收敛值，绝不能随重试变化。
	assert.Equal(t, first.sessionID, second.sessionID, "session_id 必须保持账号级恒定")
	assert.Equal(t, first.installationID, second.installationID)
	// 同一轮次内再次读取仍复用。
	assert.Same(t, second, resolveCodexFingerprintIDsFromContext(c, account))

	// 第二次轮换继续递增。
	assert.Equal(t, 2, BumpCodexFingerprintAttempt(c))
	third := resolveCodexFingerprintIDsFromContext(c, account)
	require.NotNil(t, third)
	assert.NotEqual(t, second.threadID, third.threadID)
	assert.NotEqual(t, first.threadID, third.threadID)
}

// full 模式下 thread_id 恒等于 session_id，轮换不得改变它（该模式的语义就是全收敛）。
func TestBumpCodexFingerprintAttempt_FullModeKeepsThreadEqualToSession(t *testing.T) {
	account := newTestOAuthAccount(9, map[string]any{codexFingerprintModeExtraKey: "full"})
	c := newFingerprintTestContext("client-session-aaa")

	first := resolveCodexFingerprintIDsFromContext(c, account)
	BumpCodexFingerprintAttempt(c)
	second := resolveCodexFingerprintIDsFromContext(c, account)

	require.NotNil(t, first)
	require.NotNil(t, second)
	assert.Equal(t, first.sessionID, first.threadID)
	assert.Equal(t, second.sessionID, second.threadID)
	assert.Equal(t, first.sessionID, second.sessionID)
}

func TestBumpCodexFingerprintAttempt_NilContext(t *testing.T) {
	assert.NotPanics(t, func() { BumpCodexFingerprintAttempt(nil) })
	assert.Equal(t, 0, CodexFingerprintAttempt(nil))
}

// --- 过载标记与通道分类 ---

func TestIsOpenAIServerOverloadedError(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"嵌套 error.code", `{"error":{"code":"server_is_overloaded"}}`, true},
		{"response.error.code", `{"response":{"error":{"code":"server_is_overloaded"}}}`, true},
		{"slow_down", `{"error":{"code":"slow_down"}}`, true},
		{"大小写不敏感", `{"error":{"code":"Server_Is_Overloaded"}}`, true},
		{"其它错误码", `{"error":{"code":"rate_limit_exceeded"}}`, false},
		{"空体", ``, false},
		{"非法 JSON", `not json`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isOpenAIServerOverloadedError([]byte(tc.body)))
		})
	}
}

func TestNewOpenAIUpstreamFailoverError_MarksHTTP503Overload(t *testing.T) {
	body := []byte(`{"error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded"}}`)
	err := newOpenAIUpstreamFailoverError(http.StatusServiceUnavailable, http.Header{}, body, "", true)
	require.NotNil(t, err)
	assert.True(t, err.ServerOverloaded)
	assert.Equal(t, OverloadPathHTTP503, err.OverloadPath)
	assert.True(t, err.IsServerOverloaded())

	other := newOpenAIUpstreamFailoverError(http.StatusServiceUnavailable, http.Header{}, []byte(`{"error":{"code":"boom"}}`), "", true)
	require.NotNil(t, other)
	assert.False(t, other.ServerOverloaded)
	assert.Empty(t, other.OverloadPath)
	assert.False(t, other.IsServerOverloaded())
}

func TestUpstreamFailoverError_IsServerOverloaded_NilSafe(t *testing.T) {
	var e *UpstreamFailoverError
	assert.False(t, e.IsServerOverloaded())
}

// --- WS 通道的过载分类 ---

func TestClassifyOpenAIWSErrorEventFromRaw_ServerOverloaded(t *testing.T) {
	for _, code := range []string{"server_is_overloaded", "slow_down"} {
		reason, retryable := classifyOpenAIWSErrorEventFromRaw(code, "", "")
		assert.Equal(t, "server_overloaded", reason, "code=%s", code)
		assert.True(t, retryable, "过载必须可重连重试：code=%s", code)
	}
}

func TestClassifyOpenAIWSReconnectReason_ServerOverloadedIsRetryable(t *testing.T) {
	err := &openAIWSFallbackError{Reason: "server_overloaded"}
	reason, retryable := classifyOpenAIWSReconnectReason(err)
	assert.Equal(t, "server_overloaded", reason)
	assert.True(t, retryable)
}

// --- 爆发窗口预算 ---

func TestOpenAICodexOverloadRetryBudget_OutsideBurstIsBaseline(t *testing.T) {
	svc := newOverloadRotationTestService(false)
	// 非过载：即使窗口内有历史过载也不放宽。
	svc.OnOpenAICodexOverload(4242)
	svc.OnOpenAICodexOverload(4242)
	assert.Equal(t, 3, svc.OpenAICodexOverloadRetryBudget(newTestOAuthAccount(4242, nil), 3, false))

	// 窗口内首次过载也不放宽（单次是抽签而非趋势）。
	clearOverloadBurstStateForTest(4243)
	svc.OnOpenAICodexOverload(4243)
	assert.Equal(t, 3, svc.OpenAICodexOverloadRetryBudget(newTestOAuthAccount(4243, nil), 3, true))
}

func TestOpenAICodexOverloadRetryBudget_RaisesInsideBurst(t *testing.T) {
	svc := newOverloadRotationTestService(false)
	svc.OnOpenAICodexOverload(555)
	svc.OnOpenAICodexOverload(555) // 第 2 次 => 进入爆发窗口
	assert.Equal(t, 3+openAICodexOverloadBurstRetryBonus, svc.OpenAICodexOverloadRetryBudget(newTestOAuthAccount(555, nil), 3, true))
	// 其它账号不受影响。
	assert.Equal(t, 3, svc.OpenAICodexOverloadRetryBudget(newTestOAuthAccount(556, nil), 3, true))
	// 非正 base 归零后仍不越界。
	assert.Equal(t, openAICodexOverloadBurstRetryBonus, svc.OpenAICodexOverloadRetryBudget(newTestOAuthAccount(555, nil), -5, true))
}

func TestOpenAICodexOverloadRetryBudget_DisabledByConfig(t *testing.T) {
	svc := newOverloadRotationTestService(true)
	svc.OnOpenAICodexOverload(777)
	svc.OnOpenAICodexOverload(777)
	assert.False(t, svc.CodexOverloadIdentityRotationEnabled())
	assert.Equal(t, 3, svc.OpenAICodexOverloadRetryBudget(newTestOAuthAccount(777, nil), 3, true), "开关关闭时不得改动重试上限")
}

func TestCodexOverloadIdentityRotationEnabled_NilSafe(t *testing.T) {
	var svc *OpenAIGatewayService
	assert.False(t, svc.CodexOverloadIdentityRotationEnabled())
}

func TestOnOpenAICodexOverload_SequenceAndWindowReset(t *testing.T) {
	svc := newOverloadRotationTestService(false)
	assert.Equal(t, 1, svc.OnOpenAICodexOverload(888))
	assert.Equal(t, 2, svc.OnOpenAICodexOverload(888))
	assert.Equal(t, 3, svc.OnOpenAICodexOverload(888))
	assert.Equal(t, 3, svc.OpenAICodexOverloadBurstAttempt(888))
	assert.Equal(t, 0, svc.OpenAICodexOverloadBurstAttempt(999))
	assert.Equal(t, 0, svc.OnOpenAICodexOverload(0), "非法账号不记录")

	// 窗口过期（最后一次过载早于窗口）后重新从 1 开始。
	stale := time.Now().Add(-2 * openAICodexOverloadBurstWindow)
	setOverloadBurstStateForTest(888, codexOverloadBurstState{
		windowStart: stale, attempts: 3, lastSeen: stale,
	})
	assert.Equal(t, 0, svc.OpenAICodexOverloadBurstAttempt(888))
	assert.Equal(t, 1, svc.OnOpenAICodexOverload(888))
}

// --- 过载即时清票 ---

func TestHandleOpenAICodexOverload_ClearsPinnedTicket(t *testing.T) {
	account := newTestOAuthAccount(31, nil)
	account.Type = AccountTypeOAuth
	svc := newOverloadRotationTestService(false)

	// 先放一张被钉住的票。
	svc.storeOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", reusableTurnStateToken(), "gpt-5.6-luna")
	require.NotEmpty(t, lookupCachedOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", time.Now(), openAICodexTurnStateDefaultTTL))

	svc.HandleOpenAICodexOverload(account, OverloadPathHTTP503, "client-session-aaa")

	assert.Empty(t, lookupCachedOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", time.Now(), openAICodexTurnStateDefaultTTL),
		"首次容量过载即应清掉被钉住的票")
}

func TestHandleOpenAICodexOverload_RespectsDisableSwitch(t *testing.T) {
	account := newTestOAuthAccount(32, nil)
	account.Type = AccountTypeOAuth
	svc := newOverloadRotationTestService(true)

	svc.storeOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", reusableTurnStateToken(), "gpt-5.6-luna")
	svc.HandleOpenAICodexOverload(account, OverloadPathHTTP503, "client-session-aaa")

	assert.NotEmpty(t, lookupCachedOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", time.Now(), openAICodexTurnStateDefaultTTL),
		"开关关闭时必须保留旧行为（过载不清票）")
}

func TestHandleOpenAICodexOverload_NilSafe(t *testing.T) {
	var svc *OpenAIGatewayService
	assert.NotPanics(t, func() { svc.HandleOpenAICodexOverload(nil, OverloadPathHTTP503, "") })
	svc2 := newOverloadRotationTestService(false)
	assert.NotPanics(t, func() { svc2.HandleOpenAICodexOverload(nil, OverloadPathHTTP503, "") })
}

// --- 流式过载：可重放 vs 不可重放 ---

// newCapacityShedStreamResponse 构造一段以容量降载 response.failed 收尾的 SSE 流。
// withDelta=true 时先输出一段可见内容，使网关判定「已向客户端输出」。
func newCapacityShedStreamResponse(withDelta bool) (*http.Response, func()) {
	pr, pw := io.Pipe()
	var b strings.Builder
	b.WriteString(`data: {"type":"response.created","response":{"id":"resp_shed","model":"gpt-5.6-luna"}}` + "\n\n")
	if withDelta {
		b.WriteString(`data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n")
	}
	b.WriteString(`data: {"type":"response.failed","response":{"id":"resp_shed","status":"failed",` +
		`"error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}` + "\n\n")
	go func() {
		_, _ = pw.Write([]byte(b.String()))
		_ = pw.Close()
	}()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
	return resp, func() { _ = pr.Close() }
}

func newCapacityShedTestAccount(id int64) *Account {
	account := newTestOAuthAccount(id, nil)
	account.Type = AccountTypeOAuth
	account.Platform = PlatformOpenAI
	return account
}

// 未向客户端输出时，流内过载走 failover 分支，返回 *UpstreamFailoverError。
func TestOpenAIStreamingCapacityShedBeforeOutputFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newOverloadRotationTestService(false)
	account := newCapacityShedTestAccount(41)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)

	resp, closeFn := newCapacityShedStreamResponse(false)
	defer closeFn()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-astra", "gpt-6-astra")
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr, "未输出可见内容时应返回可 failover 的错误")
	require.True(t, failoverErr.IsServerOverloaded())
	require.Equal(t, OverloadPathStreamFailed, failoverErr.OverloadPath)
}

// 已向客户端输出可见内容时不得重放（既有边界），
// 但仍必须清掉被钉住的票：过载是上游容量压力，下一次请求要重抽。
func TestOpenAIStreamingCapacityShedAfterOutputStillDiscardsTicket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newOverloadRotationTestService(false)
	account := newCapacityShedTestAccount(42)

	svc.storeOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", reusableTurnStateToken(), "gpt-5.6-luna")
	require.NotEmpty(t, lookupCachedOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", time.Now(), openAICodexTurnStateDefaultTTL))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
	// 生产里 Codex 客户端会带 session-id；epoch 需要它才能推进。
	c.Request.Header.Set("session-id", "client-session-shed")

	resp, closeFn := newCapacityShedStreamResponse(true)
	defer closeFn()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-astra", "gpt-5.6-luna")
	// 不得重放：返回普通错误而非 *UpstreamFailoverError。
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "已输出可见内容后不得重放")

	require.Empty(t, lookupCachedOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", time.Now(), openAICodexTurnStateDefaultTTL),
		"即使无法重放，容量过载也必须清掉被钉住的票")
	require.GreaterOrEqual(t, svc.OpenAICodexOverloadBurstAttempt(account.ID), 1,
		"过载应记入爆发窗口，供后续请求的重试预算使用")
	// 关键：真实调用点必须把入站 session-id 传到 epoch，否则跨请求轮换在生产等于死代码。
	require.GreaterOrEqual(t, openAICodexOverloadSessionEpoch(account.ID, "client-session-shed"), 1,
		"过载后该(账号,客户端会话)的 epoch 必须推进")
}

// 对照组：非容量降载的流内失败（普通 upstream 错误）不得清票。
func TestOpenAIStreamingNonShedFailureKeepsPinnedTicket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newOverloadRotationTestService(false)
	account := newCapacityShedTestAccount(43)

	svc.storeOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", reusableTurnStateToken(), "gpt-5.6-luna")

	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte(`data: {"type":"response.created","response":{"id":"resp_x","model":"gpt-5.6-luna"}}` + "\n\n"))
		_, _ = pw.Write([]byte(`data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n"))
		_, _ = pw.Write([]byte(`data: {"type":"response.failed","response":{"id":"resp_x","status":"failed",` +
			`"error":{"code":"upstream_timeout","message":"upstream timed out"}}}` + "\n\n"))
		_ = pw.Close()
	}()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
	defer func() { _ = pr.Close() }()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)

	_, _ = svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-astra", "gpt-5.6-luna")

	require.NotEmpty(t, lookupCachedOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", time.Now(), openAICodexTurnStateDefaultTTL),
		"非容量降载的流内失败不应清票")
}

// --- 跨请求过载 epoch ---

func resetSessionEpochForTest(accountID int64, clientSessionID string) {
	key := openAICodexOverloadSessionKey(accountID, clientSessionID)
	openAICodexOverloadSessionEpochs.mu.Lock()
	defer openAICodexOverloadSessionEpochs.mu.Unlock()
	if key != "" {
		if _, ok := openAICodexOverloadSessionEpochs.state[key]; ok {
			delete(openAICodexOverloadSessionEpochs.state, key)
			if openAICodexOverloadSessionEpochs.active > 0 {
				openAICodexOverloadSessionEpochs.active--
			}
		}
	}
}

// 关键用例：过载后，同一客户端会话的**下一条请求**（轮次归零）必须拿到不同的 thread-id。
func TestOverloadEpoch_RotatesThreadIDAcrossRequests(t *testing.T) {
	account := newTestOAuthAccount(501, map[string]any{codexFingerprintModeExtraKey: "session"})
	clientSession := "client-session-epoch"
	resetSessionEpochForTest(account.ID, clientSession)
	clearOverloadBurstStateForTest(account.ID)

	// 过载前的确定性派生（也是历史行为）。
	first := resolveCodexFingerprintIDsFromContext(newFingerprintTestContext(clientSession), account)
	require.NotNil(t, first)

	// 上游报过载。
	svc := newOverloadRotationTestService(false)
	svc.HandleOpenAICodexOverload(account, OverloadPathHTTP503, clientSession)
	require.Equal(t, 1, openAICodexOverloadSessionEpoch(account.ID, clientSession))

	// 下一条请求是全新的 gin context（轮次 0），但 epoch 生效 → thread-id 必须不同。
	second := resolveCodexFingerprintIDsFromContext(newFingerprintTestContext(clientSession), account)
	require.NotNil(t, second)
	require.NotEqual(t, first.threadID, second.threadID,
		"过载后同一客户端会话的下一条请求必须轮换 thread-id，否则会回到同一后端")
	require.Equal(t, 0, CodexFingerprintAttempt(newFingerprintTestContext(clientSession)),
		"新请求的请求内轮次应为 0；轮换只能来自 epoch")
	// session_id 仍必须是账号级恒定值。
	require.Equal(t, first.sessionID, second.sessionID)

	// 再次过载 → epoch 递增 → 又是一个新值。
	svc.HandleOpenAICodexOverload(account, OverloadPathStreamFailed, clientSession)
	third := resolveCodexFingerprintIDsFromContext(newFingerprintTestContext(clientSession), account)
	require.NotNil(t, third)
	require.NotEqual(t, second.threadID, third.threadID)
	require.NotEqual(t, first.threadID, third.threadID)
}

// 未过载的会话必须保持历史派生（逐字节）——这是防回归的核心。
func TestOverloadEpoch_UnaffectedSessionKeepsLegacyDerivation(t *testing.T) {
	account := newTestOAuthAccount(502, map[string]any{codexFingerprintModeExtraKey: "session"})
	clientSession := "client-session-clean"
	resetSessionEpochForTest(account.ID, clientSession)

	require.Equal(t, 0, openAICodexOverloadSessionEpoch(account.ID, clientSession))
	got := resolveConvergedThreadID(account, clientSession, 0, 0)
	legacy := deriveStableUUIDv4("codex:thread-id:v1:502:client-session-clean")
	require.Equal(t, legacy, got)

	ids := resolveCodexFingerprintIDsFromContext(newFingerprintTestContext(clientSession), account)
	require.NotNil(t, ids)
	require.Equal(t, legacy, ids.threadID)
}

// 窗口过期后必须回到确定性派生。
func TestOverloadEpoch_ExpiresWithBurstWindow(t *testing.T) {
	account := newTestOAuthAccount(503, map[string]any{codexFingerprintModeExtraKey: "session"})
	clientSession := "client-session-expiry"
	resetSessionEpochForTest(account.ID, clientSession)

	svc := newOverloadRotationTestService(false)
	svc.HandleOpenAICodexOverload(account, OverloadPathHTTP503, clientSession)
	require.Equal(t, 1, openAICodexOverloadSessionEpoch(account.ID, clientSession))

	// 手工把 lastSeen 推到窗口外（模拟时间流逝，避免测试 sleep 30 分钟）。
	key := openAICodexOverloadSessionKey(account.ID, clientSession)
	openAICodexOverloadSessionEpochs.mu.Lock()
	st := openAICodexOverloadSessionEpochs.state[key]
	st.lastSeen = time.Now().Add(-2 * openAICodexOverloadBurstWindow)
	openAICodexOverloadSessionEpochs.state[key] = st
	openAICodexOverloadSessionEpochs.mu.Unlock()

	require.Equal(t, 0, openAICodexOverloadSessionEpoch(account.ID, clientSession),
		"窗口外 epoch 不生效")
	ids := resolveCodexFingerprintIDsFromContext(newFingerprintTestContext(clientSession), account)
	require.NotNil(t, ids)
	require.Equal(t, deriveStableUUIDv4("codex:thread-id:v1:503:client-session-expiry"), ids.threadID,
		"窗口过期后必须回到确定性派生")

	// epoch 值本身单调递增，不回退到用过的值。
	resetSessionEpochForTest(account.ID, clientSession)
	svc.HandleOpenAICodexOverload(account, OverloadPathHTTP503, clientSession)
	require.Equal(t, 1, openAICodexOverloadSessionEpoch(account.ID, clientSession))
}

// 不同账号 / 不同客户端会话互不影响。
func TestOverloadEpoch_ScopedToAccountAndSession(t *testing.T) {
	accountA := newTestOAuthAccount(511, map[string]any{codexFingerprintModeExtraKey: "session"})
	accountB := newTestOAuthAccount(512, map[string]any{codexFingerprintModeExtraKey: "session"})
	sessionX, sessionY := "client-x", "client-y"
	for _, acc := range []*Account{accountA, accountB} {
		for _, s := range []string{sessionX, sessionY} {
			resetSessionEpochForTest(acc.ID, s)
		}
	}

	svc := newOverloadRotationTestService(false)
	svc.HandleOpenAICodexOverload(accountA, OverloadPathHTTP503, sessionX)

	require.Equal(t, 1, openAICodexOverloadSessionEpoch(accountA.ID, sessionX))
	require.Equal(t, 0, openAICodexOverloadSessionEpoch(accountA.ID, sessionY), "同账号其它会话不受影响")
	require.Equal(t, 0, openAICodexOverloadSessionEpoch(accountB.ID, sessionX), "其它账号不受影响")

	baseA := deriveStableUUIDv4(fmt.Sprintf("codex:thread-id:v1:%d:%s", accountA.ID, sessionY))
	require.Equal(t, baseA, resolveConvergedThreadID(accountA, sessionY, 0, 0))
}

// 关闭开关时不得写入 epoch（整体回滚语义）。
func TestOverloadEpoch_DisabledByConfig(t *testing.T) {
	account := newTestOAuthAccount(521, map[string]any{codexFingerprintModeExtraKey: "session"})
	clientSession := "client-session-disabled"
	resetSessionEpochForTest(account.ID, clientSession)

	svc := newOverloadRotationTestService(true)
	svc.HandleOpenAICodexOverload(account, OverloadPathHTTP503, clientSession)

	require.Equal(t, 0, openAICodexOverloadSessionEpoch(account.ID, clientSession),
		"开关关闭时不得推进 epoch")
}

// epoch 与请求内轮次互不干扰，且两者都为 0 时与历史派生一致。
func TestResolveConvergedThreadID_EpochAndAttemptCombine(t *testing.T) {
	account := newTestOAuthAccount(531, nil)
	legacy := deriveStableUUIDv4("codex:thread-id:v1:531:s1")
	byAttempt := deriveStableUUIDv4("codex:thread-id:v1:531:s1:1")
	byEpoch := deriveStableUUIDv4("codex:thread-id:v2:531:s1:1")
	byBoth := deriveStableUUIDv4("codex:thread-id:v2:531:s1:1:2")

	require.Equal(t, legacy, resolveConvergedThreadID(account, "s1", 0, 0))
	require.Equal(t, byAttempt, resolveConvergedThreadID(account, "s1", 1, 0))
	require.Equal(t, byEpoch, resolveConvergedThreadID(account, "s1", 0, 1))
	require.Equal(t, byBoth, resolveConvergedThreadID(account, "s1", 2, 1))

	// epoch=1/attempt=1 与仅 attempt=1 必须不同（种子版本位隔离）。
	require.NotEqual(t, byAttempt, resolveConvergedThreadID(account, "s1", 1, 1))
}

func TestOpenAICodexOverloadSessionKey_EmptyInputs(t *testing.T) {
	require.Empty(t, openAICodexOverloadSessionKey(0, "s"))
	require.Empty(t, openAICodexOverloadSessionKey(1, ""))
	require.NotEmpty(t, openAICodexOverloadSessionKey(1, "s"))
	// 键不包含原始 session-id。
	require.NotContains(t, openAICodexOverloadSessionKey(1, "secret-session-value"), "secret-session-value")
}

// --- 账号级覆盖（灰度 A/B 用）---

func TestGetCodexOverloadRotationOverride(t *testing.T) {
	cases := []struct {
		extra   map[string]any
		enabled bool
		ok      bool
	}{
		{nil, false, false},
		{map[string]any{codexOverloadRotationExtraKey: ""}, false, false},
		{map[string]any{codexOverloadRotationExtraKey: "bogus"}, false, false},
		{map[string]any{codexOverloadRotationExtraKey: "on"}, true, true},
		{map[string]any{codexOverloadRotationExtraKey: "ON"}, true, true},
		{map[string]any{codexOverloadRotationExtraKey: "true"}, true, true},
		{map[string]any{codexOverloadRotationExtraKey: "1"}, true, true},
		{map[string]any{codexOverloadRotationExtraKey: "off"}, false, true},
		{map[string]any{codexOverloadRotationExtraKey: "Off"}, false, true},
		{map[string]any{codexOverloadRotationExtraKey: "0"}, false, true},
	}
	for _, tc := range cases {
		enabled, ok := GetCodexOverloadRotationOverride(newTestOAuthAccount(1, tc.extra))
		require.Equal(t, tc.ok, ok, "extra=%v", tc.extra)
		if tc.ok {
			require.Equal(t, tc.enabled, enabled, "extra=%v", tc.extra)
		}
	}
	// nil 账号不参与覆盖。
	if _, ok := GetCodexOverloadRotationOverride(nil); ok {
		t.Fatal("nil 账号不应有覆盖值")
	}
}

// 账号级覆盖优先于全局开关：这是 A/B 的基础（同指纹模式下只用开关区分两组）。
func TestCodexOverloadIdentityRotationEnabledFor_AccountOverrideWins(t *testing.T) {
	// 全局开，账号显式关 → 关
	svc := newOverloadRotationTestService(false)
	offAccount := newTestOAuthAccount(601, map[string]any{codexOverloadRotationExtraKey: "off"})
	require.False(t, svc.CodexOverloadIdentityRotationEnabledFor(offAccount))
	// 全局开，账号显式开/未配置 → 开
	require.True(t, svc.CodexOverloadIdentityRotationEnabledFor(newTestOAuthAccount(602, map[string]any{codexOverloadRotationExtraKey: "on"})))
	require.True(t, svc.CodexOverloadIdentityRotationEnabledFor(newTestOAuthAccount(603, nil)))

	// 全局关，账号显式开 → 开
	svcOff := newOverloadRotationTestService(true)
	require.False(t, svcOff.CodexOverloadIdentityRotationEnabled())
	require.True(t, svcOff.CodexOverloadIdentityRotationEnabledFor(newTestOAuthAccount(604, map[string]any{codexOverloadRotationExtraKey: "on"})))
	require.False(t, svcOff.CodexOverloadIdentityRotationEnabledFor(newTestOAuthAccount(605, nil)))
}

// 对照组账号（extra=off）不得清票、不得推进 epoch、不得放宽重试预算。
func TestOverloadRecovery_ControlAccountIsUnaffected(t *testing.T) {
	account := newTestOAuthAccount(611, map[string]any{codexOverloadRotationExtraKey: "off"})
	clientSession := "client-session-control"
	resetSessionEpochForTest(account.ID, clientSession)
	clearOverloadBurstStateForTest(account.ID)

	svc := newOverloadRotationTestService(false)
	svc.storeOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", reusableTurnStateToken(), "gpt-5.6-luna")

	svc.HandleOpenAICodexOverload(account, OverloadPathHTTP503, clientSession)

	require.NotEmpty(t, lookupCachedOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", time.Now(), openAICodexTurnStateDefaultTTL),
		"对照组账号不得清票")
	require.Equal(t, 0, openAICodexOverloadSessionEpoch(account.ID, clientSession),
		"对照组账号不得推进 epoch")
	require.Equal(t, 3, svc.OpenAICodexOverloadRetryBudget(account, 3, true),
		"对照组账号不得放宽重试预算")
}

// 实验组账号（extra=on）在全局关闭时仍然生效。
func TestOverloadRecovery_TreatmentAccountWinsOverGlobalOff(t *testing.T) {
	account := newTestOAuthAccount(612, map[string]any{codexOverloadRotationExtraKey: "on"})
	clientSession := "client-session-treatment"
	resetSessionEpochForTest(account.ID, clientSession)
	clearOverloadBurstStateForTest(account.ID)

	svc := newOverloadRotationTestService(true) // 全局关
	svc.HandleOpenAICodexOverload(account, OverloadPathHTTP503, clientSession)

	require.Equal(t, 1, openAICodexOverloadSessionEpoch(account.ID, clientSession))
}
