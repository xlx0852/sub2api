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
}

// candidateLoad 与 OrderByPriorityAndLoad 的负载判断保持一致。
func candidateLoad(c CandidateSnapshot) (float64, bool) {
	value := c.Signals.LoadRate.Value
	return value, c.Signals.LoadRate.Available && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// sameOrderingGroup 判断两个候选是否属于同一「优先级 + 负载」并列组。
func sameOrderingGroup(a, b CandidateSnapshot) bool {
	if a.Priority != b.Priority {
		return false
	}
	av, ak := candidateLoad(a)
	bv, bk := candidateLoad(b)
	if ak != bk {
		return false
	}
	return !ak || av == bv
}

// OrderByPriorityLoadAndLatency 沿用 OrderByPriorityAndLoad 的排序，再在并列组内
// 按近期首字节中位数重排：仅当双方样本数都达到 MinSamples，且一方比另一方快
// 至少 MinAdvantagePct% 时才交换，避免样本噪声与只顾快号的流量倾斜。
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
	samplesOK := func(id int64) (int, bool) {
		value, samples, ok := pref.Provider.AccountTTFT(id, pref.Model)
		if !ok || samples < minSamples || value <= 0 {
			return 0, false
		}
		return value, true
	}
	fasterBy := func(a, b int) bool { return a*100 <= b*(100-advantage) }
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if !sameOrderingGroup(a, b) {
			return false
		}
		am, aok := samplesOK(a.AccountID)
		bm, bok := samplesOK(b.AccountID)
		if !aok || !bok {
			return false
		}
		return fasterBy(am, bm)
	})
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
