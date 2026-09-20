package config

import (
	"github.com/stretchr/testify/require"
	"testing"
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
