package service

import (
	"sort"
	"sync"
)

// 采票异常的连续计数与告警。
//
// 为什么需要这一层：2026-09-22 23:12 → 2026-09-23 19:21 的 **19 小时**里，
// 上游把票的形态从 ~300 字符改成 780，而本地上限是 400，
// 于是**每一张票都被静默丢掉**：每次探测都 status=200 completed=true、看起来像"上游不发"，
// 账号长期停在已清理状态，靠人肉比对才定位。
//
// 这里把「同一种异常连续出现 N 次」升级成 ERROR 级别日志，
// 让上游的形态漂移在**一两分钟内**可见，而不是 19 小时后。
//
// 阈值按异常类型区分：
//   - rejected_shape（上游发了票、被本地形态过滤拒掉）：4 次即告警。
//     这是纯本侧问题、必然可修，且一次 seed 作业就会连续触发 6 次，所以阈值取小。
//   - upstream_sent_none / incomplete_stream：12 次才告警。
//     这两类可能是上游短时行为（容量、抖动），阈值取大以免噪声，
//     但持续两轮以上仍然说明采票停摆，值得报出来。
const (
	codexTurnStateRejectedShapeStreak = 4
	codexTurnStateOtherAnomalyStreak  = 12
	codexTurnStateWatchMaxLengths     = 8
)

type codexTurnStateProbeWatch struct {
	mu      sync.Mutex
	verdict string
	streak  int
	lengths map[int]struct{}
	fired   bool
}

// observeCodexTurnStateProbeOutcome 记录一次探测结果。
//
// verdict 为空串表示**成功**（拿到可复用票），此时清零。
// 返回 (是否需要告警, 当前连续次数, 观察到的原始票长排序列表)。
// 同一段连续异常**只告警一次**，避免刷屏；异常类型变化或成功都会重置。
func (w *codexTurnStateProbeWatch) observe(verdict string, rawLen int) (bool, int, []int) {
	if w == nil {
		return false, 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if verdict == "" {
		w.verdict, w.streak, w.fired = "", 0, false
		w.lengths = map[int]struct{}{}
		return false, 0, nil
	}
	if w.verdict != verdict {
		w.verdict, w.streak, w.fired = verdict, 1, false
		w.lengths = map[int]struct{}{}
	} else {
		w.streak++
	}
	if rawLen > 0 && len(w.lengths) < codexTurnStateWatchMaxLengths {
		w.lengths[rawLen] = struct{}{}
	}

	threshold := codexTurnStateOtherAnomalyStreak
	if verdict == "rejected_shape" {
		threshold = codexTurnStateRejectedShapeStreak
	}
	if w.streak < threshold || w.fired {
		return false, w.streak, nil
	}
	w.fired = true

	lengths := make([]int, 0, len(w.lengths))
	for l := range w.lengths {
		lengths = append(lengths, l)
	}
	sort.Ints(lengths)
	return true, w.streak, lengths
}

// reset 只清理当前实例；生产代码通过 OpenAIGatewayService 持有 watcher。
func (w *codexTurnStateProbeWatch) reset() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.verdict, w.streak, w.fired = "", 0, false
	w.lengths = map[int]struct{}{}
}
