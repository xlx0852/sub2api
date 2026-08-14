package config

import "sync/atomic"

type SecurityConfig struct {
	URLAllowlist                 URLAllowlistConfig   `mapstructure:"url_allowlist"`
	ResponseHeaders              ResponseHeaderConfig `mapstructure:"response_headers"`
	CSP                          CSPConfig            `mapstructure:"csp"`
	ProxyFallback                ProxyFallbackConfig  `mapstructure:"proxy_fallback"`
	ProxyProbe                   ProxyProbeConfig     `mapstructure:"proxy_probe"`
	TrustForwardedIPForAPIKeyACL bool                 `mapstructure:"trust_forwarded_ip_for_api_key_acl"`
	// RateLimitFailClosed 为 true 时，限流/RPM/配额检查在依赖（Redis/DB）故障时拒绝请求（fail-closed），
	// 而非默认放行（fail-open）。默认 false 以优先可用性；对正确性敏感的部署可开启。
	RateLimitFailClosed              bool         `mapstructure:"rate_limit_fail_closed"`
	trustForwardedIPForAPIKeyACLLive *atomic.Bool `mapstructure:"-"`
}

func (c *Config) TrustForwardedIPForAPIKeyACL() bool {
	if c == nil {
		return false
	}
	live := c.Security.trustForwardedIPForAPIKeyACLLive
	if live == nil {
		return c.Security.TrustForwardedIPForAPIKeyACL
	}
	return live.Load()
}

func (c *Config) SetTrustForwardedIPForAPIKeyACL(enabled bool) {
	if c == nil {
		return
	}
	c.Security.TrustForwardedIPForAPIKeyACL = enabled
	if c.Security.trustForwardedIPForAPIKeyACLLive == nil {
		c.Security.trustForwardedIPForAPIKeyACLLive = &atomic.Bool{}
	}
	c.Security.trustForwardedIPForAPIKeyACLLive.Store(enabled)
}

type URLAllowlistConfig struct {
	Enabled           bool     `mapstructure:"enabled"`
	UpstreamHosts     []string `mapstructure:"upstream_hosts"`
	PricingHosts      []string `mapstructure:"pricing_hosts"`
	CRSHosts          []string `mapstructure:"crs_hosts"`
	AllowPrivateHosts bool     `mapstructure:"allow_private_hosts"`
	// 关闭 URL 白名单校验时，是否允许 http URL（默认只允许 https）
	AllowInsecureHTTP bool `mapstructure:"allow_insecure_http"`
}

type ResponseHeaderConfig struct {
	Enabled           bool     `mapstructure:"enabled"`
	AdditionalAllowed []string `mapstructure:"additional_allowed"`
	ForceRemove       []string `mapstructure:"force_remove"`
}

type CSPConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Policy  string `mapstructure:"policy"`
}

type ProxyFallbackConfig struct {
	// AllowDirectOnError 当辅助服务的代理初始化失败时是否允许回退直连。
	// 仅影响以下非 AI 账号连接的辅助服务：
	//   - GitHub Release 更新检查
	//   - 定价数据拉取
	// 不影响 AI 账号网关连接（Claude/OpenAI/Gemini/Antigravity），
	// 这些关键路径的代理失败始终返回错误，不会回退直连。
	// 默认 false：避免因代理配置错误导致服务器真实 IP 泄露。
	AllowDirectOnError bool `mapstructure:"allow_direct_on_error"`
}

type ProxyProbeConfig struct {
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"` // 已禁用：禁止跳过 TLS 证书验证
}
