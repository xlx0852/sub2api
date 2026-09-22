package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// 上游签发的会话 cookie：采集与回放。
//
// 为什么需要它（2026-09-22 mitm 全量抓包实证）：
//   - 上游在 codex 端点上下发 Set-Cookie：`__oailb`（Max-Age=3600，OpenAI 侧负载均衡
//     亲和）与 `__cf_bm`（Cloudflare bot management，Expires ≈ 30 分钟后）。
//   - 真实客户端每一轮都携带 `__cf_bm` / `__cflb` / `__oailb` / `_cfuvid`（123/123）。
//   - **亲和性丢失会当场被重新签发**：抓包里 2 次「没带 __oailb」的请求，响应立刻下发
//     `Set-Cookie: __oailb`；而带着它的 116 次响应都不再下发。
//   - 网关此前**一个 cookie 都不发**，也是出站相对真实客户端唯一稳定缺失的头。
//
// 安全边界：只回放**上游签发给同一账号**的 cookie。绝不转发用户端 cookie——
// 那属于用户自己的 ChatGPT 登录态，既跨账号也泄露凭据。
//
// 已知边界：cookie 不单独决定票的形态。抓包里同一个 `__oailb` 值既产出过 54 次 312，
// 也产出过那唯一一条 292，所以回放 cookie 是「保持一致」而非「保证拿到目标池」。

const (
	// openAICodexTurnStateCookieExtra 是账号级 cookie 列表（JSON 数组，见 codexTurnStateCookie）。
	openAICodexTurnStateCookieExtra = "openai_codex_turn_state_cookies"
	// openAICodexTurnStateCookieEgressExtra 是采集时的出口指纹。
	// 出口变了就不回放：cookie 是绑定出口的亲和令牌，跨出口回放只会制造新的不一致。
	openAICodexTurnStateCookieEgressExtra = "openai_codex_turn_state_cookie_egress"
)

// codexTurnStateCookieFallbackTTL 用于上游没给 Max-Age/Expires 的 cookie。
// 观测到的两个 cookie 都带属性（__oailb 带 Max-Age=3600，__cf_bm 带 Expires），
// 这里只是兜底。
const codexTurnStateCookieFallbackTTL = 30 * time.Minute

// codexTurnStateCookieMaxReuse 是回放上限。
// 取与票同量级（票 TTL）的理由：cookie 的意义是与那张票同属一个会话，
// 票过期后单独回放一个更老的 cookie 没有依据。上游若给了更短的 Expires 则按更短的来。
const codexTurnStateCookieMaxReuse = 60 * time.Minute

// codexTurnStateCookie 是单个 cookie 的最小持久化形态。
// 键名压缩过：Extra 会被序列化进账号行，列表里每一项都省一点。
type codexTurnStateCookie struct {
	Name    string `json:"n"`
	Value   string `json:"v"`
	Expires int64  `json:"e,omitempty"` // unix 秒；0 表示上游未给寿命
	SetAt   int64  `json:"t,omitempty"` // unix 秒；无 Expires 时用 fallback TTL 计算
}

// codexTurnStateEgressFingerprint 计算当前出口的指纹。
//
// 只落指纹不落地址：代理 URL 可能带凭据，持久化到账号 extra 里等于把凭据抄一份。
// 粒度取 协议://主机:端口——这正是管理员切换出口时会变的部分。
func codexTurnStateEgressFingerprint(account *Account) string {
	if account == nil {
		return ""
	}
	egress := "direct"
	if account.Proxy != nil {
		proto := strings.TrimSpace(account.Proxy.Protocol)
		if proto == "" {
			proto = "http"
		}
		host := strings.TrimSpace(account.Proxy.Host)
		if host == "" {
			return ""
		}
		egress = proto + "://" + host + ":" + strconv.Itoa(account.Proxy.Port)
	}
	sum := sha256.Sum256([]byte("codex:cookie-egress:v1:" + egress))
	return hex.EncodeToString(sum[:8])
}

func parseCodexTurnStateCookies(raw string) []codexTurnStateCookie {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var list []codexTurnStateCookie
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		// 存坏了就当没有：cookie 是增强项，解析失败不得影响出站请求。
		return nil
	}
	out := list[:0]
	for _, c := range list {
		if strings.TrimSpace(c.Name) == "" || c.Value == "" {
			continue
		}
		out = append(out, c)
	}
	return out
}

func encodeCodexTurnStateCookies(list []codexTurnStateCookie) string {
	if len(list) == 0 {
		return ""
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return ""
	}
	return string(raw)
}

// codexTurnStateCookieExpiry 返回该 cookie 的失效时刻；零值表示无依据。
func codexTurnStateCookieExpiry(c codexTurnStateCookie) time.Time {
	if c.Expires > 0 {
		return time.Unix(c.Expires, 0)
	}
	if c.SetAt > 0 {
		return time.Unix(c.SetAt, 0).Add(codexTurnStateCookieFallbackTTL)
	}
	return time.Time{}
}

// codexTurnStateCookieFresh 判断 cookie 此刻是否还能回放。
func codexTurnStateCookieFresh(c codexTurnStateCookie, now time.Time) bool {
	if exp := codexTurnStateCookieExpiry(c); !exp.IsZero() && !now.Before(exp) {
		return false
	}
	if c.SetAt > 0 && now.Sub(time.Unix(c.SetAt, 0)) >= codexTurnStateCookieMaxReuse {
		return false
	}
	return true
}

// captureCodexTurnStateCookies 从**已被接受**的响应里采集/刷新 cookie。
//
// 只在调用方已确认该轮被接受（拿到可复用票）时调用：被拒或失败的响应不得刷新 cookie，
// 否则一次失败就把上一份可用会话冲掉。
// 同名 cookie 覆盖，列表里没出现过的保持不变（上游本来就只会下发变化的那几个）。
func (s *OpenAIGatewayService) captureCodexTurnStateCookies(account *Account, headers http.Header, now time.Time) {
	if account == nil || account.Type != AccountTypeOAuth || headers == nil {
		return
	}
	// 用 http.Response 借用标准库的 Set-Cookie 解析（含 Max-Age/Expires 语义）。
	set := (&http.Response{Header: headers}).Cookies()
	if len(set) == 0 {
		return
	}
	fingerprint := codexTurnStateEgressFingerprint(account)
	openAICodexTurnStateMu.Lock()
	merged := parseCodexTurnStateCookies(account.GetExtraString(openAICodexTurnStateCookieExtra))
	index := make(map[string]int, len(merged))
	for i, c := range merged {
		index[c.Name] = i
	}
	changed := false
	for _, c := range set {
		if c == nil || strings.TrimSpace(c.Name) == "" {
			continue
		}
		// 删除型 Set-Cookie 有三种写法：Max-Age<=0、空值、Expires 已过。
		// 必须在这三种情况下把已存的同名 cookie 移除，否则网关会继续回放
		// 一个上游已经作废的会话令牌。
		// （注意：删除型 cookie 的值本来就是空的，不能先被空值守卫拦掉。）
		if c.MaxAge < 0 || strings.TrimSpace(c.Value) == "" || (!c.Expires.IsZero() && !now.Before(c.Expires)) {
			if i, ok := index[c.Name]; ok {
				merged = append(merged[:i], merged[i+1:]...)
				index = make(map[string]int, len(merged))
				for j, k := range merged {
					index[k.Name] = j
				}
				changed = true
			}
			continue
		}
		entry := codexTurnStateCookie{Name: c.Name, Value: c.Value, SetAt: now.Unix()}
		if c.MaxAge > 0 {
			entry.Expires = now.Add(time.Duration(c.MaxAge) * time.Second).Unix()
		} else if !c.Expires.IsZero() {
			entry.Expires = c.Expires.Unix()
		}
		if i, ok := index[c.Name]; ok {
			if merged[i] == entry {
				continue
			}
			merged[i] = entry
		} else {
			merged = append(merged, entry)
			index[c.Name] = len(merged) - 1
		}
		changed = true
	}
	if !changed {
		openAICodexTurnStateMu.Unlock()
		return
	}
	account.Extra[openAICodexTurnStateCookieExtra] = encodeCodexTurnStateCookies(merged)
	account.Extra[openAICodexTurnStateCookieEgressExtra] = fingerprint
	count := len(merged)
	openAICodexTurnStateMu.Unlock()
	s.persistOpenAICodexTurnStateUpdates(account.ID, map[string]any{
		openAICodexTurnStateCookieExtra:       account.GetExtraString(openAICodexTurnStateCookieExtra),
		openAICodexTurnStateCookieEgressExtra: fingerprint,
	})
	if count == 0 {
		logger.LegacyPrintf("service.openai_codex_turn_state", "[CodexTurnStateCookie] cleared account=%d", account.ID)
	}
}

// codexTurnStateCookieHeaderValue 返回可注入出站的 Cookie 头值；空串表示不注入。
//
// 不注入的三种情况：没有采集过、采集时的出口与当前不一致、全部已过期。
func codexTurnStateCookieHeaderValue(account *Account, now time.Time) string {
	if account == nil || account.Type != AccountTypeOAuth {
		return ""
	}
	fingerprint := codexTurnStateEgressFingerprint(account)
	if fingerprint == "" {
		return ""
	}
	openAICodexTurnStateMu.Lock()
	defer openAICodexTurnStateMu.Unlock()
	if stored := strings.TrimSpace(account.GetExtraString(openAICodexTurnStateCookieEgressExtra)); stored != fingerprint {
		return ""
	}
	list := parseCodexTurnStateCookies(account.GetExtraString(openAICodexTurnStateCookieExtra))
	if len(list) == 0 {
		return ""
	}
	parts := make([]string, 0, len(list))
	for _, c := range list {
		if !codexTurnStateCookieFresh(c, now) {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

// codexTurnStateCookieCount 返回当前可回放的 cookie 数量（仅供日志，不含值）。
func codexTurnStateCookieCount(account *Account, now time.Time) int {
	if account == nil || account.Type != AccountTypeOAuth {
		return 0
	}
	fingerprint := codexTurnStateEgressFingerprint(account)
	openAICodexTurnStateMu.Lock()
	defer openAICodexTurnStateMu.Unlock()
	if stored := strings.TrimSpace(account.GetExtraString(openAICodexTurnStateCookieEgressExtra)); stored != fingerprint {
		return 0
	}
	count := 0
	for _, c := range parseCodexTurnStateCookies(account.GetExtraString(openAICodexTurnStateCookieExtra)) {
		if codexTurnStateCookieFresh(c, now) {
			count++
		}
	}
	return count
}
