package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTryAcquireAccountSlotNilConcurrencyFailOpen(t *testing.T) {
	got, err := tryAcquireAccountSlot(context.Background(), nil, "gpt-4", 7, 3)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.True(t, got.Acquired)
	require.False(t, got.ModelLimited)
	require.NotNil(t, got.ReleaseFunc)
	got.ReleaseFunc()
}

func TestGatewayAndOpenAISlotAcquireShareHelper(t *testing.T) {
	gateway := &GatewayService{}
	openai := &OpenAIGatewayService{}

	g, gerr := gateway.tryAcquireAccountSlot(context.Background(), "claude-sonnet", 1, 2)
	o, oerr := openai.tryAcquireAccountSlot(context.Background(), "gpt-4", 1, 2)
	require.NoError(t, gerr)
	require.NoError(t, oerr)
	require.True(t, g.Acquired)
	require.True(t, o.Acquired)
	require.False(t, g.ModelLimited)
	require.False(t, o.ModelLimited)
}
