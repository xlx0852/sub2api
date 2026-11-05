package service

import (
	"context"
	"strings"
	"sync"
	"time"
)

// SettingKeyPricingPipeline 是后台可切的定价管线开关（设置项，未配置时回落到
// 配置项 pricing.pipeline，再回落到 legacy）。
const SettingKeyPricingPipeline = "pricing_pipeline"

// billingSettingsReader 是计费侧读取运维开关的最小依赖，避免 service 之间硬耦合。
type billingSettingsReader interface {
	GetRawValue(ctx context.Context, key string) (string, error)
}

// pipelineSettingTTL 是设置项的进程内缓存时间：开关改动最多 10 秒生效，
// 又不至于让每个请求都查一次库。
const pipelineSettingTTL = 10 * time.Second

type pipelineSettingCache struct {
	mu       sync.RWMutex
	mode     string
	loadedAt time.Time
}

// SetSettingsReader 注入设置读取器（由依赖注入在启动时调用，可为 nil）。
func (s *BillingService) SetSettingsReader(reader billingSettingsReader) {
	if s == nil {
		return
	}
	s.settingsReader = reader
}

// resolvePipelineMode 依次读取：设置项 → 配置文件 → legacy。
func (s *BillingService) resolvePipelineMode(ctx context.Context) string {
	if s == nil {
		return PricingPipelineLegacy
	}

	fallback := PricingPipelineLegacy
	if s.cfg != nil {
		fallback = NormalizePricingPipelineMode(s.cfg.Pricing.Pipeline)
	}

	if s.settingsReader == nil {
		return fallback
	}

	s.pipelineCache.mu.RLock()
	cached, loadedAt := s.pipelineCache.mode, s.pipelineCache.loadedAt
	s.pipelineCache.mu.RUnlock()
	if !loadedAt.IsZero() && time.Since(loadedAt) < pipelineSettingTTL {
		if cached == "" {
			return fallback
		}
		return NormalizePricingPipelineMode(cached)
	}

	if ctx == nil {
		return fallback
	}
	raw, err := s.settingsReader.GetRawValue(ctx, SettingKeyPricingPipeline)
	if err != nil {
		// 读失败时保持上一份缓存，避免瞬时故障把管线切回 legacy。
		if cached != "" {
			return NormalizePricingPipelineMode(cached)
		}
		return fallback
	}

	s.pipelineCache.mu.Lock()
	s.pipelineCache.mode = strings.TrimSpace(raw)
	s.pipelineCache.loadedAt = time.Now()
	s.pipelineCache.mu.Unlock()

	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	return NormalizePricingPipelineMode(raw)
}
