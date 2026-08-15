package scheduling

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectTopKUsesScoreAndLegacyTieBreak(t *testing.T) {
	candidates := []ScoredCandidate{
		{Candidate: CandidateSnapshot{AccountID: 4, Priority: 0}, Breakdown: ScoreBreakdown{Total: 1}},
		{Candidate: CandidateSnapshot{AccountID: 3, Priority: 1}, Breakdown: ScoreBreakdown{Total: 9}},
		{Candidate: CandidateSnapshot{AccountID: 2, Priority: 0}, Breakdown: ScoreBreakdown{Total: 9}},
		{Candidate: CandidateSnapshot{AccountID: 1, Priority: 0}, Breakdown: ScoreBreakdown{Total: 10}},
	}

	top := SelectTopK(candidates, 3)
	require.Equal(t, []int64{1, 2, 3}, scoredCandidateIDs(top))
}

func TestBuildWeightedSelectionOrderIsReplayableWithFixedSeed(t *testing.T) {
	candidates := []ScoredCandidate{
		{Candidate: CandidateSnapshot{AccountID: 1}, Breakdown: ScoreBreakdown{Total: 10}},
		{Candidate: CandidateSnapshot{AccountID: 2}, Breakdown: ScoreBreakdown{Total: 5}},
		{Candidate: CandidateSnapshot{AccountID: 3}, Breakdown: ScoreBreakdown{Total: 1}},
	}
	firstRNG := NewSelectionRNG(12345)
	secondRNG := NewSelectionRNG(12345)

	first := BuildWeightedSelectionOrder(candidates, &firstRNG)
	second := BuildWeightedSelectionOrder(candidates, &secondRNG)

	require.Equal(t, scoredCandidateIDs(first), scoredCandidateIDs(second))
	require.ElementsMatch(t, []int64{1, 2, 3}, scoredCandidateIDs(first))
}

func TestDeriveSelectionSeedPreservesAnchoredAndUnanchoredEntropyRules(t *testing.T) {
	groupID := int64(7)
	anchored := SelectionSeedInput{StickyKey: "session", RequestedModel: "gpt", GroupID: &groupID, Entropy: 1}
	anchoredOtherEntropy := anchored
	anchoredOtherEntropy.Entropy = 2
	require.Equal(t, DeriveSelectionSeed(anchored), DeriveSelectionSeed(anchoredOtherEntropy))

	unanchored := SelectionSeedInput{RequestedModel: "gpt", GroupID: &groupID, Entropy: 1}
	unanchoredOtherEntropy := unanchored
	unanchoredOtherEntropy.Entropy = 2
	require.NotEqual(t, DeriveSelectionSeed(unanchored), DeriveSelectionSeed(unanchoredOtherEntropy))
}

func scoredCandidateIDs(candidates []ScoredCandidate) []int64 {
	result := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.Candidate.AccountID)
	}
	return result
}
