package account

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultKeyName = "sub2api-tray"
	httpTimeout    = 12 * time.Second
)

// SiteURL normalizes a user-entered site root or gateway base into the site root
// used for control-plane APIs (…/api/v1/…).
//
// Accepts:
//   - https://code.example
//   - https://code.example/
//   - https://code.example/v1
//   - https://code.example/v1/
func SiteURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("site url is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid site url")
	}
	u.RawQuery = ""
	u.Fragment = ""
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	path = strings.TrimSuffix(path, "/v1")
	path = strings.TrimSuffix(path, "/")
	u.Path = path
	u.RawPath = ""
	return strings.TrimRight(u.String(), "/"), nil
}

// GatewayBaseURL is the OpenAI-compatible base Codex should use (…/v1).
func GatewayBaseURL(site string) (string, error) {
	root, err := SiteURL(site)
	if err != nil {
		return "", err
	}
	return root + "/v1", nil
}

type Client struct {
	HTTP *http.Client
}

func (c *Client) http() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: httpTimeout}
}

type LoginInput struct {
	SiteURL  string
	Email    string
	Password string
	// TotpCode is optional; if login returns requires_2fa, call Login2FA.
	TotpCode string
	// TurnstileToken optional; only needed when site enables Turnstile.
	TurnstileToken string
	// KeyName used when creating a dedicated tray API key.
	KeyName string
	// PreferExisting reuses an existing key named KeyName when list returns full key.
	PreferExisting bool
}

type LoginResult struct {
	SiteURL      string `json:"site_url"`
	GatewayBase  string `json:"gateway_base"`
	Email        string `json:"email"`
	UserID       int64  `json:"user_id,omitempty"`
	APIKey       string `json:"api_key"`
	APIKeyID     int64  `json:"api_key_id,omitempty"`
	APIKeyName   string `json:"api_key_name,omitempty"`
	AccessToken  string `json:"-"`
	RefreshToken string `json:"-"`
	CreatedKey   bool   `json:"created_key"`
	ReusedKey    bool   `json:"reused_key"`
}

type Pending2FA struct {
	SiteURL         string `json:"site_url"`
	TempToken       string `json:"temp_token"`
	UserEmailMasked string `json:"user_email_masked,omitempty"`
}

type LoginOutcome struct {
	Result    *LoginResult
	Pending2FA *Pending2FA
}

// LoginWithPassword authenticates and ensures a usable gateway API key.
func (c *Client) LoginWithPassword(ctx context.Context, in LoginInput) (*LoginOutcome, error) {
	site, err := SiteURL(in.SiteURL)
	if err != nil {
		return nil, err
	}
	gateway, err := GatewayBaseURL(site)
	if err != nil {
		return nil, err
	}
	email := strings.TrimSpace(in.Email)
	if email == "" || strings.TrimSpace(in.Password) == "" {
		return nil, fmt.Errorf("email and password are required")
	}
	keyName := strings.TrimSpace(in.KeyName)
	if keyName == "" {
		keyName = DefaultKeyName
	}

	auth, pending, err := c.login(ctx, site, email, in.Password, in.TurnstileToken)
	if err != nil {
		return nil, err
	}
	if pending != nil {
		pending.SiteURL = site
		if strings.TrimSpace(in.TotpCode) == "" {
			return &LoginOutcome{Pending2FA: pending}, nil
		}
		auth, err = c.login2FA(ctx, site, pending.TempToken, in.TotpCode)
		if err != nil {
			return nil, err
		}
	}
	if auth == nil || strings.TrimSpace(auth.AccessToken) == "" {
		return nil, fmt.Errorf("login returned empty access token")
	}

	res, err := c.ensureAPIKey(ctx, site, gateway, email, auth, keyName, in.PreferExisting)
	if err != nil {
		return nil, err
	}
	return &LoginOutcome{Result: res}, nil
}

// Complete2FA finishes a pending 2FA challenge then ensures an API key.
func (c *Client) Complete2FA(ctx context.Context, site, tempToken, totpCode, email, keyName string, preferExisting bool) (*LoginResult, error) {
	site, err := SiteURL(site)
	if err != nil {
		return nil, err
	}
	gateway, err := GatewayBaseURL(site)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(keyName) == "" {
		keyName = DefaultKeyName
	}
	auth, err := c.login2FA(ctx, site, tempToken, totpCode)
	if err != nil {
		return nil, err
	}
	return c.ensureAPIKey(ctx, site, gateway, email, auth, keyName, preferExisting)
}

type authTokens struct {
	AccessToken  string
	RefreshToken string
	UserID       int64
	Email        string
}

func (c *Client) login(ctx context.Context, site, email, password, turnstile string) (*authTokens, *Pending2FA, error) {
	body := map[string]any{
		"email":    email,
		"password": password,
	}
	if strings.TrimSpace(turnstile) != "" {
		body["turnstile_token"] = turnstile
	}
	raw, err := c.doJSON(ctx, http.MethodPost, site+"/api/v1/auth/login", "", body)
	if err != nil {
		return nil, nil, err
	}
	if b, _ := raw["requires_2fa"].(bool); b {
		temp, _ := raw["temp_token"].(string)
		masked, _ := raw["user_email_masked"].(string)
		if strings.TrimSpace(temp) == "" {
			return nil, nil, fmt.Errorf("2fa required but temp_token missing")
		}
		return nil, &Pending2FA{TempToken: temp, UserEmailMasked: masked}, nil
	}
	tok, err := parseAuth(raw)
	return tok, nil, err
}

func (c *Client) login2FA(ctx context.Context, site, tempToken, code string) (*authTokens, error) {
	raw, err := c.doJSON(ctx, http.MethodPost, site+"/api/v1/auth/login/2fa", "", map[string]any{
		"temp_token": tempToken,
		"totp_code":  code,
	})
	if err != nil {
		return nil, err
	}
	return parseAuth(raw)
}

func parseAuth(raw map[string]any) (*authTokens, error) {
	access, _ := raw["access_token"].(string)
	if access == "" {
		return nil, fmt.Errorf("missing access_token in login response")
	}
	refresh, _ := raw["refresh_token"].(string)
	out := &authTokens{AccessToken: access, RefreshToken: refresh}
	if user, ok := raw["user"].(map[string]any); ok {
		out.UserID = asInt64(user["id"])
		if e, _ := user["email"].(string); e != "" {
			out.Email = e
		}
	}
	return out, nil
}

func (c *Client) ensureAPIKey(ctx context.Context, site, gateway, email string, auth *authTokens, keyName string, preferExisting bool) (*LoginResult, error) {
	res := &LoginResult{
		SiteURL:      site,
		GatewayBase:  gateway,
		Email:        firstNonEmpty(auth.Email, email),
		UserID:       auth.UserID,
		AccessToken:  auth.AccessToken,
		RefreshToken: auth.RefreshToken,
		APIKeyName:   keyName,
	}

	if preferExisting {
		if key, id, ok, err := c.findKeyByName(ctx, site, auth.AccessToken, keyName); err != nil {
			return nil, err
		} else if ok && key != "" {
			res.APIKey = key
			res.APIKeyID = id
			res.ReusedKey = true
			return res, nil
		}
	}

	key, id, err := c.createKey(ctx, site, auth.AccessToken, keyName)
	if err != nil {
		return nil, err
	}
	res.APIKey = key
	res.APIKeyID = id
	res.CreatedKey = true
	return res, nil
}

func (c *Client) findKeyByName(ctx context.Context, site, accessToken, name string) (key string, id int64, ok bool, err error) {
	raw, err := c.doJSON(ctx, http.MethodGet, site+"/api/v1/keys?page=1&page_size=100&search="+url.QueryEscape(name), accessToken, nil)
	if err != nil {
		return "", 0, false, err
	}
	items := extractItems(raw)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		n, _ := m["name"].(string)
		if !strings.EqualFold(strings.TrimSpace(n), name) {
			continue
		}
		k, _ := m["key"].(string)
		if strings.TrimSpace(k) == "" {
			continue
		}
		return k, asInt64(m["id"]), true, nil
	}
	return "", 0, false, nil
}

func extractItems(raw map[string]any) []any {
	if raw == nil {
		return nil
	}
	if items := asList(raw["items"]); items != nil {
		return items
	}
	if items := asList(raw["data"]); items != nil {
		return items
	}
	// nested paginated: data: { items: [...] }
	if nested, ok := raw["data"].(map[string]any); ok {
		if items := asList(nested["items"]); items != nil {
			return items
		}
	}
	return nil
}

func (c *Client) createKey(ctx context.Context, site, accessToken, name string) (string, int64, error) {
	raw, err := c.doJSON(ctx, http.MethodPost, site+"/api/v1/keys", accessToken, map[string]any{
		"name": name,
	})
	if err != nil {
		return "", 0, err
	}
	// data may be the key object directly
	obj := raw
	if nested, ok := raw["data"].(map[string]any); ok {
		obj = nested
	}
	key, _ := obj["key"].(string)
	if strings.TrimSpace(key) == "" {
		return "", 0, fmt.Errorf("create key response missing key material")
	}
	return key, asInt64(obj["id"]), nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint, bearer string, body any) (map[string]any, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))

	var top struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		if res.StatusCode >= 300 {
			return nil, fmt.Errorf("%s: %s", res.Status, strings.TrimSpace(string(raw)))
		}
		return nil, fmt.Errorf("invalid json response")
	}
	if res.StatusCode >= 300 || (top.Code != 0 && len(top.Data) == 0) {
		msg := strings.TrimSpace(top.Message)
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return nil, fmt.Errorf("%s: %s", res.Status, msg)
	}
	if len(top.Data) == 0 {
		// bare object without envelope
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, err
		}
		return obj, nil
	}
	// data may be object, array, or paginated object
	var obj map[string]any
	if err := json.Unmarshal(top.Data, &obj); err == nil {
		return obj, nil
	}
	var arr []any
	if err := json.Unmarshal(top.Data, &arr); err == nil {
		return map[string]any{"data": arr}, nil
	}
	return nil, fmt.Errorf("unsupported response data shape")
}

func asInt64(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	default:
		return 0
	}
}

func asList(v any) []any {
	if v == nil {
		return nil
	}
	if a, ok := v.([]any); ok {
		return a
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
