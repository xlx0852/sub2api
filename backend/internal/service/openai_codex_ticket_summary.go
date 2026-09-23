package service

import (
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// BuildCodexTicketSummary 汇总账号的打票状态，供管理后台展示。
//
// **只输出派生信息**：票长（形态）、票龄、是否仍在回放窗口、池位、cookie 数量与出口指纹。
// 票本体（会话令牌）与 cookie 值绝不出现在返回值里——本仓对 credentials 的既有约定是
// 不下发前端、只给存在性，票与 cookie 同属会话凭据，遵守同一条规则。
//
// 逻辑放在 service 而不是 dto：回放窗口（TTL）、cookie 新鲜期、池槽位键名与
// 兼容单值兜底的口径都在本包内，放到别处只能靠复制粘贴，迟早与真实行为漂移。
func BuildCodexTicketSummary(account *Account, now time.Time) *domain.CodexTicketSummary {
	if account == nil || account.Type != AccountTypeOAuth || account.Extra == nil {
		return nil
	}
	ttl := openAICodexTurnStateDefaultTTL
	rotationEnabled, _ := GetCodexOverloadRotationOverride(account)
	out := &domain.CodexTicketSummary{
		ReplayTTLSeconds:        int64(ttl.Seconds()),
		OverloadRotationEnabled: rotationEnabled,
	}

	openAICodexTurnStateMu.Lock()
	pools := make([]domain.CodexTicketPool, 0, 6)
	for k, raw := range account.Extra {
		if !strings.HasPrefix(k, openAICodexTurnStateExtraPoolPrefix) {
			continue
		}
		pool := strings.TrimPrefix(k, openAICodexTurnStateExtraPoolPrefix)
		if pool == "" || strings.HasSuffix(pool, "_model") || strings.HasSuffix(pool, "_captured_at") {
			continue
		}
		// 空值不是"没有这个槽位"，而是"票被清理了、等待重采"：
		// 失效清理（过载/连续 5xx/312）把值写成 null 但**保留键**。
		// 两者必须区分，否则面板会把"刚被清理"误报成"从未打过票"。
		token, _ := raw.(string)
		token = strings.TrimSpace(token)
		_, modelKey, capturedKey := openAICodexTurnStatePoolKeys(pool)
		model, _ := account.Extra[modelKey].(string)
		captured := parseOpenAICodexTurnStateTimeValue(account.Extra[capturedKey])
		entry := domain.CodexTicketPool{
			Pool:  pool,
			Model: strings.TrimSpace(model),
			// Shape=0 表示该槽位当前没有有效票（已被清理/尚未采到）。
			Shape:      len(token),
			AgeSeconds: 0,
			Fresh:      false,
		}
		if token == "" {
			pools = append(pools, entry)
			continue
		}
		if !captured.IsZero() && !captured.After(now) {
			age := now.Sub(captured)
			entry.AgeSeconds = int64(age.Seconds())
			entry.Fresh = age < ttl
			entry.CapturedAt = captured.UTC().Format(time.RFC3339)
		}
		pools = append(pools, entry)
	}
	// 兼容单值槽位只在没有任何池票时兜底，避免同一张票被展示两次。
	if len(pools) == 0 {
		if token, _ := account.Extra[openAICodexTurnStateExtraToken].(string); strings.TrimSpace(token) != "" {
			captured := parseOpenAICodexTurnStateTimeValue(account.Extra[openAICodexTurnStateExtraCapturedAt])
			model, _ := account.Extra[openAICodexTurnStateExtraModel].(string)
			entry := domain.CodexTicketPool{
				Pool:  "(legacy)",
				Model: strings.TrimSpace(model),
				Shape: len(strings.TrimSpace(token)),
			}
			if !captured.IsZero() && !captured.After(now) {
				age := now.Sub(captured)
				entry.AgeSeconds = int64(age.Seconds())
				entry.Fresh = age < ttl
				entry.CapturedAt = captured.UTC().Format(time.RFC3339)
			}
			pools = append(pools, entry)
		}
	}

	// cookies：只统计数量与新鲜度。
	cookieList := parseCodexTurnStateCookies(account.GetExtraString(openAICodexTurnStateCookieExtra))
	egress := strings.TrimSpace(account.GetExtraString(openAICodexTurnStateCookieEgressExtra))
	var newest int64
	fresh := 0
	for _, c := range cookieList {
		if !codexTurnStateCookieFresh(c, now) {
			continue
		}
		fresh++
		if c.SetAt > newest {
			newest = c.SetAt
		}
	}
	openAICodexTurnStateMu.Unlock()

	if fresh > 0 {
		entry := &domain.CodexTicketCookies{Count: fresh, Fresh: true, Egress: egress}
		if newest > 0 {
			entry.AgeSeconds = int64(now.Sub(time.Unix(newest, 0)).Seconds())
		}
		out.Cookies = entry
	} else if len(cookieList) > 0 {
		// 有记录但全部过期：让面板能显示"已过期"而不是"从没采到"。
		out.Cookies = &domain.CodexTicketCookies{Count: 0, Fresh: false, Egress: egress}
	}

	// 池按名称稳定排序，避免每次刷新顺序跳动。
	sort.Slice(pools, func(i, j int) bool { return pools[i].Pool < pools[j].Pool })
	out.Pools = pools
	if len(out.Pools) == 0 && out.Cookies == nil && !out.OverloadRotationEnabled {
		return nil
	}
	return out
}

// RedactCodexTicketExtra 返回 extra 的副本，剥离票与 cookie 的**值**。
//
// 为什么必须剥：账号列表/详情接口此前是 `Extra: a.Extra` 原样下发，而 extra 里
// 现在既有 `openai_codex_turn_state`（上游会话令牌）也有
// `openai_codex_turn_state_cookies`（会话 cookie 值）。浏览器端只需要看状态，
// 不需要令牌本体；凭据一旦进了前端，就会出现在截图、日志、浏览器存储里。
// 删除后的信息由 domain.CodexTicketSummary 以派生形式提供。
func RedactCodexTicketExtra(in map[string]any) map[string]any {
	if len(in) == 0 {
		return in
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if k == openAICodexTurnStateExtraToken || strings.HasPrefix(k, openAICodexTurnStateExtraPoolPrefix) ||
			k == openAICodexTurnStateCookieExtra || k == openAICodexTurnStateCookieEgressExtra {
			continue
		}
		out[k] = v
	}
	return out
}
