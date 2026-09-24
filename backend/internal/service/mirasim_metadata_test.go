//go:build unit

package service

// Mirasim 只读元数据（账号维度可用模型 /v1/models + 额度窗口 limits）单测。
//
// 覆盖 openspec/changes/restrict-mirasim-whitelist-to-available-models/tasks.md 与
// add-mirasim-model-roster-and-limits/tasks.md 的 1.8 / 2.5 /
// 3.4 / 4.7。测试全部是白盒：直接断言真实发出的 HTTP 形状（方法/路径/查询串/头/
// 签名/超时），而不是断言"函数被调用过"。
//
// 关键约束（改动前请先读）：
//   - 本文件必须自包含。`upstream_models_test.go` 在隔离跑法里会被移走，helper
//     名不能与既有测试冲突（因此 fake 叫 mirasimMetaFake，不复用 relay 的 fake）；
//   - 断言签名时用的 canonical string 必须基于"实际发出的 path/credential"，
//     一旦有人给只读请求加回 query、改用密封头或多发一次重试，这里会直接失败；
//   - 只读链路不得触发 refresh token 轮换（除 401/403 兜底外），因此多处显式断言
//     refresh / session 的调用次数。

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 上游假实现：记录每一次 HTTP 调用，并按路径返回可编程响应。
// ---------------------------------------------------------------------------

type mirasimMetaCall struct {
	Method      string
	Path        string
	URL         string
	RawQuery    string
	Auth        string
	Body        string
	Header      http.Header
	At          time.Time
	Deadline    time.Time
	HasDeadline bool
	Proxy       string
	AccountID   int64
	Concurrency int
}

type mirasimMetaFake struct {
	mu     sync.Mutex
	calls  []mirasimMetaCall
	counts map[string]int

	models  func(call int) (int, string)
	limits  func(call int) (int, string)
	session func(call int) (int, string)
	refresh func(call int) (int, string)
}

func (f *mirasimMetaFake) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	rec := mirasimMetaCall{
		Method:      req.Method,
		Path:        req.URL.Path,
		URL:         req.URL.String(),
		RawQuery:    req.URL.RawQuery,
		Auth:        req.Header.Get("Authorization"),
		Body:        string(body),
		Header:      req.Header.Clone(),
		At:          time.Now(),
		Proxy:       proxyURL,
		AccountID:   accountID,
		Concurrency: concurrency,
	}
	if deadline, ok := req.Context().Deadline(); ok {
		rec.Deadline, rec.HasDeadline = deadline, true
	}

	f.mu.Lock()
	f.calls = append(f.calls, rec)
	if f.counts == nil {
		f.counts = map[string]int{}
	}
	f.counts[req.URL.Path]++
	callIndex := f.counts[req.URL.Path]
	responder := f.responderForLocked(req.URL.Path)
	f.mu.Unlock()

	status, payload := http.StatusOK, "{}"
	if responder != nil {
		status, payload = responder(callIndex)
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(payload)),
	}, nil
}

func (f *mirasimMetaFake) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *domain.TLSFingerprintRuntimeProfile) (*http.Response, error) {
	return f.Do(req, proxyURL, accountID, concurrency)
}

func (f *mirasimMetaFake) responderForLocked(path string) func(int) (int, string) {
	switch path {
	case mirasimAccessRefreshPath:
		if f.refresh != nil {
			return f.refresh
		}
		return mirasimMetaRefreshResponse
	case mirasimDeviceSessionPath:
		if f.session != nil {
			return f.session
		}
		return mirasimMetaSessionResponse
	case mirasimAvailableModelsPath:
		if f.models != nil {
			return f.models
		}
		return mirasimMetaModelsResponse
	case mirasimLimitsPath:
		if f.limits != nil {
			return f.limits
		}
		return mirasimMetaLimitsResponse
	}
	return nil
}

func (f *mirasimMetaFake) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[path]
}

func (f *mirasimMetaFake) callsFor(path string) []mirasimMetaCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]mirasimMetaCall, 0, len(f.calls))
	for _, call := range f.calls {
		if call.Path == path {
			out = append(out, call)
		}
	}
	return out
}

func (f *mirasimMetaFake) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.calls))
	for _, call := range f.calls {
		out = append(out, call.Path)
	}
	return out
}

var _ HTTPUpstream = (*mirasimMetaFake)(nil)

func mirasimMetaRefreshResponse(int) (int, string) {
	return http.StatusOK, `{"access_token":"meta-access-token","refresh_token":"meta-rotated-refresh","expires_in":3600}`
}

func mirasimMetaSessionResponse(int) (int, string) {
	return http.StatusOK, fmt.Sprintf(`{"ticket":%q,"expires_in":900}`, mirasimMetaTicket)
}

func mirasimMetaModelsResponse(int) (int, string) {
	return http.StatusOK, mirasimModelsFixture
}

func mirasimMetaLimitsResponse(int) (int, string) {
	return http.StatusOK, mirasimLimitsFixture
}

// ---------------------------------------------------------------------------
// 夹具
// ---------------------------------------------------------------------------

const mirasimMetaTicket = "meta-device-ticket"

// mirasimModelsFixture 覆盖：id 的 trim、trim 后重复去重、非字符串 / 空 id 丢弃、
// 非对象条目（字符串/null/缺 id）丢弃、大小写不同视为不同 id（不做 lowercase）。
const mirasimModelsFixture = `{
  "object": "list",
  "data": [
    {"id": " glm-5.3-flash ", "object": "model", "owned_by": "mirasim-router"},
    {"id": "kimi-k3", "object": "model", "owned_by": "mirasim-router"},
    {"id": "kimi-k3", "object": "model", "owned_by": "mirasim-router"},
    {"id": "KIMI-K3", "object": "model", "owned_by": "mirasim-router"},
    {"id": "deepseek-flash", "object": "model", "owned_by": "mirasim-router"},
    {"id": "", "object": "model"},
    {"id": 42, "object": "model"},
    {"id": "   ", "object": "model"},
    "not-an-object",
    null,
    {"object": "model"}
  ]
}`

// mirasimLimitsFixture 覆盖：budget==0 不限量、负数 budget 丢弃、used 非法丢弃、
// name 为空丢弃、model_scoped 标记、reset_at 秒级时间戳与非法值。
const mirasimLimitsFixture = `{
  "paid": true,
  "degraded": true,
  "windows": [
    {"name": "5h", "budget": 100, "used": 50, "reset_at": "1750000000"},
    {"name": "7d", "budget": 0, "used": 12},
    {"name": "7d-opus", "budget": 10, "used": 9.5, "model_scoped": true, "reset_at": -1},
    {"name": "bad-budget", "budget": -1, "used": 1},
    {"name": "bad-used", "budget": 10, "used": "abc"},
    {"name": "", "budget": 10, "used": 1}
  ]
}`

// mirasimMetaTestAccount 构造账号并清理运行时 client 缓存，避免跨用测试互相污染
// （运行时缓存按 account id 索引，device key / ticket 会串味）。
func mirasimMetaTestAccount(t *testing.T, accountID int64, extras map[string]any) *Account {
	t.Helper()
	mirasimForgetAccount(accountID)
	t.Cleanup(func() { mirasimForgetAccount(accountID) })
	account, _ := mirasimTestAccount(t, accountID, extras)
	return account
}

// ---------------------------------------------------------------------------
// 1.8 只读请求形状：路径 / 查询串 / 空 body / 明文签名头 / 超时 / 凭据复用
// ---------------------------------------------------------------------------

func TestMirasimMetadataReadOnlyRequestShape(t *testing.T) {
	const accountID = int64(942001)
	account := mirasimMetaTestAccount(t, accountID, nil)
	fake := &mirasimMetaFake{}
	service := NewMirasimMetadataService(nil, fake)

	ctx := context.Background()
	models, err := service.AvailableModels(ctx, account, false)
	require.NoError(t, err)
	require.NotEmpty(t, models)

	snapshot, err := service.Limits(ctx, account, false)
	require.NoError(t, err)
	require.NotNil(t, snapshot)

	// 冷启动：1 次 refresh + 1 次 session 换出 ticket，随后两次只读请求复用同一 ticket。
	require.Equal(t, 1, fake.count(mirasimAccessRefreshPath), "只读链路首次换票只应 refresh 一次")
	require.Equal(t, 1, fake.count(mirasimDeviceSessionPath))
	require.Equal(t, 1, fake.count(mirasimAvailableModelsPath))
	require.Equal(t, 1, fake.count(mirasimLimitsPath))
	require.Equal(t, []string{
		mirasimAccessRefreshPath,
		mirasimDeviceSessionPath,
		mirasimAvailableModelsPath,
		mirasimLimitsPath,
	}, fake.paths())

	modelsCalls := fake.callsFor(mirasimAvailableModelsPath)
	require.Len(t, modelsCalls, 1)
	modelsCall := modelsCalls[0]

	// 空 body + 无查询串：只读契约的一部分（有 query 会让上游签名校验失败）。
	require.Equal(t, http.MethodGet, modelsCall.Method)
	require.Empty(t, modelsCall.RawQuery)
	require.Empty(t, modelsCall.Body)
	require.Equal(t, mirasimDefaultRelayURL+mirasimAvailableModelsPath, modelsCall.URL)
	require.Equal(t, "Bearer "+mirasimMetaTicket, modelsCall.Auth)

	// 明文签名头；不得出现密封头，也不得出现 probe 之外的自定义头。
	require.Empty(t, modelsCall.Header.Get(mirasimHeaderEnc), "只读请求不得密封")
	require.Empty(t, modelsCall.Header.Get(mirasimHeaderProbe))
	require.Equal(t, mirasimDefaultClientVersion, modelsCall.Header.Get(mirasimHeaderClient))
	require.Equal(t, accountID, modelsCall.AccountID)

	// 独立超时：8s（available models）/ 10s（limits），且不得复用模型转发的超时。
	require.True(t, modelsCall.HasDeadline)
	require.LessOrEqual(t, modelsCall.Deadline.Sub(modelsCall.At), mirasimModelsTimeout)
	require.Greater(t, modelsCall.Deadline.Sub(modelsCall.At), mirasimModelsTimeout-time.Second)

	// 签名可用账号自己的设备公钥验证：canonical string 必须用"实际发出的 path +
	// 实际使用的 credential（ticket）"重建。
	priv, err := mirasimParsePrivateKeyPEM(account.GetCredential(mirasimCredentialDeviceKey))
	require.NoError(t, err)
	deviceID, _ := mirasimDeviceIdentity(priv)
	require.Equal(t, deviceID, modelsCall.Header.Get(mirasimHeaderDevice))
	ts, err := strconv.ParseInt(modelsCall.Header.Get(mirasimHeaderTS), 10, 64)
	require.NoError(t, err)
	signature, err := base64.RawURLEncoding.DecodeString(modelsCall.Header.Get(mirasimHeaderSig))
	require.NoError(t, err)
	canonical := mirasimCanonicalString(
		http.MethodGet, mirasimAvailableModelsPath, ts,
		modelsCall.Header.Get(mirasimHeaderNonce), deviceID, mirasimDefaultClientVersion,
		mirasimMetaTicket, nil, nil,
	)
	require.True(t, ed25519.Verify(priv.Public().(ed25519.PublicKey), []byte(canonical), signature),
		"只读请求的签名必须能被设备公钥验证")

	// /v1/limits 唯一允许的额外头：x-mirasim-probe: usage。
	limitsCalls := fake.callsFor(mirasimLimitsPath)
	require.Len(t, limitsCalls, 1)
	limitsCall := limitsCalls[0]
	require.Equal(t, mirasimProbeUsage, limitsCall.Header.Get(mirasimHeaderProbe))
	require.Equal(t, "Bearer "+mirasimMetaTicket, limitsCall.Auth)
	require.Empty(t, limitsCall.Header.Get(mirasimHeaderEnc))
	require.Empty(t, limitsCall.RawQuery)
	require.Empty(t, limitsCall.Body)
	require.True(t, limitsCall.HasDeadline)
	require.LessOrEqual(t, limitsCall.Deadline.Sub(limitsCall.At), mirasimLimitsTimeout)
	require.Greater(t, limitsCall.Deadline.Sub(limitsCall.At), mirasimLimitsTimeout-time.Second)
}

// ---------------------------------------------------------------------------
// 1.8 / 4.7 401 兜底：失效缓存凭据 + 换另一凭据重试恰好一次
// ---------------------------------------------------------------------------

func TestMirasimMetadataRetriesOnceAfterUnauthorizedWithFreshCredential(t *testing.T) {
	const accountID = int64(942002)
	account := mirasimMetaTestAccount(t, accountID, nil)

	fake := &mirasimMetaFake{
		models: func(call int) (int, string) {
			if call == 1 {
				return http.StatusUnauthorized, `{"error":"ticket expired"}`
			}
			return http.StatusOK, mirasimModelsFixture
		},
		session: func(call int) (int, string) {
			return http.StatusOK, fmt.Sprintf(`{"ticket":"meta-ticket-%d","expires_in":900}`, call)
		},
	}
	service := NewMirasimMetadataService(nil, fake)

	// 预热一份 ticket，证明第一次尝试复用缓存、而不是每次重新换票。
	client, err := service.client(mustMirasimMetaConfig(t, account))
	require.NoError(t, err)
	client.mu.Lock()
	client.cachedTicket = "warm-ticket"
	client.ticketExp = time.Now().Add(time.Hour)
	client.mu.Unlock()

	models, err := service.AvailableModels(context.Background(), account, false)
	require.NoError(t, err)
	require.NotEmpty(t, models)

	require.Equal(t, 2, fake.count(mirasimAvailableModelsPath), "401 后必须重试一次")
	require.Equal(t, 1, fake.count(mirasimDeviceSessionPath), "失效后只换一次凭据")
	require.Equal(t, 1, fake.count(mirasimAccessRefreshPath))

	calls := fake.callsFor(mirasimAvailableModelsPath)
	require.Equal(t, "Bearer warm-ticket", calls[0].Auth)
	require.NotEqual(t, calls[0].Auth, calls[1].Auth, "重试必须换用另一份凭据")
	require.Equal(t, "Bearer meta-ticket-1", calls[1].Auth)
}

func TestMirasimMetadataDoesNotInvalidateCredentialOnServerError(t *testing.T) {
	const accountID = int64(942003)
	account := mirasimMetaTestAccount(t, accountID, nil)

	fake := &mirasimMetaFake{
		models: func(int) (int, string) {
			return http.StatusInternalServerError, `{"error":"upstream exploded with secret body"}`
		},
	}
	service := NewMirasimMetadataService(nil, fake)

	client, err := service.client(mustMirasimMetaConfig(t, account))
	require.NoError(t, err)
	client.mu.Lock()
	client.cachedTicket = "warm-ticket"
	client.ticketExp = time.Now().Add(time.Hour)
	client.mu.Unlock()

	_, err = service.AvailableModels(context.Background(), account, false)
	require.Error(t, err)

	// 500 不是凭据问题：不重试、不动缓存凭据、不触发 refresh。
	require.Equal(t, 1, fake.count(mirasimAvailableModelsPath))
	require.Zero(t, fake.count(mirasimDeviceSessionPath))
	require.Zero(t, fake.count(mirasimAccessRefreshPath))

	client.mu.Lock()
	cachedTicket := client.cachedTicket
	client.mu.Unlock()
	require.Equal(t, "warm-ticket", cachedTicket, "非鉴权失败不得失效 ticket")

	require.Contains(t, err.Error(), "HTTP 500")
	require.NotContains(t, err.Error(), "secret body", "错误消息不得携带上游正文")

	var metaErr *mirasimMetadataHTTPError
	require.ErrorAs(t, err, &metaErr)
	require.Equal(t, mirasimMetadataErrorCategory(err), "upstream_http_500")

	// 失败不写负缓存：下一次调用应当重新请求上游。
	_, err = service.AvailableModels(context.Background(), account, false)
	require.Error(t, err)
	require.Equal(t, 2, fake.count(mirasimAvailableModelsPath))
}

func TestMirasimMetadataRetriesAtMostOnce(t *testing.T) {
	const accountID = int64(942004)
	account := mirasimMetaTestAccount(t, accountID, nil)

	fake := &mirasimMetaFake{
		models: func(int) (int, string) {
			return http.StatusForbidden, `{"error":"forbidden"}`
		},
	}
	service := NewMirasimMetadataService(nil, fake)

	_, err := service.AvailableModels(context.Background(), account, false)
	require.Error(t, err)
	// spec：401/403 后失效 ticket/access 缓存，用**新领的**凭据重试一次。
	// 重试路径会重新走 换取 access token → 申领 device ticket → 再发一次只读请求，
	// 因此 refresh / session / available-models 各为 2（首次 + 重试一次，绝不多于一次）。
	require.Equal(t, 2, fake.count(mirasimAvailableModelsPath), "最多重试一次")
	require.Equal(t, 2, fake.count(mirasimDeviceSessionPath), "重试必须换一份新 ticket")
	require.Equal(t, 2, fake.count(mirasimAccessRefreshPath))
	require.Equal(t, 0, fake.count(mirasimLimitsPath))
}

// ---------------------------------------------------------------------------
// 4.7 缓存：TTL、force、按账号 / 按配置指纹隔离、显式失效
// ---------------------------------------------------------------------------

func TestMirasimMetadataCacheTTLForceAndFingerprint(t *testing.T) {
	const accountID = int64(942005)
	account := mirasimMetaTestAccount(t, accountID, nil)

	fake := &mirasimMetaFake{}
	service := NewMirasimMetadataService(nil, fake)

	now := time.Unix(1800000000, 0)
	service.now = func() time.Time { return now }

	ctx := context.Background()
	first, err := service.AvailableModels(ctx, account, false)
	require.NoError(t, err)
	require.NotEmpty(t, first)
	second, err := service.AvailableModels(ctx, account, false)
	require.NoError(t, err)
	require.Equal(t, first, second, "TTL 内必须命中缓存")
	require.Equal(t, 1, fake.count(mirasimAvailableModelsPath), "缓存命中不得再打上游")

	// available models TTL 10 分钟：到点即失效（>= TTL）。
	now = now.Add(mirasimModelsCacheTTL)
	_, err = service.AvailableModels(ctx, account, false)
	require.NoError(t, err)
	require.Equal(t, 2, fake.count(mirasimAvailableModelsPath), "TTL 到点必须重新拉取")

	// force 绕过缓存。
	_, err = service.AvailableModels(ctx, account, true)
	require.NoError(t, err)
	require.Equal(t, 3, fake.count(mirasimAvailableModelsPath))

	// limits TTL 30 秒，与 available models TTL 相互独立。
	require.NoError(t, err)
	_, err = service.Limits(ctx, account, false)
	require.NoError(t, err)
	_, err = service.Limits(ctx, account, false)
	require.NoError(t, err)
	require.Equal(t, 1, fake.count(mirasimLimitsPath))
	now = now.Add(mirasimLimitsCacheTTL)
	_, err = service.Limits(ctx, account, false)
	require.NoError(t, err)
	require.Equal(t, 2, fake.count(mirasimLimitsPath))
	require.Equal(t, 3, fake.count(mirasimAvailableModelsPath), "limits 拉取不得影响 available models 缓存")

	// 显式失效后重新拉取。
	service.InvalidateMirasimMetadata(accountID)
	_, err = service.AvailableModels(ctx, account, false)
	require.NoError(t, err)
	require.Equal(t, 4, fake.count(mirasimAvailableModelsPath))

	// 配置指纹变化（client version）让旧缓存失效。
	account.Extra[mirasimExtraClientVersion] = "9.9.9"
	_, err = service.AvailableModels(ctx, account, false)
	require.NoError(t, err)
	require.Equal(t, 5, fake.count(mirasimAvailableModelsPath))

	// 另一个账号不共享缓存。
	otherAccount := mirasimMetaTestAccount(t, accountID+1, nil)
	_, err = service.AvailableModels(ctx, otherAccount, false)
	require.NoError(t, err)
	require.Equal(t, 6, fake.count(mirasimAvailableModelsPath))
}

func TestMirasimMetadataConfigErrorSkipsUpstream(t *testing.T) {
	const accountID = int64(942006)
	mirasimForgetAccount(accountID)
	t.Cleanup(func() { mirasimForgetAccount(accountID) })

	account := &Account{
		ID:          accountID,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{},
		Extra:       map[string]any{mirasimExtraRelayEnabled: true},
	}
	fake := &mirasimMetaFake{}
	service := NewMirasimMetadataService(nil, fake)

	_, err := service.AvailableModels(context.Background(), account, false)
	require.Error(t, err)
	require.True(t, mirasimMetadataIsConfigError(err), "凭据缺失必须归类为配置错误")
	require.Empty(t, fake.paths(), "凭据缺失时不得发起任何上游请求")

	_, err = service.Limits(context.Background(), account, false)
	require.Error(t, err)
	require.True(t, mirasimMetadataIsConfigError(err))
	require.Empty(t, fake.paths())
}

// mustMirasimMetaConfig 是测试里取配置快照的捷径（仅在账号凭据完整时可用）。
func mustMirasimMetaConfig(t *testing.T, account *Account) mirasimRelayConfig {
	t.Helper()
	cfg, err := mirasimRelayConfigForAccount(account)
	require.NoError(t, err)
	return cfg
}

// ---------------------------------------------------------------------------
// 2.5 可用模型解析（GET /v1/models，账号维度）
// ---------------------------------------------------------------------------

func TestMirasimAvailableModelsParseNormalizesEntries(t *testing.T) {
	models, err := parseMirasimAvailableModels([]byte(mirasimModelsFixture))
	require.NoError(t, err)
	require.Equal(t, []string{"KIMI-K3", "deepseek-flash", "glm-5.3-flash", "kimi-k3"}, models,
		"trim + 去重 + 升序；刻意不做 lowercase（模型 id 区分大小写）")
}

func TestMirasimAvailableModelsParseTrimsAndDedupes(t *testing.T) {
	models, err := parseMirasimAvailableModels([]byte(`{"data":[{"id":"  a  "},{"id":"a"},{"id":"b"}]}`))
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, models)
}

func TestMirasimAvailableModelsParseRejectsInvalidDocuments(t *testing.T) {
	cases := map[string]string{
		"根不是 JSON":  `not-json`,
		"根是数组":      `[]`,
		"根是 null":   `null`,
		"缺少 data":   `{"object":"list"}`,
		"data 不是数组": `{"object":"list","data":{"id":"x"}}`,
		"data 为空数组": `{"object":"list","data":[]}`,
		"全部条目非法":    `{"data":[{"id":""},{"id":42},"x",null]}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			models, err := parseMirasimAvailableModels([]byte(payload))
			require.Error(t, err)
			require.Empty(t, models, "失败时不得返回部分结果")
		})
	}
}

// ---------------------------------------------------------------------------
// 2.5 limits 解析与窗口归一化
// ---------------------------------------------------------------------------

func TestMirasimWindowUsedPercentMatchesClientFormula(t *testing.T) {
	cases := []struct {
		name   string
		used   float64
		budget float64
		want   float64
		ok     bool
	}{
		{name: "half", used: 50, budget: 100, want: 50, ok: true},
		{name: "one decimal", used: 1, budget: 3, want: 33.3, ok: true},
		{name: ">=99 caps to 100", used: 99, budget: 100, want: 100, ok: true},
		// 客户端 `Qir`：先保留 1 位小数，只有结果 >= 99 才上取为 100。
		// 98.9 归一化后不触顶，必须原样保留；98.96 会先四舍五入到 99 再触顶。
		{name: "just below cap stays 98.9", used: 98.9, budget: 100, want: 98.9, ok: true},
		{name: "rounds to 99 then caps to 100", used: 98.96, budget: 100, want: 100, ok: true},
		{name: "over budget clamps to 100 not cap", used: 300, budget: 100, want: 100, ok: true},
		{name: "negative used clamps to 0", used: -5, budget: 100, want: 0, ok: true},
		{name: "unlimited budget has no percent", used: 5, budget: 0, ok: false},
		{name: "negative budget has no percent", used: 5, budget: -1, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := mirasimWindowUsedPercent(tc.used, tc.budget)
			require.Equal(t, tc.ok, ok)
			if tc.ok {
				require.InDelta(t, tc.want, got, 1e-9)
			}
		})
	}

	require.Equal(t, "allowed", mirasimWindowStatusFor(0))
	require.Equal(t, "allowed", mirasimWindowStatusFor(79.9))
	require.Equal(t, "warning", mirasimWindowStatusFor(80))
	require.Equal(t, "warning", mirasimWindowStatusFor(99.9))
	require.Equal(t, "limit_reached", mirasimWindowStatusFor(100))
}

func TestMirasimLimitsParseNormalizesWindows(t *testing.T) {
	snapshot, err := parseMirasimLimits([]byte(mirasimLimitsFixture))
	require.NoError(t, err)

	require.NotNil(t, snapshot.Paid)
	require.True(t, *snapshot.Paid)
	require.True(t, snapshot.Degraded)
	require.False(t, snapshot.CapturedAt.IsZero())
	require.Len(t, snapshot.Windows, 3, "负 budget / 非法 used / 空 name 的窗口必须被丢弃")
	require.Equal(t, "allowed", snapshot.Status, "顶层状态只看非 model_scoped 窗口")

	fiveHour := snapshot.Windows[0]
	require.Equal(t, "5h", fiveHour.Name)
	require.Equal(t, "5h", fiveHour.Label)
	require.False(t, fiveHour.Unlimited)
	require.NotNil(t, fiveHour.Utilization)
	require.InDelta(t, 50, *fiveHour.Utilization, 1e-9)
	require.NotNil(t, fiveHour.RemainingPercent)
	require.InDelta(t, 50, *fiveHour.RemainingPercent, 1e-9)
	require.Equal(t, "allowed", fiveHour.Status)
	require.Equal(t, time.Unix(1750000000, 0).UTC().Format(time.RFC3339), fiveHour.ResetAt)

	sevenDay := snapshot.Windows[1]
	require.Equal(t, "7d", sevenDay.Name)
	require.True(t, sevenDay.Unlimited)
	require.Nil(t, sevenDay.Utilization, "budget == 0 表示不限量，不计算百分比")
	require.Nil(t, sevenDay.RemainingPercent)
	require.Equal(t, "allowed", sevenDay.Status)
	require.Empty(t, sevenDay.ResetAt, "缺失 / 非法 reset_at 必须留空")

	scoped := snapshot.Windows[2]
	require.Equal(t, "7d-opus", scoped.Name)
	require.True(t, scoped.ModelScoped)
	require.NotNil(t, scoped.Utilization)
	require.InDelta(t, 95, *scoped.Utilization, 1e-9)
	require.Equal(t, "warning", scoped.Status)
	require.Empty(t, scoped.ResetAt, "reset_at <= 0 必须留空")
}

func TestMirasimLimitsParseRejectsDocumentsWithoutUsableWindows(t *testing.T) {
	cases := map[string]string{
		"根不是对象":       `[]`,
		"根不是 JSON":    `nope`,
		"windows 缺失":  `{"paid":true}`,
		"windows 非数组": `{"windows":{"name":"5h"}}`,
		"windows 全非法": `{"windows":[{"name":"5h","budget":-1,"used":1}]}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseMirasimLimits([]byte(payload))
			require.Error(t, err)
		})
	}
}

func TestMirasimLimitsStatusFallsBackToScopedWindows(t *testing.T) {
	snapshot, err := parseMirasimLimits([]byte(`{"windows":[
		{"name":"5h","budget":100,"used":10},
		{"name":"opus","budget":100,"used":100,"model_scoped":true}
	]}`))
	require.NoError(t, err)
	require.Equal(t, "allowed", snapshot.Status, "model_scoped 窗口不得决定顶层状态")

	scopedOnly, err := parseMirasimLimits([]byte(`{"windows":[
		{"name":"opus","budget":100,"used":100,"model_scoped":true}
	]}`))
	require.NoError(t, err)
	require.Equal(t, "limit_reached", scopedOnly.Status, "没有任何非 model_scoped 窗口时才回退全部窗口")

	warningOnly, err := parseMirasimLimits([]byte(`{"windows":[
		{"name":"5h","budget":100,"used":85}
	]}`))
	require.NoError(t, err)
	require.Equal(t, "warning", warningOnly.Status)
}

// ---------------------------------------------------------------------------
// 4.7 快照 → UsageInfo 映射（5h / 7d 与 model_scoped）
// ---------------------------------------------------------------------------

func TestMirasimApplyLimitsSnapshotMapsFiveHourAndSevenDay(t *testing.T) {
	resetUnix := time.Now().Add(time.Hour).Unix()
	payload := fmt.Sprintf(`{
	  "paid": false,
	  "windows": [
	    {"name": "5h", "budget": 200, "used": 20, "reset_at": %d},
	    {"name": "7d", "budget": 200, "used": 100},
	    {"name": "7d-opus", "budget": 100, "used": 100, "model_scoped": true}
	  ]
	}`, resetUnix)

	snapshot, err := parseMirasimLimits([]byte(payload))
	require.NoError(t, err)

	info := &UsageInfo{}
	mirasimApplyLimitsSnapshot(info, snapshot)

	require.Equal(t, "active", info.Source)
	require.NotNil(t, info.UpdatedAt)
	require.NotNil(t, info.MirasimPaid)
	require.False(t, *info.MirasimPaid)
	require.False(t, info.MirasimDegraded)
	require.Len(t, info.MirasimWindows, 3, "全部窗口（含 model_scoped）都要透出")

	require.NotNil(t, info.FiveHour)
	require.Equal(t, "5h", info.FiveHour.WindowLabel)
	require.InDelta(t, 10, info.FiveHour.Utilization, 1e-9)
	require.NotNil(t, info.FiveHour.ResetsAt)
	require.Greater(t, info.FiveHour.RemainingSeconds, 3500)
	require.LessOrEqual(t, info.FiveHour.RemainingSeconds, 3600)

	require.NotNil(t, info.SevenDay)
	require.Equal(t, "7d", info.SevenDay.WindowLabel)
	require.InDelta(t, 50, info.SevenDay.Utilization, 1e-9)
	require.Nil(t, info.SevenDay.ResetsAt)

	require.Equal(t, mirasimWindowKeyFiveHour, info.MirasimWindows[0].WindowKey)
	require.Equal(t, mirasimWindowKeySevenDay, info.MirasimWindows[1].WindowKey)
	require.Empty(t, info.MirasimWindows[2].WindowKey, "model_scoped 窗口不参与映射")
	require.Equal(t, "limit_reached", info.MirasimWindows[2].Status)

	// 不限量窗口不映射；nil 入参不得 panic。
	unlimited, err := parseMirasimLimits([]byte(`{"windows":[{"name":"7d","budget":0,"used":1}]}`))
	require.NoError(t, err)
	unlimitedInfo := &UsageInfo{}
	mirasimApplyLimitsSnapshot(unlimitedInfo, unlimited)
	require.Nil(t, unlimitedInfo.SevenDay)
	require.Empty(t, unlimitedInfo.MirasimWindows[0].WindowKey)

	mirasimApplyLimitsSnapshot(nil, snapshot)
	mirasimApplyLimitsSnapshot(&UsageInfo{}, nil)
}

// ---------------------------------------------------------------------------
// 4.7 降级：HTTP 200 + error 字段、不动调度状态
// ---------------------------------------------------------------------------

type mirasimMetaQuerierStub struct {
	models []string
	limits *mirasimLimitsSnapshot
	err    error

	modelsErr   error
	limitsErr   error
	modelsCalls int
	limitsCalls int
	forceFlags  []bool
	invalidated []int64
}

func (s *mirasimMetaQuerierStub) AvailableModels(_ context.Context, _ *Account, force bool) ([]string, error) {
	s.modelsCalls++
	s.forceFlags = append(s.forceFlags, force)
	if s.modelsErr != nil {
		return nil, s.modelsErr
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.models, nil
}

func (s *mirasimMetaQuerierStub) Limits(_ context.Context, _ *Account, force bool) (*mirasimLimitsSnapshot, error) {
	s.limitsCalls++
	s.forceFlags = append(s.forceFlags, force)
	if s.limitsErr != nil {
		return nil, s.limitsErr
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.limits, nil
}

func (s *mirasimMetaQuerierStub) InvalidateMirasimMetadata(accountID int64) {
	s.invalidated = append(s.invalidated, accountID)
}

var _ MirasimMetadataQuerier = (*mirasimMetaQuerierStub)(nil)

func TestGetMirasimUsageDegradesWithoutTouchingScheduling(t *testing.T) {
	const accountID = int64(942007)
	account := mirasimMetaTestAccount(t, accountID, nil)
	account.Schedulable = true
	rateLimitResetAt := account.RateLimitResetAt
	tempUntil := account.TempUnschedulableUntil

	stub := &mirasimMetaQuerierStub{
		limitsErr: fmt.Errorf("mirasim relay: 拉取 limits HTTP 500: %w",
			&mirasimHTTPError{Op: "拉取 limits", StatusCode: 500, Body: "upstream-secret-body"}),
	}
	service := &AccountUsageService{}
	service.SetMirasimMetadataService(stub)

	info, err := service.getMirasimUsage(context.Background(), account, true)
	require.NoError(t, err, "只读元数据失败必须降级为 HTTP 200，不得返回错误")
	require.NotNil(t, info)
	require.Equal(t, "active", info.Source)
	require.Equal(t, "failed to fetch mirasim usage: upstream_http_500", info.Error)
	require.NotContains(t, info.Error, "upstream-secret-body", "降级消息不得含上游正文")
	require.Empty(t, info.MirasimWindows)
	require.Nil(t, info.UpdatedAt)
	require.Nil(t, info.FiveHour)
	require.Nil(t, info.SevenDay)

	// 强制刷新的语义必须透传给查询器（管理端手动刷新依赖它）。
	require.Equal(t, []bool{true}, stub.forceFlags)
	require.Equal(t, 1, stub.limitsCalls)
	require.Zero(t, stub.modelsCalls)

	// 不得改动账号调度状态。
	require.True(t, account.Schedulable)
	require.Equal(t, rateLimitResetAt, account.RateLimitResetAt)
	require.Equal(t, tempUntil, account.TempUnschedulableUntil)
}

func TestGetMirasimUsageWithoutQuerierAndWithSnapshot(t *testing.T) {
	const accountID = int64(942008)
	account := mirasimMetaTestAccount(t, accountID, nil)

	unconfigured := &AccountUsageService{}
	info, err := unconfigured.getMirasimUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.Equal(t, "mirasim metadata query is not configured", info.Error)
	require.Empty(t, info.MirasimWindows)

	snapshot, err := parseMirasimLimits([]byte(mirasimLimitsFixture))
	require.NoError(t, err)
	stub := &mirasimMetaQuerierStub{limits: snapshot}
	service := &AccountUsageService{}
	service.SetMirasimMetadataService(stub)

	info, err = service.getMirasimUsage(context.Background(), account, false)
	require.NoError(t, err)
	require.Empty(t, info.Error)
	require.Len(t, info.MirasimWindows, 3)
	require.NotNil(t, info.UpdatedAt)
	require.Equal(t, []bool{false}, stub.forceFlags)

	// 账号为 nil 时日志兜底不得 panic。
	info, err = service.getMirasimUsage(context.Background(), nil, false)
	require.NoError(t, err)
	require.Len(t, info.MirasimWindows, 3)
}

// ---------------------------------------------------------------------------
// 4.2 模型同步：mirasim 走账号维度 /v1/models（不再用全局 roster）
// ---------------------------------------------------------------------------

func TestFetchUpstreamSupportedModelsUsesMirasimAvailableModels(t *testing.T) {
	const accountID = int64(942009)
	account := mirasimMetaTestAccount(t, accountID, nil)

	fake := &mirasimMetaFake{}
	metadata := NewMirasimMetadataService(nil, fake)

	testService := NewAccountTestService(nil, nil, nil, nil, nil, fake, nil, nil)
	testService.SetMirasimMetadataService(metadata)

	models, err := testService.FetchUpstreamSupportedModels(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{
		"KIMI-K3",
		"deepseek-flash",
		"glm-5.3-flash",
		"kimi-k3",
	}, models)
	require.Equal(t, 1, fake.count(mirasimAvailableModelsPath))
	require.Equal(t, 0, fake.count("/v1/model-roster"), "白名单不得再走全局 roster")

	// 显式同步必须绕过缓存；账号级 roster agent 覆盖对新来源不再有任何影响。
	account.Extra["mirasim_roster_agent"] = "codex" // 遗留键：现已不参与白名单
	models, err = testService.FetchUpstreamSupportedModels(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"KIMI-K3", "deepseek-flash", "glm-5.3-flash", "kimi-k3"}, models,
		"遗留的 extra.mirasim_roster_agent 不得影响账号维度可用模型")
	require.Equal(t, 2, fake.count(mirasimAvailableModelsPath), "显式同步不得复用旧缓存")
}

func TestFetchUpstreamSupportedModelsMirasimErrorMapping(t *testing.T) {
	t.Run("metadata client 未配置 → configuration", func(t *testing.T) {
		const accountID = int64(942010)
		account := mirasimMetaTestAccount(t, accountID, nil)
		testService := NewAccountTestService(nil, nil, nil, nil, nil, &mirasimMetaFake{}, nil, nil)

		var syncErr *UpstreamModelSyncError
		_, err := testService.FetchUpstreamSupportedModels(context.Background(), account)
		require.ErrorAs(t, err, &syncErr)
		require.Equal(t, UpstreamModelSyncErrorConfiguration, syncErr.Kind)
		require.Contains(t, syncErr.SafeMessage(), "Mirasim metadata client is not configured")
	})

	t.Run("凭据缺失 → configuration 且不发请求", func(t *testing.T) {
		const accountID = int64(942011)
		mirasimForgetAccount(accountID)
		t.Cleanup(func() { mirasimForgetAccount(accountID) })

		account := &Account{
			ID:          accountID,
			Platform:    PlatformAnthropic,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{},
			Extra:       map[string]any{mirasimExtraRelayEnabled: true},
		}
		fake := &mirasimMetaFake{}
		testService := NewAccountTestService(nil, nil, nil, nil, nil, fake, nil, nil)
		testService.SetMirasimMetadataService(NewMirasimMetadataService(nil, fake))

		var syncErr *UpstreamModelSyncError
		_, err := testService.FetchUpstreamSupportedModels(context.Background(), account)
		require.ErrorAs(t, err, &syncErr)
		require.Equal(t, UpstreamModelSyncErrorConfiguration, syncErr.Kind)
		require.Contains(t, syncErr.SafeMessage(), "Mirasim relay credentials are not configured")
		require.Empty(t, fake.paths(), "凭据缺失不得发起上游请求")
	})

	t.Run("上游 500 → upstream", func(t *testing.T) {
		const accountID = int64(942012)
		account := mirasimMetaTestAccount(t, accountID, nil)
		fake := &mirasimMetaFake{
			models: func(int) (int, string) {
				return http.StatusInternalServerError, `{"error":"boom"}`
			},
		}
		testService := NewAccountTestService(nil, nil, nil, nil, nil, fake, nil, nil)
		testService.SetMirasimMetadataService(NewMirasimMetadataService(nil, fake))

		var syncErr *UpstreamModelSyncError
		_, err := testService.FetchUpstreamSupportedModels(context.Background(), account)
		require.ErrorAs(t, err, &syncErr)
		require.Equal(t, UpstreamModelSyncErrorUpstream, syncErr.Kind)
		require.Contains(t, syncErr.SafeMessage(), "upstream_http_500")
		require.NotContains(t, syncErr.Error(), "boom")
	})

	t.Run("响应解析失败 → upstream", func(t *testing.T) {
		const accountID = int64(942013)
		account := mirasimMetaTestAccount(t, accountID, nil)
		fake := &mirasimMetaFake{
			models: func(int) (int, string) {
				return http.StatusOK, `{"object":"list","data":[]}`
			},
		}
		testService := NewAccountTestService(nil, nil, nil, nil, nil, fake, nil, nil)
		testService.SetMirasimMetadataService(NewMirasimMetadataService(nil, fake))

		var syncErr *UpstreamModelSyncError
		_, err := testService.FetchUpstreamSupportedModels(context.Background(), account)
		require.ErrorAs(t, err, &syncErr)
		require.Equal(t, UpstreamModelSyncErrorUpstream, syncErr.Kind)
		require.Contains(t, syncErr.SafeMessage(), "upstream")
	})

	t.Run("可用模型为空 → upstream（防御性）", func(t *testing.T) {
		const accountID = int64(942014)
		account := mirasimMetaTestAccount(t, accountID, nil)
		stub := &mirasimMetaQuerierStub{}
		testService := NewAccountTestService(nil, nil, nil, nil, nil, &mirasimMetaFake{}, nil, nil)
		testService.SetMirasimMetadataService(stub)

		var syncErr *UpstreamModelSyncError
		_, err := testService.FetchUpstreamSupportedModels(context.Background(), account)
		require.ErrorAs(t, err, &syncErr)
		require.Equal(t, UpstreamModelSyncErrorUpstream, syncErr.Kind)
		require.Contains(t, syncErr.SafeMessage(), "Mirasim returned no available models")
		require.Equal(t, 1, stub.modelsCalls)
		require.Equal(t, []bool{true}, stub.forceFlags, "显式模型同步必须绕过缓存")
	})
}

// ---------------------------------------------------------------------------
// 7.5 失败日志脱敏：只允许 account_id + category，不得出现上游正文/凭据
// ---------------------------------------------------------------------------

// mirasimMetaLogCapture 是并发安全的日志缓冲（slog 可能来自被锁保护的 goroutine）。
type mirasimMetaLogCapture struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (c *mirasimMetaLogCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *mirasimMetaLogCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// TestMirasimMetadataFailureLogsStaySanitized 钉死 7.5 的日志契约：
// mirasim metadata 的失败日志只能是 {account_id, category}，禁止出现上游响应正文、
// access token、device ticket、轮换后的 refresh token、设备私钥或任何 err.Error()
// （mirasimHTTPError.Error() 会带截断的上游 body）。
func TestMirasimMetadataFailureLogsStaySanitized(t *testing.T) {
	const upstreamBodySentinel = "upstream-body-must-never-be-logged"

	cases := []struct {
		name      string
		event     string
		category  string
		models    func(int) (int, string)
		limits    func(int) (int, string)
		useLimits bool
	}{
		{
			name:     "available models 上游 500",
			event:    "mirasim_models_fetch_failed",
			category: "upstream_http_500",
			models: func(int) (int, string) {
				return http.StatusInternalServerError, `{"error":"` + upstreamBodySentinel + `"}`
			},
		},
		{
			name:     "available models 401 重试一次后仍失败",
			event:    "mirasim_models_fetch_failed",
			category: "upstream_http_401",
			models: func(int) (int, string) {
				return http.StatusUnauthorized, `{"error":"` + upstreamBodySentinel + `"}`
			},
		},
		{
			name:     "available models 解析失败",
			event:    "mirasim_models_parse_failed",
			category: "upstream",
			models: func(int) (int, string) {
				return http.StatusOK, `{"object":"list","data":[],"note":"` + upstreamBodySentinel + `"}`
			},
		},
		{
			name:      "limits 上游 500",
			event:     "mirasim_limits_fetch_failed",
			category:  "upstream_http_500",
			useLimits: true,
			limits: func(int) (int, string) {
				return http.StatusInternalServerError, `{"error":"` + upstreamBodySentinel + `"}`
			},
		},
		{
			name:      "limits 解析失败",
			event:     "mirasim_limits_parse_failed",
			category:  "upstream",
			useLimits: true,
			limits: func(int) (int, string) {
				return http.StatusOK, `{"windows":[{"name":"7d","budget":` + upstreamBodySentinel + `}]}`
			},
		},
	}

	for i, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			accountID := int64(942100 + i)
			account := mirasimMetaTestAccount(t, accountID, nil)
			fake := &mirasimMetaFake{models: tc.models, limits: tc.limits}
			metadataService := NewMirasimMetadataService(nil, fake)

			capture := &mirasimMetaLogCapture{}
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(capture, nil)))
			defer slog.SetDefault(previous)

			ctx := context.Background()
			var err error
			if tc.useLimits {
				_, err = metadataService.Limits(ctx, account, true)
			} else {
				_, err = metadataService.AvailableModels(ctx, account, true)
			}
			slog.SetDefault(previous)

			require.Error(t, err)
			out := capture.String()
			require.Contains(t, out, tc.event, "失败必须留一条事件日志")
			require.Contains(t, out, `"account_id":`+strconv.FormatInt(accountID, 10))
			require.Contains(t, out, `"category":"`+tc.category+`"`)

			// 禁止项：上游正文、凭据、err.Error() 新增字段。
			for _, banned := range []string{
				upstreamBodySentinel,
				"meta-access-token",    // refresh 响应里的 access token
				"meta-rotated-refresh", // 轮换后的 refresh token
				mirasimMetaTicket,      // device ticket
				"BEGIN",                // 设备私钥 PEM
				"PRIVATE KEY",
				"\"error\":",
				"\"err\":",
				"\"body\":",
				"\"credential\":",
				"\"token\":",
			} {
				require.NotContains(t, out, banned, "日志不得包含 %q", banned)
			}

			// 事件日志只允许这些键（slog handler 自带 time/level/msg）。
			for _, field := range []string{"account_id", "category", "msg", "level", "time"} {
				require.Contains(t, out, `"`+field+`"`)
			}
		})
	}
}

// TestMirasimMetadataClientErrorLogIsNeverWritten 证明凭据/配置缺失不走日志也不请求：
// 这类错误要在管理端以 configuration 暴露，日志里只应看到调用方的事件（当前为 0 条）。
func TestMirasimMetadataConfigFailureLogsNothing(t *testing.T) {
	const accountID = int64(942199)
	mirasimForgetAccount(accountID)
	t.Cleanup(func() { mirasimForgetAccount(accountID) })

	account := &Account{
		ID:          accountID,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{},
		Extra:       map[string]any{mirasimExtraRelayEnabled: true},
	}
	fake := &mirasimMetaFake{}
	metadataService := NewMirasimMetadataService(nil, fake)

	capture := &mirasimMetaLogCapture{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(capture, nil)))
	defer slog.SetDefault(previous)

	_, err := metadataService.AvailableModels(context.Background(), account, true)
	slog.SetDefault(previous)

	require.Error(t, err)
	require.True(t, mirasimMetadataIsConfigError(err))
	require.Empty(t, fake.paths(), "配置缺失不得发起上游请求")
	require.NotContains(t, capture.String(), strconv.FormatInt(accountID, 10), "配置错误不应污染日志")
	require.NotContains(t, capture.String(), "mirasim_", "配置错误不应产生 mirasim 事件日志")
}
