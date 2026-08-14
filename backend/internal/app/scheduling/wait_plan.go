package scheduling

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// DefaultConfig 是调度器在 *Config 缺失时使用的等待/负载默认值。
// 与 YAML 加载默认值无关；两套遗留调度器的 schedulingConfig() 原先各自硬编码同一份。
func DefaultConfig() config.GatewaySchedulingConfig {
	return config.GatewaySchedulingConfig{
		StickySessionMaxWaiting:  3,
		StickySessionWaitTimeout: 45 * time.Second,
		FallbackWaitTimeout:      30 * time.Second,
		FallbackMaxWaiting:       100,
		LoadBatchEnabled:         true,
		SlotCleanupInterval:      30 * time.Second,
	}
}

// StickyWaitPlan 构造粘性会话排队计划。
// MaxConcurrency 必须传账号原始 Concurrency（不是 EffectiveLoadFactor），
// 因为 WaitPlan 控制的是 Redis 实际并发槽位等待。
func StickyWaitPlan(accountID int64, maxConcurrency int, cfg config.GatewaySchedulingConfig) WaitPlan {
	return WaitPlan{
		AccountID:      accountID,
		MaxConcurrency: maxConcurrency,
		Timeout:        cfg.StickySessionWaitTimeout,
		MaxWaiting:     cfg.StickySessionMaxWaiting,
	}
}

// FallbackWaitPlan 构造兜底排队计划。
func FallbackWaitPlan(accountID int64, maxConcurrency int, cfg config.GatewaySchedulingConfig) WaitPlan {
	return WaitPlan{
		AccountID:      accountID,
		MaxConcurrency: maxConcurrency,
		Timeout:        cfg.FallbackWaitTimeout,
		MaxWaiting:     cfg.FallbackMaxWaiting,
	}
}
