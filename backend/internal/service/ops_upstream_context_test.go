package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeUpstreamURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"strips query", "https://api.anthropic.com/v1/messages?beta=true", "https://api.anthropic.com/v1/messages"},
		{"strips fragment", "https://api.openai.com/v1/responses#frag", "https://api.openai.com/v1/responses"},
		{"strips both", "https://host/path?token=secret#x", "https://host/path"},
		{"no query or fragment", "https://host/path", "https://host/path"},
		{"empty string", "", ""},
		{"whitespace only", "  ", ""},
		{"query before fragment", "https://h/p?a=1#f", "https://h/p"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, safeUpstreamURL(tt.input))
		})
	}
}

func TestSanitizeOpsUpstreamErrors_MaxEventsClamped(t *testing.T) {
	events := make([]*OpsUpstreamErrorEvent, 0, 15)
	for i := 0; i < 15; i++ {
		events = append(events, &OpsUpstreamErrorEvent{
			Platform:           "anthropic",
			UpstreamStatusCode: 500,
			Message:            "error",
		})
	}

	input := &OpsInsertErrorLogInput{
		UpstreamErrors: events,
	}

	err := sanitizeOpsUpstreamErrors(input)
	require.NoError(t, err)
	require.NotNil(t, input.UpstreamErrorsJSON)

	parsed, err := ParseOpsUpstreamErrors(*input.UpstreamErrorsJSON)
	require.NoError(t, err)
	require.Len(t, parsed, 8, "should clamp upstream errors to max 8 events")
}

func TestSanitizeErrorBodyForStorage_PayloadBounded(t *testing.T) {
	// Generate a 10KB string payload
	largePayload := make([]byte, 10*1024)
	for i := range largePayload {
		largePayload[i] = 'a'
	}

	sanitized, truncated := sanitizeErrorBodyForStorage(string(largePayload), opsMaxStoredErrorBodyBytes)
	require.True(t, truncated)
	require.LessOrEqual(t, len(sanitized), opsMaxStoredErrorBodyBytes)
}
