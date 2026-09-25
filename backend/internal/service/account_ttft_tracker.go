package service

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

// AccountTTFTTracker 维护「账号 × 模型」的近期首字节中位数，用于选号时的
// 延迟偏好：同优先级、同负载档并列时，优先选择首字节明显更短的账号。
//
// 设计约束（改动前请先读）：
//   - 只在内存中保存最近 capacity 个样本，读取时丢弃超 TTL 的旧样本；
//   - 样本不足（少于 minSamples）时不参与比较，避免冷启动抖动；
//   - 纯观测数据，不影响计费、粘性会话与优先级语义。
type AccountTTFTTracker struct {
	mu       sync.RWMutex
	samples  map[accountTTFTKey][]ttftSample
	capacity int
	ttl      time.Duration
	minCount int
	now      func() time.Time
}

type accountTTFTKey struct {
	accountID int64
	model     string
}

type ttftSample struct {
	ms int
	at time.Time
}

const (
	defaultAccountTTFTCapacity = 20
	defaultAccountTTFTTTL      = 30 * time.Minute
	defaultAccountTTFTMinCount = 5
)

// NewAccountTTFTTracker 创建追踪器（容量 20、TTL 30 分钟、最少 5 个样本）。
func NewAccountTTFTTracker() *AccountTTFTTracker {
	return &AccountTTFTTracker{
		samples:  make(map[accountTTFTKey][]ttftSample),
		capacity: defaultAccountTTFTCapacity,
		ttl:      defaultAccountTTFTTTL,
		minCount: defaultAccountTTFTMinCount,
		now:      time.Now,
	}
}

// Observe 记录一次首字节耗时（毫秒）。非正值被忽略。
func (t *AccountTTFTTracker) Observe(accountID int64, model string, ttftMS int) {
	if t == nil || accountID <= 0 || ttftMS <= 0 {
		return
	}
	key := accountTTFTKey{accountID: accountID, model: normalizeTTFTModel(model)}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	list := append(t.samples[key], ttftSample{ms: ttftMS, at: now})
	if len(list) > t.capacity {
		list = list[len(list)-t.capacity:]
	}
	t.samples[key] = list
}

// AccountTTFT 返回账号在该模型上的近期首字节中位数与有效样本数。
func (t *AccountTTFTTracker) AccountTTFT(accountID int64, model string) (int, int, bool) {
	if t == nil || accountID <= 0 {
		return 0, 0, false
	}
	key := accountTTFTKey{accountID: accountID, model: normalizeTTFTModel(model)}
	cutoff := t.now().Add(-t.ttl)
	t.mu.RLock()
	list := t.samples[key]
	t.mu.RUnlock()
	values := make([]int, 0, len(list))
	for _, sample := range list {
		if sample.at.After(cutoff) {
			values = append(values, sample.ms)
		}
	}
	if len(values) < t.minCount {
		return 0, len(values), false
	}
	sort.Ints(values)
	return values[len(values)/2], len(values), true
}

func normalizeTTFTModel(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

// ttftLatencyPreference 由配置与追踪器组装选号用的延迟偏好；
// 开关关闭或追踪器缺失时返回零值（保持原有排序）。
func (s *OpenAIGatewayService) ttftLatencyPreference(model string) scheduling.LatencyPreference {
	if s == nil || s.ttftTracker == nil || s.cfg == nil || !s.cfg.Gateway.SchedulerTTFTPreference {
		return scheduling.LatencyPreference{}
	}
	return scheduling.LatencyPreference{
		Enabled:         true,
		Model:           model,
		Provider:        s.ttftTracker,
		MinSamples:      s.cfg.Gateway.SchedulerTTFTMinSamples,
		MinAdvantagePct: s.cfg.Gateway.SchedulerTTFTMinAdvantagePct,
	}
}

// ttftLatencyPreference 同 OpenAIGatewayService，用于 Anthropic/Gemini 网关路径。
func (s *GatewayService) ttftLatencyPreference(model string) scheduling.LatencyPreference {
	if s == nil || s.ttftTracker == nil || s.cfg == nil || !s.cfg.Gateway.SchedulerTTFTPreference {
		return scheduling.LatencyPreference{}
	}
	return scheduling.LatencyPreference{
		Enabled:         true,
		Model:           model,
		Provider:        s.ttftTracker,
		MinSamples:      s.cfg.Gateway.SchedulerTTFTMinSamples,
		MinAdvantagePct: s.cfg.Gateway.SchedulerTTFTMinAdvantagePct,
	}
}
