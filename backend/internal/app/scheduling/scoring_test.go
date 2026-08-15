package scheduling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScoreCandidatesNormalizesExistingOpenAIDimensions(t *testing.T) {
	candidates := []CandidateSnapshot{
		{
			AccountID: 1,
			Priority:  1,
			Signals: CandidateSignals{
				LoadRate:      AvailableSignal(20, timeZero),
				QueueDepth:    AvailableSignal(0, timeZero),
				ErrorRate:     AvailableSignal(0.1, timeZero),
				TTFTMillis:    AvailableSignal(1000, timeZero),
				QuotaHeadroom: AvailableSignal(0.8, timeZero),
			},
			Affinity: AffinitySignals{SessionSticky: true},
		},
		{
			AccountID: 2,
			Priority:  3,
			Signals: CandidateSignals{
				LoadRate:      AvailableSignal(80, timeZero),
				QueueDepth:    AvailableSignal(4, timeZero),
				ErrorRate:     AvailableSignal(0.5, timeZero),
				TTFTMillis:    AvailableSignal(5000, timeZero),
				QuotaHeadroom: AvailableSignal(0.2, timeZero),
			},
		},
	}
	weights := ScoreWeights{
		Priority: 1, Load: 1, Queue: 1, ErrorRate: 1, TTFT: 1,
		QuotaHeadroom: 1, SessionSticky: 3,
	}

	scored := ScoreCandidates(candidates, weights, ScoreOptions{StickyWeighted: true})

	require.Len(t, scored, 2)
	require.InDelta(t, 8.5, scored[0].Score(), 1e-12)
	require.InDelta(t, 0.9, scored[1].Score(), 1e-12)
	require.Equal(t, float64(3), scored[0].Breakdown.SessionSticky)
}

func TestScoreCandidatesTreatsUnavailableSignalsAsNeutral(t *testing.T) {
	scored := ScoreCandidates([]CandidateSnapshot{{AccountID: 1}}, ScoreWeights{
		Load: 1, Queue: 1, ErrorRate: 1, TTFT: 1, QuotaHeadroom: 1,
	}, ScoreOptions{})

	require.Len(t, scored, 1)
	require.InDelta(t, 2.5, scored[0].Score(), 1e-12)
}

func TestScoreCandidatesPreservesResetQuotaAndCompactFactors(t *testing.T) {
	candidates := []CandidateSnapshot{
		{
			AccountID: 1,
			Signals: CandidateSignals{
				ResetSeconds:  AvailableSignal(100, timeZero),
				QuotaHeadroom: AvailableSignal(0.8, timeZero),
				CompactError:  AvailableSignal(0.1, timeZero),
				CompactMillis: AvailableSignal(1000, timeZero),
			},
		},
		{
			AccountID: 2,
			Signals: CandidateSignals{
				ResetSeconds:  AvailableSignal(200, timeZero),
				QuotaHeadroom: AvailableSignal(0.2, timeZero),
				CompactError:  AvailableSignal(0.5, timeZero),
				CompactMillis: AvailableSignal(5000, timeZero),
			},
		},
	}
	weights := ScoreWeights{Reset: 1, QuotaHeadroom: 1, CompactError: 1, CompactLatency: 1}

	scored := ScoreCandidates(candidates, weights, ScoreOptions{RequireCompact: true})

	require.InDelta(t, 3.7, scored[0].Score(), 1e-12)
	require.InDelta(t, 0.7, scored[1].Score(), 1e-12)
}

func TestScoreCandidatesEqualResetWindowsReceiveLegacyFullFactor(t *testing.T) {
	candidates := []CandidateSnapshot{
		{AccountID: 1, Signals: CandidateSignals{ResetSeconds: AvailableSignal(100, timeZero)}},
		{AccountID: 2, Signals: CandidateSignals{ResetSeconds: AvailableSignal(100, timeZero)}},
	}

	scored := ScoreCandidates(candidates, ScoreWeights{Reset: 2}, ScoreOptions{})

	require.Equal(t, float64(2), scored[0].Breakdown.Reset)
	require.Equal(t, float64(2), scored[1].Breakdown.Reset)
}

func TestScoreCandidatesAcceptsAdapterResetNormalizationRange(t *testing.T) {
	candidates := []CandidateSnapshot{
		{AccountID: 1, Signals: CandidateSignals{ResetSeconds: AvailableSignal(90, timeZero)}},
		{AccountID: 2, Signals: CandidateSignals{ResetSeconds: AvailableSignal(190, timeZero)}},
	}
	rangeFromEarlierClock := &NormalizationRange{Min: 100, Max: 200, Available: true}

	scored := ScoreCandidates(candidates, ScoreWeights{Reset: 1}, ScoreOptions{ResetRange: rangeFromEarlierClock})

	require.Equal(t, float64(1), scored[0].Breakdown.Reset)
	require.InDelta(t, 0.1, scored[1].Breakdown.Reset, 1e-12)
}

func TestSortScoredCandidatesUsesLegacyTieBreakAndUnknownIsNotFavored(t *testing.T) {
	candidates := []ScoredCandidate{
		{Candidate: CandidateSnapshot{AccountID: 3, Priority: 1, Signals: CandidateSignals{LoadRate: UnavailableSignal()}}, Breakdown: ScoreBreakdown{Total: 10}},
		{Candidate: CandidateSnapshot{AccountID: 2, Priority: 1, Signals: CandidateSignals{LoadRate: AvailableSignal(20, timeZero), QueueDepth: AvailableSignal(2, timeZero)}}, Breakdown: ScoreBreakdown{Total: 10}},
		{Candidate: CandidateSnapshot{AccountID: 1, Priority: 0, Signals: CandidateSignals{LoadRate: AvailableSignal(90, timeZero)}}, Breakdown: ScoreBreakdown{Total: 10}},
	}

	SortScoredCandidates(candidates)
	require.Equal(t, []int64{1, 2, 3}, []int64{
		candidates[0].Candidate.AccountID,
		candidates[1].Candidate.AccountID,
		candidates[2].Candidate.AccountID,
	})
}

var timeZero = time.Time{}
