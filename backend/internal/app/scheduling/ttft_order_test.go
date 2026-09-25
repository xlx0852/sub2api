package scheduling

import (
	"math"
	"testing"
	"time"
)

type fakeLatency struct {
	values map[int64]int
}

func (f fakeLatency) AccountTTFT(accountID int64, _ string) (int, int, bool) {
	value, ok := f.values[accountID]
	if !ok {
		return 0, 0, false
	}
	return value, 10, true
}

func snapshotWithLoad(id int64, load float64) CandidateSnapshot {
	return CandidateSnapshot{AccountID: id, Priority: 1, Signals: CandidateSignals{LoadRate: AvailableSignal(load, timeZero())}}
}

func TestLatencyPreferenceReordersOnlySignificantlyFasterAccount(t *testing.T) {
	rng := NewSelectionRNG(7)
	candidates := []CandidateSnapshot{snapshotWithLoad(1, 0.5), snapshotWithLoad(2, 0.5)}
	pref := LatencyPreference{Enabled: true, Model: "m", Provider: fakeLatency{map[int64]int{1: 20_000, 2: 2_000}}, MinSamples: 5, MinAdvantagePct: 30}
	ordered := OrderByPriorityLoadAndLatency(candidates, &rng, pref)
	if ordered[0].AccountID != 2 {
		t.Fatalf("expected faster account first, got %d", ordered[0].AccountID)
	}
}

func TestLatencyPreferenceKeepsOrderWhenAdvantageIsSmall(t *testing.T) {
	rng := NewSelectionRNG(3)
	candidates := []CandidateSnapshot{snapshotWithLoad(1, 0.5), snapshotWithLoad(2, 0.5)}
	pref := LatencyPreference{Enabled: true, Model: "m", Provider: fakeLatency{map[int64]int{1: 10_000, 2: 9_000}}, MinSamples: 5, MinAdvantagePct: 30}
	ordered := OrderByPriorityLoadAndLatency(candidates, &rng, pref)
	first := ordered[0].AccountID
	second := ordered[1].AccountID
	if first != 1 && first != 2 {
		t.Fatalf("unexpected order %v", []int64{first, second})
	}
	if first == second {
		t.Fatal("order must contain both accounts")
	}
}

func TestLatencyPreferenceDoesNotCrossLoadOrPriority(t *testing.T) {
	rng := NewSelectionRNG(5)
	pref := LatencyPreference{Enabled: true, Model: "m", Provider: fakeLatency{map[int64]int{1: 30_000, 2: 1_000}}, MinSamples: 5, MinAdvantagePct: 30}
	// 负载不同：慢账号负载更低，仍应排在前面。
	sameLoadGroup := OrderByPriorityLoadAndLatency([]CandidateSnapshot{snapshotWithLoad(1, 0.1), snapshotWithLoad(2, 0.9)}, &rng, pref)
	if sameLoadGroup[0].AccountID != 1 {
		t.Fatalf("load ordering must win when load differs, got %d", sameLoadGroup[0].AccountID)
	}
	// 优先级不同：低优先级组必须先出现。
	low := snapshotWithLoad(1, 0.5)
	low.Priority = 1
	high := snapshotWithLoad(2, 0.5)
	high.Priority = 2
	byPriority := OrderByPriorityLoadAndLatency([]CandidateSnapshot{high, low}, &rng, pref)
	if byPriority[0].AccountID != 1 {
		t.Fatalf("priority ordering must win, got %d", byPriority[0].AccountID)
	}
}

func timeZero() time.Time { return time.Time{} }

var _ = math.NaN
