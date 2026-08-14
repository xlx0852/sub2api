package service

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/google/uuid"
)

// codexUpstreamMinVersion 上游 /backend-api/codex 接受的最低 version 头：
// 若请求携带 version 且低于该值，上游直接 404（issue #3901，2026-07 实测）。
const codexUpstreamMinVersion = "0.144.0"

// codexClientVersionMaxLen 官方版本号均为短 ASCII 串，远低于此上限。
const codexClientVersionMaxLen = 64

// codexClientVersionPattern 允许 0.146.0 与 0.147.0-alpha.4 两类官方形态。
var codexClientVersionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){1,3}(-[0-9A-Za-z.]+)?$`)

// NormalizeCodexClientVersion 校验并归一化 Codex 客户端版本号，非法值返回空串。
// 该值会被拼进出站 User-Agent 与 version 头，必须拒绝任意字节，避免管理员误填或
// 自动同步拿到异常值时把不可控内容透给上游。
func NormalizeCodexClientVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || len(version) > codexClientVersionMaxLen || !codexClientVersionPattern.MatchString(version) {
		return ""
	}
	return version
}

// buildCodexCLIUserAgent 按版本号拼出规范 Codex CLI User-Agent。
// UA 形态只在 codexCLIUserAgentSuffix 一处定义，避免多处拼装漂移。
func buildCodexCLIUserAgent(version string) string {
	if version = NormalizeCodexClientVersion(version); version == "" {
		return codexCLIUserAgent
	}
	return openai.CodexCLIOriginator + "/" + version + codexCLIUserAgentSuffix
}

// CodexIdentity 持有 Codex 出站身份的构造期快照：强制统一开关与规范 UA 解析器。
// 由 NewOpenAIGatewayService 从 Config / SettingService 构造并注入调用方，
// 不再经包级 Set* 发布进程级状态。
//
// 零值 / nil 接收者与历史默认一致：强制统一开启、规范 UA 回退编译期常量。
type CodexIdentity struct {
	enabled  bool
	resolver func() string
}

func newCodexIdentity(enforcementEnabled bool, resolver func() string) *CodexIdentity {
	return &CodexIdentity{enabled: enforcementEnabled, resolver: resolver}
}

func newCodexIdentityFromConfig(cfg *config.Config, settingService *SettingService) *CodexIdentity {
	enabled := true
	if cfg != nil {
		// 配置取反义，零值即「强制统一出口开启」。
		enabled = !cfg.Gateway.DisableCodexIdentityEnforcement
	}
	var resolver func() string
	if settingService != nil {
		// enforce 收口点拿不到 ctx；解析器内部自带 60s TTL 缓存，热路径不触库。
		resolver = func() string {
			return settingService.GetOpenAICodexCanonicalUserAgent(context.Background())
		}
	}
	return newCodexIdentity(enabled, resolver)
}

func (c *CodexIdentity) enforcementEnabled() bool {
	if c == nil {
		return true
	}
	return c.enabled
}

// canonicalUserAgent 返回出站规范 User-Agent。
func (c *CodexIdentity) canonicalUserAgent() string {
	if c != nil && c.resolver != nil {
		if ua := strings.TrimSpace(c.resolver()); ua != "" {
			return ua
		}
	}
	return codexCLIUserAgent
}

// codexOutboundIdentity 出站身份三元组，三者必须同源自洽：
// originator 与 User-Agent 首段配套（否则上游 404，issue #3901），
// version 等于 User-Agent 的版本段且不低于上游门槛。
type codexOutboundIdentity struct {
	userAgent  string
	originator string
	version    string
}

// resolveOutboundIdentity 由候选 User-Agent 推导自洽的出站身份。
// candidateUA 为空时使用规范 User-Agent；推导不出官方身份时整体回退为规范 CLI 身份。
//
// 候选 UA（面板 / 账号级的管理员显式配置）只贡献客户端名与 OS / 架构 / 终端指纹，
// 其自带的版本段一律用当前生效版本重建：一条填写于某个历史版本的 UA 否则会把出站身份
// 永久钉死在陈旧版本上，绕过版本自动同步，落回上游优先降载的那一侧。
// 需要固定版本请填「Codex 客户端版本号」并关闭自动同步。
func (c *CodexIdentity) resolveOutboundIdentity(candidateUA string) codexOutboundIdentity {
	canonical := c.canonicalUserAgent()
	ua := strings.TrimSpace(candidateUA)
	if ua == "" {
		ua = canonical
	}
	originator, pairedUA, ok := openai.PairCodexClientIdentity(ua)
	if !ok {
		if originator, pairedUA, ok = openai.PairCodexClientIdentity(canonical); !ok {
			originator, pairedUA = openai.CodexCLIOriginator, codexCLIUserAgent
		}
	}
	// 生效版本只有一个来源：规范身份（面板版本号 → 自动同步值 → 内置常量，见
	// SettingService.GetOpenAICodexClientVersion）。UA 与 version 头由此同源派生。
	version := codexClientVersionFromUA(canonical)
	if rebuilt := openai.SetCodexUserAgentVersion(pairedUA, version); rebuilt != "" {
		pairedUA = rebuilt
	}
	return codexOutboundIdentity{userAgent: pairedUA, originator: originator, version: version}
}

// codexClientVersionFromUA 取 UA 的版本段作为生效版本；
// 非法或低于上游门槛（低于则上游 404，issue #3901）时回退编译期常量。
func codexClientVersionFromUA(ua string) string {
	version := NormalizeCodexClientVersion(openai.CodexUserAgentVersion(ua))
	if version == "" || CompareVersions(version, codexUpstreamMinVersion) < 0 {
		return codexCLIVersion
	}
	return version
}

// ensureHeaders 补齐 OAuth（ChatGPT 内部接口）出站请求所需的 Codex 身份头。
// 已有 User-Agent 与 version 保持不变，交给紧随其后的 enforceHeaders 收口。
func (c *CodexIdentity) ensureHeaders(h http.Header) {
	if h == nil {
		return
	}
	identity := c.resolveOutboundIdentity("")
	if strings.TrimSpace(h.Get("user-agent")) == "" {
		h.Set("user-agent", identity.userAgent)
	}
	if strings.TrimSpace(h.Get("originator")) == "" {
		h.Set("originator", identity.originator)
	}
	if strings.TrimSpace(h.Get("version")) == "" {
		h.Set("version", identity.version)
	}
	h.Set("OpenAI-Beta", "responses=experimental")
}

// applyProbeHeaders 为合成探测请求补齐 Codex 身份和引擎指纹。
func (c *CodexIdentity) applyProbeHeaders(h http.Header) {
	if h == nil {
		return
	}
	c.ensureHeaders(h)
	h.Set("X-Codex-Window-ID", uuid.NewString())
}

// enforceHeaders 收口 OAuth（ChatGPT 内部接口）出站请求的客户端身份头。
// 见 enforceHeadersWithUA；无账号级自定义 User-Agent 时使用本方法。
func (c *CodexIdentity) enforceHeaders(h http.Header) {
	c.enforceHeadersWithUA(h, "")
}

// enforceHeadersWithUA 强制统一 OAuth 出站身份：User-Agent / originator / version
// 一律改写为网关的规范身份，客户端自报身份不参与构造。上游在容量紧张时按客户端身份分优先级
// 降载，被降载的请求会拿到 HTTP 200 + 流内 server_is_overloaded；统一出口可确保没有请求带着
// 第三方或陈旧身份出站，也天然满足 originator 与 UA 首段配套的上游校验（issue #3901）。
//
// overrideUA 是账号级自定义 User-Agent：管理员的显式配置仍然生效，但只贡献客户端名与
// OS / 架构 / 终端指纹——版本段与 originator 都由规范身份重建，不允许出现自相矛盾或陈旧的身份。
//
// 强制统一被 gateway.disable_codex_identity_enforcement 关闭时，退回「按最终 User-Agent 配对
// originator + version 门槛校正」的收口语义，供上游策略变动时回滚。
//
// 仅对携带 originator 的请求生效：compat 桥接等非 ChatGPT 内部接口路径会显式删除 originator，
// 不应被补回。需要从缺失身份头恢复的调用方应先调用 ensureHeaders。
// 必须在所有 User-Agent 改写之后调用。
func (c *CodexIdentity) enforceHeadersWithUA(h http.Header, overrideUA string) {
	if h == nil || h.Get("originator") == "" {
		return
	}
	if !c.enforcementEnabled() {
		c.pairIdentityHeaders(h)
		return
	}
	identity := c.resolveOutboundIdentity(overrideUA)
	h.Set("user-agent", identity.userAgent)
	h.Set("originator", identity.originator)
	h.Set("version", identity.version)
}

// pairIdentityHeaders 是关闭强制统一后的兜底收口：保留客户端真实身份，
// 仅保证 originator 与最终 User-Agent 首段配套、version 不低于上游门槛（issue #3901）。
func (c *CodexIdentity) pairIdentityHeaders(h http.Header) {
	originator, pairedUA, ok := openai.PairCodexClientIdentity(h.Get("user-agent"))
	if !ok {
		identity := c.resolveOutboundIdentity("")
		originator, pairedUA = identity.originator, identity.userAgent
		h.Set("version", identity.version)
	}
	h.Set("user-agent", pairedUA)
	h.Set("originator", originator)
	if v := strings.TrimSpace(h.Get("version")); v != "" && CompareVersions(v, codexUpstreamMinVersion) < 0 {
		h.Set("version", codexCLIVersion)
	}
}

// 包级包装保留给默认身份用例（nil 接收者 = 强制统一开启 + 编译期规范 UA）。
// 生产路径必须走注入的 *CodexIdentity，否则会丢掉面板版本号与配置开关。
func ensureCodexIdentityHeaders(h http.Header) {
	(*CodexIdentity)(nil).ensureHeaders(h)
}

func applyOpenAICodexProbeHeaders(h http.Header) {
	(*CodexIdentity)(nil).applyProbeHeaders(h)
}

func enforceCodexIdentityHeaders(h http.Header) {
	(*CodexIdentity)(nil).enforceHeaders(h)
}

func enforceCodexIdentityHeadersWithUA(h http.Header, overrideUA string) {
	(*CodexIdentity)(nil).enforceHeadersWithUA(h, overrideUA)
}
