//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAICredentialClassFilterInLoadBalance(t *testing.T) {
	oauth := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	require.True(t, OpenAICredentialClassMatches(oauth, OpenAICredentialClassOAuth))
	require.False(t, OpenAICredentialClassMatches(apiKey, OpenAICredentialClassOAuth))
	require.True(t, OpenAICredentialClassMatches(apiKey, OpenAICredentialClassAPIKey))
	require.Equal(t, OpenAICredentialClassOAuth, OpenAIAccountCredentialClass(oauth))
	require.Equal(t, OpenAICredentialClassAPIKey, OpenAIAccountCredentialClass(apiKey))
}

func TestComputeOpenAIQuotaCooldownUntilFallsBackDefault(t *testing.T) {
	now := time.Now()
	until := ComputeOpenAIQuotaCooldownUntil(nil, nil, now)
	require.WithinDuration(t, now.Add(openAIDefaultQuotaCooldown), until, time.Second)
}

func TestComputeOpenAIUsageScorePrefersHottestWindow(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{Extra: map[string]any{
		"codex_primary_used_percent":   12.0,
		"codex_secondary_used_percent": 88.0,
		"codex_usage_updated_at":       now.Format(time.RFC3339),
	}}
	require.Equal(t, 88.0, ComputeOpenAIUsageScore(account, now))
}

func TestFreshSessionCandidateOrderByUsage(t *testing.T) {
	now := time.Now()
	candidates := []openAIAccountCandidateScore{
		{account: &Account{ID: 1, Extra: map[string]any{
			"codex_primary_used_percent": 90.0,
			"codex_usage_updated_at":     now.UTC().Format(time.RFC3339),
		}}, score: 10},
		{account: &Account{ID: 2, Extra: map[string]any{
			"codex_primary_used_percent": 10.0,
			"codex_usage_updated_at":     now.UTC().Format(time.RFC3339),
		}}, score: 1},
	}
	// Mimic buildLoadPlan fresh-session sort.
	sortFn := func(i, j int) bool {
		si := ComputeOpenAIUsageScore(candidates[i].account, now)
		sj := ComputeOpenAIUsageScore(candidates[j].account, now)
		if si != sj {
			return si < sj
		}
		return candidates[i].account.ID < candidates[j].account.ID
	}
	if sortFn(1, 0) {
		candidates[0], candidates[1] = candidates[1], candidates[0]
	}
	require.Equal(t, int64(2), candidates[0].account.ID)
}

func TestBumpOpenAIAccountStickyEpochInvalidatesMatch(t *testing.T) {
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	// Ensure deterministic baseline.
	openAIStickyInvalidEpoch.Store(account.ID, int64(0))
	before := OpenAIAccountStickyEpoch(account)
	BumpOpenAIAccountStickyEpoch(account.ID)
	after := OpenAIAccountStickyEpoch(account)
	require.NotEqual(t, before, after)
	require.Greater(t, after, before)
}

func TestStickyEpochCacheKeyHelpers(t *testing.T) {
	require.Equal(t, "openai:sticky-epoch:abc", openAIStickyEpochCacheKey("abc"))
	require.Equal(t, "openai:resp-epoch:resp_1", openAIResponseStickyEpochCacheKey("resp_1"))
	require.Equal(t, "", openAIStickyEpochCacheKey(" "))
}
