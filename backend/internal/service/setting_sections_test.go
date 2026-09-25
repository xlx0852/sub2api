package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestParseAuthSettingsSectionPreservesLegacyDefaults(t *testing.T) {
	cfg := &config.Config{Security: config.SecurityConfig{TrustForwardedIPForAPIKeyACL: true}}
	got := parseAuthSettingsSection(map[string]string{
		SettingKeyRegistrationEnabled:     "true",
		SettingKeyEmailVerifyEnabled:      "true",
		SettingKeyPasswordResetEnabled:    "true",
		SettingKeyLoginAgreementEnabled:   "true",
		SettingKeyLoginAgreementDocuments: "[]",
	}, cfg)

	require.True(t, got.RegistrationEnabled)
	require.True(t, got.EmailVerifyEnabled)
	require.True(t, got.PasswordResetEnabled)
	require.True(t, got.LoginAgreementEnabled)
	require.True(t, got.APIKeyACLTrustForwardedIP)
	require.NotEmpty(t, got.LoginAgreementUpdatedAt)
	require.NotEmpty(t, got.LoginAgreementDocuments)
}

func TestParseAuthSettingsSectionUnknownKeysAreIgnored(t *testing.T) {
	got := parseAuthSettingsSection(map[string]string{"unknown_setting": "true"}, nil)
	require.False(t, got.RegistrationEnabled)
	require.False(t, got.EmailVerifyEnabled)
	require.False(t, got.APIKeyACLTrustForwardedIP)
}
