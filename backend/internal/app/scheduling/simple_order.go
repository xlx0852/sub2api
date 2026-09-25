package scheduling

import (
	"math"
	"sort"
)

// LatencyProvider 提供账号在某模型上的近期首字节中位数（毫秒）。
type LatencyProvider interface {
	AccountTTFT(accountID int64, model string) (medianMS int, samples int, ok bool)
}

// LatencyPreference 描述是否在并列组内偏好低延迟账号。
type LatencyPreference struct {
	Enabled         bool
	Model           string
	Provider        LatencyProvider
	MinSamples      int
	MinAdvantagePct int
	// MaxLoadDeltaPct 是参与延迟比较允许的负载差（百分点，默认 10）。
	MaxLoadDeltaPct float64
}

// candidateLoad 与 OrderByPriorityAndLoad 的负载判断保持一致。
func candidateLoad(c CandidateSnapshot) (float64, bool) {
	value := c.Signals.LoadRate.Value
	return value, c.Signals.LoadRate.Available && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// OrderByPriorityLoadAndLatency 沿用 OrderByPriorityAndLoad 的排序，再做一遍延迟优选：
// 从队首开始，若同优先级、负载与当前候选相差不超过 MaxLoadDeltaPct 的候选中有账号
// 的近期首字节明显更快（快至少 MinAdvantagePct%，且双方样本数达到 MinSamples），
// 就把最快者提升到当前位置。该过程只做单趟「提升」，不跨越优先级，也不跨越负载容差，
// 保证不因延迟把流量全部压到一个账号上。
func OrderByPriorityLoadAndLatency(candidates []CandidateSnapshot, rng *SelectionRNG, pref LatencyPreference) []CandidateSnapshot {
	ordered := OrderByPriorityAndLoad(candidates, rng)
	if !pref.Enabled || pref.Provider == nil || len(ordered) < 2 {
		return ordered
	}
	minSamples := pref.MinSamples
	if minSamples <= 0 {
		minSamples = 5
	}
	advantage := pref.MinAdvantagePct
	if advantage <= 0 {
		advantage = 30
	}
	loadDelta := pref.MaxLoadDeltaPct
	if loadDelta <= 0 {
		loadDelta = 10
	}
	median := func(id int64) (int, bool) {
		value, samples, ok := pref.Provider.AccountTTFT(id, pref.Model)
		if !ok || samples < minSamples || value <= 0 {
			return 0, false
		}
		return value, true
	}
	for i := 0; i < len(ordered)-1; i++ {
		current := ordered[i]
		currentLoad, currentOK := candidateLoad(current)
		currentMedian, currentHas := median(current.AccountID)
		bestIdx := -1
		bestMedian := currentMedian
		for j := i + 1; j < len(ordered); j++ {
			candidate := ordered[j]
			if candidate.Priority != current.Priority {
				break // 同优先级区段结束
			}
			candidateLoad, candidateOK := candidateLoad(candidate)
			if currentOK && candidateOK && math.Abs(candidateLoad-currentLoad) > loadDelta {
				continue // 负载差距超出容差，保持负载优先
			}
			candidateMedian, ok := median(candidate.AccountID)
			if !ok {
				continue
			}
			// 只有当候选明显更快时才考虑提升。
			if currentHas && candidateMedian*100 > currentMedian*(100-advantage) {
				continue
			}
			if bestIdx < 0 || candidateMedian < bestMedian {
				bestIdx = j
				bestMedian = candidateMedian
			}
		}
		if bestIdx > i {
			promoted := ordered[bestIdx]
			copy(ordered[i+1:bestIdx+1], ordered[i:bestIdx])
			ordered[i] = promoted
		}
	}
	return ordered
}

// OrderByPriorityAndLoad orders all eligible candidates without score/TopK
// truncation. The caller owns hard gates and atomic slot acquisition.
func OrderByPriorityAndLoad(candidates []CandidateSnapshot, rng *SelectionRNG) []CandidateSnapshot {
	result := make([]CandidateSnapshot, len(candidates))
	for i, c := range candidates {
		result[i] = NewCandidateSnapshot(c)
	}
	load := func(c CandidateSnapshot) (float64, bool) {
		s := c.Signals.LoadRate
		return s.Value, s.Available && s.Value >= 0 && !math.IsNaN(s.Value) && !math.IsInf(s.Value, 0)
	}
	less := func(a, b CandidateSnapshot) bool {
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		av, ak := load(a)
		bv, bk := load(b)
		if ak != bk {
			return ak
		}
		return ak && av < bv
	}
	sort.SliceStable(result, func(i, j int) bool { return less(result[i], result[j]) })
	if rng == nil {
		r := NewSelectionRNG(0)
		rng = &r
	}
	for start := 0; start < len(result); {
		end := start + 1
		for end < len(result) && !less(result[start], result[end]) && !less(result[end], result[start]) {
			end++
		}
		for i := end - 1; i > start; i-- {
			j := start + int(rng.NextUint64()%uint64(i-start+1))
			result[i], result[j] = result[j], result[i]
		}
		start = end
	}
	return result
}
