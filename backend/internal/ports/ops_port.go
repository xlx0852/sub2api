// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type OpsRepository interface {
	InsertErrorLog(ctx context.Context, input *domain.OpsInsertErrorLogInput) (int64, error)
	BatchInsertErrorLogs(ctx context.Context, inputs []*domain.OpsInsertErrorLogInput) (int64, error)
	ListErrorLogs(ctx context.Context, filter *domain.OpsErrorLogFilter) (*domain.OpsErrorLogList, error)
	GetErrorLogByID(ctx context.Context, id int64) (*domain.OpsErrorLogDetail, error)
	// LookupDeletedKeyAudit 按明文 key 反查最近一条已删除 key 审计;未命中返回 (nil, nil)。
	LookupDeletedKeyAudit(ctx context.Context, key string) (*domain.DeletedKeyAuditResult, error)
	ListRequestDetails(ctx context.Context, filter *domain.OpsRequestDetailFilter) ([]*domain.OpsRequestDetail, int64, error)
	BatchInsertSystemLogs(ctx context.Context, inputs []*domain.OpsInsertSystemLogInput) (int64, error)
	ListSystemLogs(ctx context.Context, filter *domain.OpsSystemLogFilter) (*domain.OpsSystemLogList, error)
	DeleteSystemLogs(ctx context.Context, filter *domain.OpsSystemLogCleanupFilter) (int64, error)
	InsertSystemLogCleanupAudit(ctx context.Context, input *domain.OpsSystemLogCleanupAudit) error

	UpdateErrorResolution(ctx context.Context, errorID int64, resolved bool, resolvedByUserID *int64, resolvedAt *time.Time) error

	// Lightweight window stats (for realtime WS / quick sampling).
	GetWindowStats(ctx context.Context, filter *domain.OpsDashboardFilter) (*domain.OpsWindowStats, error)
	// Lightweight realtime traffic summary (for the Ops dashboard header card).
	GetRealtimeTrafficSummary(ctx context.Context, filter *domain.OpsDashboardFilter) (*domain.OpsRealtimeTrafficSummary, error)

	GetDashboardOverview(ctx context.Context, filter *domain.OpsDashboardFilter) (*domain.OpsDashboardOverview, error)
	GetThroughputTrend(ctx context.Context, filter *domain.OpsDashboardFilter, bucketSeconds int) (*domain.OpsThroughputTrendResponse, error)
	GetLatencyHistogram(ctx context.Context, filter *domain.OpsDashboardFilter) (*domain.OpsLatencyHistogramResponse, error)
	GetErrorTrend(ctx context.Context, filter *domain.OpsDashboardFilter, bucketSeconds int) (*domain.OpsErrorTrendResponse, error)
	GetErrorDistribution(ctx context.Context, filter *domain.OpsDashboardFilter) (*domain.OpsErrorDistributionResponse, error)
	GetOpenAITokenStats(ctx context.Context, filter *domain.OpsOpenAITokenStatsFilter) (*domain.OpsOpenAITokenStatsResponse, error)

	InsertSystemMetrics(ctx context.Context, input *domain.OpsInsertSystemMetricsInput) error
	GetLatestSystemMetrics(ctx context.Context, windowMinutes int) (*domain.OpsSystemMetricsSnapshot, error)

	UpsertJobHeartbeat(ctx context.Context, input *domain.OpsUpsertJobHeartbeatInput) error
	ListJobHeartbeats(ctx context.Context) ([]*domain.OpsJobHeartbeat, error)

	InsertProviderStatusSnapshot(ctx context.Context, input *domain.ProviderStatusSnapshotRecord) (bool, error)
	GetLatestProviderStatusSnapshot(ctx context.Context, provider string) (*domain.ProviderStatusSnapshotRecord, error)
	ListProviderStatusSnapshots(ctx context.Context, filter *domain.ProviderStatusHistoryFilter) ([]*domain.ProviderStatusSnapshotRecord, error)

	// Alerts (rules + events)
	ListAlertRules(ctx context.Context) ([]*domain.OpsAlertRule, error)
	CreateAlertRule(ctx context.Context, input *domain.OpsAlertRule) (*domain.OpsAlertRule, error)
	UpdateAlertRule(ctx context.Context, input *domain.OpsAlertRule) (*domain.OpsAlertRule, error)
	DeleteAlertRule(ctx context.Context, id int64) error

	ListAlertEvents(ctx context.Context, filter *domain.OpsAlertEventFilter) ([]*domain.OpsAlertEvent, error)
	GetAlertEventByID(ctx context.Context, eventID int64) (*domain.OpsAlertEvent, error)
	GetActiveAlertEvent(ctx context.Context, ruleID int64) (*domain.OpsAlertEvent, error)
	GetLatestAlertEvent(ctx context.Context, ruleID int64) (*domain.OpsAlertEvent, error)
	CreateAlertEvent(ctx context.Context, event *domain.OpsAlertEvent) (*domain.OpsAlertEvent, error)
	UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, resolvedAt *time.Time) error
	UpdateAlertEventEmailSent(ctx context.Context, eventID int64, emailSent bool) error

	// Alert silences
	CreateAlertSilence(ctx context.Context, input *domain.OpsAlertSilence) (*domain.OpsAlertSilence, error)
	IsAlertSilenced(ctx context.Context, ruleID int64, platform string, groupID *int64, region *string, now time.Time) (bool, error)

	// Pre-aggregation (hourly/daily) used for long-window dashboard performance.
	UpsertHourlyMetrics(ctx context.Context, startTime, endTime time.Time) error
	UpsertDailyMetrics(ctx context.Context, startTime, endTime time.Time) error
	GetLatestHourlyBucketStart(ctx context.Context) (time.Time, bool, error)
	GetLatestDailyBucketDate(ctx context.Context) (time.Time, bool, error)
}
