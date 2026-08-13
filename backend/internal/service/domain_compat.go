package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

const (
	OpenAIWSIngressModeOff         = domain.OpenAIWSIngressModeOff
	OpenAIWSIngressModeShared      = domain.OpenAIWSIngressModeShared
	OpenAIWSIngressModeDedicated   = domain.OpenAIWSIngressModeDedicated
	OpenAIWSIngressModeCtxPool     = domain.OpenAIWSIngressModeCtxPool
	OpenAIWSIngressModePassthrough = domain.OpenAIWSIngressModePassthrough
	OpenAIWSIngressModeHTTPBridge  = domain.OpenAIWSIngressModeHTTPBridge

	OpenAICompactModeAuto     = domain.OpenAICompactModeAuto
	OpenAICompactModeForceOn  = domain.OpenAICompactModeForceOn
	OpenAICompactModeForceOff = domain.OpenAICompactModeForceOff

	openAIAuthModeCredentialKey        = domain.OpenAIAuthModeCredentialKey
	openAIAuthModeLegacyCredentialKey  = domain.OpenAIAuthModeLegacyCredentialKey
	openAILongContextBillingEnabledKey = domain.OpenAILongContextBillingEnabledKey
)

func isOpenAIPersonalAccessTokenAuthMode(value string) bool {
	return domain.IsOpenAIPersonalAccessTokenAuthMode(value)
}

func matchModelPattern(pattern, model string) bool {
	return domain.MatchModelPattern(pattern, model)
}

func parseMinutes(hhmm string) (int, bool) {
	return domain.ParseMinutes(hhmm)
}

func normalizeOpenAIWSIngressMode(mode string) string {
	return domain.NormalizeOpenAIWSIngressMode(mode)
}

func normalizeOpenAIWSIngressDefaultMode(mode string) string {
	return domain.NormalizeOpenAIWSIngressDefaultMode(mode)
}
