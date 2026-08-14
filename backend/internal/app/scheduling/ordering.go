package scheduling

import "github.com/Wei-Shaw/sub2api/internal/domain"

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
