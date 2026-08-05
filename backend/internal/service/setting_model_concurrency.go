package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"
)

// modelConcurrencyLimitsCacheTTL 是模型级并发预算的进程内缓存 TTL。
// 预算配置变更后最多延迟 TTL 生效；刷新走 stale-while-revalidate，
// 不阻塞配置变更后的首个请求。
const modelConcurrencyLimitsCacheTTL = 60 * time.Second

// modelConcurrencyLimitsErrorTTL 是 DB 读取失败时的短 TTL 退避，避免故障期间打爆 DB。
const modelConcurrencyLimitsErrorTTL = 5 * time.Second

// modelConcurrencyLimitsDBTimeout 是单次 DB 读取的超时。
const modelConcurrencyLimitsDBTimeout = 5 * time.Second

// cachedModelConcurrencyLimits 模型并发预算的进程内缓存条目。
type cachedModelConcurrencyLimits struct {
	limits    map[string]int
	expiresAt int64 // unix nano
}

// GetModelConcurrencyLimits 返回模型级全局并发预算（canonical model → 并发上限）。
// 进程内缓存 ~60s，热路径调用不访问 DB；读取失败或未配置返回 nil
// （等价于未配置预算，所有模型行为不变）。
// 返回的 map 不应被修改，调用方按需拷贝。
func (s *SettingService) GetModelConcurrencyLimits(ctx context.Context) map[string]int {
	if cached, ok := s.modelConcurrencyLimitsCache.Load().(*cachedModelConcurrencyLimits); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.limits
		}
	}
	result, _, _ := s.modelConcurrencyLimitsSF.Do("model_concurrency_limits", func() (any, error) {
		if cached, ok := s.modelConcurrencyLimitsCache.Load().(*cachedModelConcurrencyLimits); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached, nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), modelConcurrencyLimitsDBTimeout)
		defer cancel()

		raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyModelConcurrencyLimits)
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			slog.Warn("failed to get model_concurrency_limits setting", "error", err)
			entry := &cachedModelConcurrencyLimits{
				limits:    nil,
				expiresAt: time.Now().Add(modelConcurrencyLimitsErrorTTL).UnixNano(),
			}
			s.modelConcurrencyLimitsCache.Store(entry)
			return entry, nil
		}

		limits := map[string]int{}
		if err == nil && strings.TrimSpace(raw) != "" {
			parsed := map[string]int{}
			if unmarshalErr := json.Unmarshal([]byte(raw), &parsed); unmarshalErr != nil {
				slog.Warn("[Setting] unmarshal model_concurrency_limits failed", "error", unmarshalErr)
			} else {
				limits = sanitizeModelConcurrencyLimits(parsed)
			}
		}

		entry := &cachedModelConcurrencyLimits{
			limits:    limits,
			expiresAt: time.Now().Add(modelConcurrencyLimitsCacheTTL).UnixNano(),
		}
		s.modelConcurrencyLimitsCache.Store(entry)
		return entry, nil
	})
	if entry, ok := result.(*cachedModelConcurrencyLimits); ok && entry != nil {
		return entry.limits
	}
	return nil
}

// SetModelConcurrencyLimitsCache 以指定预算直接刷新缓存（供 refreshCachedSettings 使用），
// 避免管理员改配置后需等待一个 TTL 才生效。
func (s *SettingService) SetModelConcurrencyLimitsCache(limits map[string]int) {
	if s == nil {
		return
	}
	s.modelConcurrencyLimitsSF.Forget("model_concurrency_limits")
	s.modelConcurrencyLimitsCache.Store(&cachedModelConcurrencyLimits{
		limits:    limits,
		expiresAt: time.Now().Add(modelConcurrencyLimitsCacheTTL).UnixNano(),
	})
}
