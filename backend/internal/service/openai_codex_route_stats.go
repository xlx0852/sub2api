package service

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// 出站路由形态的滚动统计。
//
// 存在的理由：要回答「经网关出去的客户端拿到的东西，跟直连真实客户端一不一样」，
// 需要**几百条量级**的样本。探针采票只有 2~3 次/小时（按票龄 45 分钟触发），
// 靠它验证「astra 约 1% 命中」要等几周；而业务响应上的票采集有几百次/小时。
//
// 所以这里累计「实际落池 + 票长形态」并每 10 分钟只吐**一行**汇总，
// 噪声极低但样本量足够。用法：
//
//	docker logs <容器> | grep CodexRouteStat
//
// 与真实客户端基线的对比口径见 openspec 第十七章（归档抓包 123 条：
// 312 占 99.2%、292 占 0.8%、astra 命中 1.0%）。
var openAICodexRouteStats = struct {
	mu      sync.Mutex
	pool    map[string]int
	shape   map[int]int
	routes  map[string]int
	total   int
	windowT time.Time
}{pool: map[string]int{}, shape: map[int]int{}, routes: map[string]int{}, windowT: time.Now()}

// recordOpenAICodexRouteMapping 记录一次「请求模型 → 实际落池」的映射。
//
// 与 recordOpenAICodexRouteStat 分开的理由：两个业务采集点传进 capture 的 model
// **语义并不一致**——WS 路径传的是响应真实落点（正确），HTTP 路径传的是请求模型。
// 所以落池统计不能用来回答「请求 astra 到底有没有拿到 astra」，只有在**两个模型都明确
// 在作用域里**的调用点（如 WS 终端事件）才能记这条映射。探针的 seeded 日志同理。
func recordOpenAICodexRouteMapping(requested, served string) {
	requested = strings.TrimSpace(requested)
	served = strings.TrimSpace(served)
	if requested == "" || served == "" {
		return
	}
	openAICodexRouteStats.mu.Lock()
	openAICodexRouteStats.routes[requested+"→"+served]++
	openAICodexRouteStats.mu.Unlock()
}

// recordOpenAICodexRouteStat 记录一次「该轮被接受」的落池与票形态。
// 只计数，不存内容；不参与任何决策。
func recordOpenAICodexRouteStat(pool string, tokenLen int) {
	if tokenLen <= 0 {
		return
	}
	pool = strings.TrimSpace(pool)
	if pool == "" {
		pool = "(unknown)"
	}
	openAICodexRouteStats.mu.Lock()
	openAICodexRouteStats.total++
	openAICodexRouteStats.pool[pool]++
	openAICodexRouteStats.shape[tokenLen]++
	openAICodexRouteStats.mu.Unlock()
}

// flushOpenAICodexRouteStats 输出一行窗口汇总并清零；窗口内无样本时返回空串。
func flushOpenAICodexRouteStats(now time.Time) string {
	openAICodexRouteStats.mu.Lock()
	defer openAICodexRouteStats.mu.Unlock()
	if openAICodexRouteStats.total == 0 {
		openAICodexRouteStats.windowT = now
		return ""
	}
	total := openAICodexRouteStats.total
	pool := openAICodexRouteStats.pool
	shape := openAICodexRouteStats.shape
	routes := openAICodexRouteStats.routes
	elapsed := now.Sub(openAICodexRouteStats.windowT)
	openAICodexRouteStats.pool = map[string]int{}
	openAICodexRouteStats.shape = map[int]int{}
	openAICodexRouteStats.routes = map[string]int{}
	openAICodexRouteStats.total = 0
	openAICodexRouteStats.windowT = now

	line := fmt.Sprintf("[CodexRouteStat] window=%s accepted=%d rates=%.1f/min pools=%s shapes=%s",
		elapsed.Round(time.Second), total, perMinute(total, elapsed),
		formatCountMap(pool, total), formatShapeMap(shape, total))
	if len(routes) > 0 {
		line += " routes=" + formatCountMap(routes, sumCounts(routes))
	}
	return line
}

func sumCounts(m map[string]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

func perMinute(n int, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) / d.Minutes()
}

func formatCountMap(m map[string]int, total int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d(%.1f%%)", k, m[k], 100*float64(m[k])/float64(total)))
	}
	return strings.Join(parts, ",")
}

func formatShapeMap(m map[int]int, total int) string {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(keys)))
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("len%d=%d(%.1f%%)", k, m[k], 100*float64(m[k])/float64(total)))
	}
	return strings.Join(parts, ",")
}
