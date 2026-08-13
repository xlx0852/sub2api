package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ClaudeUsageFetcher fetches usage data from Anthropic OAuth API.
type ClaudeUsageFetcher interface {
	FetchUsage(ctx context.Context, accessToken, proxyURL string) (*domain.ClaudeUsageResponse, error)
	FetchUsageWithOptions(ctx context.Context, opts *domain.ClaudeUsageFetchOptions) (*domain.ClaudeUsageResponse, error)
}
