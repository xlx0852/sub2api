package handler

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSameAccountRetryDelayFor(t *testing.T) {
	err := &service.UpstreamFailoverError{RequestScopedTransient: true}
	require.Equal(t, 500*time.Millisecond, sameAccountRetryDelayFor(err, 1))
	require.Equal(t, time.Second, sameAccountRetryDelayFor(err, 2))
	require.Equal(t, 8*time.Second, sameAccountRetryDelayFor(err, 9))
	require.Equal(t, 500*time.Millisecond, sameAccountRetryDelayFor(&service.UpstreamFailoverError{}, 4))
}
