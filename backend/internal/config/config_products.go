package config

type LogConfig struct {
	Level           string            `mapstructure:"level"`
	Format          string            `mapstructure:"format"`
	ServiceName     string            `mapstructure:"service_name"`
	Environment     string            `mapstructure:"env"`
	Caller          bool              `mapstructure:"caller"`
	StacktraceLevel string            `mapstructure:"stacktrace_level"`
	Output          LogOutputConfig   `mapstructure:"output"`
	Rotation        LogRotationConfig `mapstructure:"rotation"`
	Sampling        LogSamplingConfig `mapstructure:"sampling"`
}

type LogOutputConfig struct {
	ToStdout bool   `mapstructure:"to_stdout"`
	ToFile   bool   `mapstructure:"to_file"`
	FilePath string `mapstructure:"file_path"`
}

type LogRotationConfig struct {
	MaxSizeMB  int  `mapstructure:"max_size_mb"`
	MaxBackups int  `mapstructure:"max_backups"`
	MaxAgeDays int  `mapstructure:"max_age_days"`
	Compress   bool `mapstructure:"compress"`
	LocalTime  bool `mapstructure:"local_time"`
}

type LogSamplingConfig struct {
	Enabled    bool `mapstructure:"enabled"`
	Initial    int  `mapstructure:"initial"`
	Thereafter int  `mapstructure:"thereafter"`
}

type GeminiConfig struct {
	OAuth GeminiOAuthConfig `mapstructure:"oauth"`
	Quota GeminiQuotaConfig `mapstructure:"quota"`
}

type GeminiOAuthConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	Scopes       string `mapstructure:"scopes"`
}

type GeminiQuotaConfig struct {
	Tiers  map[string]GeminiTierQuotaConfig `mapstructure:"tiers"`
	Policy string                           `mapstructure:"policy"`
}

type GeminiTierQuotaConfig struct {
	ProRPD          *int64 `mapstructure:"pro_rpd" json:"pro_rpd"`
	FlashRPD        *int64 `mapstructure:"flash_rpd" json:"flash_rpd"`
	CooldownMinutes *int   `mapstructure:"cooldown_minutes" json:"cooldown_minutes"`
}

type UpdateConfig struct {
	// ProxyURL 用于访问 GitHub 的代理地址
	// 支持 http/https/socks5/socks5h 协议
	// 例如: "http://127.0.0.1:7890", "socks5://127.0.0.1:1080"
	ProxyURL string `mapstructure:"proxy_url"`
}

type IdempotencyConfig struct {
	// ObserveOnly 为 true 时处于观察期：未携带 Idempotency-Key 的请求继续放行。
	ObserveOnly bool `mapstructure:"observe_only"`
	// DefaultTTLSeconds 关键写接口的幂等记录默认 TTL（秒）。
	DefaultTTLSeconds int `mapstructure:"default_ttl_seconds"`
	// SystemOperationTTLSeconds 系统操作接口的幂等记录 TTL（秒）。
	SystemOperationTTLSeconds int `mapstructure:"system_operation_ttl_seconds"`
	// ProcessingTimeoutSeconds processing 状态锁超时（秒）。
	ProcessingTimeoutSeconds int `mapstructure:"processing_timeout_seconds"`
	// FailedRetryBackoffSeconds 失败退避窗口（秒）。
	FailedRetryBackoffSeconds int `mapstructure:"failed_retry_backoff_seconds"`
	// MaxStoredResponseLen 持久化响应体最大长度（字节）。
	MaxStoredResponseLen int `mapstructure:"max_stored_response_len"`
	// CleanupIntervalSeconds 过期记录清理周期（秒）。
	CleanupIntervalSeconds int `mapstructure:"cleanup_interval_seconds"`
	// CleanupBatchSize 每次清理的最大记录数。
	CleanupBatchSize int `mapstructure:"cleanup_batch_size"`
}

type BatchImageConfig struct {
	Enabled                           bool   `mapstructure:"enabled"`
	MaxItemsPerJobDefault             int    `mapstructure:"max_items_per_job_default"`
	MaxItemsPerJobTrial               int    `mapstructure:"max_items_per_job_trial"`
	MaxOutputImagesPerJob             int    `mapstructure:"max_output_images_per_job"`
	MaxOutputImagesPerItem            int    `mapstructure:"max_output_images_per_item"`
	MaxPromptCharsPerItem             int    `mapstructure:"max_prompt_chars_per_item"`
	MaxReferenceImagesPerJob          int    `mapstructure:"max_reference_images_per_job"`
	MaxReferenceInlineBytesPerJob     int    `mapstructure:"max_reference_inline_bytes_per_job"`
	DefaultResponseMimeType           string `mapstructure:"default_response_mime_type"`
	DefaultImageSize                  string `mapstructure:"default_image_size"`
	MaxDownloadItemsZip               int    `mapstructure:"max_download_items_zip"`
	MaxDownloadBytesPerRequest        int64  `mapstructure:"max_download_bytes_per_request"`
	MaxDownloadDurationSeconds        int    `mapstructure:"max_download_duration_seconds"`
	MaxDownloadConcurrencyPerUser     int    `mapstructure:"max_download_concurrency_per_user"`
	InputRetentionAfterTerminalHours  int    `mapstructure:"input_retention_after_terminal_hours"`
	OutputRetentionAfterTerminalHours int    `mapstructure:"output_retention_after_terminal_hours"`
	OutputRetentionMaxDays            int    `mapstructure:"output_retention_max_days"`
	CleanupIntervalMinutes            int    `mapstructure:"cleanup_interval_minutes"`
	CleanupBatchSize                  int    `mapstructure:"cleanup_batch_size"`
	QueueEnabled                      bool   `mapstructure:"queue_enabled"`
	QueueReadyKey                     string `mapstructure:"queue_ready_key"`
	QueueDelayedKey                   string `mapstructure:"queue_delayed_key"`
	QueueActiveKey                    string `mapstructure:"queue_active_key"`
	InflightKeyPrefix                 string `mapstructure:"inflight_key_prefix"`
	LockKeyPrefix                     string `mapstructure:"lock_key_prefix"`
	IdempotencyKeyPrefix              string `mapstructure:"idempotency_key_prefix"`
	InflightTTLSeconds                int    `mapstructure:"inflight_ttl_seconds"`
	JobLockTTLSeconds                 int    `mapstructure:"job_lock_ttl_seconds"`
	DefaultRequeueDelaySeconds        int    `mapstructure:"default_requeue_delay_seconds"`
	ErrorRetryDelaySeconds            int    `mapstructure:"error_retry_delay_seconds"`
	LockConflictDelaySeconds          int    `mapstructure:"lock_conflict_delay_seconds"`
	StaleActiveAfterSeconds           int    `mapstructure:"stale_active_after_seconds"`
	DelayedMoverIntervalSeconds       int    `mapstructure:"delayed_mover_interval_seconds"`
	RecoveryIntervalSeconds           int    `mapstructure:"recovery_interval_seconds"`
	DelayedMoveLimit                  int    `mapstructure:"delayed_move_limit"`
	RecoverLimit                      int    `mapstructure:"recover_limit"`
	VertexEnabled                     bool   `mapstructure:"vertex_enabled"`
	VertexProjectID                   string `mapstructure:"vertex_project_id"`
	VertexLocation                    string `mapstructure:"vertex_location"`
	// VertexManagedGCSBucket is a server-owned bucket for batch JSONL input/output.
	// Disable Cloud Storage soft delete on this bucket to avoid retaining deleted batch objects.
	VertexManagedGCSBucket       string `mapstructure:"vertex_managed_gcs_bucket"`
	VertexManagedGCSPrefix       string `mapstructure:"vertex_managed_gcs_prefix"`
	VertexInputRetentionHours    int    `mapstructure:"vertex_input_retention_hours"`
	VertexOutputRetentionHours   int    `mapstructure:"vertex_output_retention_hours"`
	VertexBatchPredictionBaseURL string `mapstructure:"vertex_batch_prediction_base_url"`
	VertexGCSBaseURL             string `mapstructure:"vertex_gcs_base_url"`
}

// TokenRefreshConfig OAuth token自动刷新配置
type TokenRefreshConfig struct {
	// 是否启用自动刷新
	Enabled bool `mapstructure:"enabled"`
	// 检查间隔（分钟）
	CheckIntervalMinutes int `mapstructure:"check_interval_minutes"`
	// 提前刷新时间（小时），在token过期前多久开始刷新
	RefreshBeforeExpiryHours float64 `mapstructure:"refresh_before_expiry_hours"`
	// 最大重试次数
	MaxRetries int `mapstructure:"max_retries"`
	// 重试退避基础时间（秒）
	RetryBackoffSeconds int `mapstructure:"retry_backoff_seconds"`
}

type PricingConfig struct {
	// 价格数据远程URL（默认使用LiteLLM镜像）
	RemoteURL string `mapstructure:"remote_url"`
	// 哈希校验文件URL
	HashURL string `mapstructure:"hash_url"`
	// 本地数据目录
	DataDir string `mapstructure:"data_dir"`
	// 回退文件路径
	FallbackFile string `mapstructure:"fallback_file"`
	// 更新间隔（小时）
	UpdateIntervalHours int `mapstructure:"update_interval_hours"`
	// 哈希校验间隔（分钟）
	HashCheckIntervalMinutes int `mapstructure:"hash_check_interval_minutes"`
}

// ModelCatalogConfig controls the optional remote first-party model catalog.
type ModelCatalogConfig struct {
	RemoteEnabled            bool   `mapstructure:"remote_enabled"`
	RemoteURL                string `mapstructure:"remote_url"`
	HashURL                  string `mapstructure:"hash_url"`
	DataDir                  string `mapstructure:"data_dir"`
	FallbackFile             string `mapstructure:"fallback_file"`
	UpdateIntervalHours      int    `mapstructure:"update_interval_hours"`
	HashCheckIntervalMinutes int    `mapstructure:"hash_check_interval_minutes"`
	RequestTimeoutSeconds    int    `mapstructure:"request_timeout_seconds"`
	MaxBodyBytes             int64  `mapstructure:"max_body_bytes"`
}
