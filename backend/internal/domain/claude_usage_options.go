package domain

// ClaudeUsageFetchOptions 包含获取 Claude 用量数据所需的所有选项。
type ClaudeUsageFetchOptions struct {
	AccessToken string                        // OAuth access token
	ProxyURL    string                        // 代理 URL（可选）
	AccountID   int64                         // 账号 ID（用于连接池隔离）
	TLSProfile  *TLSFingerprintRuntimeProfile // TLS 指纹 Profile（nil 表示不启用）
	Fingerprint *Fingerprint                  // 缓存的指纹信息（User-Agent 等）
}
