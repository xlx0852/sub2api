package scheduling

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

type accountListSnapshotFake struct {
	accounts []domain.Account
	mixed    bool
	calls    int
}

func (f *accountListSnapshotFake) ListSchedulableAccounts(context.Context, *int64, string, bool) ([]domain.Account, bool, error) {
	f.calls++
	return f.accounts, f.mixed, nil
}

type accountListRepoFake struct {
	call     string
	accounts []domain.Account
}

func (f *accountListRepoFake) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]domain.Account, error) {
	f.call = "group_single"
	return f.accounts, nil
}
func (f *accountListRepoFake) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]domain.Account, error) {
	f.call = "group_multi"
	return f.accounts, nil
}
func (f *accountListRepoFake) ListSchedulableUngroupedByPlatform(context.Context, string) ([]domain.Account, error) {
	f.call = "ungrouped_single"
	return f.accounts, nil
}
func (f *accountListRepoFake) ListSchedulableUngroupedByPlatforms(context.Context, []string) ([]domain.Account, error) {
	f.call = "ungrouped_multi"
	return f.accounts, nil
}

func TestListAccountsUsesSnapshotFirst(t *testing.T) {
	snapshot := &accountListSnapshotFake{accounts: []domain.Account{{ID: 1}}, mixed: true}
	repo := &accountListRepoFake{}
	result, err := ListAccounts(context.Background(), snapshot, repo, AccountListRequest{Platform: "anthropic"})
	require.NoError(t, err)
	require.True(t, result.FromSnapshot)
	require.True(t, result.UseMixed)
	require.Equal(t, []int64{1}, []int64{result.Accounts[0].ID})
	require.Empty(t, repo.call)
}

func TestListAccountsRepositoryBranchesAndFilter(t *testing.T) {
	groupID := int64(7)
	tests := []struct {
		name, wantCall string
		groupID        *int64
		platforms      []string
	}{
		{name: "group single", wantCall: "group_single", groupID: &groupID, platforms: []string{"openai"}},
		{name: "group multi", wantCall: "group_multi", groupID: &groupID, platforms: []string{"anthropic", "antigravity"}},
		{name: "ungrouped single", wantCall: "ungrouped_single", platforms: []string{"openai"}},
		{name: "ungrouped multi", wantCall: "ungrouped_multi", platforms: []string{"anthropic", "antigravity"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &accountListRepoFake{accounts: []domain.Account{{ID: 1}, {ID: 2}}}
			result, err := ListAccounts(context.Background(), nil, repo, AccountListRequest{
				GroupID: test.groupID, RepoPlatforms: test.platforms, UseMixed: len(test.platforms) > 1,
				RepoFilter: func(account domain.Account) bool { return account.ID == 2 },
			})
			require.NoError(t, err)
			require.Equal(t, test.wantCall, repo.call)
			require.Equal(t, 2, result.RawCount)
			require.Equal(t, []int64{2}, []int64{result.Accounts[0].ID})
		})
	}
}
