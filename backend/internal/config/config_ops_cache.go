package config

import "time"

type OpsConfig struct {
	// Enabled controls whether ops features should run.
	//
	// NOTE: vNext still has a DB-backed feature flag (ops_monitoring_enabled) for runtime on/off.
	// This config flag is the "hard switch" for deployments that want to disable ops completely.
	Enabled bool `mapstructure:"enabled"`

	// UsePreaggregatedTables prefers ops_metrics_hourly/daily for long-window dashboard queries.
	UsePreaggregatedTables bool `mapstructure:"use_preaggregated_tables"`

	// Cleanup controls periodic deletion of old ops data to prevent unbounded growth.
	Cleanup OpsCleanupConfig `mapstructure:"cleanup"`

	// MetricsCollectorCache controls Redis caching for expensive per-window collector queries.
	MetricsCollectorCache OpsMetricsCollectorCacheConfig `mapstructure:"metrics_collector_cache"`

	// Pre-aggregation configuration.
	Aggregation OpsAggregationConfig `mapstructure:"aggregation"`

	// ProviderStatus controls best-effort collection of public provider status data.
	ProviderStatus OpsProviderStatusConfig `mapstructure:"provider_status"`
}

type OpsProviderStatusConfig struct {
	Enabled               bool `mapstructure:"enabled"`
	PollIntervalSeconds   int  `mapstructure:"poll_interval_seconds"`
	StaleAfterSeconds     int  `mapstructure:"stale_after_seconds"`
	RequestTimeoutSeconds int  `mapstructure:"request_timeout_seconds"`
	MaxBodyBytes          int  `mapstructure:"max_body_bytes"`
}

type OpsCleanupConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Schedule string `mapstructure:"schedule"`

	// Retention days (0 disables that cleanup target).
	//
	// vNext requirement: default 30 days across ops datasets.
	ErrorLogRetentionDays      int `mapstructure:"error_log_retention_days"`
	MinuteMetricsRetentionDays int `mapstructure:"minute_metrics_retention_days"`
	HourlyMetricsRetentionDays int `mapstructure:"hourly_metrics_retention_days"`
}

type OpsAggregationConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

type OpsMetricsCollectorCacheConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	TTL     time.Duration `mapstructure:"ttl"`
}

type JWTConfig struct {
	Secret     string `mapstructure:"secret"`
	ExpireHour int    `mapstructure:"expire_hour"`
	// AccessTokenExpireMinutes: Access Token有效期（分钟）
	// - >0: 使用分钟配置（优先级高于 ExpireHour）
	// - =0: 回退使用 ExpireHour（向后兼容旧配置）
	AccessTokenExpireMinutes int `mapstructure:"access_token_expire_minutes"`
	// RefreshTokenExpireDays: Refresh Token有效期（天），默认30天
	RefreshTokenExpireDays int `mapstructure:"refresh_token_expire_days"`
	// RefreshWindowMinutes: 刷新窗口（分钟），在Access Token过期前多久开始允许刷新
	RefreshWindowMinutes int `mapstructure:"refresh_window_minutes"`
}

// TotpConfig TOTP 双因素认证配置
type TotpConfig struct {
	// EncryptionKey 用于加密 TOTP 密钥的 AES-256 密钥（32 字节 hex 编码）
	// 如果为空，将自动生成一个随机密钥（仅适用于开发环境）
	EncryptionKey string `mapstructure:"encryption_key"`
	// EncryptionKeyConfigured 标记加密密钥是否为手动配置（非自动生成）
	// 只有手动配置了密钥才允许在管理后台启用 TOTP 功能
	EncryptionKeyConfigured bool `mapstructure:"-"`
}

type TurnstileConfig struct {
	Required bool `mapstructure:"required"`
}

type DefaultConfig struct {
	AdminEmail      string  `mapstructure:"admin_email"`
	AdminPassword   string  `mapstructure:"admin_password"`
	UserConcurrency int     `mapstructure:"user_concurrency"`
	UserBalance     float64 `mapstructure:"user_balance"`
	APIKeyPrefix    string  `mapstructure:"api_key_prefix"`
	RateMultiplier  float64 `mapstructure:"rate_multiplier"`
}

type RateLimitConfig struct {
	OverloadCooldownMinutes int `mapstructure:"overload_cooldown_minutes"`  // 529过载冷却时间(分钟)
	OAuth401CooldownMinutes int `mapstructure:"oauth_401_cooldown_minutes"` // OAuth 401临时不可调度冷却(分钟)
}

// APIKeyAuthCacheConfig API Key 认证缓存配置
type APIKeyAuthCacheConfig struct {
	L1Size             int  `mapstructure:"l1_size"`
	L1TTLSeconds       int  `mapstructure:"l1_ttl_seconds"`
	L2TTLSeconds       int  `mapstructure:"l2_ttl_seconds"`
	NegativeTTLSeconds int  `mapstructure:"negative_ttl_seconds"`
	JitterPercent      int  `mapstructure:"jitter_percent"`
	Singleflight       bool `mapstructure:"singleflight"`
}

// SubscriptionCacheConfig 订阅认证 L1 缓存配置
type SubscriptionCacheConfig struct {
	L1Size        int `mapstructure:"l1_size"`
	L1TTLSeconds  int `mapstructure:"l1_ttl_seconds"`
	JitterPercent int `mapstructure:"jitter_percent"`
}

// SubscriptionMaintenanceConfig 订阅窗口维护后台任务配置。
// 用于将“请求路径触发的维护动作”有界化，避免高并发下 goroutine 膨胀。
type SubscriptionMaintenanceConfig struct {
	WorkerCount int `mapstructure:"worker_count"`
	QueueSize   int `mapstructure:"queue_size"`
}

// DashboardCacheConfig 仪表盘统计缓存配置
type DashboardCacheConfig struct {
	// Enabled: 是否启用仪表盘缓存
	Enabled bool `mapstructure:"enabled"`
	// KeyPrefix: Redis key 前缀，用于多环境隔离
	KeyPrefix string `mapstructure:"key_prefix"`
	// StatsFreshTTLSeconds: 缓存命中认为“新鲜”的时间窗口（秒）
	StatsFreshTTLSeconds int `mapstructure:"stats_fresh_ttl_seconds"`
	// StatsTTLSeconds: Redis 缓存总 TTL（秒）
	StatsTTLSeconds int `mapstructure:"stats_ttl_seconds"`
	// StatsRefreshTimeoutSeconds: 异步刷新超时（秒）
	StatsRefreshTimeoutSeconds int `mapstructure:"stats_refresh_timeout_seconds"`
}

// DashboardAggregationConfig 仪表盘预聚合配置
type DashboardAggregationConfig struct {
	// Enabled: 是否启用预聚合作业
	Enabled bool `mapstructure:"enabled"`
	// IntervalSeconds: 聚合刷新间隔（秒）
	IntervalSeconds int `mapstructure:"interval_seconds"`
	// LookbackSeconds: 回看窗口（秒）
	LookbackSeconds int `mapstructure:"lookback_seconds"`
	// BackfillEnabled: 是否允许全量回填
	BackfillEnabled bool `mapstructure:"backfill_enabled"`
	// BackfillMaxDays: 回填最大跨度（天）
	BackfillMaxDays int `mapstructure:"backfill_max_days"`
	// Retention: 各表保留窗口（天）
	Retention DashboardAggregationRetentionConfig `mapstructure:"retention"`
	// RecomputeDays: 启动时重算最近 N 天
	RecomputeDays int `mapstructure:"recompute_days"`
}

// DashboardAggregationRetentionConfig 预聚合保留窗口
type DashboardAggregationRetentionConfig struct {
	UsageLogsDays         int `mapstructure:"usage_logs_days"`
	UsageBillingDedupDays int `mapstructure:"usage_billing_dedup_days"`
	HourlyDays            int `mapstructure:"hourly_days"`
	DailyDays             int `mapstructure:"daily_days"`
}

// UsageCleanupConfig 使用记录清理任务配置
type UsageCleanupConfig struct {
	// Enabled: 是否启用清理任务执行器
	Enabled bool `mapstructure:"enabled"`
	// MaxRangeDays: 单次任务允许的最大时间跨度（天）
	MaxRangeDays int `mapstructure:"max_range_days"`
	// BatchSize: 单批删除数量
	BatchSize int `mapstructure:"batch_size"`
	// WorkerIntervalSeconds: 后台任务轮询间隔（秒）
	WorkerIntervalSeconds int `mapstructure:"worker_interval_seconds"`
	// TaskTimeoutSeconds: 单次任务最大执行时长（秒）
	TaskTimeoutSeconds int `mapstructure:"task_timeout_seconds"`
}
