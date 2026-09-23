package dto

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAccountFromServiceShallow_RedactsSensitiveCredentials(t *testing.T) {
	src := &service.Account{
		ID:       42,
		Name:     "demo",
		Platform: "anthropic",
		Type:     "oauth",
		Credentials: map[string]any{
			"access_token":  "at-secret",
			"refresh_token": "rt-secret",
			"id_token":      "id-secret",
			"api_key":       "sk-secret",
			"base_url":      "https://api.example.com",
			"model_mapping": map[string]any{"foo": "bar"},
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)

	// 敏感键不在 Credentials 里
	require.NotContains(t, got.Credentials, "access_token")
	require.NotContains(t, got.Credentials, "refresh_token")
	require.NotContains(t, got.Credentials, "id_token")
	require.NotContains(t, got.Credentials, "api_key")
	// 非敏感键保留
	require.Equal(t, "https://api.example.com", got.Credentials["base_url"])
	require.Equal(t, map[string]any{"foo": "bar"}, got.Credentials["model_mapping"])

	// 状态 map 标记敏感键存在
	require.True(t, got.CredentialsStatus["has_access_token"])
	require.True(t, got.CredentialsStatus["has_refresh_token"])
	require.True(t, got.CredentialsStatus["has_id_token"])
	require.True(t, got.CredentialsStatus["has_api_key"])

	// JSON 序列化校验：响应体里不会出现敏感子串
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "rt-secret")
	require.NotContains(t, string(raw), "at-secret")
	require.NotContains(t, string(raw), "sk-secret")
	require.NotContains(t, string(raw), "id-secret")
	// 状态标识应序列化进 JSON
	require.Contains(t, string(raw), "credentials_status")
	require.Contains(t, string(raw), "has_refresh_token")

	// 原始 service.Account 不应被改动
	require.Equal(t, "rt-secret", src.Credentials["refresh_token"])
}

func TestAccountFromServiceShallow_NilCredentialsOmitsStatus(t *testing.T) {
	src := &service.Account{ID: 1, Name: "n", Platform: "anthropic", Type: "oauth"}
	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)
	require.Nil(t, got.Credentials)
	require.Nil(t, got.CredentialsStatus)
}

// 票与 cookie 是上游签发的会话凭据，出线到浏览器前必须被摘掉，
// 改由 codex_ticket 提供派生摘要。这里断言的是**序列化后的字节**，
// 因为那才是真正离开进程的东西。
func TestAccountFromService_RedactsCodexTicketAndExposesSummary(t *testing.T) {
	const (
		ticket = "gAAAAABqq_20UDF7SECRETTICKET"
		cookie = `[{"n":"__oailb","v":"SECRETCOOKIEVALUE","t":1758550000}]`
	)
	now := time.Now()
	src := &service.Account{
		ID: 42, Name: "codex", Platform: "openai", Type: "oauth",
		Extra: map[string]any{
			// 键名写成字面量：这里同时钉住存储格式。
			"openai_codex_turn_state_pool_gpt-5.6-luna":             ticket,
			"openai_codex_turn_state_pool_gpt-5.6-luna_model":       "gpt-5.6-luna",
			"openai_codex_turn_state_pool_gpt-5.6-luna_captured_at": now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
			"openai_codex_turn_state":                               ticket,
			"openai_codex_turn_state_cookies":                       cookie,
			"openai_codex_turn_state_cookie_egress":                 "5ee7a93798f061c8",
			"codex_fingerprint_mode":                                "session",
		},
	}

	raw, err := json.Marshal(AccountFromService(src))
	require.NoError(t, err)
	body := string(raw)

	require.NotContains(t, body, ticket, "票本体不得出线")
	require.NotContains(t, body, "SECRETCOOKIEVALUE", "cookie 值不得出线")
	require.NotContains(t, body, "gAAAAA")
	// 摘要必须在，且带形态与 cookie 数量。
	require.Contains(t, body, `"codex_ticket"`)
	require.Contains(t, body, `"shape":`)
	require.Contains(t, body, fmt.Sprintf(`"shape":%d`, len(ticket)))
	// 其它 extra 键照旧下发（前端设置项从这里读）。
	require.Contains(t, body, `"codex_fingerprint_mode":"session"`)
}
