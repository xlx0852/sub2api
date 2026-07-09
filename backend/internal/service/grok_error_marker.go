package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

const (
	grokSpendingLimitUpstreamCode      = "personal-team-blocked:spending-limit"
	grokSpendingLimitMarker            = "grok_spending_limit"
	grokSpendingLimitTempUnschedReason = grokSpendingLimitMarker + ": credits exhausted or SuperGrok subscription required"
	grokSpendingLimitFallbackMessage   = grokSpendingLimitMarker + " (403): credits exhausted or SuperGrok subscription required"
)

func isGrokSpendingLimitError(responseBody []byte, upstreamMsg string) bool {
	if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(responseBody, "code").String()), grokSpendingLimitUpstreamCode) {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(responseBody, "error.code").String()), grokSpendingLimitUpstreamCode) {
		return true
	}

	text := strings.ToLower(strings.TrimSpace(upstreamMsg + " " + gjson.GetBytes(responseBody, "error").String() + " " + string(responseBody)))
	return strings.Contains(text, grokSpendingLimitUpstreamCode) ||
		strings.Contains(text, "run out of credits") ||
		strings.Contains(text, "need a grok subscription")
}

func buildGrokSpendingLimitErrorMessage(responseBody []byte, upstreamMsg string) string {
	msg := strings.TrimSpace(upstreamMsg)
	if msg == "" {
		msg = strings.TrimSpace(gjson.GetBytes(responseBody, "error").String())
	}
	if msg == "" {
		return grokSpendingLimitFallbackMessage
	}
	return grokSpendingLimitMarker + " (403): " + sanitizeUpstreamErrorMessage(msg)
}
