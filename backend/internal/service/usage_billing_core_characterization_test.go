package service

import (
	"context"
	"testing"

	appbilling "github.com/Wei-Shaw/sub2api/internal/app/billing"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestValidateUsageBillingPolicyRejectsImplicitProvider(t *testing.T) {
	require.Error(t, validateUsageBillingPolicy(appbilling.RecordPolicy{}))
	require.Error(t, validateUsageBillingPolicy(appbilling.RecordPolicy{
		Provider:       appbilling.ProviderOpenAI,
		PricingMissing: appbilling.PricingMissingLogZeroCost,
		CostErrors:     appbilling.CostErrorLogZeroPricingOnly,
		TokenBuckets:   appbilling.TokenBucketsGateway,
	}))
}

func newGatewayBillingCharacterizationService(usageRepo UsageLogRepository) *GatewayService {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	return &GatewayService{
		usageLogRepo:        usageRepo,
		userRepo:            &openAIRecordUsageUserRepoStub{},
		userSubRepo:         &openAIRecordUsageSubRepoStub{},
		cfg:                 cfg,
		billingService:      NewBillingService(cfg, nil),
		billingCacheService: &BillingCacheService{},
		deferredService:     &DeferredService{},
	}
}

func TestGatewayBillingCharacterizationMissingPricingRecordsZeroCost(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newGatewayBillingCharacterizationService(usageRepo)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway-missing-pricing",
			Model:     "definitely-unpriced-gateway-model",
			Usage:     ClaudeUsage{InputTokens: 100, OutputTokens: 20},
		},
		APIKey:  &APIKey{ID: 801, Group: &Group{RateMultiplier: 1}},
		User:    &User{ID: 802},
		Account: &Account{ID: 803},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Zero(t, usageRepo.lastLog.TotalCost)
	require.Zero(t, usageRepo.lastLog.ActualCost)
	require.Equal(t, 100, usageRepo.lastLog.InputTokens)
	require.Equal(t, 20, usageRepo.lastLog.OutputTokens)
}

func TestGatewayBillingCharacterizationForceCacheMovesInputBucket(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newGatewayBillingCharacterizationService(usageRepo)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway-force-cache",
			Model:     "claude-sonnet-4",
			Usage:     ClaudeUsage{InputTokens: 100, OutputTokens: 20},
		},
		APIKey:            &APIKey{ID: 811, Group: &Group{RateMultiplier: 1}},
		User:              &User{ID: 812},
		Account:           &Account{ID: 813},
		ForceCacheBilling: true,
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Zero(t, usageRepo.lastLog.InputTokens)
	require.Equal(t, 100, usageRepo.lastLog.CacheReadTokens)
	require.Zero(t, usageRepo.lastLog.InputCost)
	require.Greater(t, usageRepo.lastLog.CacheReadCost, 0.0)
}
