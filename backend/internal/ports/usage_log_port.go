// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type UsageLogRepository interface {
	// Create creates a usage log and returns whether it was actually inserted.
	// inserted is false when the insert was skipped due to conflict (idempotent retries).
	Create(ctx context.Context, log *domain.UsageLog) (inserted bool, err error)
	GetByID(ctx context.Context, id int64) (*domain.UsageLog, error)
	Delete(ctx context.Context, id int64) error

	ListByUser(ctx context.Context, userID int64, params domain.PaginationParams) ([]domain.UsageLog, *domain.PaginationResult, error)
	ListByAPIKey(ctx context.Context, apiKeyID int64, params domain.PaginationParams) ([]domain.UsageLog, *domain.PaginationResult, error)
	ListByAccount(ctx context.Context, accountID int64, params domain.PaginationParams) ([]domain.UsageLog, *domain.PaginationResult, error)

	ListByUserAndTimeRange(ctx context.Context, userID int64, startTime, endTime time.Time) ([]domain.UsageLog, *domain.PaginationResult, error)
	ListByAPIKeyAndTimeRange(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) ([]domain.UsageLog, *domain.PaginationResult, error)
	ListByAccountAndTimeRange(ctx context.Context, accountID int64, startTime, endTime time.Time) ([]domain.UsageLog, *domain.PaginationResult, error)
	ListByModelAndTimeRange(ctx context.Context, modelName string, startTime, endTime time.Time) ([]domain.UsageLog, *domain.PaginationResult, error)

	GetAccountWindowStats(ctx context.Context, accountID int64, startTime time.Time) (*domain.AccountStats, error)
	GetAccountTodayStats(ctx context.Context, accountID int64) (*domain.AccountStats, error)

	// Admin dashboard stats
	GetDashboardStats(ctx context.Context) (*domain.DashboardStats, error)
	GetUsageTrendWithFilters(ctx context.Context, startTime, endTime time.Time, granularity string, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]domain.TrendDataPoint, error)
	GetModelStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]domain.ModelStat, error)
	GetEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]domain.EndpointStat, error)
	GetUpstreamEndpointStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, model string, requestType *int16, stream *bool, billingType *int8) ([]domain.EndpointStat, error)
	GetGroupStatsWithFilters(ctx context.Context, startTime, endTime time.Time, userID, apiKeyID, accountID, groupID int64, requestType *int16, stream *bool, billingType *int8) ([]domain.GroupStat, error)
	GetUserBreakdownStats(ctx context.Context, startTime, endTime time.Time, dim domain.UserBreakdownDimension, limit int) ([]domain.UserBreakdownItem, error)
	GetAllGroupUsageSummary(ctx context.Context, todayStart time.Time) ([]domain.GroupUsageSummary, error)
	GetAPIKeyUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]domain.APIKeyUsageTrendPoint, error)
	GetUserUsageTrend(ctx context.Context, startTime, endTime time.Time, granularity string, limit int) ([]domain.UserUsageTrendPoint, error)
	GetUserSpendingRanking(ctx context.Context, startTime, endTime time.Time, limit int) (*domain.UserSpendingRankingResponse, error)
	GetBatchUserUsageStats(ctx context.Context, userIDs []int64, startTime, endTime time.Time) (map[int64]*domain.BatchUserUsageStats, error)
	GetBatchAPIKeyUsageStats(ctx context.Context, apiKeyIDs []int64, startTime, endTime time.Time) (map[int64]*domain.BatchAPIKeyUsageStats, error)

	// domain.User dashboard stats
	GetUserDashboardStats(ctx context.Context, userID int64) (*domain.UserDashboardStats, error)
	GetTrafficAvailability(ctx context.Context, start, end time.Time, bucketSize time.Duration, userID *int64, platform string) (*domain.TrafficAvailability, error)
	// GetGroupTrafficAvailability aggregates real-traffic availability for a group.
	GetGroupTrafficAvailability(ctx context.Context, groupID int64, start, end time.Time, bucketSize time.Duration) (*domain.GroupTrafficAvailability, error)
	// GetGroupTrafficAvailabilityRollup is a cheap single-window aggregate (no dedup/buckets) for long windows.
	GetGroupTrafficAvailabilityRollup(ctx context.Context, groupID int64, start, end time.Time) (*domain.GroupTrafficAvailability, error)
	GetAPIKeyDashboardStats(ctx context.Context, apiKeyID int64) (*domain.UserDashboardStats, error)
	GetUserUsageTrendByUserID(ctx context.Context, userID int64, startTime, endTime time.Time, granularity string) ([]domain.TrendDataPoint, error)
	GetUserModelStats(ctx context.Context, userID int64, startTime, endTime time.Time) ([]domain.ModelStat, error)

	// Admin usage listing/stats
	ListWithFilters(ctx context.Context, params domain.PaginationParams, filters domain.UsageLogFilters) ([]domain.UsageLog, *domain.PaginationResult, error)
	GetGlobalStats(ctx context.Context, startTime, endTime time.Time) (*domain.UsageStats, error)
	GetStatsWithFilters(ctx context.Context, filters domain.UsageLogFilters) (*domain.UsageStats, error)

	// domain.Account stats
	GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*domain.AccountUsageStatsResponse, error)

	// Aggregated stats (optimized)
	GetUserStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) (*domain.UsageStats, error)
	GetAPIKeyStatsAggregated(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (*domain.UsageStats, error)
	GetAccountStatsAggregated(ctx context.Context, accountID int64, startTime, endTime time.Time) (*domain.UsageStats, error)
	GetModelStatsAggregated(ctx context.Context, modelName string, startTime, endTime time.Time) (*domain.UsageStats, error)
	GetDailyStatsAggregated(ctx context.Context, userID int64, startTime, endTime time.Time) ([]map[string]any, error)
}
