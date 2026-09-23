package service

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// OpenCode Ai 上游的两个强制要求（对齐上游 v0.2.8 的 openai_opencode_session.go）。
//
// 1) **每个推理请求必须带 `x-opencode-session`**。自 2026-09-05 起缺失会被拒：
//
//	400 {"type":"error","error":{"type":"MissingSessionID",
//	     "message":"Request is missing x-opencode-session and cannot be routed efficiently."}}
//
// 2) **User-Agent 必须是 opencode 客户端形态**。opencode.ai 前置 Cloudflare 按 UA
// 做 bot 拦截，编程库/其它客户端 UA 会被 CF error code 1010 拒成 403；而这个 403
// 会被本仓计成**账号凭据类故障**，健康账号因此可能被自动禁用。
//
// 判定与 session 头同源：**任意账号**只要目标主机是官方 opencode.ai 就要遵守，
// 不要求先把账号做成 opencode 平台。这正是本仓当前接法（openai + apikey +
// credentials.base_url 指向官方基址）需要它的原因。
const (
	openCodeSessionHeader     = "X-OpenCode-Session"
	openCodeUpstreamUserAgent = "opencode/1.0.0"
)

// isOfficialOpenCodeHost 判断目标 URL 是否为官方 opencode.ai（强制 https）。
// 只比主机名，不做前缀匹配——前缀匹配会把 opencode.ai.evil.example 也算进来。
func isOfficialOpenCodeHost(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") && strings.EqualFold(parsed.Hostname(), "opencode.ai")
}

// openCodeRequiresSessionHeader 判断本次出站是否需要带 session 头。
// 与上游一致：opencode 计划账号，或（任意账号）目标为官方主机。
func openCodeRequiresSessionHeader(account *Account, targetURL string) bool {
	if account != nil && isOpenCodeGoPlanAccount(account) {
		return true
	}
	return isOfficialOpenCodeHost(targetURL)
}

// isOpenCodeGoPlanAccount 判断账号是否就是 OpenCode Go 账号
// （base URL 精确指向官方 Go 基址的 API-key 账号）。
func isOpenCodeGoPlanAccount(account *Account) bool {
	return IsOpenCodeGoAccount(account)
}

// sanitizeOpenCodeSessionID 校验客户端自带的 session 值是否可用。
// 与上游同款策略：只接受短、可打印、无分隔歧义的取值，避免把任意头内容反射给上游。
func sanitizeOpenCodeSessionID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 128 {
		return ""
	}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == ':':
		default:
			return ""
		}
	}
	return raw
}

// openCodeSessionSeedForRequest 从入站头/体里挑一个**稳定**的会话标识作种子。
// 稳定性是关键：同一会话每一轮必须得到同一个 session 值，否则上游 prompt cache 持续 miss。
func openCodeSessionSeedForRequest(headers http.Header, body []byte) string {
	if headers != nil {
		for _, key := range []string{openCodeSessionHeader, "session-id", "thread-id", "x-client-request-id"} {
			if v := sanitizeOpenCodeSessionID(headers.Get(key)); v != "" {
				return v
			}
		}
	}
	if len(body) > 0 {
		if v := sanitizeOpenCodeSessionID(gjson.GetBytes(body, "prompt_cache_key").String()); v != "" {
			return v
		}
	}
	return ""
}

// openCodeGoUsesChatCompletions 返回 OpenCode Go 模型是否应走 Chat Completions。
//
// 对齐上游 v0.2.8 的 DefaultOpenCodeGoProtocolRules：gpt/grok/muse-spark 明确走
// Responses，其余未命中规则的 Go 模型默认走 Chat Completions——包括 DeepSeek。
// 当前本仓的最小 OpenCode Go 接入不引入 Anthropic Messages 多协议层，
// 因此 minimax/qwen 也会走 Chat；这些模型若需要原生 Anthropic 协议，属于后续扩展。
func openCodeGoUsesChatCompletions(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"opencode-go/", "opencode_go/", "opencode/"} {
		model = strings.TrimPrefix(model, prefix)
	}
	for _, prefix := range []string{"gpt-", "grok-", "muse-spark-"} {
		if strings.HasPrefix(model, prefix) {
			return false
		}
	}
	return true
}

// openCodeSessionValue 决定 session 值。
//
// **合法值原样使用，不做哈希派生**（与上游一致）：session 值是客户端会话的名称，
// 上游按它做 prompt cache 路由。把它换成派生值等于凭空造一个客户端不认识的新会话，
// 反而丢失连续性；而每轮换一个随机值会持续 miss（上游注释明确警告过这一点）。
// 只有拿不到任何稳定标识（例如后台探测没有客户端会话）时才退一次性 UUID。
func openCodeSessionValue(seed string) string {
	if seed = sanitizeOpenCodeSessionID(seed); seed != "" {
		return seed
	}
	return uuid.NewString()
}

// applyOpenCodeUpstreamHeaders 在出站请求上落实上述两个要求。
//
// 必须先于任何"按客户端镜像身份"的改写、或在之后显式覆盖：发往 opencode.ai 时
// UA 只能是 opencode 客户端形态，否则会被 Cloudflare 拦成 403 并牵连账号健康。
// 因此这里对 UA 是**强制覆盖**，并清掉同名 session 头避免重复。
func applyOpenCodeUpstreamHeaders(headers http.Header, account *Account, targetURL, sessionSeed string) {
	if headers == nil || !openCodeRequiresSessionHeader(account, targetURL) {
		return
	}
	// 客户端自带且合法时优先沿用它的值（保留客户端自己的会话语义）；
	// 否则用调用方给的稳定种子派生；再没有才退一次性 UUID。
	if clientValue := sanitizeOpenCodeSessionID(headers.Get(openCodeSessionHeader)); clientValue != "" {
		sessionSeed = clientValue
	}
	// 先清同名头再去重写入：透传白名单可能已经放进了客户端原值。
	for key := range headers {
		if strings.EqualFold(key, openCodeSessionHeader) {
			headers.Del(key)
		}
	}
	headers.Set(openCodeSessionHeader, openCodeSessionValue(sessionSeed))
	headers.Set("user-agent", openCodeUpstreamUserAgent)
}
