package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/stretchr/testify/require"
)

func TestAccountLifecycleStagesRejectMissingAccount(t *testing.T) {
	ex := &pipeline.Exchange{}
	require.EqualError(t, admitAccount(ex), "admission: account is required")
	require.EqualError(t, selectAccount(ex), "select: account is required")
}
