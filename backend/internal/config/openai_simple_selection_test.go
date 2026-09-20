package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSimpleSelectionGroupConfig(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Empty(t, cfg.Gateway.OpenAIScheduler.SimpleSelectionGroupIDs)
	cfg.Gateway.OpenAIScheduler.SimpleSelectionGroupIDs = []int64{4, 42}
	require.NoError(t, cfg.Validate())
	for _, id := range []int64{0, -1} {
		cfg.Gateway.OpenAIScheduler.SimpleSelectionGroupIDs = []int64{id}
		require.ErrorContains(t, cfg.Validate(), "simple_selection_group_ids")
	}
}

func TestSimpleSelectionGroupConfigFromEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_OPENAI_SCHEDULER_SIMPLE_SELECTION_GROUP_IDS", "4,42")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, []int64{4, 42}, cfg.Gateway.OpenAIScheduler.SimpleSelectionGroupIDs)
}
