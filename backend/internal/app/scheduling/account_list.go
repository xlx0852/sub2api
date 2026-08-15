package scheduling

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type AccountListSnapshot interface {
	ListSchedulableAccounts(ctx context.Context, groupID *int64, platform string, hasForcePlatform bool) ([]domain.Account, bool, error)
}

type AccountListRepository interface {
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]domain.Account, error)
	ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]domain.Account, error)
	ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]domain.Account, error)
	ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]domain.Account, error)
}

type AccountListRequest struct {
	GroupID          *int64
	Platform         string
	HasForcePlatform bool
	UseMixed         bool
	RepoPlatforms    []string
	RepoFilter       func(domain.Account) bool
}

type AccountListResult struct {
	Accounts     []domain.Account
	UseMixed     bool
	FromSnapshot bool
	RawCount     int
}

// ListAccounts uses the scheduling snapshot when present and otherwise runs
// the same grouped/ungrouped repository query skeleton for every Strategy.
func ListAccounts(ctx context.Context, snapshot AccountListSnapshot, repo AccountListRepository, request AccountListRequest) (AccountListResult, error) {
	if snapshot != nil {
		accounts, useMixed, err := snapshot.ListSchedulableAccounts(ctx, request.GroupID, request.Platform, request.HasForcePlatform)
		return AccountListResult{Accounts: accounts, UseMixed: useMixed, FromSnapshot: true, RawCount: len(accounts)}, err
	}

	platforms := request.RepoPlatforms
	if len(platforms) == 0 {
		platforms = []string{request.Platform}
	}
	var (
		accounts []domain.Account
		err      error
	)
	if request.GroupID != nil {
		if len(platforms) == 1 {
			accounts, err = repo.ListSchedulableByGroupIDAndPlatform(ctx, *request.GroupID, platforms[0])
		} else {
			accounts, err = repo.ListSchedulableByGroupIDAndPlatforms(ctx, *request.GroupID, platforms)
		}
	} else if len(platforms) == 1 {
		accounts, err = repo.ListSchedulableUngroupedByPlatform(ctx, platforms[0])
	} else {
		accounts, err = repo.ListSchedulableUngroupedByPlatforms(ctx, platforms)
	}
	result := AccountListResult{Accounts: accounts, UseMixed: request.UseMixed, RawCount: len(accounts)}
	if err != nil || request.RepoFilter == nil {
		return result, err
	}
	filtered := make([]domain.Account, 0, len(accounts))
	for _, account := range accounts {
		if request.RepoFilter(account) {
			filtered = append(filtered, account)
		}
	}
	result.Accounts = filtered
	return result, nil
}
