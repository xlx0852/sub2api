package service

import (
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// validateUpstreamBaseURLWithConfig 校验用户可配置的上游 base_url。
//
// 安全策略（SSRF 防护）：
//   - 始终按 scheme 与私网策略校验。即使未启用 allowlist，也会阻断 localhost / 私网 /
//     环回 / 链路本地等字面量 IP，除非显式开启 security.url_allowlist.allow_private_hosts。
//     这修复了此前「allowlist 关闭时退化为纯格式校验、不拦私网」导致的 SSRF 面。
//   - 启用 allowlist 时额外强制 host 命中 upstream_hosts 白名单。
//
// 注意：DNS Rebinding 的解析后 IP 校验需在实际发起请求时执行
// （见 urlvalidator.ValidateResolvedIP），此处仅做请求前的静态校验。
func validateUpstreamBaseURLWithConfig(cfg *config.Config, raw string) (string, error) {
	if cfg == nil {
		return "", errors.New("config is not available")
	}
	opts := urlvalidator.ValidationOptions{
		AllowPrivate: cfg.Security.URLAllowlist.AllowPrivateHosts,
	}
	if cfg.Security.URLAllowlist.Enabled {
		opts.AllowedHosts = cfg.Security.URLAllowlist.UpstreamHosts
		opts.RequireAllowlist = true
	}
	normalized, err := urlvalidator.ValidateHTTPURL(raw, cfg.Security.URLAllowlist.AllowInsecureHTTP, opts)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	return normalized, nil
}
