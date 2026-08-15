package scheduling

import (
	"container/heap"
	"hash/fnv"
	"math"
	"strconv"
	"strings"
)

type scoredCandidateHeap []ScoredCandidate

func (h scoredCandidateHeap) Len() int { return len(h) }

func (h scoredCandidateHeap) Less(i, j int) bool {
	// 最小堆根节点保存当前 topK 中最差候选。
	return ScoredCandidateBetter(h[j], h[i])
}

func (h scoredCandidateHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *scoredCandidateHeap) Push(value any) {
	*h = append(*h, value.(ScoredCandidate))
}

func (h *scoredCandidateHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}

func SelectTopK(candidates []ScoredCandidate, topK int) []ScoredCandidate {
	if len(candidates) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = 1
	}
	if topK >= len(candidates) {
		result := append([]ScoredCandidate(nil), candidates...)
		SortScoredCandidates(result)
		return result
	}

	best := make(scoredCandidateHeap, 0, topK)
	for _, candidate := range candidates {
		if len(best) < topK {
			heap.Push(&best, candidate)
			continue
		}
		if ScoredCandidateBetter(candidate, best[0]) {
			best[0] = candidate
			heap.Fix(&best, 0)
		}
	}
	result := append([]ScoredCandidate(nil), best...)
	SortScoredCandidates(result)
	return result
}

type SelectionRNG struct {
	state uint64
}

func NewSelectionRNG(seed uint64) SelectionRNG {
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	return SelectionRNG{state: seed}
}

func (r *SelectionRNG) NextUint64() uint64 {
	x := r.state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	r.state = x
	return x * 2685821657736338717
}

func (r *SelectionRNG) NextFloat64() float64 {
	return float64(r.NextUint64()>>11) / (1 << 53)
}

type SelectionSeedInput struct {
	StickyKey          string
	PreviousResponseID string
	RequestedModel     string
	GroupID            *int64
	// Entropy 由生产 adapter 在没有会话锚点时传入 time.Now().UnixNano()；
	// 测试传固定值，避免纯函数直接读取时钟。
	Entropy uint64
}

func DeriveSelectionSeed(input SelectionSeedInput) uint64 {
	hasher := fnv.New64a()
	writeValue := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		_, _ = hasher.Write([]byte(trimmed))
		_, _ = hasher.Write([]byte{0})
	}
	writeValue(input.StickyKey)
	writeValue(input.PreviousResponseID)
	writeValue(input.RequestedModel)
	if input.GroupID != nil {
		_, _ = hasher.Write([]byte(strconv.FormatInt(*input.GroupID, 10)))
	}
	seed := hasher.Sum64()
	if strings.TrimSpace(input.StickyKey) == "" && strings.TrimSpace(input.PreviousResponseID) == "" {
		seed ^= input.Entropy
	}
	if seed == 0 {
		seed = input.Entropy ^ 0x9e3779b97f4a7c15
	}
	return seed
}

// BuildWeightedSelectionOrder 使用与现有 OpenAI 调度器一致的分数平移和
// 加权无放回抽样。调用方应先用 SelectTopK 约束候选池。
func BuildWeightedSelectionOrder(candidates []ScoredCandidate, rng *SelectionRNG) []ScoredCandidate {
	if len(candidates) <= 1 {
		return append([]ScoredCandidate(nil), candidates...)
	}
	if rng == nil {
		fallback := NewSelectionRNG(0)
		rng = &fallback
	}

	pool := append([]ScoredCandidate(nil), candidates...)
	weights := make([]float64, len(pool))
	minScore := pool[0].Score()
	for i := 1; i < len(pool); i++ {
		if pool[i].Score() < minScore {
			minScore = pool[i].Score()
		}
	}
	for i := range pool {
		weight := (pool[i].Score() - minScore) + 1
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
			weight = 1
		}
		weights[i] = weight
	}

	result := make([]ScoredCandidate, 0, len(pool))
	for len(pool) > 0 {
		total := 0.0
		for _, weight := range weights {
			total += weight
		}
		selected := 0
		if total > 0 {
			value := rng.NextFloat64() * total
			accumulated := 0.0
			for i, weight := range weights {
				accumulated += weight
				if value <= accumulated {
					selected = i
					break
				}
			}
		} else {
			selected = int(rng.NextUint64() % uint64(len(pool)))
		}
		result = append(result, pool[selected])
		pool = append(pool[:selected], pool[selected+1:]...)
		weights = append(weights[:selected], weights[selected+1:]...)
	}
	return result
}
