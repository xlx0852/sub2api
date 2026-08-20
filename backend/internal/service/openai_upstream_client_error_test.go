//go:build unit

package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsOpenAIDeterministicClientError(t *testing.T) {
	t.Parallel()

	require.True(t, isOpenAIDeterministicClientError(http.StatusBadRequest, nil))
	require.True(t, isOpenAIDeterministicClientError(http.StatusUnprocessableEntity, nil))
	require.True(t, isOpenAIDeterministicClientError(
		http.StatusNotFound,
		[]byte(`{"error":{"code":"model_not_found","message":"model not found"}}`),
	))
	require.False(t, isOpenAIDeterministicClientError(
		http.StatusNotFound,
		[]byte(`{"error":{"message":"no route to host"}}`),
	))
	require.False(t, isOpenAIDeterministicClientError(http.StatusUnauthorized, nil))
	require.False(t, isOpenAIDeterministicClientError(http.StatusForbidden, nil))
}

func TestRedactAndTruncateUpstreamDetailStripsSecrets(t *testing.T) {
	t.Parallel()

	got := redactAndTruncateUpstreamDetail(`{"error":{"message":"denied","authorization":"Bearer sk-secret","access_token":"tok_abc"}}`, 2048)
	require.NotContains(t, got, "sk-secret")
	require.NotContains(t, got, "tok_abc")
	require.Contains(t, got, "***")
}
