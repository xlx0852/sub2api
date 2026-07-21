//go:build unit

package service

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClassifyUpstreamOutcome(t *testing.T) {
	require.Equal(t, UpstreamOutcomeSuccess, ClassifyUpstreamOutcome(200, nil, false))
	require.Equal(t, UpstreamOutcomeCaller, ClassifyUpstreamOutcome(400, nil, false))
	require.Equal(t, UpstreamOutcomeCredential, ClassifyUpstreamOutcome(401, nil, false))
	require.Equal(t, UpstreamOutcomeCredential, ClassifyUpstreamOutcome(403, nil, false))
	require.Equal(t, UpstreamOutcomeQuota, ClassifyUpstreamOutcome(429, nil, false))
	require.Equal(t, UpstreamOutcomeTransient, ClassifyUpstreamOutcome(503, nil, false))
	require.Equal(t, UpstreamOutcomeTransient, ClassifyUpstreamOutcome(0, errors.New("connect"), false))
	require.Equal(t, UpstreamOutcomeTransient, ClassifyUpstreamOutcome(200, nil, true))
}

func TestParseRetryAfterDuration(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	delay, ok := ParseRetryAfterDuration("120", now)
	require.True(t, ok)
	require.Equal(t, 120*time.Second, delay)

	httpDate := now.Add(90 * time.Second).UTC().Format(http.TimeFormat)
	delay, ok = ParseRetryAfterDuration(httpDate, now)
	require.True(t, ok)
	require.InDelta(t, 90*time.Second, delay, float64(time.Second))

	_, ok = ParseRetryAfterDuration("", now)
	require.False(t, ok)
}

func TestComputeOpenAIQuotaCooldownUntilPrefersRetryAfter(t *testing.T) {
	now := time.Now()
	headers := http.Header{}
	headers.Set("Retry-After", "45")
	reset := now.Add(10 * time.Minute)
	until := ComputeOpenAIQuotaCooldownUntil(headers, &reset, now)
	require.WithinDuration(t, now.Add(45*time.Second), until, time.Second)
}

func TestComputeOpenAIUsageScore(t *testing.T) {
	now := time.Now()
	account := &Account{Extra: map[string]any{
		"codex_primary_used_percent":   40.0,
		"codex_secondary_used_percent": 70.0,
		"codex_usage_updated_at":       now.UTC().Format(time.RFC3339),
	}}
	score := ComputeOpenAIUsageScore(account, now)
	require.Equal(t, 70.0, score)

	require.Equal(t, openAIUnknownUsageScore, ComputeOpenAIUsageScore(nil, now))
}

func TestOpenAICredentialClassMatches(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	require.True(t, OpenAICredentialClassMatches(oauth, OpenAICredentialClassOAuth))
	require.False(t, OpenAICredentialClassMatches(oauth, OpenAICredentialClassAPIKey))
	require.True(t, OpenAICredentialClassMatches(apiKey, OpenAICredentialClassAPIKey))
	require.True(t, OpenAICredentialClassMatches(apiKey, OpenAICredentialClassUnspecified))
}

func TestAPIKeyPoolCoolAndRotateCAS(t *testing.T) {
	now := time.Now()
	state := newAPIKeyPoolState("A")
	candidates := []string{"A", "B", "C"}

	live, rotated := state.CoolAndRotate("A", candidates, time.Minute, now)
	require.True(t, rotated)
	require.Equal(t, "B", live)
	require.True(t, state.IsCooled("A", now))
	require.False(t, state.IsCooled("B", now))

	// Concurrent stale failure on A must not cool/rotate away from B.
	live, rotated = state.CoolAndRotate("A", candidates, time.Minute, now)
	require.False(t, rotated)
	require.Equal(t, "B", live)
	require.False(t, state.IsCooled("B", now))
}
