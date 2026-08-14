package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestMapAccountSelectionPreservesWaitPlanAndRelease(t *testing.T) {
	released := false
	sel := &AccountSelectionResult{
		Account:  &Account{ID: 42},
		Acquired: false,
		ReleaseFunc: func() {
			released = true
		},
		WaitPlan: &AccountWaitPlan{
			AccountID:      42,
			MaxConcurrency: 3,
			Timeout:        2 * time.Second,
			MaxWaiting:     8,
		},
		ModelLimited: false,
	}

	out, err := mapAccountSelection(sel, nil)
	require.NoError(t, err)
	require.Equal(t, int64(42), out.Account.ID)
	require.False(t, out.Acquired)
	require.False(t, out.ModelLimited)
	require.Equal(t, scheduling.WaitPlan{
		AccountID:      42,
		MaxConcurrency: 3,
		Timeout:        2 * time.Second,
		MaxWaiting:     8,
	}, *out.WaitPlan)
	out.Release()
	require.True(t, released)
}

func TestMapAccountSelectionPropagatesError(t *testing.T) {
	out, err := mapAccountSelection(&AccountSelectionResult{Account: &Account{ID: 1}}, ErrNoAvailableAccounts)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Nil(t, out)
}

func TestAccountSelectionFromSchedulingRoundTrip(t *testing.T) {
	in := &scheduling.Result{
		Account:      &domain.Account{ID: 9},
		Acquired:     true,
		Release:      func() {},
		ModelLimited: true,
	}
	out, err := AccountSelectionFromScheduling(in, nil)
	require.NoError(t, err)
	require.Equal(t, int64(9), out.Account.ID)
	require.True(t, out.Acquired)
	require.True(t, out.ModelLimited)
	require.NotNil(t, out.ReleaseFunc)
}

func TestNewStickyAndFallbackWaitPlansMapSchedulingPolicy(t *testing.T) {
	cfg := scheduling.DefaultConfig()
	cfg.StickySessionWaitTimeout = 12 * time.Second
	cfg.StickySessionMaxWaiting = 4
	cfg.FallbackWaitTimeout = 9 * time.Second
	cfg.FallbackMaxWaiting = 11

	sticky := newStickyWaitPlan(7, 3, cfg)
	require.Equal(t, &AccountWaitPlan{
		AccountID:      7,
		MaxConcurrency: 3,
		Timeout:        12 * time.Second,
		MaxWaiting:     4,
	}, sticky)

	fallback := newFallbackWaitPlan(8, 2, cfg)
	require.Equal(t, &AccountWaitPlan{
		AccountID:      8,
		MaxConcurrency: 2,
		Timeout:        9 * time.Second,
		MaxWaiting:     11,
	}, fallback)
}

func TestResolveSelectorGroupIDPrefersExplicitID(t *testing.T) {
	explicit := int64(7)
	req := scheduling.Request{
		GroupID: &explicit,
		Group:   &domain.Group{ID: 9},
	}
	require.Equal(t, int64(7), *resolveSelectorGroupID(req))
}

func TestSchedulingStrategiesHaveDisjointPlatformOwnership(t *testing.T) {
	gateway := gatewaySchedulingStrategy{}
	openAI := openAISchedulingStrategy{}

	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity} {
		require.True(t, gateway.Supports(scheduling.Request{Platform: platform}))
		require.False(t, openAI.Supports(scheduling.Request{Platform: platform}))
	}
	for _, platform := range []string{PlatformOpenAI, PlatformGrok, PlatformKimi} {
		require.False(t, gateway.Supports(scheduling.Request{Platform: platform}))
		require.True(t, openAI.Supports(scheduling.Request{Platform: platform}))
	}
	require.False(t, gateway.Supports(scheduling.Request{Platform: "unknown"}))
	require.False(t, openAI.Supports(scheduling.Request{Platform: "unknown"}))
}

func TestUnifiedAccountSelectorUsesEngine(t *testing.T) {
	selector := NewUnifiedAccountSelector(nil, nil)
	_, ok := selector.(*scheduling.Engine)
	require.True(t, ok)
}
