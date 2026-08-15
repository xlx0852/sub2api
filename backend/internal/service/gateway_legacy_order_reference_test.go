package service

// These pure helpers preserve the removed Gateway selector as a test-only
// characterization oracle. Production selection uses app/scheduling ordering.

import (
	"context"
	mathrand "math/rand"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

// Legacy private method names remain as test adapters so the existing platform
// and mixed-scheduling regression matrix exercises the shared Engine path.
func (s *GatewayService) selectAccountForModelWithPlatform(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, platform string) (*Account, error) {
	selection, err := AccountSelectionFromScheduling(NewGatewayAccountSelector(s).Select(ctx, scheduling.Request{
		Platform:       platform,
		ForcePlatform:  platform,
		GroupID:        groupID,
		StickyKey:      sessionHash,
		RequestedModel: requestedModel,
		ExcludedIDs:    excludedIDs,
		SkipAcquire:    true,
	}))
	if err != nil {
		return nil, err
	}
	return selection.Account, nil
}

func (s *GatewayService) selectAccountWithMixedScheduling(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, platform string) (*Account, error) {
	selection, err := AccountSelectionFromScheduling(NewGatewayAccountSelector(s).Select(ctx, scheduling.Request{
		Platform:           platform,
		AllowMixedPlatform: true,
		GroupID:            groupID,
		StickyKey:          sessionHash,
		RequestedModel:     requestedModel,
		ExcludedIDs:        excludedIDs,
		SkipAcquire:        true,
	}))
	if err != nil {
		return nil, err
	}
	return selection.Account, nil
}

func filterByMinPriority(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) == 0 {
		return accounts
	}
	minPriority := accounts[0].account.Priority
	for _, candidate := range accounts[1:] {
		if candidate.account.Priority < minPriority {
			minPriority = candidate.account.Priority
		}
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, candidate := range accounts {
		if candidate.account.Priority == minPriority {
			result = append(result, candidate)
		}
	}
	return result
}

func filterByMinLoadRate(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) == 0 {
		return accounts
	}
	minLoadRate := accounts[0].loadInfo.LoadRate
	for _, candidate := range accounts[1:] {
		if candidate.loadInfo.LoadRate < minLoadRate {
			minLoadRate = candidate.loadInfo.LoadRate
		}
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, candidate := range accounts {
		if candidate.loadInfo.LoadRate == minLoadRate {
			result = append(result, candidate)
		}
	}
	return result
}

func filterBySoonestReset(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) <= 1 {
		return accounts
	}
	now := time.Now()
	var minEnd *time.Time
	for _, candidate := range accounts {
		end := candidate.account.SessionWindowEnd
		if end == nil || !now.Before(*end) {
			continue
		}
		if minEnd == nil || end.Before(*minEnd) {
			minEnd = end
		}
	}
	if minEnd == nil {
		return accounts
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, candidate := range accounts {
		end := candidate.account.SessionWindowEnd
		if end != nil && now.Before(*end) && end.Equal(*minEnd) {
			result = append(result, candidate)
		}
	}
	return result
}

func selectByLRU(accounts []accountWithLoad, preferOAuth bool) *accountWithLoad {
	if len(accounts) == 0 {
		return nil
	}
	if len(accounts) == 1 {
		return &accounts[0]
	}
	var minTime *time.Time
	hasNil := false
	for _, candidate := range accounts {
		if candidate.account.LastUsedAt == nil {
			hasNil = true
			break
		}
		if minTime == nil || candidate.account.LastUsedAt.Before(*minTime) {
			minTime = candidate.account.LastUsedAt
		}
	}
	indices := make([]int, 0, len(accounts))
	for index, candidate := range accounts {
		if hasNil && candidate.account.LastUsedAt == nil ||
			!hasNil && candidate.account.LastUsedAt != nil && candidate.account.LastUsedAt.Equal(*minTime) {
			indices = append(indices, index)
		}
	}
	if preferOAuth {
		oauth := make([]int, 0, len(indices))
		for _, index := range indices {
			if accounts[index].account.Type == AccountTypeOAuth {
				oauth = append(oauth, index)
			}
		}
		if len(oauth) > 0 {
			indices = oauth
		}
	}
	return &accounts[indices[mathrand.Intn(len(indices))]]
}
