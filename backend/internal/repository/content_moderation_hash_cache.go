package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const contentModerationFlaggedHashSetKey = "content_moderation:flagged_hashes"
const contentModerationCleanHashPrefix   = "content_moderation:clean_hash:"
const contentModerationElevatedUserPrefix = "content_moderation:elevated_user:"

type contentModerationHashCache struct {
	rdb *redis.Client
}

func NewContentModerationHashCache(rdb *redis.Client) service.ContentModerationHashCache {
	return &contentModerationHashCache{rdb: rdb}
}

func (c *contentModerationHashCache) RecordFlaggedInputHash(ctx context.Context, inputHash string) error {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return nil
	}
	return c.rdb.SAdd(ctx, contentModerationFlaggedHashSetKey, inputHash).Err()
}

func (c *contentModerationHashCache) HasFlaggedInputHash(ctx context.Context, inputHash string) (bool, error) {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return false, nil
	}
	return c.rdb.SIsMember(ctx, contentModerationFlaggedHashSetKey, inputHash).Result()
}

func (c *contentModerationHashCache) cleanHashKey(inputHash string) string {
	return contentModerationCleanHashPrefix + strings.TrimSpace(inputHash)
}

func (c *contentModerationHashCache) RecordCleanInputHash(ctx context.Context, inputHash string, ttl time.Duration) error {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return c.rdb.Set(ctx, c.cleanHashKey(inputHash), "1", ttl).Err()
}

func (c *contentModerationHashCache) HasCleanInputHash(ctx context.Context, inputHash string) (bool, error) {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return false, nil
	}
	n, err := c.rdb.Exists(ctx, c.cleanHashKey(inputHash)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (c *contentModerationHashCache) DeleteCleanInputHash(ctx context.Context, inputHash string) error {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return nil
	}
	return c.rdb.Del(ctx, c.cleanHashKey(inputHash)).Err()
}

func (c *contentModerationHashCache) elevatedUserKey(userID int64) string {
	return contentModerationElevatedUserPrefix + fmt.Sprintf("%d", userID)
}

func (c *contentModerationHashCache) SetElevatedUserSampling(ctx context.Context, userID int64, elevated bool, ttl time.Duration) error {
	if c == nil || c.rdb == nil || userID <= 0 {
		return nil
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	val := "0"
	if elevated {
		val = "1"
	}
	return c.rdb.Set(ctx, c.elevatedUserKey(userID), val, ttl).Err()
}

func (c *contentModerationHashCache) GetElevatedUserSampling(ctx context.Context, userID int64) (bool, bool, error) {
	if c == nil || c.rdb == nil || userID <= 0 {
		return false, false, nil
	}
	val, err := c.rdb.Get(ctx, c.elevatedUserKey(userID)).Result()
	if err == redis.Nil {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, val == "1", nil
}

func (c *contentModerationHashCache) ListElevatedUsers(ctx context.Context) ([]service.ContentModerationElevatedUserCacheEntry, error) {
	out := make([]service.ContentModerationElevatedUserCacheEntry, 0)
	if c == nil || c.rdb == nil {
		return out, nil
	}
	var cursor uint64
	for {
		keys, next, err := c.rdb.Scan(ctx, cursor, contentModerationElevatedUserPrefix+"*", 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			val, err := c.rdb.Get(ctx, key).Result()
			if err == redis.Nil {
				continue
			}
			if err != nil {
				return nil, err
			}
			if val != "1" {
				continue
			}
			idRaw := strings.TrimPrefix(key, contentModerationElevatedUserPrefix)
			uid, err := strconv.ParseInt(idRaw, 10, 64)
			if err != nil || uid <= 0 {
				continue
			}
			ttl, err := c.rdb.TTL(ctx, key).Result()
			if err != nil {
				return nil, err
			}
			sec := int64(ttl.Seconds())
			if sec < 0 {
				// -1 no expire / -2 missing; treat missing as skip, noexpire as large
				if ttl < 0 && ttl.Seconds() == -2 {
					continue
				}
				if sec < 0 {
					sec = 0
				}
			}
			out = append(out, service.ContentModerationElevatedUserCacheEntry{UserID: uid, TTLSeconds: sec})
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return out, nil
}

func (c *contentModerationHashCache) DeleteFlaggedInputHash(ctx context.Context, inputHash string) (bool, error) {
	inputHash = strings.TrimSpace(inputHash)
	if c == nil || c.rdb == nil || inputHash == "" {
		return false, nil
	}
	deleted, err := c.rdb.SRem(ctx, contentModerationFlaggedHashSetKey, inputHash).Result()
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func (c *contentModerationHashCache) ClearFlaggedInputHashes(ctx context.Context) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	deleted, err := c.rdb.SCard(ctx, contentModerationFlaggedHashSetKey).Result()
	if err != nil {
		return 0, err
	}
	if deleted == 0 {
		return 0, nil
	}
	if err := c.rdb.Del(ctx, contentModerationFlaggedHashSetKey).Err(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (c *contentModerationHashCache) CountFlaggedInputHashes(ctx context.Context) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	return c.rdb.SCard(ctx, contentModerationFlaggedHashSetKey).Result()
}
