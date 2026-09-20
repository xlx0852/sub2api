package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type dailyResetTrackingUserSubRepo struct {
	userSubRepoNoop

	resetDailyCalled bool
}

func (r *dailyResetTrackingUserSubRepo) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	r.resetDailyCalled = true
	return nil
}

func TestAssignOrExtendSubscription_Retired(t *testing.T) {
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}, newSubscriptionUserSubRepoStub(), nil, nil, nil)

	renewed, reused, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       200,
		GroupID:      1,
		ValidityDays: 1,
		Notes:        "new",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "SUBSCRIPTION_MODE_RETIRED")
	require.False(t, reused)
	require.Nil(t, renewed)
}

func TestUserSubscriptionNeedsDailyReset_DailyCardKeepsOneTimeQuota(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	dailyWindowStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        start.Add(24 * time.Hour),
		DailyWindowStart: &dailyWindowStart,
		DailyUsageUSD:    10,
	}

	require.True(t, sub.HasOneTimeDailyQuota())
	require.False(t, sub.NeedsDailyResetAt(dailyWindowStart.Add(25*time.Hour)), "日卡应作为一次性配额，跨 0 点后不再刷新日额度")
}

func TestUserSubscriptionNeedsDailyReset_MultiDaySubscriptionStillRefreshes(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	dailyWindowStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        start.AddDate(0, 0, 2),
		DailyWindowStart: &dailyWindowStart,
	}

	require.False(t, sub.HasOneTimeDailyQuota())
	require.True(t, sub.NeedsDailyResetAt(dailyWindowStart.Add(24*time.Hour)), "多日订阅仍应按 24 小时日窗口刷新")
}

func TestUserSubscriptionDailyResetTime_DailyCardReturnsExpiry(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	dailyWindowStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	expiresAt := start.Add(24 * time.Hour)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        expiresAt,
		DailyWindowStart: &dailyWindowStart,
	}

	resetAt := sub.DailyResetTime()
	require.NotNil(t, resetAt)
	require.Equal(t, expiresAt, *resetAt, "日卡展示的日额度结束时间应为订阅过期时间")
}

func TestCheckAndResetWindows_DailyCardDoesNotResetDailyUsage(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-23 * time.Hour)
	dailyWindowStart := now.Add(-25 * time.Hour)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.Add(24 * time.Hour),
		DailyUsageUSD:    10,
		DailyWindowStart: &dailyWindowStart,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub)

	require.NoError(t, err)
	require.False(t, repo.resetDailyCalled, "日卡作为一次性配额，过了 24 小时日窗口也不应重置 daily usage")
	require.Equal(t, 10.0, sub.DailyUsageUSD)
}

func TestCheckAndResetWindows_MultiDaySubscriptionStillResetsDailyUsage(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-48 * time.Hour)
	dailyWindowStart := now.Add(-25 * time.Hour)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.AddDate(0, 0, 2),
		DailyUsageUSD:    10,
		DailyWindowStart: &dailyWindowStart,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub)

	require.NoError(t, err)
	require.True(t, repo.resetDailyCalled, "多日订阅仍应重置过期 daily window")
	require.Equal(t, 0.0, sub.DailyUsageUSD)
}

func TestValidateAndCheckLimits_DailyCardDoesNotAllowSecondQuotaAfterMidnight(t *testing.T) {
	start := time.Now().Add(-23 * time.Hour)
	dailyWindowStart := time.Now().Add(-25 * time.Hour)
	dailyLimit := 10.0
	sub := &UserSubscription{
		Status:           SubscriptionStatusActive,
		StartsAt:         start,
		ExpiresAt:        start.Add(24 * time.Hour),
		DailyWindowStart: &dailyWindowStart,
		DailyUsageUSD:    dailyLimit + 0.01,
	}
	group := &Group{
		SubscriptionType: SubscriptionTypeSubscription,
		DailyLimitUSD:    &dailyLimit,
	}
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepoNoop{}, nil, nil, nil)

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, group)

	require.False(t, needsMaintenance, "日卡跨过日窗口后不应触发 daily reset 维护")
	require.True(t, errors.Is(err, ErrDailyLimitExceeded))
	require.Equal(t, dailyLimit+0.01, sub.DailyUsageUSD, "热路径不应清零日卡已用额度")
}
