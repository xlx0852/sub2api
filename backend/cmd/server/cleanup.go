package main

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/app/lifecycle"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

func provideCleanup(
	entClient *ent.Client,
	rdb *redis.Client,
	opsMetricsCollector *service.OpsMetricsCollector,
	opsAggregation *service.OpsAggregationService,
	opsAlertEvaluator *service.OpsAlertEvaluatorService,
	openAIStatusCollector *service.OpenAIStatusCollectorService,
	opsCleanup *service.OpsCleanupService,
	opsScheduledReport *service.OpsScheduledReportService,
	opsSystemLogSink *service.OpsSystemLogSink,
	schedulerSnapshot *service.SchedulerSnapshotService,
	tokenRefresh *service.TokenRefreshService,
	accountExpiry *service.AccountExpiryService,
	proxyExpiry *service.ProxyExpiryService,
	subscriptionExpiry *service.SubscriptionExpiryService,
	usageCleanup *service.UsageCleanupService,
	idempotencyCleanup *service.IdempotencyCleanupService,
	subscriptionAutoRenew *service.SubscriptionAutoRenewService,
	pricing *service.PricingService,
	emailQueue *service.EmailQueueService,
	billingCache *service.BillingCacheService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	usageSettlementBindings *usageSettlementBindings,
	subscriptionService *service.SubscriptionService,
	openaiOAuth *service.OpenAIOAuthService,
	geminiOAuth *service.GeminiOAuthService,
	antigravityOAuth *service.AntigravityOAuthService,
	grokOAuth *service.GrokOAuthService,
	openAIGateway *service.OpenAIGatewayService,
	scheduledTestRunner *service.ScheduledTestRunnerService,
	backupSvc *service.BackupService,
	paymentOrderExpiry *service.PaymentOrderExpiryService,
	quotaFlusher *service.UserPlatformQuotaUsageFlusher,
	upstreamBillingProbe *service.UpstreamBillingProbeService,
	quotaWindowSweep *service.QuotaWindowSweepService,
	retrospectiveRisk *service.RetrospectiveRiskService,
	openAICodexVersionSync *service.OpenAICodexVersionSyncService,
) func() {
	return func() {
		reg := lifecycle.New()

		// 应用层清理步骤可并行执行，基础设施资源（Redis/Ent）最后按顺序关闭。
		for _, step := range []struct {
			name string
			fn   func() error
		}{
			{"OpenAICodexVersionSyncService", func() error {
				if openAICodexVersionSync != nil {
					openAICodexVersionSync.Stop()
				}
				return nil
			}},
			{"OpenAIStatusCollectorService", func() error {
				if openAIStatusCollector != nil {
					openAIStatusCollector.Stop()
				}
				return nil
			}},
			{"QuotaWindowSweepService", func() error {
				if quotaWindowSweep != nil {
					quotaWindowSweep.Stop()
				}
				return nil
			}},
			{"OpsScheduledReportService", func() error {
				if opsScheduledReport != nil {
					opsScheduledReport.Stop()
				}
				return nil
			}},
			{"OpsCleanupService", func() error {
				if opsCleanup != nil {
					opsCleanup.Stop()
				}
				return nil
			}},
			{"OpsSystemLogSink", func() error {
				if opsSystemLogSink != nil {
					opsSystemLogSink.Stop()
				}
				return nil
			}},
			{"OpsAlertEvaluatorService", func() error {
				if opsAlertEvaluator != nil {
					opsAlertEvaluator.Stop()
				}
				return nil
			}},
			{"OpsAggregationService", func() error {
				if opsAggregation != nil {
					opsAggregation.Stop()
				}
				return nil
			}},
			{"OpsMetricsCollector", func() error {
				if opsMetricsCollector != nil {
					opsMetricsCollector.Stop()
				}
				return nil
			}},
			{"SchedulerSnapshotService", func() error {
				if schedulerSnapshot != nil {
					schedulerSnapshot.Stop()
				}
				return nil
			}},
			{"UsageCleanupService", func() error {
				if usageCleanup != nil {
					usageCleanup.Stop()
				}
				return nil
			}},
			{"IdempotencyCleanupService", func() error {
				if idempotencyCleanup != nil {
					idempotencyCleanup.Stop()
				}
				return nil
			}},
			{"SubscriptionAutoRenewService", func() error {
				if subscriptionAutoRenew != nil {
					subscriptionAutoRenew.Stop()
				}
				return nil
			}},
			{"TokenRefreshService", func() error {
				tokenRefresh.Stop()
				return nil
			}},
			{"AccountExpiryService", func() error {
				accountExpiry.Stop()
				return nil
			}},
			{"ProxyExpiryService", func() error {
				proxyExpiry.Stop()
				return nil
			}},
			{"SubscriptionExpiryService", func() error {
				subscriptionExpiry.Stop()
				return nil
			}},
			{"SubscriptionService", func() error {
				if subscriptionService != nil {
					subscriptionService.Stop()
				}
				return nil
			}},
			{"PricingService", func() error {
				pricing.Stop()
				return nil
			}},
			{"EmailQueueService", func() error {
				emailQueue.Stop()
				return nil
			}},
			{"BillingCacheService", func() error {
				billingCache.Stop()
				return nil
			}},
			{"UsageRecordWorkerPool", func() error {
				if usageRecordWorkerPool != nil {
					usageRecordWorkerPool.Stop()
				}
				return nil
			}},
			{"DurableSettlement", func() error {
				if usageSettlementBindings == nil || usageSettlementBindings.worker == nil {
					return nil
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				return usageSettlementBindings.worker.StopAndWait(ctx)
			}},
			{"OpenAIOAuthService", func() error {
				openaiOAuth.Stop()
				return nil
			}},
			{"GeminiOAuthService", func() error {
				geminiOAuth.Stop()
				return nil
			}},
			{"AntigravityOAuthService", func() error {
				antigravityOAuth.Stop()
				return nil
			}},
			{"GrokOAuthService", func() error {
				if grokOAuth != nil {
					grokOAuth.Stop()
				}
				return nil
			}},
			{"OpenAIWSPool", func() error {
				if openAIGateway != nil {
					openAIGateway.CloseOpenAIWSPool()
				}
				return nil
			}},
			{"ScheduledTestRunnerService", func() error {
				if scheduledTestRunner != nil {
					scheduledTestRunner.Stop()
				}
				return nil
			}},
			{"BackupService", func() error {
				if backupSvc != nil {
					backupSvc.Stop()
				}
				return nil
			}},
			{"PaymentOrderExpiryService", func() error {
				if paymentOrderExpiry != nil {
					paymentOrderExpiry.Stop()
				}
				return nil
			}}, {"UserPlatformQuotaUsageFlusher", func() error {
				if quotaFlusher != nil {
					quotaFlusher.Stop()
				}
				return nil
			}},
			{"UpstreamBillingProbeService", func() error {
				if upstreamBillingProbe != nil {
					upstreamBillingProbe.Stop()
				}
				return nil
			}},
			{"RetrospectiveRiskService", func() error {
				if retrospectiveRisk != nil {
					retrospectiveRisk.Stop()
				}
				return nil
			}},
		} {
			reg.AddParallel(step.name, step.fn)
		}

		reg.AddSequential("Redis", func() error {
			if rdb == nil {
				return nil
			}
			return rdb.Close()
		})
		reg.AddSequential("Ent", func() error {
			if entClient == nil {
				return nil
			}
			return entClient.Close()
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		reg.Stop(ctx)
	}
}
