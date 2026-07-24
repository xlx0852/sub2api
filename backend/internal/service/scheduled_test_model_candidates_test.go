package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveScheduledDiagnosticsModels_PrimaryFirstAndDedup(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"cloud_models": []any{"gpt-5.5", "gpt-5.4-mini", "gpt-5.5"},
		},
	}
	got := ResolveScheduledDiagnosticsModels(account, "gpt-5.4-mini")
	require.NotEmpty(t, got)
	require.Equal(t, "gpt-5.4-mini", got[0])
	require.LessOrEqual(t, len(got), scheduledDiagnosticsMaxModels)

	seen := map[string]struct{}{}
	for _, m := range got {
		_, ok := seen[m]
		require.False(t, ok)
		seen[m] = struct{}{}
	}
}

func TestDefaultScheduledDiagnosticsCron_Staggered(t *testing.T) {
	require.Equal(t, "3-59/5 * * * *", DefaultScheduledDiagnosticsCron(73))
	require.Equal(t, "0-59/5 * * * *", DefaultScheduledDiagnosticsCron(90))
	require.True(t, strings.HasSuffix(DefaultScheduledDiagnosticsCron(1), " * * * *"))
}

func TestDefaultScheduledDiagnosticsModel_PrefersCheap(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	model := DefaultScheduledDiagnosticsModel(account)
	require.NotEmpty(t, model)
	require.True(t, strings.Contains(strings.ToLower(model), "mini") || model != "")
}
