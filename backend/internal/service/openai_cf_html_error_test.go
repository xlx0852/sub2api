package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSummarizeNonJSONUpstreamErrorBody_CloudflarePages(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "524",
			body: `<html><head><title>Error</title></head><body>Error code 524 A Timeout Occurred</body></html>`,
			want: "Upstream Cloudflare timeout (524)",
		},
		{
			name: "504",
			body: `<!DOCTYPE html><html><body>Gateway Time-out error code 504</body></html>`,
			want: "Upstream Cloudflare gateway timeout (504)",
		},
		{
			name: "json ignored",
			body: `{"error":{"message":"boom"}}`,
			want: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, SummarizeNonJSONUpstreamErrorBody([]byte(tc.body)))
		})
	}
}

func TestExtractUpstreamErrorMessage_HTMLTimeout(t *testing.T) {
	t.Parallel()

	msg := ExtractUpstreamErrorMessage([]byte(`<html><body>Error code 524 A Timeout Occurred</body></html>`))
	require.Equal(t, "Upstream Cloudflare timeout (524)", msg)
}

func TestMapOpenAIUpstreamErrorStatus_Includes524(t *testing.T) {
	t.Parallel()

	status, errType, errMsg := MapOpenAIUpstreamErrorStatus(524)
	require.Equal(t, http.StatusBadGateway, status)
	require.Equal(t, "upstream_error", errType)
	require.Equal(t, "Upstream service temporarily unavailable", errMsg)

	status, errType, errMsg = MapOpenAIUpstreamErrorStatus(504)
	require.Equal(t, http.StatusBadGateway, status)
	require.Equal(t, "upstream_error", errType)
	require.Equal(t, "Upstream service temporarily unavailable", errMsg)
}

func TestShouldFailoverOpenAIPassthroughResponse_5xx(t *testing.T) {
	t.Parallel()

	// OAuth/nil: capacity-class only. JSON 5xx stay raw-proxied for Codex-like clients.
	require.True(t, shouldFailoverOpenAIPassthroughResponse(nil, http.StatusTooManyRequests, nil))
	require.True(t, shouldFailoverOpenAIPassthroughResponse(nil, 529, nil))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(nil, 500, nil))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(nil, 502, nil))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(nil, 503, nil))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(nil, 504, nil))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(nil, 524, nil))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(nil, 400, nil))

	apiKey := &Account{Type: AccountTypeAPIKey}
	require.True(t, shouldFailoverOpenAIPassthroughResponse(apiKey, http.StatusBadGateway, []byte(`{"error":{"message":"bad gateway"}}`)))
	require.True(t, shouldFailoverOpenAIPassthroughResponse(apiKey, 524, nil))
	require.True(t, shouldFailoverOpenAIPassthroughResponse(apiKey, http.StatusRequestEntityTooLarge, []byte(`{"error":{"message":"request body too large"}}`)))

	html524 := []byte(`<html><body>Error code 524 A Timeout Occurred</body></html>`)
	html504 := []byte(`<!DOCTYPE html><html><body>Gateway Time-out</body></html>`)
	json502 := []byte(`{"error":{"message":"bad gateway"}}`)
	require.True(t, shouldFailoverOpenAIPassthroughHTMLGatewayError(524, html524))
	require.True(t, shouldFailoverOpenAIPassthroughHTMLGatewayError(504, html504))
	require.True(t, shouldFailoverOpenAIPassthroughHTMLGatewayError(524, nil))
	require.False(t, shouldFailoverOpenAIPassthroughHTMLGatewayError(502, json502))
	require.False(t, shouldFailoverOpenAIPassthroughHTMLGatewayError(400, html524))
}

func TestSanitizeUpstreamErrorMessage_HTML(t *testing.T) {
	t.Parallel()

	raw := `<html><head><meta name="viewport" content="width=device-width"></head><body>Error code 504</body></html>`
	require.Equal(t, "Upstream Cloudflare gateway timeout (504)", sanitizeUpstreamErrorMessage(raw))
}
