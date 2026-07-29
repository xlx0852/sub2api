package service

import (
	"sync"
	"time"

	gocache "github.com/patrickmn/go-cache"
)

var (
	sharedUserGroupRateCacheOnce sync.Once
	sharedUserGroupRateCache     *gocache.Cache
	sharedUserGroupRateCacheTTL  time.Duration
)

// sharedUserGroupRateCacheInstance 返回进程内共享的 user:group 倍率覆盖缓存。
// Gateway / OpenAIGateway 必须共用，否则 admin 失效只能清到其中一个。
func sharedUserGroupRateCacheInstance(ttl time.Duration) *gocache.Cache {
	if ttl <= 0 {
		ttl = defaultUserGroupRateCacheTTL
	}
	sharedUserGroupRateCacheOnce.Do(func() {
		sharedUserGroupRateCacheTTL = ttl
		sharedUserGroupRateCache = gocache.New(ttl, time.Minute)
	})
	return sharedUserGroupRateCache
}

func invalidateSharedUserGroupRateCacheForUser(userID int64) {
	if sharedUserGroupRateCache == nil || userID <= 0 {
		return
	}
	newUserGroupRateResolver(nil, sharedUserGroupRateCache, sharedUserGroupRateCacheTTL, nil, "service.user_group_rate").InvalidateUser(userID)
}

func invalidateSharedUserGroupRateCacheForGroup(groupID int64) {
	if sharedUserGroupRateCache == nil || groupID <= 0 {
		return
	}
	newUserGroupRateResolver(nil, sharedUserGroupRateCache, sharedUserGroupRateCacheTTL, nil, "service.user_group_rate").InvalidateGroup(groupID)
}
