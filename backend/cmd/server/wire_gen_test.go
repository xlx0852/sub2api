package main

import (
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestProvideServiceBuildInfo(t *testing.T) {
	in := handler.BuildInfo{
		Version:            "v-test",
		MinFrontendVersion: "v-frontend-min",
		BuildType:          "release",
	}
	out := provideServiceBuildInfo(in)
	require.Equal(t, in.Version, out.Version)
	require.Equal(t, in.BuildType, out.BuildType)
}

func TestProvideCleanup_WithMinimalDependencies_NoPanic(t *testing.T) {
	cfg := &config.Config{}

	openAIOAuthSvc := service.NewOpenAIOAuthService(nil, nil)
	geminiOAuthSvc := service.NewGeminiOAuthService(nil, nil, nil, nil, cfg)
	antigravityOAuthSvc := service.NewAntigravityOAuthService(nil)

	tokenRefreshSvc := service.NewTokenRefreshService(
		nil,
		openAIOAuthSvc,
		geminiOAuthSvc,
		antigravityOAuthSvc,
		nil,
		nil,
		cfg,
		nil,
	)
	accountExpirySvc := service.NewAccountExpiryService(nil, time.Second)
	proxyExpirySvc := service.NewProxyExpiryService(nil, time.Second)
	subscriptionExpirySvc := service.NewSubscriptionExpiryService(nil, time.Second)
	pricingSvc := service.NewPricingService(cfg, nil)
	emailQueueSvc := service.NewEmailQueueService(nil, 1)
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	idempotencyCleanupSvc := service.NewIdempotencyCleanupService(nil, cfg)
	schedulerSnapshotSvc := service.NewSchedulerSnapshotService(nil, nil, nil, nil, cfg)
	opsSystemLogSinkSvc := service.NewOpsSystemLogSink(nil)

	cleanup := provideCleanup(
		nil, // entClient
		nil, // redis
		&service.OpsMetricsCollector{},
		&service.OpsAggregationService{},
		&service.OpsAlertEvaluatorService{},
		nil, // openAIStatusCollector
		&service.OpsCleanupService{},
		&service.OpsScheduledReportService{},
		opsSystemLogSinkSvc,
		schedulerSnapshotSvc,
		tokenRefreshSvc,
		accountExpirySvc,
		proxyExpirySvc,
		subscriptionExpirySvc,
		&service.UsageCleanupService{},
		idempotencyCleanupSvc,
		nil, // subscriptionAutoRenew
		pricingSvc,
		emailQueueSvc,
		billingCacheSvc,
		&service.UsageRecordWorkerPool{},
		(*usageSettlementBindings)(nil),
		&service.SubscriptionService{},
		openAIOAuthSvc,
		geminiOAuthSvc,
		antigravityOAuthSvc,
		nil, // grokOAuth
		nil, // openAIGateway
		nil, // scheduledTestRunner
		nil, // backupSvc
		nil, // paymentOrderExpiry
		nil, // quotaFlusher
		nil, // upstreamBillingProbe
		nil, // quotaWindowSweep
		nil, // retrospectiveRisk
		nil, // openAICodexVersionSyncService
	)

	require.NotPanics(t, func() {
		cleanup()
	})
}

// 装配回归守卫：wire 会静默丢弃 ProviderSet 中没有消费者的 provider——Codex 版本同步
// 曾因此在线上从未启动，同步值冻结了三周多，而 `wire check` 不报任何诊断。
// 这个断言盯着生成代码，确保同步服务确实被构造并交给 provideCleanup。
func TestCodexVersionSyncIsWiredIntoGeneratedInjector(t *testing.T) {
	source, err := os.ReadFile("wire_gen.go")
	require.NoError(t, err)

	code := string(source)
	require.Contains(t, code, "service.ProvideOpenAICodexVersionSyncService(")
	require.Contains(t, code, "openAICodexVersionSyncService)")
}
