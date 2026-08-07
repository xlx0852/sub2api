// Package ttlcache 提供带 TTL 的泛型进程内缓存（singleflight 合并并发回源）。
// 语义参照 internal/handler/admin/snapshot_cache.go：Get 命中且未过期直接返回，
// 未命中走 Load 的 singleflight + 回填；fn 失败不缓存。用于网关/报表热路径
// 避免重复 DB 往返（如 group 读取、usage 聚合）。
package ttlcache

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type entry[V any] struct {
	value     V
	expiresAt int64 // unix nano
}

// Cache 是带 TTL 的泛型进程内缓存。零值不可用，需通过 New 创建。
type Cache[V any] struct {
	ttl  time.Duration
	mu   sync.Mutex
	data map[string]entry[V]
	sf   singleflight.Group
}

// New 创建 TTL 缓存。ttl <= 0 表示禁用缓存：Get 恒 miss，Load 直接透传 fn
// （不缓存、不合并），便于测试关闭缓存或语义上关闭某条路径的缓存。
func New[V any](ttl time.Duration) *Cache[V] {
	if ttl <= 0 {
		ttl = 0
	}
	return &Cache[V]{
		ttl:  ttl,
		data: make(map[string]entry[V]),
	}
}

// Get 返回 key 对应且未过期的缓存值。禁用缓存或未命中返回零值 + false。
func (c *Cache[V]) Get(key string) (V, bool) {
	var zero V
	if c == nil || c.ttl <= 0 {
		return zero, false
	}
	now := time.Now().UnixNano()
	c.mu.Lock()
	e, ok := c.data[key]
	if ok && now >= e.expiresAt {
		delete(c.data, key)
		ok = false
	}
	c.mu.Unlock()
	if !ok {
		return zero, false
	}
	return e.value, true
}

// Load 返回 key 的缓存值；未命中时通过 fn 回填并缓存。
// 并发对同一 key 的 Load 由 singleflight 合并，fn 只执行一次（所有调用者拿到同一结果）。
// fn 返回错误时不缓存，错误透传给所有等待者。
// 禁用缓存（ttl <= 0）时直接调用 fn，不缓存不合并。
func (c *Cache[V]) Load(ctx context.Context, key string, fn func(ctx context.Context) (V, error)) (V, error) {
	var zero V
	if c == nil || c.ttl <= 0 {
		return fn(ctx)
	}
	if v, ok := c.Get(key); ok {
		return v, nil
	}
	v, err, _ := c.sf.Do(key, func() (any, error) {
		// singleflight 合并窗口内可能已有其他调用者回填
		if cached, ok := c.Get(key); ok {
			return cached, nil
		}
		value, fnErr := fn(ctx)
		if fnErr != nil {
			return zero, fnErr
		}
		c.set(key, value)
		return value, nil
	})
	if err != nil {
		return zero, err
	}
	value, _ := v.(V)
	return value, nil
}

// set 写入缓存条目（内部用，不判空 ttl）。
func (c *Cache[V]) set(key string, value V) {
	now := time.Now().UnixNano()
	c.mu.Lock()
	if c.data == nil {
		c.data = make(map[string]entry[V])
	}
	c.data[key] = entry[V]{value: value, expiresAt: now + int64(c.ttl)}
	c.mu.Unlock()
}

// Delete 删除指定 key（缓存失效）。禁用缓存时为空操作。
func (c *Cache[V]) Delete(key string) {
	if c == nil || c.ttl <= 0 {
		return
	}
	c.sf.Forget(key)
	c.mu.Lock()
	delete(c.data, key)
	c.mu.Unlock()
}

// Clear 清空全部缓存条目。禁用缓存时为空操作。
func (c *Cache[V]) Clear() {
	if c == nil || c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	c.data = make(map[string]entry[V])
	c.mu.Unlock()
}
