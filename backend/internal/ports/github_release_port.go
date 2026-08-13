// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// GitHubReleaseClient 获取 GitHub release 信息的接口（版本同步 / 更新检查共用）。
type GitHubReleaseClient interface {
	FetchLatestRelease(ctx context.Context, repo string) (*domain.GitHubRelease, error)
	FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*domain.GitHubRelease, error)
}
