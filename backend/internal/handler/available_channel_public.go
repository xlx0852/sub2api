package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/modelcatalog"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	publicPricingCacheVersion = 2
	publicPricingRedisKey     = "public:pricing:snapshot:v2"
	publicPricingLockKey      = "public:pricing:rebuild-lock:v2"
	publicPricingFreshTTL     = time.Hour
	publicPricingRedisTTL     = 24 * time.Hour
)

type publicPricingCacheEntry struct {
	snapshot  *publicPricingSnapshot
	etag      string
	loadedAt  time.Time
	generated time.Time
}

type publicPricingSnapshot struct {
	Version     int                  `json:"version"`
	GeneratedAt time.Time            `json:"generated_at"`
	Models      []publicPricingModel `json:"models"`
}

type publicPricingModel struct {
	Name        string                     `json:"name"`
	Platform    string                     `json:"platform"`
	DisplayName string                     `json:"display_name"`
	Family      string                     `json:"family,omitempty"`
	IsReasoning bool                       `json:"is_reasoning"`
	Media       map[string]bool            `json:"media"`
	Pricing     *userSupportedModelPricing `json:"pricing"`
}

// PublicPricing returns a cached anonymous-safe price catalog.
// GET /api/v1/public/pricing
func (h *AvailableChannelHandler) PublicPricing(c *gin.Context) {
	entry, err := h.getPublicPricing(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Public pricing is temporarily unavailable")
		return
	}
	setPublicPricingCacheHeaders(c, entry.etag)
	if strings.TrimSpace(c.GetHeader("If-None-Match")) == entry.etag {
		c.Status(304)
		return
	}
	response.Success(c, entry.snapshot)
}

func setPublicPricingCacheHeaders(c *gin.Context, etag string) {
	c.Header("Cache-Control", "public, max-age=300, s-maxage=3600, stale-while-revalidate=86400, stale-if-error=86400")
	c.Header("ETag", etag)
	c.Header("Vary", "Accept-Encoding")
}

func (h *AvailableChannelHandler) getPublicPricing(ctx context.Context) (*publicPricingCacheEntry, error) {
	if cached := h.loadPublicPricingL1(); cached != nil {
		if time.Since(cached.loadedAt) < publicPricingFreshTTL {
			return cached, nil
		}
		go h.refreshPublicPricing(context.Background())
		return cached, nil
	}

	value, err, _ := h.publicPricingSF.Do("cold", func() (any, error) {
		if cached := h.loadPublicPricingL1(); cached != nil {
			return cached, nil
		}
		if cached, err := h.loadPublicPricingRedis(ctx); err == nil && cached != nil {
			h.storePublicPricingL1(cached)
			return cached, nil
		}
		return h.rebuildPublicPricing(ctx)
	})
	if err != nil {
		return nil, err
	}
	entry, ok := value.(*publicPricingCacheEntry)
	if !ok || entry == nil {
		return nil, fmt.Errorf("invalid public pricing cache entry")
	}
	return entry, nil
}

func (h *AvailableChannelHandler) refreshPublicPricing(ctx context.Context) {
	_, _, _ = h.publicPricingSF.Do("refresh", func() (any, error) {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		return h.rebuildPublicPricing(ctx)
	})
}

func (h *AvailableChannelHandler) rebuildPublicPricing(ctx context.Context) (*publicPricingCacheEntry, error) {
	if h.redisClient == nil {
		return h.buildAndStorePublicPricing(ctx)
	}
	locked, err := h.redisClient.SetNX(ctx, publicPricingLockKey, "1", 30*time.Second).Result()
	if err != nil {
		return nil, err
	}
	if !locked {
		if cached, err := h.loadPublicPricingRedis(ctx); err == nil && cached != nil {
			return cached, nil
		}
		return nil, fmt.Errorf("public pricing snapshot rebuild already in progress")
	}
	defer h.redisClient.Del(context.Background(), publicPricingLockKey)
	return h.buildAndStorePublicPricing(ctx)
}

func (h *AvailableChannelHandler) buildAndStorePublicPricing(ctx context.Context) (*publicPricingCacheEntry, error) {
	channels, err := h.channelService.ListAvailable(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := buildPublicPricingSnapshot(channels, modelcatalog.PublicView(), time.Now().UTC())
	entry, payload, err := encodePublicPricingSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	h.storePublicPricingL1(entry)
	if h.redisClient != nil {
		_ = h.redisClient.Set(ctx, publicPricingRedisKey, payload, publicPricingRedisTTL).Err()
	}
	return entry, nil
}

func (h *AvailableChannelHandler) loadPublicPricingRedis(ctx context.Context) (*publicPricingCacheEntry, error) {
	if h.redisClient == nil {
		return nil, redis.Nil
	}
	payload, err := h.redisClient.Get(ctx, publicPricingRedisKey).Bytes()
	if err != nil {
		return nil, err
	}
	var snapshot publicPricingSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Version != publicPricingCacheVersion {
		return nil, fmt.Errorf("unsupported public pricing cache version: %d", snapshot.Version)
	}
	entry, _, err := encodePublicPricingSnapshot(&snapshot)
	return entry, err
}

func encodePublicPricingSnapshot(snapshot *publicPricingSnapshot) (*publicPricingCacheEntry, []byte, error) {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, nil, err
	}
	sum := sha256.Sum256(payload)
	return &publicPricingCacheEntry{
		snapshot:  snapshot,
		etag:      `"` + hex.EncodeToString(sum[:]) + `"`,
		loadedAt:  time.Now(),
		generated: snapshot.GeneratedAt,
	}, payload, nil
}

func (h *AvailableChannelHandler) loadPublicPricingL1() *publicPricingCacheEntry {
	h.publicPricingMu.RLock()
	defer h.publicPricingMu.RUnlock()
	return h.publicPricingCache
}

func (h *AvailableChannelHandler) storePublicPricingL1(entry *publicPricingCacheEntry) {
	h.publicPricingMu.Lock()
	h.publicPricingCache = entry
	h.publicPricingMu.Unlock()
}

func buildPublicPricingSnapshot(channels []service.AvailableChannel, catalog *modelcatalog.Catalog, generatedAt time.Time) *publicPricingSnapshot {
	byKey := make(map[string]publicPricingModel)
	catalogModels := publicCatalogModelIndex(catalog)

	for _, channel := range channels {
		if channel.Status != service.StatusActive {
			continue
		}
		for _, model := range channel.SupportedModels {
			platform := strings.ToLower(strings.TrimSpace(model.Platform))
			if platform == "" || model.Pricing == nil {
				continue
			}
			for _, group := range channel.Groups {
				if group.IsExclusive || !strings.EqualFold(group.Platform, platform) {
					continue
				}
				pricing := multiplyPublicPricing(toUserPricing(model.Pricing), group.RateMultiplier)
				key := platform + "::" + strings.ToLower(model.Name)
				current, exists := byKey[key]
				if exists && publicPricingSortValue(current.Pricing) <= publicPricingSortValue(pricing) {
					continue
				}
				meta := catalogModels[key]
				byKey[key] = publicPricingModel{
					Name:        model.Name,
					Platform:    platform,
					DisplayName: firstNonBlank(meta.displayName, model.Name),
					Family:      meta.family,
					IsReasoning: meta.reasoning,
					Media:       meta.media,
					Pricing:     pricing,
				}
			}
		}
	}

	models := make([]publicPricingModel, 0, len(byKey))
	for _, model := range byKey {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Platform != models[j].Platform {
			return models[i].Platform < models[j].Platform
		}
		return strings.ToLower(models[i].DisplayName) < strings.ToLower(models[j].DisplayName)
	})
	return &publicPricingSnapshot{
		Version:     publicPricingCacheVersion,
		GeneratedAt: generatedAt,
		Models:      models,
	}
}

type publicCatalogMeta struct {
	displayName string
	family      string
	reasoning   bool
	media       map[string]bool
}

func publicCatalogModelIndex(catalog *modelcatalog.Catalog) map[string]publicCatalogMeta {
	out := make(map[string]publicCatalogMeta)
	if catalog == nil {
		return out
	}
	for platform, cfg := range catalog.Platforms {
		for _, model := range cfg.Models {
			out[strings.ToLower(platform)+"::"+strings.ToLower(model.ID)] = publicCatalogMeta{
				displayName: model.DisplayName, family: model.Family,
				reasoning: model.IsReasoning, media: model.Media,
			}
		}
	}
	return out
}

func multiplyPublicPricing(pricing *userSupportedModelPricing, multiplier float64) *userSupportedModelPricing {
	if pricing == nil {
		return nil
	}
	if multiplier <= 0 {
		multiplier = 1
	}
	out := *pricing
	out.InputPrice = multiplyFloatPtr(out.InputPrice, multiplier)
	out.OutputPrice = multiplyFloatPtr(out.OutputPrice, multiplier)
	out.CacheWritePrice = multiplyFloatPtr(out.CacheWritePrice, multiplier)
	out.CacheReadPrice = multiplyFloatPtr(out.CacheReadPrice, multiplier)
	out.ImageOutputPrice = multiplyFloatPtr(out.ImageOutputPrice, multiplier)
	out.PerRequestPrice = multiplyFloatPtr(out.PerRequestPrice, multiplier)
	out.Intervals = append([]userPricingIntervalDTO(nil), pricing.Intervals...)
	for i := range out.Intervals {
		out.Intervals[i].InputPrice = multiplyFloatPtr(out.Intervals[i].InputPrice, multiplier)
		out.Intervals[i].OutputPrice = multiplyFloatPtr(out.Intervals[i].OutputPrice, multiplier)
		out.Intervals[i].CacheWritePrice = multiplyFloatPtr(out.Intervals[i].CacheWritePrice, multiplier)
		out.Intervals[i].CacheReadPrice = multiplyFloatPtr(out.Intervals[i].CacheReadPrice, multiplier)
		out.Intervals[i].PerRequestPrice = multiplyFloatPtr(out.Intervals[i].PerRequestPrice, multiplier)
	}
	return &out
}

func multiplyFloatPtr(value *float64, multiplier float64) *float64 {
	if value == nil {
		return nil
	}
	v := *value * multiplier
	return &v
}

func publicPricingSortValue(pricing *userSupportedModelPricing) float64 {
	if pricing == nil {
		return 1e308
	}
	for _, value := range []*float64{pricing.InputPrice, pricing.PerRequestPrice, pricing.ImageOutputPrice, pricing.OutputPrice} {
		if value != nil {
			return *value
		}
	}
	return 1e308
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
