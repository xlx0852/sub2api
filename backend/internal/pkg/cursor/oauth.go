package cursor

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultWebsiteURL = "https://cursor.com"
	DefaultClientID   = "KbZUR41cY7W6zRSdpSUJ7I7mLYBKOCmB"
)

// JWTPayload represents the decoded Cursor JWT claims.
type JWTPayload struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
}

// TokenResponse represents tokens returned by Cursor authentication endpoints.
type TokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ShouldLogout bool   `json:"shouldLogout,omitempty"`
}

// UnmarshalJSON handles both camelCase (auth/poll) and snake_case (oauth/token) JSON responses.
func (tr *TokenResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		AccessTokenCamel  string `json:"accessToken"`
		RefreshTokenCamel string `json:"refreshToken"`
		AccessTokenSnake  string `json:"access_token"`
		RefreshTokenSnake string `json:"refresh_token"`
		ShouldLogout      bool   `json:"shouldLogout"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.AccessTokenCamel != "" {
		tr.AccessToken = raw.AccessTokenCamel
	} else {
		tr.AccessToken = raw.AccessTokenSnake
	}
	if raw.RefreshTokenCamel != "" {
		tr.RefreshToken = raw.RefreshTokenCamel
	} else {
		tr.RefreshToken = raw.RefreshTokenSnake
	}
	tr.ShouldLogout = raw.ShouldLogout
	return nil
}

// GeneratePKCE creates a base64url-encoded code_verifier and code_challenge (S256).
func GeneratePKCE() (verifier string, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("read random bytes: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// BuildLoginURL constructs the Cursor browser authentication URL.
func BuildLoginURL(websiteURL, challenge, sessionUUID, redirectTarget string) string {
	if websiteURL == "" {
		websiteURL = DefaultWebsiteURL
	}
	if redirectTarget == "" {
		redirectTarget = "sand"
	}
	websiteURL = strings.TrimRight(websiteURL, "/")
	v := url.Values{}
	v.Set("challenge", challenge)
	v.Set("uuid", sessionUUID)
	v.Set("mode", "login")
	v.Set("redirectTarget", redirectTarget)
	v.Set("supportsSelectedTeamLogin", "true")
	return fmt.Sprintf("%s/loginDeepControl?%s", websiteURL, v.Encode())
}

// ParseJWTPayload extracts sub, email, and exp from a JWT token without verifying the signature.
func ParseJWTPayload(tokenStr string) (*JWTPayload, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid jwt: expected at least 2 parts")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payloadBytes, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("decode jwt payload: %w", err)
		}
	}
	var payload JWTPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal jwt payload: %w", err)
	}
	return &payload, nil
}

// PollAuthStatus checks if the user has authenticated in the browser.
// Returns (token, ready, err). When ready is false and err is nil, authentication is still pending (404).
func PollAuthStatus(ctx context.Context, httpClient *http.Client, backendURL, sessionUUID, verifier string) (*TokenResponse, bool, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if backendURL == "" {
		backendURL = DefaultBackendURL
	}
	endpoint := fmt.Sprintf("%s/auth/poll?uuid=%s&verifier=%s", strings.TrimRight(backendURL, "/"), url.QueryEscape(sessionUUID), url.QueryEscape(verifier))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false, fmt.Errorf("create poll request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("poll auth status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, false, fmt.Errorf("read poll response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("poll returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, false, fmt.Errorf("unmarshal poll response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, false, fmt.Errorf("poll response missing accessToken")
	}
	if tr.RefreshToken == "" {
		tr.RefreshToken = tr.AccessToken
	}
	return &tr, true, nil
}

// ExchangeUserAPIKey exchanges a Cursor user API key for session access and refresh tokens.
func ExchangeUserAPIKey(ctx context.Context, httpClient *http.Client, backendURL, apiKey string) (*TokenResponse, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if backendURL == "" {
		backendURL = DefaultBackendURL
	}
	endpoint := fmt.Sprintf("%s/auth/exchange_user_api_key", strings.TrimRight(backendURL, "/"))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("create exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange api key: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("read exchange response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("unmarshal exchange response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("exchange response missing accessToken")
	}
	if tr.RefreshToken == "" {
		tr.RefreshToken = tr.AccessToken
	}
	return &tr, nil
}

// RefreshToken exchanges an existing refresh token for new access and refresh tokens.
func RefreshToken(ctx context.Context, httpClient *http.Client, backendURL, refreshToken string) (*TokenResponse, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if backendURL == "" {
		backendURL = DefaultBackendURL
	}
	endpoint := fmt.Sprintf("%s/oauth/token", strings.TrimRight(backendURL, "/"))

	payload := map[string]string{
		"client_id":     DefaultClientID,
		"grant_type":    "refresh_token",
		"refresh_token": strings.TrimSpace(refreshToken),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal refresh payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute token refresh: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("unmarshal refresh response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("refresh response missing access_token")
	}
	if tr.RefreshToken == "" {
		tr.RefreshToken = tr.AccessToken
	}
	return &tr, nil
}
