//go:build unit

package service

import "time"

// 这些 helper 只服务于尚未迁移到 AccountSelector 的旧单元测试。
// 关键约束：它们不参与生产构建；生产调度逻辑必须通过 AccountSelector/strategy，
// 不得重新依赖这些测试兼容函数。

func int64Ptr(v int64) *int64 { return &v }

func filterByMinPriority(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) == 0 {
		return nil
	}
	min := accounts[0].account.Priority
	for _, candidate := range accounts[1:] {
		if candidate.account.Priority < min {
			min = candidate.account.Priority
		}
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, candidate := range accounts {
		if candidate.account.Priority == min {
			result = append(result, candidate)
		}
	}
	return result
}

func filterByMinLoadRate(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) == 0 {
		return nil
	}
	load := func(candidate accountWithLoad) int {
		if candidate.loadInfo == nil {
			return 0
		}
		return candidate.loadInfo.LoadRate
	}
	min := load(accounts[0])
	for _, candidate := range accounts[1:] {
		if value := load(candidate); value < min {
			min = value
		}
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, candidate := range accounts {
		if load(candidate) == min {
			result = append(result, candidate)
		}
	}
	return result
}

func selectByLRU(accounts []accountWithLoad, preferOAuth bool) *accountWithLoad {
	if len(accounts) == 0 {
		return nil
	}
	best := accounts[0]
	for _, candidate := range accounts[1:] {
		if candidate.account == nil {
			continue
		}
		if best.account == nil {
			best = candidate
			continue
		}
		switch {
		case best.account.LastUsedAt == nil && candidate.account.LastUsedAt != nil:
			continue
		case best.account.LastUsedAt != nil && candidate.account.LastUsedAt == nil:
			best = candidate
		case best.account.LastUsedAt != nil && candidate.account.LastUsedAt != nil && candidate.account.LastUsedAt.Before(*best.account.LastUsedAt):
			best = candidate
		case sameTestTime(best.account.LastUsedAt, candidate.account.LastUsedAt) && preferOAuth && candidate.account.Type == AccountTypeOAuth && best.account.Type != AccountTypeOAuth:
			best = candidate
		}
	}
	return &best
}

func sameTestTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func filterBySoonestReset(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) == 0 {
		return nil
	}
	now := time.Now()
	var earliest *time.Time
	for _, candidate := range accounts {
		if candidate.account == nil || candidate.account.SessionWindowEnd == nil || !candidate.account.SessionWindowEnd.After(now) {
			continue
		}
		if earliest == nil || candidate.account.SessionWindowEnd.Before(*earliest) {
			value := *candidate.account.SessionWindowEnd
			earliest = &value
		}
	}
	if earliest == nil {
		return accounts
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, candidate := range accounts {
		if candidate.account != nil && candidate.account.SessionWindowEnd != nil &&
			!candidate.account.SessionWindowEnd.Before(*earliest) && !candidate.account.SessionWindowEnd.After(*earliest) {
			result = append(result, candidate)
		}
	}
	return result
}
