package billing

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Usage 是跨平台结算使用的统一用量模型（design.md D5）。
// 各 provider 的 extractor 负责把协议事件映射到这些字段。
type Usage struct {
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	ImageInputTokens    int
	ImageOutputTokens   int
}

// RecordInput 是单一 settlement 入口的输入。
// 具体扣费/落库仍由现有 Gateway/OpenAI RecordUsage 实现，语义不变。
type RecordInput struct {
	RequestID          string
	Model              string
	UpstreamModel      string
	Platform           string
	Usage              Usage
	ForceCacheBilling  bool
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string
	IPAddress          string
	RequestPayloadHash string
	Account            *domain.Account
	APIKey             *domain.APIKey
	User               *domain.User
}

// Settlement 是所有平台共用的用量结算入口。
type Settlement interface {
	Record(ctx context.Context, in RecordInput) error
}
