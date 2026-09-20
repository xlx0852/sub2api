//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- helpers ---

func testTimePtr(t time.Time) *time.Time { return &t }

func makeAccWithLoad(id int64, priority int, loadRate int, lastUsed *time.Time, accType string) accountWithLoad {
	return accountWithLoad{
		account: &Account{
			ID:          id,
			Priority:    priority,
			LastUsedAt:  lastUsed,
			Type:        accType,
			Schedulable: true,
			Status:      StatusActive,
		},
		loadInfo: &AccountLoadInfo{
			AccountID:          id,
			CurrentConcurrency: 0,
			LoadRate:           loadRate,
		},
	}
}

// --- sortAccountsByPriorityAndLastUsed ---

// --- filterByMinPriority ---

func TestFilterByMinPriority_Empty(t *testing.T) {
	result := filterByMinPriority(nil)
	require.Nil(t, result)
}

func TestFilterByMinPriority_SelectsMinPriority(t *testing.T) {
	accounts := []accountWithLoad{
		makeAccWithLoad(1, 5, 10, nil, AccountTypeAPIKey),
		makeAccWithLoad(2, 1, 10, nil, AccountTypeAPIKey),
		makeAccWithLoad(3, 1, 20, nil, AccountTypeAPIKey),
		makeAccWithLoad(4, 2, 10, nil, AccountTypeAPIKey),
	}
	result := filterByMinPriority(accounts)
	require.Len(t, result, 2)
	require.Equal(t, int64(2), result[0].account.ID)
	require.Equal(t, int64(3), result[1].account.ID)
}

// --- filterByMinLoadRate ---

func TestFilterByMinLoadRate_Empty(t *testing.T) {
	result := filterByMinLoadRate(nil)
	require.Nil(t, result)
}

func TestFilterByMinLoadRate_SelectsMinLoadRate(t *testing.T) {
	accounts := []accountWithLoad{
		makeAccWithLoad(1, 1, 30, nil, AccountTypeAPIKey),
		makeAccWithLoad(2, 1, 10, nil, AccountTypeAPIKey),
		makeAccWithLoad(3, 1, 10, nil, AccountTypeAPIKey),
		makeAccWithLoad(4, 1, 20, nil, AccountTypeAPIKey),
	}
	result := filterByMinLoadRate(accounts)
	require.Len(t, result, 2)
	require.Equal(t, int64(2), result[0].account.ID)
	require.Equal(t, int64(3), result[1].account.ID)
}

// --- selectByLRU ---

func TestSelectByLRU_Empty(t *testing.T) {
	result := selectByLRU(nil, false)
	require.Nil(t, result)
}

func TestSelectByLRU_Single(t *testing.T) {
	accounts := []accountWithLoad{makeAccWithLoad(1, 1, 10, nil, AccountTypeAPIKey)}
	result := selectByLRU(accounts, false)
	require.NotNil(t, result)
	require.Equal(t, int64(1), result.account.ID)
}

func TestSelectByLRU_NilLastUsedAtWins(t *testing.T) {
	now := time.Now()
	accounts := []accountWithLoad{
		makeAccWithLoad(1, 1, 10, testTimePtr(now), AccountTypeAPIKey),
		makeAccWithLoad(2, 1, 10, nil, AccountTypeAPIKey),
		makeAccWithLoad(3, 1, 10, testTimePtr(now.Add(-1*time.Hour)), AccountTypeAPIKey),
	}
	result := selectByLRU(accounts, false)
	require.NotNil(t, result)
	require.Equal(t, int64(2), result.account.ID)
}

func TestSelectByLRU_EarliestTimeWins(t *testing.T) {
	now := time.Now()
	accounts := []accountWithLoad{
		makeAccWithLoad(1, 1, 10, testTimePtr(now), AccountTypeAPIKey),
		makeAccWithLoad(2, 1, 10, testTimePtr(now.Add(-1*time.Hour)), AccountTypeAPIKey),
		makeAccWithLoad(3, 1, 10, testTimePtr(now.Add(-2*time.Hour)), AccountTypeAPIKey),
	}
	result := selectByLRU(accounts, false)
	require.NotNil(t, result)
	require.Equal(t, int64(3), result.account.ID)
}

func TestSelectByLRU_TiePreferOAuth(t *testing.T) {
	now := time.Now()
	// 账号 1/2 LastUsedAt 相同，且同为最小值。
	accounts := []accountWithLoad{
		makeAccWithLoad(1, 1, 10, testTimePtr(now), AccountTypeAPIKey),
		makeAccWithLoad(2, 1, 10, testTimePtr(now), AccountTypeOAuth),
		makeAccWithLoad(3, 1, 10, testTimePtr(now.Add(1*time.Hour)), AccountTypeAPIKey),
	}
	for i := 0; i < 50; i++ {
		result := selectByLRU(accounts, true)
		require.NotNil(t, result)
		require.Equal(t, AccountTypeOAuth, result.account.Type)
		require.Equal(t, int64(2), result.account.ID)
	}
}
