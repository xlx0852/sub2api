package service

import (
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
	assert.Equal(t, legacy, resolveConvergedThreadID(account, "session-aaa", 0))
	// 负值按首次尝试处理，避免调用方传错时产生第三套种子。
	assert.Equal(t, legacy, resolveConvergedThreadID(account, "session-aaa", -3))
}

func TestResolveConvergedThreadID_AttemptRotatesThread(t *testing.T) {
	account := newTestOAuthAccount(7, nil)
	base := resolveConvergedThreadID(account, "session-aaa", 0)
	for attempt := 1; attempt <= 3; attempt++ {
		got := resolveConvergedThreadID(account, "session-aaa", attempt)
		assert.NotEqual(t, base, got, "attempt=%d 应轮换 thread_id", attempt)
		// 同一 attempt 内必须稳定（头改写与体改写需一致）。
		assert.Equal(t, got, resolveConvergedThreadID(account, "session-aaa", attempt))
	}
	// 不同 attempt 之间也要不同，否则重试仍会撞回同一后端。
	assert.NotEqual(t,
		resolveConvergedThreadID(account, "session-aaa", 1),
		resolveConvergedThreadID(account, "session-aaa", 2))
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
	assert.Equal(t, 3, svc.OpenAICodexOverloadRetryBudget(4242, 3, false))

	// 窗口内首次过载也不放宽（单次是抽签而非趋势）。
	clearOverloadBurstStateForTest(4243)
	svc.OnOpenAICodexOverload(4243)
	assert.Equal(t, 3, svc.OpenAICodexOverloadRetryBudget(4243, 3, true))
}

func TestOpenAICodexOverloadRetryBudget_RaisesInsideBurst(t *testing.T) {
	svc := newOverloadRotationTestService(false)
	svc.OnOpenAICodexOverload(555)
	svc.OnOpenAICodexOverload(555) // 第 2 次 => 进入爆发窗口
	assert.Equal(t, 3+openAICodexOverloadBurstRetryBonus, svc.OpenAICodexOverloadRetryBudget(555, 3, true))
	// 其它账号不受影响。
	assert.Equal(t, 3, svc.OpenAICodexOverloadRetryBudget(556, 3, true))
	// 非正 base 归零后仍不越界。
	assert.Equal(t, openAICodexOverloadBurstRetryBonus, svc.OpenAICodexOverloadRetryBudget(555, -5, true))
}

func TestOpenAICodexOverloadRetryBudget_DisabledByConfig(t *testing.T) {
	svc := newOverloadRotationTestService(true)
	svc.OnOpenAICodexOverload(777)
	svc.OnOpenAICodexOverload(777)
	assert.False(t, svc.CodexOverloadIdentityRotationEnabled())
	assert.Equal(t, 3, svc.OpenAICodexOverloadRetryBudget(777, 3, true), "开关关闭时不得改动重试上限")
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

func TestDiscardOpenAICodexTurnStateForOverload_ClearsPinnedTicket(t *testing.T) {
	account := newTestOAuthAccount(31, nil)
	account.Type = AccountTypeOAuth
	svc := newOverloadRotationTestService(false)

	// 先放一张被钉住的票。
	svc.storeOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", reusableTurnStateToken(), "gpt-5.6-luna")
	require.NotEmpty(t, lookupCachedOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", time.Now(), openAICodexTurnStateDefaultTTL))

	svc.DiscardOpenAICodexTurnStateForOverload(account, OverloadPathHTTP503)

	assert.Empty(t, lookupCachedOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", time.Now(), openAICodexTurnStateDefaultTTL),
		"首次容量过载即应清掉被钉住的票")
}

func TestDiscardOpenAICodexTurnStateForOverload_RespectsDisableSwitch(t *testing.T) {
	account := newTestOAuthAccount(32, nil)
	account.Type = AccountTypeOAuth
	svc := newOverloadRotationTestService(true)

	svc.storeOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", reusableTurnStateToken(), "gpt-5.6-luna")
	svc.DiscardOpenAICodexTurnStateForOverload(account, OverloadPathHTTP503)

	assert.NotEmpty(t, lookupCachedOpenAICodexTurnStateForPool(account, "gpt-5.6-luna", time.Now(), openAICodexTurnStateDefaultTTL),
		"开关关闭时必须保留旧行为（过载不清票）")
}

func TestDiscardOpenAICodexTurnStateForOverload_NilSafe(t *testing.T) {
	var svc *OpenAIGatewayService
	assert.NotPanics(t, func() { svc.DiscardOpenAICodexTurnStateForOverload(nil, OverloadPathHTTP503) })
	svc2 := newOverloadRotationTestService(false)
	assert.NotPanics(t, func() { svc2.DiscardOpenAICodexTurnStateForOverload(nil, OverloadPathHTTP503) })
}
