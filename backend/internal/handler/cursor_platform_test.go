package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCursorRequestPlatform(t *testing.T) {
	key := &service.APIKey{Group: &service.Group{Platform: service.PlatformCursor}}
	require.Equal(t, service.PlatformCursor, openAICompatibleRequestPlatform(key))
	require.True(t, allowOpenAICompatibleMessagesDispatch(key))
}
