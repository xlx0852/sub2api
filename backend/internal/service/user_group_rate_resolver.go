package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

// userGroupRateCacheEntry 只缓存“用户专属覆盖”查询结果，不缓存最终计费倍率。
// 这样分组默认倍率变更后无需等待 TTL，也不会被旧默认值污染。
type userGroupRateCacheEntry struct {
	// hasOverride=true 表示 user_group_rate_multipliers 有专属值；false 表示无专属覆盖。
	hasOverride bool
	// override 仅在 hasOverride=true 时有效。
	override float64
}

type userGroupRateResolver struct {
	repo         UserGroupRateRepository
	cache        *gocache.Cache
	cacheTTL     time.Duration
	sf           *singleflight.Group
	logComponent string
}

func newUserGroupRateResolver(repo UserGroupRateRepository, cache *gocache.Cache, cacheTTL time.Duration, sf *singleflight.Group, logComponent string) *userGroupRateResolver {
	if cacheTTL <= 0 {
		cacheTTL = defaultUserGroupRateCacheTTL
	}
	if cache == nil {
		cache = gocache.New(cacheTTL, time.Minute)
	}
	if logComponent == "" {
		logComponent = "service.gateway"
	}
	if sf == nil {
		sf = &singleflight.Group{}
	}

	return &userGroupRateResolver{
		repo:         repo,
		cache:        cache,
		cacheTTL:     cacheTTL,
		sf:           sf,
		logComponent: logComponent,
	}
}

func (r *userGroupRateResolver) Resolve(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if r == nil || userID <= 0 || groupID <= 0 {
		return groupDefaultMultiplier
	}

	key := fmt.Sprintf("%d:%d", userID, groupID)
	if entry, ok := r.getCachedEntry(key); ok {
		userGroupRateCacheHitTotal.Add(1)
		if entry.hasOverride {
			return entry.override
		}
		return groupDefaultMultiplier
	}
	if r.repo == nil {
		return groupDefaultMultiplier
	}
	userGroupRateCacheMissTotal.Add(1)

	value, err, shared := r.sf.Do(key, func() (any, error) {
		if entry, ok := r.getCachedEntry(key); ok {
			userGroupRateCacheHitTotal.Add(1)
			return entry, nil
		}

		userGroupRateCacheLoadTotal.Add(1)
		userRate, repoErr := r.repo.GetByUserAndGroup(ctx, userID, groupID)
		if repoErr != nil {
			return nil, repoErr
		}

		entry := userGroupRateCacheEntry{}
		if userRate != nil {
			entry.hasOverride = true
			entry.override = *userRate
		}
		if r.cache != nil {
			r.cache.Set(key, entry, r.cacheTTL)
		}
		return entry, nil
	})
	if shared {
		userGroupRateCacheSFSharedTotal.Add(1)
	}
	if err != nil {
		userGroupRateCacheFallbackTotal.Add(1)
		logger.LegacyPrintf(r.logComponent, "get user group rate failed, fallback to group default: user=%d group=%d err=%v", userID, groupID, err)
		return groupDefaultMultiplier
	}

	entry, ok := value.(userGroupRateCacheEntry)
	if !ok {
		userGroupRateCacheFallbackTotal.Add(1)
		return groupDefaultMultiplier
	}
	if entry.hasOverride {
		return entry.override
	}
	return groupDefaultMultiplier
}

func (r *userGroupRateResolver) getCachedEntry(key string) (userGroupRateCacheEntry, bool) {
	if r == nil || r.cache == nil {
		return userGroupRateCacheEntry{}, false
	}
	cached, ok := r.cache.Get(key)
	if !ok {
		return userGroupRateCacheEntry{}, false
	}
	switch entry := cached.(type) {
	case userGroupRateCacheEntry:
		return entry, true
	case *userGroupRateCacheEntry:
		if entry == nil {
			return userGroupRateCacheEntry{}, false
		}
		return *entry, true
	default:
		// 兼容旧缓存形态（直接缓存最终 float64）：视为失效，强制回源。
		r.cache.Delete(key)
		return userGroupRateCacheEntry{}, false
	}
}

// InvalidateUser 清除某用户在所有分组上的专属倍率缓存。
func (r *userGroupRateResolver) InvalidateUser(userID int64) {
	if r == nil || r.cache == nil || userID <= 0 {
		return
	}
	prefix := fmt.Sprintf("%d:", userID)
	for key := range r.cache.Items() {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			r.cache.Delete(key)
		}
	}
}

// InvalidateGroup 清除某分组下所有用户的专属倍率缓存。
func (r *userGroupRateResolver) InvalidateGroup(groupID int64) {
	if r == nil || r.cache == nil || groupID <= 0 {
		return
	}
	suffix := fmt.Sprintf(":%d", groupID)
	for key := range r.cache.Items() {
		if len(key) >= len(suffix) && key[len(key)-len(suffix):] == suffix {
			r.cache.Delete(key)
		}
	}
}

// InvalidateUserGroup 清除单个 user+group 的专属倍率缓存。
func (r *userGroupRateResolver) InvalidateUserGroup(userID, groupID int64) {
	if r == nil || r.cache == nil || userID <= 0 || groupID <= 0 {
		return
	}
	r.cache.Delete(fmt.Sprintf("%d:%d", userID, groupID))
}
