package scheduling

import (
	"math"
	"sort"
)

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
