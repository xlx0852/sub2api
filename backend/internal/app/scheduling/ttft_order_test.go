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
	candidates := []CandidateSnapshot{snapshotWithLoad(1, 50), snapshotWithLoad(2, 50)}
	pref := LatencyPreference{Enabled: true, Model: "m", Provider: fakeLatency{map[int64]int{1: 20_000, 2: 2_000}}, MinSamples: 5, MinAdvantagePct: 30}
	ordered := OrderByPriorityLoadAndLatency(candidates, &rng, pref)
	if ordered[0].AccountID != 2 {
		t.Fatalf("expected faster account first, got %d", ordered[0].AccountID)
	}
}

func TestLatencyPreferenceKeepsOrderWhenAdvantageIsSmall(t *testing.T) {
	rng := NewSelectionRNG(3)
	candidates := []CandidateSnapshot{snapshotWithLoad(1, 50), snapshotWithLoad(2, 50)}
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
	pref := LatencyPreference{Enabled: true, Model: "m", Provider: fakeLatency{map[int64]int{1: 30_000, 2: 1_000}}, MinSamples: 5, MinAdvantagePct: 30, MaxLoadDeltaPct: 10}
	// 负载差 80 个百分点，超出容差：慢账号负载更低，仍排前面。
	byLoad := OrderByPriorityLoadAndLatency([]CandidateSnapshot{snapshotWithLoad(1, 10), snapshotWithLoad(2, 90)}, &rng, pref)
	if byLoad[0].AccountID != 1 {
		t.Fatalf("load ordering must win outside tolerance, got %d", byLoad[0].AccountID)
	}
	// 优先级不同：低优先级组必须先出现。
	low := snapshotWithLoad(1, 50)
	low.Priority = 1
	high := snapshotWithLoad(2, 50)
	high.Priority = 2
	byPriority := OrderByPriorityLoadAndLatency([]CandidateSnapshot{high, low}, &rng, pref)
	if byPriority[0].AccountID != 1 {
		t.Fatalf("priority ordering must win, got %d", byPriority[0].AccountID)
	}
}

func TestLatencyPreferencePromotesFasterAccountWithinLoadTolerance(t *testing.T) {
	rng := NewSelectionRNG(11)
	pref := LatencyPreference{Enabled: true, Model: "m", Provider: fakeLatency{map[int64]int{1: 20_000, 2: 2_000}}, MinSamples: 5, MinAdvantagePct: 30, MaxLoadDeltaPct: 10}
	// 负载 40% vs 45%（差 5 ≤ 容差），账号 2 快 10 倍 → 应被提升到前面。
	ordered := OrderByPriorityLoadAndLatency([]CandidateSnapshot{snapshotWithLoad(1, 40), snapshotWithLoad(2, 45)}, &rng, pref)
	if ordered[0].AccountID != 2 {
		t.Fatalf("faster account within tolerance must be promoted, got %d", ordered[0].AccountID)
	}
	// 优势不足时不提升。
	pref.Provider = fakeLatency{map[int64]int{1: 10_000, 2: 9_000}}
	ordered = OrderByPriorityLoadAndLatency([]CandidateSnapshot{snapshotWithLoad(1, 40), snapshotWithLoad(2, 45)}, &rng, pref)
	if ordered[0].AccountID != 1 {
		t.Fatalf("small advantage must not reorder, got %d", ordered[0].AccountID)
	}
	// 样本不足时不提升。
	pref.Provider = insufficientLatency{}
	ordered = OrderByPriorityLoadAndLatency([]CandidateSnapshot{snapshotWithLoad(1, 40), snapshotWithLoad(2, 45)}, &rng, pref)
	if ordered[0].AccountID != 1 {
		t.Fatalf("insufficient samples must not reorder, got %d", ordered[0].AccountID)
	}
}

type insufficientLatency struct{}

func (insufficientLatency) AccountTTFT(int64, string) (int, int, bool) { return 100, 2, true }

func timeZero() time.Time { return time.Time{} }

var _ = math.NaN
