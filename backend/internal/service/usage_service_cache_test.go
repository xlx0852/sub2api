//go:build unit

package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

// cacheCountingUsageRepo 统计各聚合方法的调用次数, 用于验证 30s 缓存只回源一次。
type cacheCountingUsageRepo struct {
	UsageLogRepository
	groupSummaryCalls       atomic.Int64
	userDashboardStatsCalls atomic.Int64
	groupAvailCalls         atomic.Int64
	groupAvailRollupCalls   atomic.Int64
}

func (r *cacheCountingUsageRepo) GetAllGroupUsageSummary(ctx context.Context, todayStart time.Time) ([]usagestats.GroupUsageSummary, error) {
	r.groupSummaryCalls.Add(1)
	return []usagestats.GroupUsageSummary{{GroupID: 1, TodayCost: 1, TotalCost: 10}}, nil
}

func (r *cacheCountingUsageRepo) GetUserDashboardStats(ctx context.Context, userID int64) (*usagestats.UserDashboardStats, error) {
	r.userDashboardStatsCalls.Add(1)
	return &usagestats.UserDashboardStats{TotalRequests: 42}, nil
}

func (r *cacheCountingUsageRepo) GetGroupTrafficAvailability(ctx context.Context, groupID int64, start, end time.Time, bucket time.Duration) (*GroupTrafficAvailability, error) {
	r.groupAvailCalls.Add(1)
	return &GroupTrafficAvailability{GroupID: groupID, StartAt: start, EndAt: end, BucketMinutes: int(bucket.Minutes())}, nil
}

func (r *cacheCountingUsageRepo) GetGroupTrafficAvailabilityRollup(ctx context.Context, groupID int64, start, end time.Time) (*GroupTrafficAvailability, error) {
	r.groupAvailRollupCalls.Add(1)
	return &GroupTrafficAvailability{GroupID: groupID, StartAt: start, EndAt: end}, nil
}

func TestDashboardService_GetGroupUsageSummary_Cached(t *testing.T) {
	repo := &cacheCountingUsageRepo{}
	svc := NewDashboardService(repo, nil, nil, nil)

	day := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	_, err := svc.GetGroupUsageSummary(context.Background(), day)
	require.NoError(t, err)
	_, err = svc.GetGroupUsageSummary(context.Background(), day)
	require.NoError(t, err)

	require.Equal(t, int64(1), repo.groupSummaryCalls.Load(), "同一天两次调用应只回源一次")

	// 不同 todayStart(跨时区/跨天) 是不同 key, 应再次回源
	nextDay := day.Add(24 * time.Hour)
	_, err = svc.GetGroupUsageSummary(context.Background(), nextDay)
	require.NoError(t, err)
	require.Equal(t, int64(2), repo.groupSummaryCalls.Load())
}

func TestUsageService_GetUserDashboardStats_Cached(t *testing.T) {
	repo := &cacheCountingUsageRepo{}
	svc := NewUsageService(repo, nil, nil, nil)

	_, err := svc.GetUserDashboardStats(context.Background(), 100)
	require.NoError(t, err)
	_, err = svc.GetUserDashboardStats(context.Background(), 100)
	require.NoError(t, err)

	require.Equal(t, int64(1), repo.userDashboardStatsCalls.Load(), "同用户两次调用应只回源一次")

	// 不同用户是不同 key
	_, err = svc.GetUserDashboardStats(context.Background(), 200)
	require.NoError(t, err)
	require.Equal(t, int64(2), repo.userDashboardStatsCalls.Load())
}

func TestUsageService_GetGroupTrafficAvailability_Cached(t *testing.T) {
	repo := &cacheCountingUsageRepo{}
	svc := NewUsageService(repo, nil, nil, nil)

	_, err := svc.GetGroupTrafficAvailability(context.Background(), 77)
	require.NoError(t, err)
	_, err = svc.GetGroupTrafficAvailability(context.Background(), 77)
	require.NoError(t, err)

	require.Equal(t, int64(1), repo.groupAvailCalls.Load(), "同 group 两次调用应只回源一次(24h)")
	require.Equal(t, int64(1), repo.groupAvailRollupCalls.Load(), "同 group 两次调用应只回源一次(7d)")

	// 不同 group 是不同 key
	_, err = svc.GetGroupTrafficAvailability(context.Background(), 88)
	require.NoError(t, err)
	require.Equal(t, int64(2), repo.groupAvailCalls.Load())
}
