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
	mu       sync.Mutex
	pool     map[string]int
	shape    map[int]int
	routes   map[string]int
	routeLen map[string]int
	// astraByAccount: accountID -> [astra 请求数, 其中真落 astra 的数]。
	// 存在的唯一理由：astra 命中率最强的混淆项是**账号不同**（套餐差异直接影响可用性）。
	// 有了账号维度，才能拿它与探针 seeded 行（带 account=）做**同账号**对照，
	// 从而区分「命中率由账号/时段决定」与「由网关的身份改动决定」。
	astraByAccount map[int64][2]int
	total          int
	windowT        time.Time
}{pool: map[string]int{}, shape: map[int]int{}, routes: map[string]int{}, routeLen: map[string]int{},
	astraByAccount: map[int64][2]int{}, windowT: time.Now()}

// recordOpenAICodexRouteMapping 记录一次「请求模型 → 实际落池」的映射。
//
// 与 recordOpenAICodexRouteStat 分开的理由：两个业务采集点传进 capture 的 model
// **语义并不一致**——WS 路径传的是响应真实落点（正确），HTTP 路径传的是请求模型。
// 所以落池统计不能用来回答「请求 astra 到底有没有拿到 astra」，只有在**两个模型都明确
// 在作用域里**的调用点（如 WS 终端事件）才能记这条映射。探针的 seeded 日志同理。
func recordOpenAICodexRouteMapping(requested, served string, ticketLen int, accountID int64) {
	requested = strings.TrimSpace(requested)
	served = strings.TrimSpace(served)
	if requested == "" || served == "" {
		return
	}
	openAICodexRouteStats.mu.Lock()
	openAICodexRouteStats.routes[requested+"→"+served]++
	if strings.Contains(requested, "astra") {
		stat := openAICodexRouteStats.astraByAccount[accountID]
		stat[0]++
		if strings.Contains(served, "astra") {
			stat[1]++
		}
		openAICodexRouteStats.astraByAccount[accountID] = stat
	}
	// 同时记下这一次响应的票长（0 = 没发可复用票）。两者必须一起看：
	// 否则会出现「落点是 astra」与「形态直方图全是 312」这种看似矛盾的组合，
	// 而真实原因往往是**某些落点的响应根本不发可复用票**，因而从不进形态统计。
	shape := "none"
	if ticketLen > 0 {
		shape = fmt.Sprintf("len%d", ticketLen)
	}
	openAICodexRouteStats.routeLen[served+"#"+shape]++
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
	routeLen := openAICodexRouteStats.routeLen
	astraByAccount := openAICodexRouteStats.astraByAccount
	elapsed := now.Sub(openAICodexRouteStats.windowT)
	openAICodexRouteStats.pool = map[string]int{}
	openAICodexRouteStats.shape = map[int]int{}
	openAICodexRouteStats.routes = map[string]int{}
	openAICodexRouteStats.routeLen = map[string]int{}
	openAICodexRouteStats.astraByAccount = map[int64][2]int{}
	openAICodexRouteStats.total = 0
	openAICodexRouteStats.windowT = now

	line := fmt.Sprintf("[CodexRouteStat] window=%s accepted=%d rates=%.1f/min pools=%s shapes=%s",
		elapsed.Round(time.Second), total, perMinute(total, elapsed),
		formatCountMap(pool, total), formatShapeMap(shape, total))
	if len(routes) > 0 {
		line += " routes=" + formatCountMap(routes, sumCounts(routes))
	}
	if len(routeLen) > 0 {
		line += " route_served_ticket=" + formatCountMap(routeLen, sumCounts(routeLen))
	}
	if len(astraByAccount) > 0 {
		// 形如 astra_by_account=69:2/3,120:0/9 —— 账号:命中/请求。
		ids := make([]int64, 0, len(astraByAccount))
		for id := range astraByAccount {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		parts := make([]string, 0, len(ids))
		for _, id := range ids {
			stat := astraByAccount[id]
			parts = append(parts, fmt.Sprintf("%d:%d/%d", id, stat[1], stat[0]))
		}
		line += " astra_by_account=" + strings.Join(parts, ",")
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

// accountIDOf 取账号 ID，nil 安全（统计用，不参与决策）。
func accountIDOf(a *Account) int64 {
	if a == nil {
		return 0
	}
	return a.ID
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
