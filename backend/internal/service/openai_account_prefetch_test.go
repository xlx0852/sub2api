//go:build unit

package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// countingPrefetchAccountRepo 在 schedulerTestOpenAIAccountRepo 基础上统计 GetByID/GetByIDs 调用次数。
type countingPrefetchAccountRepo struct {
	schedulerTestOpenAIAccountRepo
	getByIDCalls  atomic.Int64
	getByIDsCalls atomic.Int64
	getByIDsErr   error
}

func (r *countingPrefetchAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	r.getByIDCalls.Add(1)
	return r.schedulerTestOpenAIAccountRepo.GetByID(ctx, id)
}

func (r *countingPrefetchAccountRepo) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	r.getByIDsCalls.Add(1)
	if r.getByIDsErr != nil {
		return nil, r.getByIDsErr
	}
	return r.schedulerTestOpenAIAccountRepo.GetByIDs(ctx, ids)
}

func TestPrefetchOpenAIAccountsForRequest_GetByIDsOnceAndRecheckUsesMap(t *testing.T) {
	acc1 := &Account{ID: 50001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}
	acc2 := &Account{ID: 50002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5}
	repo := &countingPrefetchAccountRepo{}
	repo.accounts = []Account{*acc1, *acc2}
	snapshotService := &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{snapshotAccounts: []*Account{acc1, acc2}, accountsByID: map[int64]*Account{50001: acc1, 50002: acc2}}}
	svc := &OpenAIGatewayService{
		accountRepo:       repo,
		cfg:               &config.Config{},
		schedulerSnapshot: snapshotService,
	}

	ctx := svc.prefetchOpenAIAccountsForRequest(context.Background(), []*Account{acc1, acc2})
	require.NotNil(t, ctx)
	require.Equal(t, int64(1), repo.getByIDsCalls.Load(), "prefetch 应只做一次 GetByIDs")
	require.Equal(t, int64(0), repo.getByIDCalls.Load(), "预取阶段不应触发单查")

	// recheck 应查预取 map,不触发 GetByID
	latest := svc.recheckSelectedOpenAIAccountFromDB(ctx, acc1, PlatformOpenAI, "gpt-5.1", false, OpenAIEndpointCapabilityChatCompletions)
	require.NotNil(t, latest)
	require.Equal(t, int64(50001), latest.ID)
	require.Equal(t, int64(0), repo.getByIDCalls.Load(), "recheck 命中预取 map,不应再裸 GetByID")
}

func TestPrefetchOpenAIAccountsForRequest_ShadowParentIncluded(t *testing.T) {
	parentID := int64(60001)
	shadow := &Account{ID: 60002, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, ParentAccountID: &parentID}
	parent := &Account{ID: parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}
	repo := &countingPrefetchAccountRepo{}
	repo.accounts = []Account{*shadow, *parent}
	snapshotService := &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{snapshotAccounts: []*Account{shadow, parent}, accountsByID: map[int64]*Account{60002: shadow, parentID: parent}}}
	svc := &OpenAIGatewayService{
		accountRepo:       repo,
		cfg:               &config.Config{},
		schedulerSnapshot: snapshotService,
	}

	// 候选只有 shadow;预取应同时拉取母账号 parent
	ctx := svc.prefetchOpenAIAccountsForRequest(context.Background(), []*Account{shadow})
	require.Equal(t, int64(1), repo.getByIDsCalls.Load())

	// recheck shadow 时 parentHealthyForShadow 应查预取 map,不触发母账号裸 GetByID
	latest := svc.recheckSelectedOpenAIAccountFromDB(ctx, shadow, PlatformOpenAI, "gpt-5.1", false, OpenAIEndpointCapabilityChatCompletions)
	require.NotNil(t, latest)
	require.Equal(t, int64(60002), latest.ID)
	require.Equal(t, int64(0), repo.getByIDCalls.Load(), "母账号解析应命中预取 map")
}

func TestPrefetchOpenAIAccountsForRequest_GetByIDsErrorFallsBack(t *testing.T) {
	acc1 := &Account{ID: 70001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}
	repo := &countingPrefetchAccountRepo{getByIDsErr: errors.New("db down")}
	repo.accounts = []Account{*acc1}
	snapshotService := &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{snapshotAccounts: []*Account{acc1}, accountsByID: map[int64]*Account{70001: acc1}}}
	svc := &OpenAIGatewayService{
		accountRepo:       repo,
		cfg:               &config.Config{},
		schedulerSnapshot: snapshotService,
	}

	// GetByIDs 失败:不设置预取,recheck 回退逐账号 GetByID(fail-open)
	ctx := svc.prefetchOpenAIAccountsForRequest(context.Background(), []*Account{acc1})
	latest := svc.recheckSelectedOpenAIAccountFromDB(ctx, acc1, PlatformOpenAI, "gpt-5.1", false, OpenAIEndpointCapabilityChatCompletions)
	require.NotNil(t, latest, "GetByIDs 失败后 recheck 应回退单查并成功")
	require.Equal(t, int64(70001), latest.ID)
	require.Equal(t, int64(1), repo.getByIDCalls.Load())
}

func TestPrefetchOpenAIAccountsForRequest_NoSnapshotSkips(t *testing.T) {
	acc1 := &Account{ID: 80001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}
	repo := &countingPrefetchAccountRepo{}
	repo.accounts = []Account{*acc1}
	// schedulerSnapshot == nil:预取跳过,recheck 走无 DB 分支
	svc := &OpenAIGatewayService{accountRepo: repo, cfg: &config.Config{}}

	ctx := svc.prefetchOpenAIAccountsForRequest(context.Background(), []*Account{acc1})
	require.Equal(t, int64(0), repo.getByIDsCalls.Load(), "无快照不应触发预取")

	latest := svc.recheckSelectedOpenAIAccountFromDB(ctx, acc1, PlatformOpenAI, "gpt-5.1", false, OpenAIEndpointCapabilityChatCompletions)
	require.NotNil(t, latest)
	require.Equal(t, int64(0), repo.getByIDCalls.Load(), "无快照分支不走 DB")
}
