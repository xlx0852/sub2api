package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func newUserPinTestService(t *testing.T, cache *schedulerTestGatewayCache, pinEnabled bool) *OpenAIGatewayService {
	t.Helper()
	groupID := int64(10260)
	accounts := []Account{
		{
			ID:          38001,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
			GroupIDs:    []int64{groupID},
		},
		{
			ID:          38002,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    100,
			GroupIDs:    []int64{groupID},
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	cfg.Gateway.OpenAIUserPinEnabled = pinEnabled
	return &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
}

func TestOpenAIAccountScheduler_UserPinHitOverridesLoadBalance(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(10260)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{
		"userpin:901": 38001,
	}}
	svc := newUserPinTestService(t, cache, true)
	ctx := context.WithValue(context.Background(), ctxkey.OpenAIUserPinAPIKeyID, int64(901))
	require.NotNil(t, svc.getOpenAIAccountScheduler(ctx))

	// 38002 优先级更高（100 vs 0），负载均衡本应选它；钉号命中必须压过 LB。
	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(38001), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerUserPin, decision.Layer)
	require.True(t, decision.UserPinHit)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIAccountScheduler_UserPinBrokenByFailoverRebindsToNextAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(10260)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{
		"userpin:901": 38001,
	}}
	svc := newUserPinTestService(t, cache, true)
	ctx := context.WithValue(context.Background(), ctxkey.OpenAIUserPinAPIKeyID, int64(901))
	excluded := map[int64]struct{}{38001: {}}

	// failover 已把钉住的 38001 排除：应拆钉、落回负载均衡选 38002，并把钉重绑到 38002。
	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.1", excluded, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(38002), selection.Account.ID)
	require.Equal(t, "excluded_by_failover", decision.UserPinBroken)
	require.Equal(t, int64(38002), cache.sessionBindings["userpin:901"], "拆钉后应重绑到最终选中的账号")
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIAccountScheduler_UserPinDisabledFallsBackToLoadBalance(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(10260)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{
		"userpin:901": 38001,
	}}
	svc := newUserPinTestService(t, cache, false)
	ctx := context.WithValue(context.Background(), ctxkey.OpenAIUserPinAPIKeyID, int64(901))

	// 功能关闭：即使 Redis 里有旧钉也不读——不出现 user_pin 层、不命中钉号。
	// 具体选哪台由负载均衡决定，这里只验证钉号未被消费。
	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.NotEqual(t, openAIAccountScheduleLayerUserPin, decision.Layer)
	require.False(t, decision.UserPinHit)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
