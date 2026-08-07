//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ttlcache"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestGroupRepo_GetByIDLite_CacheHitSkipsDB 验证 GetByIDLite 缓存命中时直接返回,
// 不触碰 ent client(此处 client 为 nil, 若走了 DB 查询会 panic)。
func TestGroupRepo_GetByIDLite_CacheHitSkipsDB(t *testing.T) {
	repo := newGroupRepositoryWithSQL(nil, nil) // client 为 nil: 任何 DB 访问都会 panic
	groupID := int64(9901)
	cached := &service.Group{ID: groupID, Name: "cached-group", RequirePrivacySet: true}
	repo.liteCache = newTestGroupLiteCacheWith(groupID, cached)

	got, err := repo.GetByIDLite(context.Background(), groupID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, groupID, got.ID)
	require.Equal(t, "cached-group", got.Name)
	require.True(t, got.RequirePrivacySet, "热路径 schedGroup 依赖 RequirePrivacySet, 必须随缓存返回")
}

// TestGroupRepo_GetByIDLite_Invalidate 验证写路径失效后缓存不再命中。
func TestGroupRepo_GetByIDLite_Invalidate(t *testing.T) {
	repo := newGroupRepositoryWithSQL(nil, nil)
	groupID := int64(9902)
	cached := &service.Group{ID: groupID, Name: "before"}
	repo.liteCache = newTestGroupLiteCacheWith(groupID, cached)

	_, ok := repo.liteCache.Get(cacheKeyForGroupID(groupID))
	require.True(t, ok, "失效前应命中")

	repo.invalidateGroupLiteCache(groupID)
	_, ok = repo.liteCache.Get(cacheKeyForGroupID(groupID))
	require.False(t, ok, "失效后应 miss")
}

// newTestGroupLiteCacheWith 构造一个已预置指定 group 的 liteCache(经 Load 回填, 不走 DB)。
func newTestGroupLiteCacheWith(groupID int64, g *service.Group) *ttlcache.Cache[*service.Group] {
	c := ttlcache.New[*service.Group](groupLiteCacheTTL)
	_, _ = c.Load(context.Background(), cacheKeyForGroupID(groupID), func(ctx context.Context) (*service.Group, error) {
		return g, nil
	})
	return c
}
