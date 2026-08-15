package scheduling

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestComparePriorityLRU(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()

	tests := []struct {
		name  string
		left  *domain.Account
		right *domain.Account
		want  int
	}{
		{name: "lower priority", left: &domain.Account{Priority: 1}, right: &domain.Account{Priority: 2}, want: -1},
		{name: "never used", left: &domain.Account{Priority: 1}, right: &domain.Account{Priority: 1, LastUsedAt: &older}, want: -1},
		{name: "older last used", left: &domain.Account{Priority: 1, LastUsedAt: &older}, right: &domain.Account{Priority: 1, LastUsedAt: &newer}, want: -1},
		{name: "same keys", left: &domain.Account{Priority: 1, LastUsedAt: &older}, right: &domain.Account{Priority: 1, LastUsedAt: &older}, want: 0},
		{name: "nil sorts last", left: nil, right: &domain.Account{}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ComparePriorityLRU(tt.left, tt.right))
		})
	}
}

func TestSameLastUsedSecond(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	sameSecond := now.Add(500 * time.Millisecond)
	different := now.Add(time.Second)
	require.True(t, SameLastUsedSecond(nil, nil))
	require.False(t, SameLastUsedSecond(nil, &now))
	require.True(t, SameLastUsedSecond(&now, &sameSecond))
	require.False(t, SameLastUsedSecond(&now, &different))
}

func TestSortPriorityLRUPreservesGroupsAndPreferredPartition(t *testing.T) {
	used := time.Now().Add(-time.Hour).Truncate(time.Second)
	accounts := []*domain.Account{
		{ID: 4, Priority: 2},
		{ID: 3, Priority: 1, LastUsedAt: &used},
		{ID: 2, Priority: 1, Type: "preferred"},
		{ID: 1, Priority: 1},
	}
	SortPriorityLRU(accounts, func(account *domain.Account) bool { return account.Type == "preferred" })
	require.Equal(t, int64(2), accounts[0].ID)
	require.Equal(t, int64(1), accounts[1].ID)
	require.Equal(t, int64(3), accounts[2].ID)
	require.Equal(t, int64(4), accounts[3].ID)
}

func TestShuffleEqualRunsDoesNotCrossGroupBoundary(t *testing.T) {
	type item struct{ group, id int }
	items := []item{{1, 1}, {1, 2}, {2, 3}, {2, 4}}
	for range 20 {
		copyItems := append([]item(nil), items...)
		ShuffleEqualRuns(copyItems, func(left, right item) bool { return left.group == right.group })
		require.Equal(t, 1, copyItems[0].group)
		require.Equal(t, 1, copyItems[1].group)
		require.Equal(t, 2, copyItems[2].group)
		require.Equal(t, 2, copyItems[3].group)
	}
}
