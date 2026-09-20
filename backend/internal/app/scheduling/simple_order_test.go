package scheduling

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
	"time"
)

func TestOrderByPriorityAndLoad(t *testing.T) {
	candidates := []CandidateSnapshot{
		{AccountID: 1, Priority: 1, Signals: CandidateSignals{LoadRate: AvailableSignal(20, time.Time{})}},
		{AccountID: 2, Priority: 1, Signals: CandidateSignals{LoadRate: AvailableSignal(0, time.Time{})}},
		{AccountID: 3, Priority: 0, Signals: CandidateSignals{LoadRate: AvailableSignal(100, time.Time{})}},
		{AccountID: 4, Priority: 1},
		{AccountID: 5, Priority: 2, Signals: CandidateSignals{LoadRate: AvailableSignal(0, time.Time{})}},
	}
	rng := NewSelectionRNG(42)
	got := OrderByPriorityAndLoad(candidates, &rng)
	var ids []int64
	for _, c := range got {
		ids = append(ids, c.AccountID)
	}
	require.Equal(t, []int64{3, 2, 1, 4, 5}, ids)
	require.Equal(t, int64(1), candidates[0].AccountID, "does not mutate caller's order")
}

func TestOrderByPriorityAndLoad_UnknownHistoryDoesNotStarve(t *testing.T) {
	a := CandidateSnapshot{AccountID: 132, Priority: 1, Signals: CandidateSignals{LoadRate: AvailableSignal(0, time.Time{})}}
	b := a
	b.AccountID = 112
	b.Signals.ErrorRate = AvailableSignal(0, time.Time{})
	b.Signals.TTFTMillis = AvailableSignal(1, time.Time{})
	b.Signals.QuotaHeadroom = AvailableSignal(1, time.Time{})
	seen := map[int64]bool{}
	for seed := uint64(1); seed <= 100; seed++ {
		rng := NewSelectionRNG(seed)
		seen[OrderByPriorityAndLoad([]CandidateSnapshot{a, b}, &rng)[0].AccountID] = true
	}
	require.Len(t, seen, 2)
}

func TestOrderByPriorityAndLoad_MissingAndInvalidLoad(t *testing.T) {
	cs := []CandidateSnapshot{{AccountID: 1}, {AccountID: 2, Signals: CandidateSignals{LoadRate: AvailableSignal(math.NaN(), time.Time{})}}, {AccountID: 3, Signals: CandidateSignals{LoadRate: AvailableSignal(-1, time.Time{})}}}
	seen := map[int64]bool{}
	for seed := uint64(1); seed <= 100; seed++ {
		rng := NewSelectionRNG(seed)
		got := OrderByPriorityAndLoad(cs, &rng)
		require.Len(t, got, 3)
		seen[got[0].AccountID] = true
	}
	require.Len(t, seen, 3)
	require.Empty(t, OrderByPriorityAndLoad(nil, nil))
}
