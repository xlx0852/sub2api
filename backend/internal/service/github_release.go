package service

import "context"

// GitHubReleaseClient 获取 GitHub release 信息的接口（版本同步 / 更新检查共用）。
type GitHubReleaseClient interface {
	FetchLatestRelease(ctx context.Context, repo string) (*GitHubRelease, error)
	FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*GitHubRelease, error)
}

// GitHubRelease represents GitHub API response (subset used by version sync).
type GitHubRelease struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}
