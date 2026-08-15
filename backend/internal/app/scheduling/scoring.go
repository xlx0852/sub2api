package scheduling

import (
	"math"
	"sort"
)

type ScoreOptions struct {
	RequireCompact bool
	StickyWeighted bool
	ResetRange     *NormalizationRange
}

type ScoredCandidate struct {
	Candidate CandidateSnapshot
	Breakdown ScoreBreakdown
}

func (c ScoredCandidate) Score() float64 {
	return c.Breakdown.Total
}

type scoreRange struct {
	min       float64
	max       float64
	available bool
}

type NormalizationRange struct {
	Min       float64
	Max       float64
	Available bool
}

// ScoreCandidates 复现现有 OpenAI scorer 的归一化与加权规则，同时为
// 平台 adapter 无法提供的信号保留显式中性值，不把未知当成有利零值。
func ScoreCandidates(candidates []CandidateSnapshot, weights ScoreWeights, options ScoreOptions) []ScoredCandidate {
	if len(candidates) == 0 {
		return nil
	}

	minPriority, maxPriority := candidates[0].Priority, candidates[0].Priority
	maxQueue := 1.0
	ttftRange := scoreSignalRange(candidates, func(candidate CandidateSnapshot) Signal {
		return candidate.Signals.TTFTMillis
	}, true)
	resetRange := scoreSignalRange(candidates, func(candidate CandidateSnapshot) Signal {
		return candidate.Signals.ResetSeconds
	}, false)
	if options.ResetRange != nil {
		resetRange = scoreRange{min: options.ResetRange.Min, max: options.ResetRange.Max, available: options.ResetRange.Available}
	}
	compactLatencyRange := scoreSignalRange(candidates, func(candidate CandidateSnapshot) Signal {
		return candidate.Signals.CompactMillis
	}, true)
	for _, candidate := range candidates {
		if candidate.Priority < minPriority {
			minPriority = candidate.Priority
		}
		if candidate.Priority > maxPriority {
			maxPriority = candidate.Priority
		}
		if candidate.Signals.QueueDepth.Available && candidate.Signals.QueueDepth.Value > maxQueue {
			maxQueue = candidate.Signals.QueueDepth.Value
		}
	}

	result := make([]ScoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		priorityFactor := 1.0
		if maxPriority > minPriority {
			priorityFactor = 1 - float64(candidate.Priority-minPriority)/float64(maxPriority-minPriority)
		}
		loadFactor := lowerIsBetterUnitFactor(candidate.Signals.LoadRate, 100, 0.5)
		queueFactor := lowerIsBetterUnitFactor(candidate.Signals.QueueDepth, maxQueue, 0.5)
		errorFactor := lowerIsBetterUnitFactor(candidate.Signals.ErrorRate, 1, 0.5)
		ttftFactor := rangedLowerIsBetterFactor(candidate.Signals.TTFTMillis, ttftRange, 0.5, 0.5)
		resetFactor := 0.0
		if weights.Reset > 0 {
			resetFactor = rangedLowerIsBetterFactor(candidate.Signals.ResetSeconds, resetRange, 0, 1)
		}
		quotaHeadroomFactor := 0.5
		if candidate.Signals.QuotaHeadroom.Available {
			quotaHeadroomFactor = clampUnit(candidate.Signals.QuotaHeadroom.Value)
		}
		compactErrorFactor, compactLatencyFactor := 0.0, 0.0
		if options.RequireCompact {
			compactErrorFactor = lowerIsBetterUnitFactor(candidate.Signals.CompactError, 1, 0.5)
			compactLatencyFactor = rangedLowerIsBetterFactor(candidate.Signals.CompactMillis, compactLatencyRange, 0.5, 0.5)
		}

		breakdown := ScoreBreakdown{
			Priority:       weights.Priority * priorityFactor,
			Load:           weights.Load * loadFactor,
			Queue:          weights.Queue * queueFactor,
			ErrorRate:      weights.ErrorRate * errorFactor,
			TTFT:           weights.TTFT * ttftFactor,
			Reset:          weights.Reset * resetFactor,
			QuotaHeadroom:  weights.QuotaHeadroom * quotaHeadroomFactor,
			CompactError:   weights.CompactError * compactErrorFactor,
			CompactLatency: weights.CompactLatency * compactLatencyFactor,
		}
		if options.StickyWeighted {
			if candidate.Affinity.PreviousResponse {
				breakdown.PreviousResponse = weights.PreviousResponse
			}
			if candidate.Affinity.SessionSticky {
				breakdown.SessionSticky = weights.SessionSticky
			}
		}
		breakdown.Total = breakdown.ComponentTotal()
		result = append(result, ScoredCandidate{Candidate: NewCandidateSnapshot(candidate), Breakdown: breakdown})
	}
	return result
}

func SortScoredCandidates(candidates []ScoredCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return ScoredCandidateBetter(candidates[i], candidates[j])
	})
}

func ScoredCandidateBetter(left, right ScoredCandidate) bool {
	if left.Score() != right.Score() {
		return left.Score() > right.Score()
	}
	if left.Candidate.Priority != right.Candidate.Priority {
		return left.Candidate.Priority < right.Candidate.Priority
	}
	if comparison := compareLowerSignal(left.Candidate.Signals.LoadRate, right.Candidate.Signals.LoadRate); comparison != 0 {
		return comparison < 0
	}
	if comparison := compareLowerSignal(left.Candidate.Signals.QueueDepth, right.Candidate.Signals.QueueDepth); comparison != 0 {
		return comparison < 0
	}
	return left.Candidate.AccountID < right.Candidate.AccountID
}

func scoreSignalRange(candidates []CandidateSnapshot, signal func(CandidateSnapshot) Signal, positiveOnly bool) scoreRange {
	result := scoreRange{}
	for _, candidate := range candidates {
		value := signal(candidate)
		if !value.Available || math.IsNaN(value.Value) || math.IsInf(value.Value, 0) || (positiveOnly && value.Value <= 0) {
			continue
		}
		if !result.available {
			result.min, result.max, result.available = value.Value, value.Value, true
			continue
		}
		if value.Value < result.min {
			result.min = value.Value
		}
		if value.Value > result.max {
			result.max = value.Value
		}
	}
	return result
}

func lowerIsBetterUnitFactor(signal Signal, denominator, unavailable float64) float64 {
	if !signal.Available || denominator <= 0 || math.IsNaN(signal.Value) || math.IsInf(signal.Value, 0) {
		return unavailable
	}
	return 1 - clampUnit(signal.Value/denominator)
}

func rangedLowerIsBetterFactor(signal Signal, valueRange scoreRange, unavailable, equal float64) float64 {
	if !signal.Available || !valueRange.available || math.IsNaN(signal.Value) || math.IsInf(signal.Value, 0) {
		return unavailable
	}
	if valueRange.max <= valueRange.min {
		return equal
	}
	return 1 - clampUnit((signal.Value-valueRange.min)/(valueRange.max-valueRange.min))
}

func compareLowerSignal(left, right Signal) int {
	if left.Available != right.Available {
		if left.Available {
			return -1
		}
		return 1
	}
	if !left.Available || left.Value == right.Value {
		return 0
	}
	if left.Value < right.Value {
		return -1
	}
	return 1
}

func clampUnit(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
