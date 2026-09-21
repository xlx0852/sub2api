package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 抓包式验证：用假上游跑**真实 Forward 路径**，把网关实际发出的头与体原样打印出来。
// 目的：不依赖任何代码注释/文档断言，直接观测「服务器视角」看到的 sub 出站指纹。
// 不会发出任何真实上游请求（upstream 是内存 stub）。
func TestGatewayOutboundCodexFingerprint_ObservedShape(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 客户端入站身份：模拟 Codex Desktop 的一轮真实请求（取自 mitm 实测）
	const (
		clientThread  = "01a0c48d-5710-7c03-9065-ced0ad8c374f"
		clientInstall = "6b7282e1-b32c-4c3a-99ea-c2f025eeb2d7"
	)
	clientTurnMetadata := `{"installation_id":"` + clientInstall + `",` +
		`"session_id":"` + clientThread + `","thread_id":"` + clientThread + `",` +
		`"agent_name":"/root","turn_id":"01a0c4af-90e0-7031-9d80-a5753e2a20df",` +
		`"window_id":"` + clientThread + `:0","window_number":0,` +
		`"context_window_id":"01a0c48d-5710-7c03-9065-ceee6bbbc9fb",` +
		`"request_kind":"turn","root_turn_id":"01a0c4af-90e0-7031-9d80-a5753e2a20df",` +
		`"thread_source":"user","turn_trigger":"composer","sandbox":"none",` +
		`"sandbox_mode":"danger-full-access","auto_review_enabled":false,` +
		`"node_repl_auto_review_required":false,"node_repl_disabled":false,` +
		`"turn_started_at_unix_ms":1790006235372,"analytics_enabled":true,` +
		`"model":"gpt-5.6-terra","reasoning_effort":"high","workspace_kind":"projectless"}`

	for _, mode := range []string{"off", "device", "session", "full"} {
		t.Run(mode, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{
				resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(
						`{"id":"resp_dump","model":"gpt-5.6-terra","usage":{"input_tokens":1,"output_tokens":1}}`)),
				},
			}
			cfg := &config.Config{}
			// 必须接线 codexIdentity：否则走不到身份统一那段；nil 接收者会退化成编译期规范 UA。
			svc := &OpenAIGatewayService{
				cfg:           cfg,
				httpUpstream:  upstream,
				codexIdentity: newCodexIdentityFromConfig(cfg, nil),
			}
			c, _ := newOpenAIImageGenerationControlTestContext(true, "Codex Desktop/0.155.0-alpha.9.2")
			// 覆盖成 Codex Desktop 的真实入站头
			c.Request.Header.Set("user-agent", "Codex Desktop/0.155.0-alpha.9.2 (Mac OS 27.0.0; arm64) unknown (Codex Desktop; 26.915.31945)")
			c.Request.Header.Set("originator", "Codex Desktop")
			c.Request.Header.Set("session-id", clientThread)
			c.Request.Header.Set("session_id", clientThread)
			c.Request.Header.Set("thread-id", clientThread)
			c.Request.Header.Set("x-client-request-id", clientThread)
			c.Request.Header.Set("x-codex-window-id", clientThread+":0")
			c.Request.Header.Set("x-codex-turn-metadata", clientTurnMetadata)

			// 必须是 OAuth 账号：所有指纹改写与身份统一都是 OAuth 专属路径。
			account := &Account{
				ID:          5151,
				Name:        "codex-fingerprint-dump",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Credentials: map[string]any{
					"access_token":       "oauth-token",
					"chatgpt_account_id": "chatgpt-acc",
				},
				Extra: map[string]any{codexFingerprintModeExtraKey: mode},
			}

			embedded, _ := json.Marshal(clientTurnMetadata)
			body := []byte(`{"model":"gpt-5.6-terra","input":"hi","stream":false,` +
				`"prompt_cache_key":"` + clientThread + `",` +
				`"client_metadata":{"x-codex-installation-id":"` + clientInstall + `",` +
				`"session_id":"` + clientThread + `","thread_id":"` + clientThread + `",` +
				`"x-codex-turn-metadata":` + string(embedded) + `}}`)

			_, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, upstream.lastReq, "必须已经发出上游请求才能抓包")

			// ---------- 出站头 ----------
			names := make([]string, 0, len(upstream.lastReq.Header))
			for k := range upstream.lastReq.Header {
				names = append(names, k)
			}
			sort.Strings(names)
			t.Logf("========== mode=%s 出站头（真实发出） ==========", mode)
			for _, k := range names {
				lk := strings.ToLower(k)
				v := upstream.lastReq.Header.Get(k)
				if lk == "authorization" || strings.Contains(lk, "cookie") {
					v = "[secret]"
				}
				t.Logf("  %-28s %s", k, v)
			}

			// ---------- 出站体里的身份字段 ----------
			t.Logf("---------- mode=%s 出站体身份字段 ----------", mode)
			var out map[string]any
			require.NoError(t, json.Unmarshal(upstream.lastBody, &out))
			t.Logf("  prompt_cache_key      = %v", out["prompt_cache_key"])
			t.Logf("  store                 = %v", out["store"])
			if cm, ok := out["client_metadata"].(map[string]any); ok {
				keys := make([]string, 0, len(cm))
				for k := range cm {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					t.Logf("  client_metadata[%-24s] = %v", k, cm[k])
				}
			} else {
				t.Logf("  client_metadata       = (无)")
			}

			// ---------- 不变的客户端字段（观察是否泄漏） ----------
			// ---------- 头 turn-metadata vs 体 client_metadata 的身份是否一致 ----------
			t.Logf("---------- mode=%s 头/体身份一致性 ----------", mode)
			var hdrMeta map[string]any
			if raw := upstream.lastReq.Header.Get("x-codex-turn-metadata"); raw != "" {
				_ = json.Unmarshal([]byte(raw), &hdrMeta)
			}
			var bodyMeta map[string]any
			if cm, ok := out["client_metadata"].(map[string]any); ok {
				if raw, ok := cm["x-codex-turn-metadata"].(string); ok {
					_ = json.Unmarshal([]byte(raw), &bodyMeta)
				}
			}
			for _, k := range []string{"installation_id", "session_id", "thread_id", "turn_id", "window_id"} {
				hv := "<无>"
				if hdrMeta != nil {
					hv = fmt.Sprintf("%v", hdrMeta[k])
				}
				bv := "<无>"
				if bodyMeta != nil {
					bv = fmt.Sprintf("%v", bodyMeta[k])
				}
				mark := "一致"
				if hv != bv {
					mark = "**不一致**"
				}
				t.Logf("  %-16s 头=%-40s 体=%-40s %s", k, hv, bv, mark)
			}

			// ---------- 锁定不变量 ----------
			// 1) 身份统一对 OAuth 出站始终生效：与指纹模式无关。
			require.Equal(t, "codex_cli_rs", upstream.lastReq.Header.Get("originator"))
			require.True(t, strings.HasPrefix(upstream.lastReq.Header.Get("user-agent"), "codex_cli_rs/"))
			require.NotEmpty(t, upstream.lastReq.Header.Get("version"))
			// 2) 下划线兼容头被删除（真实客户端会发它）。
			require.Empty(t, upstream.lastReq.Header.Get("session_id"))
			// 3) off 模式并非纯透传：thread/session 被隔离为另一值。
			if mode == "off" || mode == "device" {
				require.NotEqual(t, clientThread, upstream.lastReq.Header.Get("thread-id"),
					"off/device 模式下 header thread-id 应已被隔离")
			}
			// 4) session/full 模式：头 turn-metadata 与体 client_metadata 的派生身份必须一致。
			if mode == "session" || mode == "full" {
				var hm, bm map[string]any
				require.NoError(t, json.Unmarshal([]byte(upstream.lastReq.Header.Get("x-codex-turn-metadata")), &hm))
				cm, _ := out["client_metadata"].(map[string]any)
				require.NotNil(t, cm)
				require.NoError(t, json.Unmarshal([]byte(cm["x-codex-turn-metadata"].(string)), &bm))
				for _, k := range []string{"installation_id", "session_id", "thread_id", "turn_id", "window_id"} {
					require.Equal(t, hm[k], bm[k], "%s 头体必须一致", k)
				}
			}

			t.Logf("---------- mode=%s 客户端字段是否原样透传 ----------", mode)
			_ = mode
			meta := upstream.lastReq.Header.Get("x-codex-turn-metadata")
			var leaks []string
			if meta != "" {
				var m map[string]any
				if json.Unmarshal([]byte(meta), &m) == nil {
					for _, k := range []string{"turn_trigger", "workspace_kind", "agent_name", "context_window_id", "thread_source", "analytics_enabled"} {
						if v, ok := m[k]; ok {
							t.Logf("  turn-metadata.%-20s = %v", k, v)
						}
					}
					for _, k := range []string{"session_id", "thread_id", "window_id"} {
						if v, _ := m[k].(string); v != "" && strings.Contains(v, clientThread) {
							leaks = append(leaks, k)
						}
					}
				}
			}
			require.Empty(t, leaks,
				"任何模式下会话元数据都不得回泄客户端原始 thread（否则头部的隔离/派生被绕过）")
			// 窗口头必须与 thread 头同源：否则元数据里留下的窗口号仍指向原始线程。
			windowHeader := upstream.lastReq.Header.Get("x-codex-window-id")
			if windowHeader != "" {
				require.NotContains(t, windowHeader, clientThread)
			}
		})
	}
}

// --- 会话元数据对齐的边界 ---

func TestAlignCodexTurnMetadataWithIsolatedSession(t *testing.T) {
	const raw = "01a0c48d-5710-7c03-9065-ced0ad8c374f"
	isolate := func(v string) string { return "iso(" + v + ")" }

	t.Run("同步 session/thread/window 与父子线程", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-window-id", raw+":3")
		h.Set("x-codex-turn-metadata", `{"session_id":"`+raw+`","thread_id":"`+raw+`",`+
			`"window_id":"`+raw+`:1","parent_thread_id":"`+raw+`","forked_from_thread_id":"`+raw+`",`+
			`"agent_name":"/root","turn_trigger":"composer"}`)

		alignCodexTurnMetadataWithIsolatedSession(h, "iso-session", "iso-thread", isolate)

		require.Equal(t, "iso-thread:3", h.Get("x-codex-window-id"), "窗口号后缀必须保留")
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &m))
		require.Equal(t, "iso-session", m["session_id"])
		require.Equal(t, "iso-thread", m["thread_id"])
		require.Equal(t, "iso-thread:1", m["window_id"])
		require.Equal(t, "iso("+raw+")", m["parent_thread_id"])
		require.Equal(t, "iso("+raw+")", m["forked_from_thread_id"])
		// 非身份字段必须原样保留。
		require.Equal(t, "/root", m["agent_name"])
		require.Equal(t, "composer", m["turn_trigger"])
	})

	t.Run("thread 为空时回退 session", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-turn-metadata", `{"thread_id":"`+raw+`"}`)
		alignCodexTurnMetadataWithIsolatedSession(h, "iso-session", "", isolate)
		require.Contains(t, h.Get("x-codex-turn-metadata"), "iso-session")
	})

	t.Run("元数据不可解析时不破坏原始字节", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-turn-metadata", "{not json")
		alignCodexTurnMetadataWithIsolatedSession(h, "iso", "iso", isolate)
		require.Equal(t, "{not json", h.Get("x-codex-turn-metadata"),
			"解析失败必须原样保留：身份改写不得成为新的失败点")
	})

	t.Run("缺失字段不被凭空写入", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-turn-metadata", `{"agent_name":"/root"}`)
		alignCodexTurnMetadataWithIsolatedSession(h, "iso", "iso", isolate)
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &m))
		_, hasSession := m["session_id"]
		require.False(t, hasSession, "客户端没发的字段不应被补齐")
	})

	t.Run("空隔离值等同不做处理", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-turn-metadata", `{"session_id":"`+raw+`"}`)
		alignCodexTurnMetadataWithIsolatedSession(h, "", "", isolate)
		require.Contains(t, h.Get("x-codex-turn-metadata"), raw)
	})

	t.Run("nil isolate 时跳过父子线程", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-turn-metadata", `{"session_id":"`+raw+`","parent_thread_id":"`+raw+`"}`)
		require.NotPanics(t, func() { alignCodexTurnMetadataWithIsolatedSession(h, "iso", "iso", nil) })
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &m))
		require.Equal(t, "iso", m["session_id"])
		require.Equal(t, raw, m["parent_thread_id"], "无隔离函数时不得改成无法对齐的值")
	})
}

func TestCodexWindowIDSuffix(t *testing.T) {
	cases := map[string]string{
		"01a0-abc:0":  ":0",
		"01a0-abc:12": ":12",
		"01a0-abc":    ":0",
		"01a0-abc:":   ":0",
		"01a0-abc:x":  ":0",
		"":            ":0",
		"a:b:7":       ":7",
	}
	for in, want := range cases {
		require.Equal(t, want, codexWindowIDSuffix(in), "input=%q", in)
	}
}
