package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

const (
	credKeyHeaderOverrideEnabled = "header_override_enabled"
	credKeyHeaderOverrides       = "header_overrides"
)

func NormalizeHeaderOverrideCredentials(credentials map[string]any) error {
	return domain.NormalizeHeaderOverrideCredentials(credentials)
}
