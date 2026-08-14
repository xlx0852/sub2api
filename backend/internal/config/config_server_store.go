package config

import "fmt"

type ServerConfig struct {
	Host               string    `mapstructure:"host"`
	Port               int       `mapstructure:"port"`
	Mode               string    `mapstructure:"mode"`                  // debug/release
	FrontendURL        string    `mapstructure:"frontend_url"`          // 前端基础 URL，用于生成邮件中的外部链接
	ReadHeaderTimeout  int       `mapstructure:"read_header_timeout"`   // 读取请求头超时（秒）
	IdleTimeout        int       `mapstructure:"idle_timeout"`          // 空闲连接超时（秒）
	TrustedProxies     []string  `mapstructure:"trusted_proxies"`       // 可信代理列表（CIDR/IP）
	MaxRequestBodySize int64     `mapstructure:"max_request_body_size"` // 全局最大请求体限制
	H2C                H2CConfig `mapstructure:"h2c"`                   // HTTP/2 Cleartext 配置
}

// H2CConfig HTTP/2 Cleartext 配置
type H2CConfig struct {
	Enabled                      bool   `mapstructure:"enabled"`                          // 是否启用 H2C
	MaxConcurrentStreams         uint32 `mapstructure:"max_concurrent_streams"`           // 最大并发流数量
	IdleTimeout                  int    `mapstructure:"idle_timeout"`                     // 空闲超时（秒）
	MaxReadFrameSize             int    `mapstructure:"max_read_frame_size"`              // 最大帧大小（字节）
	MaxUploadBufferPerConnection int    `mapstructure:"max_upload_buffer_per_connection"` // 每个连接的上传缓冲区（字节）
	MaxUploadBufferPerStream     int    `mapstructure:"max_upload_buffer_per_stream"`     // 每个流的上传缓冲区（字节）
}

type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
}

func (s *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// DatabaseConfig 数据库连接配置
// 性能优化：新增连接池参数，避免频繁创建/销毁连接
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
	// 连接池配置（性能优化：可配置化连接池参数）
	// MaxOpenConns: 最大打开连接数，控制数据库连接上限，防止资源耗尽
	MaxOpenConns int `mapstructure:"max_open_conns"`
	// MaxIdleConns: 最大空闲连接数，保持热连接减少建连延迟
	MaxIdleConns int `mapstructure:"max_idle_conns"`
	// ConnMaxLifetimeMinutes: 连接最大存活时间，防止长连接导致的资源泄漏
	ConnMaxLifetimeMinutes int `mapstructure:"conn_max_lifetime_minutes"`
	// ConnMaxIdleTimeMinutes: 空闲连接最大存活时间，及时释放不活跃连接
	ConnMaxIdleTimeMinutes int `mapstructure:"conn_max_idle_time_minutes"`
	// UserPlatformQuotaFlusherEnabled: 是否启用 user×platform 配额写聚合 flusher
	UserPlatformQuotaFlusherEnabled bool `mapstructure:"user_platform_quota_flusher_enabled"`
	// UserPlatformQuotaFlushIntervalMs: flusher 刷写间隔（毫秒）
	UserPlatformQuotaFlushIntervalMs int `mapstructure:"user_platform_quota_flush_interval_ms"`
	// UserPlatformQuotaFlushBatchSize: flusher 单批最大条数
	// 建议 ≤ 6000（单条 UPSERT 原子上限）
	UserPlatformQuotaFlushBatchSize int `mapstructure:"user_platform_quota_flush_batch_size"`
}

func (d *DatabaseConfig) DSN() string {
	// 当密码为空时不包含 password 参数，避免 libpq 解析错误
	if d.Password == "" {
		return fmt.Sprintf(
			"host=%s port=%d user=%s dbname=%s sslmode=%s",
			d.Host, d.Port, d.User, d.DBName, d.SSLMode,
		)
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

// DSNWithTimezone returns DSN with timezone setting
func (d *DatabaseConfig) DSNWithTimezone(tz string) string {
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	// 当密码为空时不包含 password 参数，避免 libpq 解析错误
	if d.Password == "" {
		return fmt.Sprintf(
			"host=%s port=%d user=%s dbname=%s sslmode=%s TimeZone=%s",
			d.Host, d.Port, d.User, d.DBName, d.SSLMode, tz,
		)
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode, tz,
	)
}

// RedisConfig Redis 连接配置
// 性能优化：新增连接池和超时参数，提升高并发场景下的吞吐量
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	// 连接池与超时配置（性能优化：可配置化连接池参数）
	// DialTimeoutSeconds: 建立连接超时，防止慢连接阻塞
	DialTimeoutSeconds int `mapstructure:"dial_timeout_seconds"`
	// ReadTimeoutSeconds: 读取超时，避免慢查询阻塞连接池
	ReadTimeoutSeconds int `mapstructure:"read_timeout_seconds"`
	// WriteTimeoutSeconds: 写入超时，避免慢写入阻塞连接池
	WriteTimeoutSeconds int `mapstructure:"write_timeout_seconds"`
	// PoolSize: 连接池大小，控制最大并发连接数
	PoolSize int `mapstructure:"pool_size"`
	// MinIdleConns: 最小空闲连接数，保持热连接减少冷启动延迟
	MinIdleConns int `mapstructure:"min_idle_conns"`
	// EnableTLS: 是否启用 TLS/SSL 连接
	EnableTLS bool `mapstructure:"enable_tls"`
}

func (r *RedisConfig) Address() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}
