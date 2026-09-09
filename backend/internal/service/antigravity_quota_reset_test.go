package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseAntigravitySmartRetryInfoQuotaExhaustedUsesAbsoluteReset(t *testing.T) {
	resetAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	body := fmt.Sprintf(`{
		"error": {
			"code": 429,
			"status": "RESOURCE_EXHAUSTED",
			"details": [
				{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"QUOTA_EXHAUSTED","metadata":{"model":"gemini-3-flash","quotaResetDelay":"2h0m0s","quotaResetTimeStamp":%q}},
				{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"1m"}
			]
		}
	}`, resetAt.Format(time.RFC3339))

	result := parseAntigravitySmartRetryInfo([]byte(body))
	require.NotNil(t, result)
	require.Equal(t, "gemini-3-flash", result.ModelName)
	require.False(t, result.IsModelCapacityExhausted)
	require.InDelta(t, (2 * time.Hour).Seconds(), result.RetryDelay.Seconds(), 2)
}
