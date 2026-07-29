package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type snapshotCacheEntry struct {
	ETag      string
	Payload   any
	ExpiresAt time.Time
}

type snapshotCache struct {
	mu         sync.RWMutex
	ttl        time.Duration
	maxEntries int
	generation uint64
	items      map[string]snapshotCacheEntry
	sf         singleflight.Group
}

// WithMaxEntries applies a hard entry limit. Zero keeps the historical
// unlimited behavior for existing callers.
func (c *snapshotCache) WithMaxEntries(maxEntries int) *snapshotCache {
	if c == nil {
		return c
	}
	c.mu.Lock()
	c.maxEntries = maxEntries
	c.evictLocked(time.Now())
	c.mu.Unlock()
	return c
}

type snapshotCacheLoadResult struct {
	Entry snapshotCacheEntry
	Hit   bool
}

func newSnapshotCache(ttl time.Duration) *snapshotCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &snapshotCache{
		ttl:   ttl,
		items: make(map[string]snapshotCacheEntry),
	}
}

func (c *snapshotCache) Get(key string) (snapshotCacheEntry, bool) {
	if c == nil || key == "" {
		return snapshotCacheEntry{}, false
	}
	now := time.Now()

	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return snapshotCacheEntry{}, false
	}
	if now.After(entry.ExpiresAt) {
		c.mu.Lock()
		if current, exists := c.items[key]; exists && now.After(current.ExpiresAt) {
			delete(c.items, key)
		}
		c.mu.Unlock()
		return snapshotCacheEntry{}, false
	}
	return entry, true
}

func (c *snapshotCache) Set(key string, payload any) snapshotCacheEntry {
	if c == nil {
		return snapshotCacheEntry{}
	}
	entry := snapshotCacheEntry{
		ETag:      buildETagFromAny(payload),
		Payload:   payload,
		ExpiresAt: time.Now().Add(c.ttl),
	}
	if key == "" {
		return entry
	}
	c.mu.Lock()
	c.evictLocked(time.Now())
	if c.maxEntries > 0 {
		if _, exists := c.items[key]; !exists && len(c.items) >= c.maxEntries {
			c.evictOldestLocked()
		}
	}
	c.items[key] = entry
	c.mu.Unlock()
	return entry
}

func (c *snapshotCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.items = make(map[string]snapshotCacheEntry)
	c.generation++
	c.mu.Unlock()
}

func (c *snapshotCache) generationValue() uint64 {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	generation := c.generation
	c.mu.RUnlock()
	return generation
}

func (c *snapshotCache) setIfGeneration(key string, payload any, generation uint64) snapshotCacheEntry {
	if c == nil {
		return snapshotCacheEntry{}
	}
	entry := snapshotCacheEntry{
		ETag:      buildETagFromAny(payload),
		Payload:   payload,
		ExpiresAt: time.Now().Add(c.ttl),
	}
	if key == "" {
		return entry
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation {
		return entry
	}
	c.evictLocked(time.Now())
	if c.maxEntries > 0 {
		if _, exists := c.items[key]; !exists && len(c.items) >= c.maxEntries {
			c.evictOldestLocked()
		}
	}
	c.items[key] = entry
	return entry
}

func (c *snapshotCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	c.evictLocked(time.Now())
	length := len(c.items)
	c.mu.Unlock()
	return length
}

func (c *snapshotCache) evictLocked(now time.Time) {
	for key, entry := range c.items {
		if now.After(entry.ExpiresAt) {
			delete(c.items, key)
		}
	}
	for c.maxEntries > 0 && len(c.items) > c.maxEntries {
		c.evictOldestLocked()
	}
}

func (c *snapshotCache) evictOldestLocked() {
	var oldestKey string
	var oldestExpiry time.Time
	for key, entry := range c.items {
		if oldestKey == "" || entry.ExpiresAt.Before(oldestExpiry) {
			oldestKey = key
			oldestExpiry = entry.ExpiresAt
		}
	}
	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

func (c *snapshotCache) GetOrLoad(key string, load func() (any, error)) (snapshotCacheEntry, bool, error) {
	if load == nil {
		return snapshotCacheEntry{}, false, nil
	}
	if entry, ok := c.Get(key); ok {
		return entry, true, nil
	}
	if c == nil || key == "" {
		payload, err := load()
		if err != nil {
			return snapshotCacheEntry{}, false, err
		}
		return c.Set(key, payload), false, nil
	}

	value, err, _ := c.sf.Do(key, func() (any, error) {
		if entry, ok := c.Get(key); ok {
			return snapshotCacheLoadResult{Entry: entry, Hit: true}, nil
		}
		generation := c.generationValue()
		payload, err := load()
		if err != nil {
			return nil, err
		}
		return snapshotCacheLoadResult{Entry: c.setIfGeneration(key, payload, generation), Hit: false}, nil
	})
	if err != nil {
		return snapshotCacheEntry{}, false, err
	}
	result, ok := value.(snapshotCacheLoadResult)
	if !ok {
		return snapshotCacheEntry{}, false, nil
	}
	return result.Entry, result.Hit, nil
}

// Refresh collapses concurrent forced reloads for one key and replaces any
// existing entry. Normal readers may continue using the previous valid entry
// while the refresh is in flight.
func (c *snapshotCache) Refresh(key string, load func() (any, error)) (snapshotCacheEntry, error) {
	if load == nil {
		return snapshotCacheEntry{}, nil
	}
	if c == nil || key == "" {
		payload, err := load()
		if err != nil {
			return snapshotCacheEntry{}, err
		}
		return c.Set(key, payload), nil
	}
	value, err, _ := c.sf.Do("refresh:"+key, func() (any, error) {
		generation := c.generationValue()
		payload, loadErr := load()
		if loadErr != nil {
			return nil, loadErr
		}
		return c.setIfGeneration(key, payload, generation), nil
	})
	if err != nil {
		return snapshotCacheEntry{}, err
	}
	entry, _ := value.(snapshotCacheEntry)
	return entry, nil
}

func buildETagFromAny(payload any) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "\"" + hex.EncodeToString(sum[:]) + "\""
}

func parseBoolQueryWithDefault(raw string, def bool) bool {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return def
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
