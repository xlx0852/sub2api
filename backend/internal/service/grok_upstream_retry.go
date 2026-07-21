package service

import (
	"net/http"
	"strings"
)

const grokShouldRetryHeader = "X-Should-Retry"

func grokUpstreamExplicitlyDisablesRetry(headers http.Header) bool {
	if headers == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(headers.Get(grokShouldRetryHeader)), "false")
}

func shouldSuppressGrokUpstreamPenalty(account *Account, headers http.Header) bool {
	return account != nil &&
		account.Platform == PlatformGrok &&
		grokUpstreamExplicitlyDisablesRetry(headers)
}
