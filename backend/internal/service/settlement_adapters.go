package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/app/billing"
)

var (
	_ billing.Settlement = (*GatewayService)(nil)
	_ billing.Settlement = (*OpenAIGatewayService)(nil)
)

// Record 将统一 settlement 入口适配到现有 GatewayService.RecordUsage。
// 调用方仍应优先走带完整账号/API Key 快照的 RecordUsage；本方法供管道 Settle 阶段使用。
func (s *GatewayService) Record(ctx context.Context, in billing.RecordInput) error {
	return s.RecordUsage(ctx, &RecordUsageInput{
		Result: &ForwardResult{
			RequestID:     in.RequestID,
			Model:         in.Model,
			UpstreamModel: in.UpstreamModel,
			Usage: ClaudeUsage{
				InputTokens:              in.Usage.InputTokens,
				OutputTokens:             in.Usage.OutputTokens,
				CacheCreationInputTokens: in.Usage.CacheCreationTokens,
				CacheReadInputTokens:     in.Usage.CacheReadTokens,
				ImageOutputTokens:        in.Usage.ImageOutputTokens,
			},
		},
		ForceCacheBilling:  in.ForceCacheBilling,
		InboundEndpoint:    in.InboundEndpoint,
		UpstreamEndpoint:   in.UpstreamEndpoint,
		UserAgent:          in.UserAgent,
		IPAddress:          in.IPAddress,
		RequestPayloadHash: in.RequestPayloadHash,
		QuotaPlatform:      in.Platform,
		Account:            in.Account,
		APIKey:             in.APIKey,
		User:               in.User,
	})
}

// Record 将统一 settlement 入口适配到现有 OpenAIGatewayService.RecordUsage。
func (s *OpenAIGatewayService) Record(ctx context.Context, in billing.RecordInput) error {
	return s.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:     in.RequestID,
			Model:         in.Model,
			UpstreamModel: in.UpstreamModel,
			Usage: OpenAIUsage{
				InputTokens:              in.Usage.InputTokens,
				OutputTokens:             in.Usage.OutputTokens,
				CacheCreationInputTokens: in.Usage.CacheCreationTokens,
				CacheReadInputTokens:     in.Usage.CacheReadTokens,
				ImageInputTokens:         in.Usage.ImageInputTokens,
				ImageOutputTokens:        in.Usage.ImageOutputTokens,
			},
		},
		InboundEndpoint:    in.InboundEndpoint,
		UpstreamEndpoint:   in.UpstreamEndpoint,
		UserAgent:          in.UserAgent,
		IPAddress:          in.IPAddress,
		RequestPayloadHash: in.RequestPayloadHash,
		QuotaPlatform:      in.Platform,
		Account:            in.Account,
		APIKey:             in.APIKey,
		User:               in.User,
	})
}
