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

// LoadCandidate 是平台 adapter 提供给共享兼容排序器的最小负载快照。
// Account 保留平台凭证之外的调度元数据；LoadRate 只在 UseLoad=true 时参与排序。
type LoadCandidate struct {
	Account  *domain.Account
	LoadRate int
}

type PriorityLoadOrderOptions struct {
	UseLoad            bool
	PreferSoonestReset bool
	Prefer             func(*domain.Account) bool
	Now                time.Time
}

// OrderPriorityLoadLRU 把 Gateway 旧的分层选择表达为共享、无副作用的顺序：
// priority -> optional active reset -> load -> LRU -> preferred credential.
// 完全相同的键仍组内打散，避免并发请求读取同一快照时形成热点。
func OrderPriorityLoadLRU(candidates []LoadCandidate, options PriorityLoadOrderOptions) []LoadCandidate {
	result := make([]LoadCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Account == nil || options.UseLoad && candidate.LoadRate >= 100 {
			continue
		}
		result = append(result, candidate)
	}
	if len(result) <= 1 {
		return result
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Account.Priority != right.Account.Priority {
			return left.Account.Priority < right.Account.Priority
		}
		if options.PreferSoonestReset {
			leftReset, leftActive := activeReset(left.Account, now)
			rightReset, rightActive := activeReset(right.Account, now)
			if leftActive != rightActive {
				return leftActive
			}
			if leftActive && !leftReset.Equal(rightReset) {
				return leftReset.Before(rightReset)
			}
		}
		if options.UseLoad && left.LoadRate != right.LoadRate {
			return left.LoadRate < right.LoadRate
		}
		if order := compareLastUsed(left.Account.LastUsedAt, right.Account.LastUsedAt); order != 0 {
			return order < 0
		}
		if options.Prefer != nil && options.Prefer(left.Account) != options.Prefer(right.Account) {
			return options.Prefer(left.Account)
		}
		return false
	})
	ShuffleEqualRuns(result, func(left, right LoadCandidate) bool {
		return samePriorityLoadOrderGroup(left, right, options, now)
	})
	return result
}

// BestPriorityLoadLRUTies 返回兼容排序第一层的全部等价候选，供 shadow
// 比较避免把同键随机打散误报为选择差异。
func BestPriorityLoadLRUTies(candidates []LoadCandidate, options PriorityLoadOrderOptions) []LoadCandidate {
	ordered := OrderPriorityLoadLRU(candidates, options)
	if len(ordered) <= 1 {
		return ordered
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	end := 1
	for end < len(ordered) && samePriorityLoadOrderGroup(ordered[0], ordered[end], options, now) {
		end++
	}
	return ordered[:end]
}

func samePriorityLoadOrderGroup(left, right LoadCandidate, options PriorityLoadOrderOptions, now time.Time) bool {
	if left.Account.Priority != right.Account.Priority {
		return false
	}
	if options.PreferSoonestReset {
		leftReset, leftActive := activeReset(left.Account, now)
		rightReset, rightActive := activeReset(right.Account, now)
		if leftActive != rightActive || leftActive && !leftReset.Equal(rightReset) {
			return false
		}
	}
	if options.UseLoad && left.LoadRate != right.LoadRate {
		return false
	}
	if compareLastUsed(left.Account.LastUsedAt, right.Account.LastUsedAt) != 0 {
		return false
	}
	return options.Prefer == nil || options.Prefer(left.Account) == options.Prefer(right.Account)
}

func activeReset(account *domain.Account, now time.Time) (time.Time, bool) {
	if account == nil || account.SessionWindowEnd == nil || !now.Before(*account.SessionWindowEnd) {
		return time.Time{}, false
	}
	return *account.SessionWindowEnd, true
}

func compareLastUsed(left, right *time.Time) int {
	switch {
	case left == nil && right == nil:
		return 0
	case left == nil:
		return -1
	case right == nil:
		return 1
	case left.Before(*right):
		return -1
	case right.Before(*left):
		return 1
	default:
		return 0
	}
}
