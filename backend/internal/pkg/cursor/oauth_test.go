package cursor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePKCE(t *testing.T) {
	v1, c1, err := GeneratePKCE()
	require.NoError(t, err)
	assert.NotEmpty(t, v1)
	assert.NotEmpty(t, c1)
	assert.NotEqual(t, v1, c1)

	v2, c2, err := GeneratePKCE()
	require.NoError(t, err)
	assert.NotEqual(t, v1, v2)
	assert.NotEqual(t, c1, c2)
}

func TestBuildLoginURL(t *testing.T) {
	url := BuildLoginURL("", "test-challenge", "test-uuid", "sand")
	assert.Contains(t, url, "https://cursor.com/loginDeepControl?")
	assert.Contains(t, url, "challenge=test-challenge")
	assert.Contains(t, url, "uuid=test-uuid")
	assert.Contains(t, url, "redirectTarget=sand")
	assert.Contains(t, url, "supportsSelectedTeamLogin=true")
}

func TestParseJWTPayload(t *testing.T) {
	payload := map[string]any{
		"sub":   "grok|user_12345",
		"email": "user@example.com",
		"exp":   1794071269,
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)
	fakeJWT := "header." + base64.RawURLEncoding.EncodeToString(payloadBytes) + ".sig"

	parsed, err := ParseJWTPayload(fakeJWT)
	require.NoError(t, err)
	assert.Equal(t, "grok|user_12345", parsed.Sub)
	assert.Equal(t, "user@example.com", parsed.Email)
	assert.Equal(t, int64(1794071269), parsed.Exp)
}

func TestPollAuthStatus_Mock(t *testing.T) {
	var callCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/auth/poll", r.URL.Path)
		assert.Equal(t, "uuid-1", r.URL.Query().Get("uuid"))
		assert.Equal(t, "ver-1", r.URL.Query().Get("verifier"))
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"accessToken":  "tok-access",
			"refreshToken": "tok-refresh",
		})
	}))
	defer ts.Close()

	// First call: 404 pending
	resp, ready, err := PollAuthStatus(context.Background(), ts.Client(), ts.URL, "uuid-1", "ver-1")
	require.NoError(t, err)
	assert.False(t, ready)
	assert.Nil(t, resp)

	// Second call: 200 ready
	resp, ready, err = PollAuthStatus(context.Background(), ts.Client(), ts.URL, "uuid-1", "ver-1")
	require.NoError(t, err)
	assert.True(t, ready)
	require.NotNil(t, resp)
	assert.Equal(t, "tok-access", resp.AccessToken)
	assert.Equal(t, "tok-refresh", resp.RefreshToken)
}

func TestExchangeUserAPIKey_Mock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/auth/exchange_user_api_key", r.URL.Path)
		assert.Equal(t, "Bearer cur_key_123", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"accessToken":  "tok-access-from-key",
			"refreshToken": "tok-refresh-from-key",
		})
	}))
	defer ts.Close()

	resp, err := ExchangeUserAPIKey(context.Background(), ts.Client(), ts.URL, "cur_key_123")
	require.NoError(t, err)
	assert.Equal(t, "tok-access-from-key", resp.AccessToken)
	assert.Equal(t, "tok-refresh-from-key", resp.RefreshToken)
}

func TestRefreshToken_Mock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/oauth/token", r.URL.Path)
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, DefaultClientID, body["client_id"])
		assert.Equal(t, "refresh_token", body["grant_type"])
		assert.Equal(t, "old-refresh", body["refresh_token"])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"shouldLogout":  false,
		})
	}))
	defer ts.Close()

	resp, err := RefreshToken(context.Background(), ts.Client(), ts.URL, "old-refresh")
	require.NoError(t, err)
	assert.Equal(t, "new-access", resp.AccessToken)
	assert.Equal(t, "new-refresh", resp.RefreshToken)
	assert.False(t, resp.ShouldLogout)
}
