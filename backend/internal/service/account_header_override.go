package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

func NormalizeHeaderOverrideCredentials(credentials map[string]any) error {
	return domain.NormalizeHeaderOverrideCredentials(credentials)
}
