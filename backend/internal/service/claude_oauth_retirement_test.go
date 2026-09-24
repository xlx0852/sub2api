// Claude（Anthropic 上游）OAuth / setup-token 供给已退役的回归护栏。
// 见 openspec/changes/remove-claude-oauth-login。
package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaudeOAuthRetired_GuardOnlyTargetsAnthropic 锁死守卫的平台维度：
// 必须拒绝 Anthropic 的 oauth / setup-token，且不得误伤其它平台的同类供给
// （Antigravity 同样使用 setup-token，OpenAI/Gemini/Grok/Kimi 仍走 oauth）。
func TestClaudeOAuthRetired_GuardOnlyTargetsAnthropic(t *testing.T) {
	rejected := []struct {
		name     string
		platform string
		typ      string
	}{
		{name: "anthropic oauth", platform: PlatformAnthropic, typ: AccountTypeOAuth},
		{name: "anthropic setup-token", platform: PlatformAnthropic, typ: AccountTypeSetupToken},
	}
	for _, tt := range rejected {
		t.Run("reject/"+tt.name, func(t *testing.T) {
			err := rejectDeprecatedClaudeOAuthAccount(tt.platform, tt.typ)
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)

			err = rejectRetiredClaudeOAuthAccount(&Account{Platform: tt.platform, Type: tt.typ})
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)

			// 导出包装必须与内部守卫等价（handler 层 fail-closed 依赖它）。
			err = RejectRetiredClaudeOAuthAccount(&Account{Platform: tt.platform, Type: tt.typ})
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)
		})
	}

	allowed := []struct {
		name     string
		platform string
		typ      string
	}{
		{name: "anthropic apikey", platform: PlatformAnthropic, typ: AccountTypeAPIKey},
		{name: "antigravity setup-token", platform: PlatformAntigravity, typ: AccountTypeSetupToken},
		{name: "antigravity oauth", platform: PlatformAntigravity, typ: AccountTypeOAuth},
		{name: "openai oauth", platform: PlatformOpenAI, typ: AccountTypeOAuth},
		{name: "gemini oauth", platform: PlatformGemini, typ: AccountTypeOAuth},
		{name: "grok oauth", platform: PlatformGrok, typ: AccountTypeOAuth},
		{name: "kimi oauth", platform: PlatformKimi, typ: AccountTypeOAuth},
	}
	for _, tt := range allowed {
		t.Run("allow/"+tt.name, func(t *testing.T) {
			require.NoError(t, rejectDeprecatedClaudeOAuthAccount(tt.platform, tt.typ))
			require.NoError(t, rejectRetiredClaudeOAuthAccount(&Account{Platform: tt.platform, Type: tt.typ}))
			require.NoError(t, RejectRetiredClaudeOAuthAccount(&Account{Platform: tt.platform, Type: tt.typ}))
		})
	}
}

// TestClaudeOAuthRetired_RetiredErrorIsBadRequest 确认错误语义（HTTP 400）：
// 存量 Anthropic OAuth 行命中后必须是客户端可见的 400，而不是 500。
func TestClaudeOAuthRetired_RetiredErrorIsBadRequest(t *testing.T) {
	err := RejectRetiredClaudeOAuthAccount(&Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth})
	require.Error(t, err)
	require.Contains(t, err.Error(), "CLAUDE_OAUTH_ACCOUNT_RETIRED")
	require.Contains(t, strings.ToLower(err.Error()), "api-key")
}

// TestClaudeOAuthRetired_CRSExportHasNoClaudeAccountsField 锁死 CRS 导出结构不再
// 暴露 oauth/setup-token 账号字段（`claudeAccounts`），但保留 Anthropic APIKey 的
// `claudeConsoleAccounts`。
func TestClaudeOAuthRetired_CRSExportHasNoClaudeAccountsField(t *testing.T) {
	jsonTags := collectJSONTags(reflect.TypeOf(crsExportResponse{}.Data))
	require.Contains(t, jsonTags, "claudeConsoleAccounts", "Anthropic APIKey（ClaudeConsole）导入必须保留")
	require.NotContains(t, jsonTags, "claudeAccounts", "Claude OAuth/Setup Token 导入必须已删除")
}

// TestClaudeOAuthRetired_CRSParsesLegacyPayloadWithoutOAuthAccounts 模拟旧版 CRS
// 仍返回 `claudeAccounts` 的场景：必须被忽略（不产生导入），同时
// `claudeConsoleAccounts` 仍被正常解析。
func TestClaudeOAuthRetired_CRSParsesLegacyPayloadWithoutOAuthAccounts(t *testing.T) {
	legacy := `{
	  "success": true,
	  "data": {
	    "exportedAt": "2025-01-01T00:00:00Z",
	    "claudeAccounts": [
	      {"kind":"claude","id":"oauth-1","name":"legacy-claude-oauth","platform":"anthropic","credentials":{"access_token":"x"}},
	      {"kind":"claude","id":"setup-1","name":"legacy-claude-setup","platform":"anthropic","credentials":{"access_token":"y"}}
	    ],
	    "claudeConsoleAccounts": [
	      {"kind":"claudeConsole","id":"console-1","name":"claude-apikey","platform":"anthropic","credentials":{"api_key":"sk-ant-1"}}
	    ]
	  }
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer admin-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(legacy))
	}))
	defer srv.Close()

	exported, err := crsExportAccounts(context.Background(), srv.Client(), srv.URL, "admin-token")
	require.NoError(t, err)
	require.Len(t, exported.Data.ClaudeConsoleAccounts, 1)
	require.Equal(t, "console-1", exported.Data.ClaudeConsoleAccounts[0].ID)

	// 结构体层面不存在可挂载 claudeAccounts 的字段，任何旧版条目都无处落地。
	raw, err := json.Marshal(exported.Data)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "claudeAccounts")
	require.Contains(t, string(raw), "claudeConsoleAccounts")
}

// collectJSONTags 收集结构体（含嵌套匿名结构体）的全部 json tag 名。
func collectJSONTags(t reflect.Type) []string {
	var tags []string
	var walk func(reflect.Type)
	walk = func(rt reflect.Type) {
		if rt.Kind() == reflect.Ptr {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if tag != "" && tag != "-" {
				tags = append(tags, tag)
			}
			walk(f.Type)
		}
	}
	walk(t)
	return tags
}
