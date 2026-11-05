package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// cachedVersionBounds 缓存 Claude Code 版本号上下限（进程内缓存，60s TTL）
type cachedVersionBounds struct {
	min       string // 空字符串 = 不检查
	max       string // 空字符串 = 不检查
	expiresAt int64  // unix nano
}

// versionBoundsCacheTTL 缓存有效期
const versionBoundsCacheTTL = 60 * time.Second

// versionBoundsErrorTTL DB 错误时的短缓存，快速重试
const versionBoundsErrorTTL = 5 * time.Second

// versionBoundsDBTimeout singleflight 内 DB 查询超时，独立于请求 context
const versionBoundsDBTimeout = 5 * time.Second

// cachedBackendMode Backend Mode cache (in-process, 60s TTL)
type cachedBackendMode struct {
	value     bool
	expiresAt int64 // unix nano
}

const backendModeCacheTTL = 60 * time.Second
const backendModeErrorTTL = 5 * time.Second
const backendModeDBTimeout = 5 * time.Second

// cachedGatewayForwardingSettings 缓存网关转发行为设置（进程内缓存，60s TTL）
type cachedGatewayForwardingSettings struct {
	fingerprintUnification       bool
	metadataPassthrough          bool
	cchSigning                   bool
	anthropicCacheTTL1hInjection bool
	rewriteMessageCacheControl   bool
	clientDatelineNormalization  bool
	expiresAt                    int64 // unix nano
}

const gatewayForwardingCacheTTL = 60 * time.Second
const gatewayForwardingErrorTTL = 5 * time.Second
const gatewayForwardingDBTimeout = 5 * time.Second

// cachedAntigravityUserAgentVersion 缓存 Antigravity UA 版本号（进程内缓存，60s TTL）
type cachedAntigravityUserAgentVersion struct {
	version   string
	expiresAt int64 // unix nano
}

const antigravityUserAgentVersionCacheTTL = 60 * time.Second
const antigravityUserAgentVersionErrorTTL = 5 * time.Second
const antigravityUserAgentVersionDBTimeout = 5 * time.Second

// DefaultOpenAICodexUserAgent OpenAI Codex 默认 User-Agent（用于规避 Cloudflare 对浏览器 UA 的质询）。
// 取官方 CLI 身份而非 TUI 身份：上游在容量紧张时按客户端身份分优先级降载，TUI 身份被观测到
// 优先命中降载，而该默认值是浏览器 UA 兜底路径上最主要的身份来源。版本段随 codexCLIVersion
// 一起更新——陈旧版本同样会被优先降载。
const DefaultOpenAICodexUserAgent = codexCLIUserAgent

// cachedOpenAICodexUserAgent 缓存 OpenAI Codex UA（进程内缓存，60s TTL）
type cachedOpenAICodexUserAgent struct {
	value     string
	expiresAt int64 // unix nano
}

// cachedOpenAICodexClientVersion 缓存出站 Codex 客户端版本号（进程内缓存，60s TTL）
type cachedOpenAICodexClientVersion struct {
	version   string
	expiresAt int64 // unix nano
}

const openAICodexClientVersionCacheTTL = 60 * time.Second
const openAICodexClientVersionErrorTTL = 5 * time.Second
const openAICodexClientVersionDBTimeout = 5 * time.Second

// openAICodexClientVersionSFKey singleflight 键。
const openAICodexClientVersionSFKey = "openai_codex_client_version"

const openAICodexUserAgentCacheTTL = 60 * time.Second
const openAICodexUserAgentErrorTTL = 5 * time.Second
const openAICodexUserAgentDBTimeout = 5 * time.Second

// cachedCyberSessionBlockRuntime cyber 会话屏蔽开关+TTL 进程内缓存（60s TTL）。
// GetCyberSessionBlockRuntime 在网关请求热路径上被调用，避免每次访问 DB。
type cachedCyberSessionBlockRuntime struct {
	enabled   bool
	ttl       time.Duration
	expiresAt int64 // unix nano
}

const cyberSessionBlockRuntimeCacheTTL = 60 * time.Second
const cyberSessionBlockRuntimeErrorTTL = 5 * time.Second
const cyberSessionBlockRuntimeDBTimeout = 5 * time.Second

// GetCyberSessionBlockRuntime 返回 (开关, TTL)，进程内缓存 ~60s，
// 供网关热路径读取时避免 DB 往返。
// 两个 setting key 在单次 singleflight 里一起读取，减少 DB 往返。
// 默认值：开关 false，TTL 1h（与粘性会话对齐）。
func (s *SettingService) GetCyberSessionBlockRuntime(ctx context.Context) (bool, time.Duration) {
	if cached, ok := s.cyberSessionBlockRuntimeCache.Load().(*cachedCyberSessionBlockRuntime); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.enabled, cached.ttl
		}
	}
	result, _, _ := s.cyberSessionBlockRuntimeSF.Do("cyber_session_block_runtime", func() (any, error) {
		if cached, ok := s.cyberSessionBlockRuntimeCache.Load().(*cachedCyberSessionBlockRuntime); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached, nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cyberSessionBlockRuntimeDBTimeout)
		defer cancel()

		enabledVal, enabledErr := s.settingRepo.GetValue(dbCtx, SettingKeyCyberSessionBlockEnabled)
		ttlVal, ttlErr := s.settingRepo.GetValue(dbCtx, SettingKeyCyberSessionBlockTTLSeconds)

		if enabledErr != nil && !errors.Is(enabledErr, ErrSettingNotFound) {
			slog.Warn("failed to get cyber_session_block_enabled setting", "error", enabledErr)
			entry := &cachedCyberSessionBlockRuntime{
				enabled:   false,
				ttl:       time.Hour,
				expiresAt: time.Now().Add(cyberSessionBlockRuntimeErrorTTL).UnixNano(),
			}
			s.cyberSessionBlockRuntimeCache.Store(entry)
			return entry, nil
		}

		enabled := enabledErr == nil && strings.TrimSpace(enabledVal) == "true"

		ttl := time.Hour
		if ttlErr == nil {
			if n, perr := strconv.Atoi(strings.TrimSpace(ttlVal)); perr == nil && n > 0 {
				ttl = time.Duration(n) * time.Second
			}
		}

		entry := &cachedCyberSessionBlockRuntime{
			enabled:   enabled,
			ttl:       ttl,
			expiresAt: time.Now().Add(cyberSessionBlockRuntimeCacheTTL).UnixNano(),
		}
		s.cyberSessionBlockRuntimeCache.Store(entry)
		return entry, nil
	})
	if entry, ok := result.(*cachedCyberSessionBlockRuntime); ok && entry != nil {
		return entry.enabled, entry.ttl
	}
	return false, time.Hour
}

// GetAntigravityUserAgentVersion 返回 Antigravity 上游请求使用的版本号。
// 后台设置优先；为空、缺失或非法时回退到 ANTIGRAVITY_USER_AGENT_VERSION / 内置默认值。
func (s *SettingService) GetAntigravityUserAgentVersion(ctx context.Context) string {
	fallback := antigravity.GetDefaultUserAgentVersion()
	if s == nil || s.settingRepo == nil {
		return fallback
	}
	if cached, ok := s.antigravityUAVersionCache.Load().(*cachedAntigravityUserAgentVersion); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.version
		}
	}

	result, _, _ := s.antigravityUAVersionSF.Do("antigravity_user_agent_version", func() (any, error) {
		if cached, ok := s.antigravityUAVersionCache.Load().(*cachedAntigravityUserAgentVersion); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.version, nil
			}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), antigravityUserAgentVersionDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeyAntigravityUserAgentVersion)
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			slog.Warn("failed to get antigravity user agent version setting", "error", err)
			s.antigravityUAVersionCache.Store(&cachedAntigravityUserAgentVersion{
				version:   fallback,
				expiresAt: time.Now().Add(antigravityUserAgentVersionErrorTTL).UnixNano(),
			})
			return fallback, nil
		}
		version := antigravity.NormalizeUserAgentVersion(value)
		if version == "" {
			version = fallback
		}
		s.antigravityUAVersionCache.Store(&cachedAntigravityUserAgentVersion{
			version:   version,
			expiresAt: time.Now().Add(antigravityUserAgentVersionCacheTTL).UnixNano(),
		})
		return version, nil
	})
	if version, ok := result.(string); ok && version != "" {
		return version
	}
	return fallback
}

// GetOpenAICodexUserAgent 返回 OpenAI Codex 上游请求使用的 User-Agent。
// 后台设置优先；为空时回退到内置默认值。
func (s *SettingService) GetOpenAICodexUserAgent(ctx context.Context) string {
	fallback := DefaultOpenAICodexUserAgent
	if s == nil || s.settingRepo == nil {
		return fallback
	}
	if cached, ok := s.openAICodexUACache.Load().(*cachedOpenAICodexUserAgent); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
		}
	}

	result, _, _ := s.openAICodexUASF.Do("openai_codex_user_agent", func() (any, error) {
		if cached, ok := s.openAICodexUACache.Load().(*cachedOpenAICodexUserAgent); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
			}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAICodexUserAgentDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeyOpenAICodexUserAgent)
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			slog.Warn("failed to get openai codex user agent setting", "error", err)
			s.openAICodexUACache.Store(&cachedOpenAICodexUserAgent{
				value:     fallback,
				expiresAt: time.Now().Add(openAICodexUserAgentErrorTTL).UnixNano(),
			})
			return fallback, nil
		}
		ua := strings.TrimSpace(value)
		if ua == "" {
			ua = fallback
		}
		s.openAICodexUACache.Store(&cachedOpenAICodexUserAgent{
			value:     ua,
			expiresAt: time.Now().Add(openAICodexUserAgentCacheTTL).UnixNano(),
		})
		return ua, nil
	})
	if ua, ok := result.(string); ok && ua != "" {
		return ua
	}
	return fallback
}

// GetOpenAICodexClientVersion 返回出站声明的 Codex 客户端版本号。
// 优先级：管理员在面板覆写的版本 → 自动同步到的官方最新稳定版 → 内置常量。
// 上游在容量紧张时按客户端身份分优先级降载，陈旧版本会被优先丢弃，故该值需保持跟随官方发布；
// 自动同步让运维不必为了跟版本而发新版本。
func (s *SettingService) GetOpenAICodexClientVersion(ctx context.Context) string {
	fallback := codexCLIVersion
	if s == nil || s.settingRepo == nil {
		return fallback
	}
	if cached, ok := s.openAICodexVersionCache.Load().(*cachedOpenAICodexClientVersion); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.version
		}
	}

	result, _, _ := s.openAICodexVersionSF.Do(openAICodexClientVersionSFKey, func() (any, error) {
		if cached, ok := s.openAICodexVersionCache.Load().(*cachedOpenAICodexClientVersion); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.version, nil
			}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAICodexClientVersionDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyOpenAICodexClientVersion,
			SettingKeyOpenAICodexClientVersionSynced,
		})
		if err != nil {
			slog.Warn("failed to get openai codex client version setting", "error", err)
			s.openAICodexVersionCache.Store(&cachedOpenAICodexClientVersion{
				version:   fallback,
				expiresAt: time.Now().Add(openAICodexClientVersionErrorTTL).UnixNano(),
			})
			return fallback, nil
		}
		version := NormalizeCodexClientVersion(values[SettingKeyOpenAICodexClientVersion])
		if version == "" {
			version = NormalizeCodexClientVersion(values[SettingKeyOpenAICodexClientVersionSynced])
		}
		if version == "" {
			version = fallback
		}
		s.openAICodexVersionCache.Store(&cachedOpenAICodexClientVersion{
			version:   version,
			expiresAt: time.Now().Add(openAICodexClientVersionCacheTTL).UnixNano(),
		})
		return version, nil
	})
	if version, ok := result.(string); ok && version != "" {
		return version
	}
	return fallback
}

// InvalidateOpenAICodexClientVersionCache 丢弃版本号缓存，下次读取回源。
// 面板保存与自动同步写入后调用。
func (s *SettingService) InvalidateOpenAICodexClientVersionCache() {
	if s == nil {
		return
	}
	s.openAICodexVersionSF.Forget(openAICodexClientVersionSFKey)
	s.openAICodexVersionCache.Store((*cachedOpenAICodexClientVersion)(nil))
}

// GetOpenAICodexCanonicalUserAgent 返回出站规范 Codex User-Agent。
// 未填面板 UA 时按当前生效的客户端版本号拼出标准 CLI UA。
//
// 面板 UA 只贡献客户端名与 OS / 架构 / 终端指纹，版本段一律用生效版本重建：该输入框是
// 唯一能改 UA 后缀的地方，但它填写于某个历史版本，逐字沿用会把出站身份永久钉死在陈旧
// 版本上并绕过自动同步——而陈旧身份正是上游优先降载的那一侧。
// 需要固定版本请填「Codex 客户端版本号」并关闭自动同步。
func (s *SettingService) GetOpenAICodexCanonicalUserAgent(ctx context.Context) string {
	if s == nil {
		return codexCLIUserAgent
	}
	version := s.GetOpenAICodexClientVersion(ctx)
	ua := strings.TrimSpace(s.GetOpenAICodexUserAgent(ctx))
	if ua == "" {
		return buildCodexCLIUserAgent(version)
	}
	if rebuilt := openai.SetCodexUserAgentVersion(ua, version); rebuilt != "" {
		// 规范 UA 必须是完整形态：官方客户端 UA 一律带 `(<originator>; <版本>)` 尾段，
		// 缺段本身即为可识别特征。面板只填了首段/OS 组时在这里补齐。
		if origin, _, ok := openai.PairCodexClientIdentity(rebuilt); ok {
			rebuilt = openai.EnsureCodexUserAgentTrailer(rebuilt, origin, version)
		}
		return rebuilt
	}
	// 非 `{client}/{version}` 形态：交给 PairCodexClientIdentity 判定，
	// 推导不出官方身份时由收口整体回退规范身份。
	return ua
}

var legacyClaudeCodeCodexWhitelistEntry = openai.AllowedClientEntry{
	Originator: "Claude Code",
	UAContains: []string{"Claude Code/"},
}

func (s *SettingService) IsBackendModeEnabled(ctx context.Context) bool {
	if cached, ok := s.backendModeCache.Load().(*cachedBackendMode); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
		}
	}
	result, _, _ := s.backendModeSF.Do("backend_mode", func() (any, error) {
		if cached, ok := s.backendModeCache.Load().(*cachedBackendMode); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backendModeDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeyBackendModeEnabled)
		if err != nil {
			if errors.Is(err, ErrSettingNotFound) {
				// Setting not yet created (fresh install) - default to disabled with full TTL
				s.backendModeCache.Store(&cachedBackendMode{
					value:     false,
					expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
				})
				return false, nil
			}
			slog.Warn("failed to get backend_mode_enabled setting", "error", err)
			s.backendModeCache.Store(&cachedBackendMode{
				value:     false,
				expiresAt: time.Now().Add(backendModeErrorTTL).UnixNano(),
			})
			return false, nil
		}
		enabled := value == "true"
		s.backendModeCache.Store(&cachedBackendMode{
			value:     enabled,
			expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
		})
		return enabled, nil
	})
	if val, ok := result.(bool); ok {
		return val
	}
	return false
}

type gatewayForwardingSettingsResult struct {
	fp, mp, cch, cacheTTL1h, rewriteMessageCacheControl bool
	clientDatelineNormalization                         bool
}

func (s *SettingService) getGatewayForwardingSettingsCached(ctx context.Context) gatewayForwardingSettingsResult {
	if cached, ok := s.gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return gatewayForwardingSettingsResult{
				fp:                          cached.fingerprintUnification,
				mp:                          cached.metadataPassthrough,
				cch:                         cached.cchSigning,
				cacheTTL1h:                  cached.anthropicCacheTTL1hInjection,
				rewriteMessageCacheControl:  cached.rewriteMessageCacheControl,
				clientDatelineNormalization: cached.clientDatelineNormalization,
			}
		}
	}
	val, _, _ := s.gatewayForwardingSF.Do("gateway_forwarding", func() (any, error) {
		if cached, ok := s.gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return gatewayForwardingSettingsResult{
					fp:                          cached.fingerprintUnification,
					mp:                          cached.metadataPassthrough,
					cch:                         cached.cchSigning,
					cacheTTL1h:                  cached.anthropicCacheTTL1hInjection,
					rewriteMessageCacheControl:  cached.rewriteMessageCacheControl,
					clientDatelineNormalization: cached.clientDatelineNormalization,
				}, nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gatewayForwardingDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyEnableFingerprintUnification,
			SettingKeyEnableMetadataPassthrough,
			SettingKeyEnableCCHSigning,
			SettingKeyEnableAnthropicCacheTTL1hInjection,
			SettingKeyRewriteMessageCacheControl,
			SettingKeyEnableClientDatelineNormalization,
		})
		if err != nil {
			slog.Warn("failed to get gateway forwarding settings", "error", err)
			s.gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
				fingerprintUnification:       true,
				metadataPassthrough:          false,
				cchSigning:                   false,
				anthropicCacheTTL1hInjection: false,
				rewriteMessageCacheControl:   s.defaultRewriteMessageCacheControl(),
				clientDatelineNormalization:  true,
				expiresAt:                    time.Now().Add(gatewayForwardingErrorTTL).UnixNano(),
			})
			return gatewayForwardingSettingsResult{fp: true, rewriteMessageCacheControl: s.defaultRewriteMessageCacheControl(), clientDatelineNormalization: true}, nil
		}
		fp := true
		if v, ok := values[SettingKeyEnableFingerprintUnification]; ok && v != "" {
			fp = v == "true"
		}
		mp := values[SettingKeyEnableMetadataPassthrough] == "true"
		cch := values[SettingKeyEnableCCHSigning] == "true"
		cacheTTL1h := values[SettingKeyEnableAnthropicCacheTTL1hInjection] == "true"
		rewriteMessageCacheControl := s.defaultRewriteMessageCacheControl()
		if v, ok := values[SettingKeyRewriteMessageCacheControl]; ok && v != "" {
			rewriteMessageCacheControl = v == "true"
		}
		clientDatelineNormalization := true
		if v, ok := values[SettingKeyEnableClientDatelineNormalization]; ok && v != "" {
			clientDatelineNormalization = v == "true"
		}
		s.gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
			fingerprintUnification:       fp,
			metadataPassthrough:          mp,
			cchSigning:                   cch,
			anthropicCacheTTL1hInjection: cacheTTL1h,
			rewriteMessageCacheControl:   rewriteMessageCacheControl,
			clientDatelineNormalization:  clientDatelineNormalization,
			expiresAt:                    time.Now().Add(gatewayForwardingCacheTTL).UnixNano(),
		})
		return gatewayForwardingSettingsResult{
			fp:                          fp,
			mp:                          mp,
			cch:                         cch,
			cacheTTL1h:                  cacheTTL1h,
			rewriteMessageCacheControl:  rewriteMessageCacheControl,
			clientDatelineNormalization: clientDatelineNormalization,
		}, nil
	})
	if r, ok := val.(gatewayForwardingSettingsResult); ok {
		return r
	}
	return gatewayForwardingSettingsResult{fp: true, clientDatelineNormalization: true}
}

// GetGatewayForwardingSettings returns cached gateway forwarding settings.
// Uses in-process atomic.Value cache with 60s TTL, zero-lock hot path.
// Returns (fingerprintUnification, metadataPassthrough, cchSigning).
func (s *SettingService) GetGatewayForwardingSettings(ctx context.Context) (fingerprintUnification, metadataPassthrough, cchSigning bool) {
	result := s.getGatewayForwardingSettingsCached(ctx)
	return result.fp, result.mp, result.cch
}

// IsAnthropicCacheTTL1hInjectionEnabled 检查是否对 Anthropic OAuth/SetupToken 请求体注入 1h cache_control ttl。
func (s *SettingService) IsAnthropicCacheTTL1hInjectionEnabled(ctx context.Context) bool {
	return s.getGatewayForwardingSettingsCached(ctx).cacheTTL1h
}

// IsRewriteMessageCacheControlEnabled 检查是否启用 messages cache_control 改写。
func (s *SettingService) IsRewriteMessageCacheControlEnabled(ctx context.Context) bool {
	return s.getGatewayForwardingSettingsCached(ctx).rewriteMessageCacheControl
}

// IsClientDatelineNormalizationEnabled 检查是否启用 Anthropic OAuth/SetupToken 请求体
// 的客户端 dateline 归一化。默认开启。
func (s *SettingService) IsClientDatelineNormalizationEnabled(ctx context.Context) bool {
	return s.getGatewayForwardingSettingsCached(ctx).clientDatelineNormalization
}

// GetClaudeCodeVersionBounds 获取 Claude Code 版本号上下限要求
// 使用进程内 atomic.Value 缓存，60 秒 TTL，热路径零锁开销
// singleflight 防止缓存过期时 thundering herd
// 返回空字符串表示不做对应方向的版本检查
func (s *SettingService) GetClaudeCodeVersionBounds(ctx context.Context) (min, max string) {
	if cached, ok := s.versionBoundsCache.Load().(*cachedVersionBounds); ok {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.min, cached.max
		}
	}
	// singleflight: 同一时刻只有一个 goroutine 查询 DB，其余复用结果
	type bounds struct{ min, max string }
	result, err, _ := s.versionBoundsSF.Do("version_bounds", func() (any, error) {
		// 二次检查，避免排队的 goroutine 重复查询
		if cached, ok := s.versionBoundsCache.Load().(*cachedVersionBounds); ok {
			if time.Now().UnixNano() < cached.expiresAt {
				return bounds{cached.min, cached.max}, nil
			}
		}
		// 使用独立 context：断开请求取消链，避免客户端断连导致空值被长期缓存
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), versionBoundsDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyMinClaudeCodeVersion,
			SettingKeyMaxClaudeCodeVersion,
		})
		if err != nil {
			// fail-open: DB 错误时不阻塞请求，但记录日志并使用短 TTL 快速重试
			slog.Warn("failed to get claude code version bounds setting, skipping version check", "error", err)
			s.versionBoundsCache.Store(&cachedVersionBounds{
				min:       "",
				max:       "",
				expiresAt: time.Now().Add(versionBoundsErrorTTL).UnixNano(),
			})
			return bounds{"", ""}, nil
		}
		b := bounds{
			min: values[SettingKeyMinClaudeCodeVersion],
			max: values[SettingKeyMaxClaudeCodeVersion],
		}
		s.versionBoundsCache.Store(&cachedVersionBounds{
			min:       b.min,
			max:       b.max,
			expiresAt: time.Now().Add(versionBoundsCacheTTL).UnixNano(),
		})
		return b, nil
	})
	if err != nil {
		return "", ""
	}
	b, ok := result.(bounds)
	if !ok {
		return "", ""
	}
	return b.min, b.max
}
