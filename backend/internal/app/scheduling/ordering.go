package scheduling

import (
	mathrand "math/rand"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ComparePriorityLRU 比较两账号的公共排序键：较小 Priority 优先；
// 同优先级下从未使用优先，否则 LastUsedAt 更早者优先。
// 返回 -1 表示 left 优先，1 表示 right 优先，0 表示公共键相同。
func ComparePriorityLRU(left, right *domain.Account) int {
	if left == nil || right == nil {
		switch {
		case left == nil && right == nil:
			return 0
		case left == nil:
			return 1
		default:
			return -1
		}
	}
	if left.Priority < right.Priority {
		return -1
	}
	if left.Priority > right.Priority {
		return 1
	}
	switch {
	case left.LastUsedAt == nil && right.LastUsedAt != nil:
		return -1
	case left.LastUsedAt != nil && right.LastUsedAt == nil:
		return 1
	case left.LastUsedAt == nil:
		return 0
	case left.LastUsedAt.Before(*right.LastUsedAt):
		return -1
	case right.LastUsedAt.Before(*left.LastUsedAt):
		return 1
	default:
		return 0
	}
}

// SameLastUsedSecond preserves the legacy scheduler grouping precision.
func SameLastUsedSecond(left, right *time.Time) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return left.Unix() == right.Unix()
	}
}

// ShuffleEqualRuns randomizes only contiguous runs whose complete sort keys
// are equal according to sameGroup.
func ShuffleEqualRuns[T any](items []T, sameGroup func(T, T) bool) {
	for start := 0; start < len(items); {
		end := start + 1
		for end < len(items) && sameGroup(items[start], items[end]) {
			end++
		}
		if end-start > 1 {
			mathrand.Shuffle(end-start, func(i, j int) {
				items[start+i], items[start+j] = items[start+j], items[start+i]
			})
		}
		start = end
	}
}

// SortPriorityLRU preserves the existing priority/last-used order and optional
// preferred-account partition, then shuffles only equal-key partitions.
func SortPriorityLRU(accounts []*domain.Account, prefer func(*domain.Account) bool) {
	sort.SliceStable(accounts, func(i, j int) bool {
		left, right := accounts[i], accounts[j]
		if order := ComparePriorityLRU(left, right); order != 0 {
			return order < 0
		}
		if prefer != nil && left.LastUsedAt == nil && right.LastUsedAt == nil && prefer(left) != prefer(right) {
			return prefer(left)
		}
		return false
	})
	ShufflePriorityLRURuns(accounts, prefer)
}

// ShufflePriorityLRURuns preserves preferred/non-preferred partitions inside
// each equal priority/last-used run.
func ShufflePriorityLRURuns(accounts []*domain.Account, prefer func(*domain.Account) bool) {
	for start := 0; start < len(accounts); {
		end := start + 1
		for end < len(accounts) && samePriorityLRUGroup(accounts[start], accounts[end]) {
			end++
		}
		if prefer == nil {
			ShuffleEqualRuns(accounts[start:end], samePriorityLRUGroup)
		} else {
			preferred := make([]*domain.Account, 0, end-start)
			others := make([]*domain.Account, 0, end-start)
			for _, account := range accounts[start:end] {
				if prefer(account) {
					preferred = append(preferred, account)
				} else {
					others = append(others, account)
				}
			}
			ShuffleEqualRuns(preferred, samePriorityLRUGroup)
			ShuffleEqualRuns(others, samePriorityLRUGroup)
			copy(accounts[start:], preferred)
			copy(accounts[start+len(preferred):], others)
		}
		start = end
	}
}

func samePriorityLRUGroup(left, right *domain.Account) bool {
	return left.Priority == right.Priority && SameLastUsedSecond(left.LastUsedAt, right.LastUsedAt)
}
