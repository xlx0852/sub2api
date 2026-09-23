package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// OpenCode Go 官方用量窗口查询。
//
// 为什么不做成"新平台"：OpenCode Go 是一个 **OpenAI 兼容**上游，
// 现有「API-key 账号 + credentials.base_url」已经能完整转发它
// （见 Account.GetOpenAIBaseURL）。真正缺的只是**官方用量窗口**这一件事，
// 所以这里只补用量查询，不引入新平台与多协议路由——后者是另一件事、
// 且我们目前是「一平台一协议」的结构。
//
// 协议（对齐上游 v0.2.8 的 opencode_go_usage.go）：
//
//	GET https://opencode.ai/zen/go/v1/usage
//	Authorization: Bearer <api_key>
//
// 返回可能是 {"usage":{"rolling":…,"weekly":…,"monthly":…}}，也可能三个窗口直接放在顶层；
// 缺字段按宽容策略降级为零值，只有"结构性非法 JSON"才算失败。
//
// 安全边界（**必须保持**）：
//   - 只有当账号 base URL 的 **host+path 精确等于** 官方 Go 基址时才发请求——
//     否则等于把别人的 API key 发给 opencode.ai；
//   - 禁止跟随重定向，并在拿到响应后**再校验一次最终 URL**；
//   - 只回填窗口数据与失败原因，**永不记录或回填 API key**。
const (
	openCodeGoUsageURL       = "https://opencode.ai/zen/go/v1/usage"
	openCodeGoBaseHost       = "opencode.ai"
	openCodeGoBasePath       = "/zen/go/v1"
	openCodeGoUsageTimeout   = 15 * time.Second
	openCodeGoUsageMaxBody   = 64 * 1024
	openCodeGoUsageExtraKey  = "opencode_go_usage"
	openCodeGoUsageUserAgent = "sub2api-opencode-go-usage/1"
)

var (
	// ErrOpenCodeGoUsageNotApplicable 表示该账号不是 OpenCode Go（不查询、不报错）。
	ErrOpenCodeGoUsageNotApplicable = errors.New("account is not an opencode go account")
	// ErrOpenCodeGoUsageUnauthorized 表示 key 无效。
	ErrOpenCodeGoUsageUnauthorized = errors.New("opencode go usage unauthorized")
	// ErrOpenCodeGoUsageSubscriptionRequired 表示没有 Go 订阅。
	ErrOpenCodeGoUsageSubscriptionRequired = errors.New("opencode go subscription required")
)

// OpenCodeGoUsageWindow 是单个窗口的窄化视图。
type OpenCodeGoUsageWindow struct {
	// Status 是上游给的状态字符串（原样保留，不做枚举校验——上游会加新值）。
	Status string `json:"status,omitempty"`
	// Percent 是已用百分比。
	Percent float64 `json:"percent"`
	// ResetsAt 是窗口重置时刻；解析失败时为零值。
	ResetsAt time.Time `json:"resets_at,omitempty"`
}

// OpenCodeGoUsage 是一个账号的用量快照，整份存进 extra。
type OpenCodeGoUsage struct {
	Rolling *OpenCodeGoUsageWindow `json:"rolling,omitempty"`
	Weekly  *OpenCodeGoUsageWindow `json:"weekly,omitempty"`
	Monthly *OpenCodeGoUsageWindow `json:"monthly,omitempty"`
	// FetchedAt 是本次抓取时刻（含失败）。
	FetchedAt time.Time `json:"fetched_at"`
	// Error 非空表示本次失败，内容是**给面板看的短原因**，不含凭据。
	Error string `json:"error,omitempty"`
	// HTTPStatus 仅在失败时有意义。
	HTTPStatus int `json:"http_status,omitempty"`
}

type openCodeGoUsageAPIWindow struct {
	Status   string  `json:"status"`
	Percent  float64 `json:"percent"`
	ResetsAt string  `json:"resetsAt"`
}

type openCodeGoUsageAPIWindows struct {
	Rolling *openCodeGoUsageAPIWindow `json:"rolling"`
	Weekly  *openCodeGoUsageAPIWindow `json:"weekly"`
	Monthly *openCodeGoUsageAPIWindow `json:"monthly"`
}

type openCodeGoUsageAPIResponse struct {
	Usage *openCodeGoUsageAPIWindows `json:"usage"`
}

// isOpenCodeGoUsageURL 校验 host 与 path 精确等于官方 Go 基址（或其 /usage 端点）。
//
// 只做前缀匹配是不够的：`https://opencode.ai.evil.example/zen/go/v1` 这类地址
// 会通过前缀检查却把 key 送到别处。
func isOpenCodeGoUsageURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return false
	}
	if parsed.Scheme != "https" || parsed.Host != openCodeGoBaseHost {
		return false
	}
	path := strings.TrimSuffix(strings.TrimSpace(parsed.Path), "/")
	return path == openCodeGoBasePath || path == openCodeGoBasePath+"/usage"
}

// IsOpenCodeGoAccount 判断账号是否为「指向 OpenCode Go 官方基址的 API-key 账号」。
// 这是唯一的触发条件，也是**唯一**会向 opencode.ai 发送该 key 的准入条件。
func IsOpenCodeGoAccount(account *Account) bool {
	if account == nil || !account.IsOpenAIApiKey() {
		return false
	}
	base := strings.TrimSpace(account.GetOpenAIBaseURL())
	if base == "" {
		return false
	}
	// GetOpenAIBaseURL 默认返回 https://api.openai.com（官方 OpenAI），
	// 这不是 OpenCode，必须排除。
	return isOpenCodeGoUsageURL(base)
}

// openCodeGoUsageFromExtra 读出已存的用量快照；没有则返回 nil。
func openCodeGoUsageFromExtra(account *Account) *OpenCodeGoUsage {
	if account == nil {
		return nil
	}
	raw := strings.TrimSpace(account.GetExtraString(openCodeGoUsageExtraKey))
	if raw == "" {
		return nil
	}
	var out OpenCodeGoUsage
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return &out
}

// parseOpenCodeGoUsageJSON 宽容解析官方用量载荷。
// 顶层可为 {"usage":{...}}，也可直接是三个窗口。
func parseOpenCodeGoUsageJSON(body []byte) (*OpenCodeGoUsage, error) {
	var wrapped openCodeGoUsageAPIResponse
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, err
	}
	windows := wrapped.Usage
	if windows == nil {
		var direct openCodeGoUsageAPIWindows
		if err := json.Unmarshal(body, &direct); err != nil {
			return nil, err
		}
		windows = &direct
	}
	out := &OpenCodeGoUsage{}
	if windows.Rolling != nil {
		out.Rolling = openCodeGoUsageWindowFromAPI(windows.Rolling)
	}
	if windows.Weekly != nil {
		out.Weekly = openCodeGoUsageWindowFromAPI(windows.Weekly)
	}
	if windows.Monthly != nil {
		out.Monthly = openCodeGoUsageWindowFromAPI(windows.Monthly)
	}
	return out, nil
}

func openCodeGoUsageWindowFromAPI(w *openCodeGoUsageAPIWindow) *OpenCodeGoUsageWindow {
	out := &OpenCodeGoUsageWindow{Status: strings.TrimSpace(w.Status), Percent: w.Percent}
	if resetsAt, err := time.Parse(time.RFC3339, strings.TrimSpace(w.ResetsAt)); err == nil {
		out.ResetsAt = resetsAt.UTC()
	}
	return out
}

// probeOpenCodeGoUsage 取一次用量快照。**总是返回快照**（失败时填 Error / HTTPStatus），
// 不负责持久化——持久化由各自持有 accountRepo 的 service 完成。
//
// 只依赖 HTTPUpstream：这样网关服务与账号测试服务都能复用同一份实现，
// 不需要为了一个查询端点去动依赖注入图。
func probeOpenCodeGoUsage(ctx context.Context, upstream HTTPUpstream, account *Account) *OpenCodeGoUsage {
	now := func() time.Time { return time.Now().UTC() }
	if account == nil {
		return nil
	}
	out := &OpenCodeGoUsage{FetchedAt: now()}
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		out.Error = "api key missing"
		return out
	}
	if upstream == nil {
		out.Error = "http upstream unavailable"
		return out
	}

	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy == nil || account.Proxy.ID != *account.ProxyID {
			// 代理身份不一致时不查询：宁可查不到，也不要把 key 从非预期出口发出去。
			out.Error = "proxy identity changed"
			return out
		}
		proxyURL = account.Proxy.URL()
	}

	requestCtx, cancel := context.WithTimeout(WithHTTPUpstreamRedirectsDisabled(ctx), openCodeGoUsageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, openCodeGoUsageURL, nil)
	if err != nil || !isOpenCodeGoUsageURL(req.URL.String()) {
		out.Error = "usage url invalid"
		return out
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", openCodeGoUsageUserAgent)

	resp, err := upstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		out.Error = "request failed"
		return out
	}
	if resp == nil || resp.Body == nil {
		out.Error = "empty response"
		return out
	}
	defer func() { _ = resp.Body.Close() }()
	out.HTTPStatus = resp.StatusCode

	// 重定向已禁用，仍再校验一次最终 URL：任何情况下都不接受凭据被转发到别处。
	if resp.Request != nil && !isOpenCodeGoUsageURL(resp.Request.URL.String()) {
		out.Error = "response host mismatch"
		return out
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		out.Error = "unauthorized"
		return out
	case resp.StatusCode == http.StatusForbidden:
		out.Error = "Go subscription required (403)"
		return out
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		out.Error = "redirect blocked"
		return out
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		out.Error = fmt.Sprintf("http %d", resp.StatusCode)
		return out
	}

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, openCodeGoUsageMaxBody+1))
	if readErr != nil {
		out.Error = "response read failed"
		return out
	}
	if len(body) > openCodeGoUsageMaxBody {
		out.Error = "response too large"
		return out
	}
	parsed, parseErr := parseOpenCodeGoUsageJSON(body)
	if parseErr != nil {
		out.Error = "invalid json"
		return out
	}
	parsed.FetchedAt = now()
	parsed.HTTPStatus = resp.StatusCode
	return parsed
}

// FetchOpenCodeGoUsage 在网关服务上查询并持久化（供自动刷新使用）。
func (s *OpenAIGatewayService) FetchOpenCodeGoUsage(ctx context.Context, account *Account) (*OpenCodeGoUsage, error) {
	if s == nil || !IsOpenCodeGoAccount(account) {
		return nil, ErrOpenCodeGoUsageNotApplicable
	}
	usage := probeOpenCodeGoUsage(ctx, s.httpUpstream, account)
	stored := s.persistOpenCodeGoUsage(account, usage)
	if usage != nil && usage.Error != "" {
		return stored, errors.New(usage.Error)
	}
	return stored, nil
}

// persistOpenCodeGoUsageSnapshot 把快照写进账号 extra（**只含窗口数据与短原因，不含 key**）。
//
// writer 由调用方提供：网关服务与账号测试服务各自持有落库路径，
// 这里只负责序列化、写内存与失败日志。
func persistOpenCodeGoUsageSnapshot(account *Account, usage *OpenCodeGoUsage, writer func(int64, map[string]any)) *OpenCodeGoUsage {
	if account == nil || usage == nil {
		return usage
	}
	raw, err := json.Marshal(usage)
	if err != nil {
		return usage
	}
	encoded := string(raw)
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[openCodeGoUsageExtraKey] = encoded
	if writer != nil && account.ID > 0 {
		writer(account.ID, map[string]any{openCodeGoUsageExtraKey: encoded})
	}
	if usage.Error != "" {
		logger.LegacyPrintf("service.opencode_go_usage",
			"[OpenCodeGoUsage] fetch failed account=%d status=%d reason=%s", account.ID, usage.HTTPStatus, usage.Error)
	}
	return usage
}

// persistOpenCodeGoUsage 是网关服务侧的薄包装。
func (s *OpenAIGatewayService) persistOpenCodeGoUsage(account *Account, usage *OpenCodeGoUsage) *OpenCodeGoUsage {
	if s == nil || s.accountRepo == nil {
		return persistOpenCodeGoUsageSnapshot(account, usage, nil)
	}
	return persistOpenCodeGoUsageSnapshot(account, usage, func(id int64, updates map[string]any) {
		s.persistOpenAICodexTurnStateUpdates(id, updates)
	})
}
