package service

import "github.com/Wei-Shaw/sub2api/internal/config"

// authSettingsSection 是 settings KV 的第一个类型化分区。
// 关键约束：保留旧 key/value 存储与默认值语义；新增设置应进入对应分区，
// 不要继续把所有解析逻辑堆回 SystemSettings.parseSettings。
type authSettingsSection struct {
	RegistrationEnabled              bool
	EmailVerifyEnabled               bool
	RegistrationEmailSuffixWhitelist []string
	PasswordResetEnabled             bool
	InvitationCodeEnabled            bool
	TotpEnabled                      bool
	LoginAgreementEnabled            bool
	LoginAgreementMode               string
	LoginAgreementUpdatedAt          string
	LoginAgreementDocuments          []LoginAgreementDocument
	APIKeyACLTrustForwardedIP        bool
}

func parseAuthSettingsSection(settings map[string]string, cfg *config.Config) authSettingsSection {
	emailVerifyEnabled := settings[SettingKeyEmailVerifyEnabled] == "true"
	apiKeyACLTrustForwardedIP := false
	if value, ok := settings[SettingKeyAPIKeyACLTrustForwardedIP]; ok {
		apiKeyACLTrustForwardedIP = value == "true"
	} else if cfg != nil {
		apiKeyACLTrustForwardedIP = cfg.Security.TrustForwardedIPForAPIKeyACL
	}

	updatedAt := settings[SettingKeyLoginAgreementUpdatedAt]
	if updatedAt == "" {
		updatedAt = defaultLoginAgreementDate
	}
	return authSettingsSection{
		RegistrationEnabled:              settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:               emailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: ParseRegistrationEmailSuffixWhitelist(settings[SettingKeyRegistrationEmailSuffixWhitelist]),
		PasswordResetEnabled:             emailVerifyEnabled && settings[SettingKeyPasswordResetEnabled] == "true",
		InvitationCodeEnabled:            settings[SettingKeyInvitationCodeEnabled] == "true",
		TotpEnabled:                      settings[SettingKeyTotpEnabled] == "true",
		LoginAgreementEnabled:            settings[SettingKeyLoginAgreementEnabled] == "true",
		LoginAgreementMode:               normalizeLoginAgreementMode(settings[SettingKeyLoginAgreementMode]),
		LoginAgreementUpdatedAt:          updatedAt,
		LoginAgreementDocuments:          parseLoginAgreementDocuments(settings[SettingKeyLoginAgreementDocuments]),
		APIKeyACLTrustForwardedIP:        apiKeyACLTrustForwardedIP,
	}
}
