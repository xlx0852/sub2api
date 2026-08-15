package service

import "sync/atomic"

type schedulerShadowCounters struct {
	evaluationTotal        atomic.Int64
	candidateTotal         atomic.Int64
	selectionDiffTotal     atomic.Int64
	affinityEscapeTotal    atomic.Int64
	hardGateViolationTotal atomic.Int64
	fallbackTotal          atomic.Int64
}

type schedulerShadowCountersSnapshot struct {
	EvaluationTotal        int64
	CandidateTotal         int64
	SelectionDiffTotal     int64
	AffinityEscapeTotal    int64
	HardGateViolationTotal int64
	FallbackTotal          int64
}

func (c *schedulerShadowCounters) record(candidateCount int, selectionDiff, affinityEscape, hardGateViolation, fallback bool) schedulerShadowCountersSnapshot {
	if c == nil {
		return schedulerShadowCountersSnapshot{}
	}
	c.evaluationTotal.Add(1)
	c.candidateTotal.Add(int64(candidateCount))
	if selectionDiff {
		c.selectionDiffTotal.Add(1)
	}
	if affinityEscape {
		c.affinityEscapeTotal.Add(1)
	}
	if hardGateViolation {
		c.hardGateViolationTotal.Add(1)
	}
	if fallback {
		c.fallbackTotal.Add(1)
	}
	return c.snapshot()
}

func (c *schedulerShadowCounters) snapshot() schedulerShadowCountersSnapshot {
	if c == nil {
		return schedulerShadowCountersSnapshot{}
	}
	return schedulerShadowCountersSnapshot{
		EvaluationTotal:        c.evaluationTotal.Load(),
		CandidateTotal:         c.candidateTotal.Load(),
		SelectionDiffTotal:     c.selectionDiffTotal.Load(),
		AffinityEscapeTotal:    c.affinityEscapeTotal.Load(),
		HardGateViolationTotal: c.hardGateViolationTotal.Load(),
		FallbackTotal:          c.fallbackTotal.Load(),
	}
}
