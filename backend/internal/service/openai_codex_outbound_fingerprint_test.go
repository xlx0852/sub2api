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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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
			// 用**真实客户端的完整入站头集合**（mitm 全量抓包，无白名单）灌入，
			// 这样出站差异才可归因：凡客户端发过而出站没有的头，都是网关丢的。
			for k, v := range realCodexClientRequestHeaders(clientThread, clientInstall, clientTurnMetadata) {
				c.Request.Header.Set(k, v)
			}
			// 生产里 apiKeyID 非 0；为 0 会让隔离值退化成 k0 前缀，抓包结果不可比。
			c.Set("api_key", &APIKey{ID: 4242})

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

			// ---------- 与真实客户端的逐头差异（本轮最关键的输出） ----------
			inbound := realCodexClientRequestHeaders(clientThread, clientInstall, clientTurnMetadata)
			outbound := map[string]string{}
			for k := range upstream.lastReq.Header {
				outbound[strings.ToLower(k)] = upstream.lastReq.Header.Get(k)
			}
			var added, removed, changed []string
			for k, v := range outbound {
				in, had := inbound[k]
				switch {
				case !had:
					if k != "authorization" { // 鉴权头本就必须替换
						added = append(added, k+"="+shortHeaderValueForLog(v))
					}
				case in != v:
					changed = append(changed, k)
				}
			}
			for k := range inbound {
				if _, ok := outbound[k]; !ok {
					removed = append(removed, k)
				}
			}
			sort.Strings(added)
			sort.Strings(removed)
			sort.Strings(changed)
			t.Logf("---------- mode=%s 相对真实客户端的差异 ----------", mode)
			t.Logf("  [新增] 客户端没发、网关发了 : %v", added)
			t.Logf("  [丢失] 客户端发了、网关没发 : %v", removed)
			t.Logf("  [改写] 两边都有但值不同   : %v", changed)

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
			// 1) 出站身份按入站客户端镜像（官方家族），且 UA 首段与 originator 配套。
			require.Equal(t, "codex_exec", upstream.lastReq.Header.Get("originator"),
				"应镜像入站客户端的 originator，而不是固定 codex_cli_rs")
			require.True(t, strings.HasPrefix(upstream.lastReq.Header.Get("user-agent"), "codex_exec/"))
			require.Contains(t, upstream.lastReq.Header.Get("user-agent"), "(codex_exec; ",
				"UA 必须带 (originator; 版本) 尾段")
			// 真实客户端不发 version 头 → 网关也不发（此前是纯增量特征）。
			require.Empty(t, upstream.lastReq.Header.Get("version"), "不得发送 version 头")
			// 客户端自带 attestation 应原样转发（网关无法生成，只能转发）。
			require.Equal(t, `{"v":1,"s":0,"t":"v1.client-attestation-placeholder"}`,
				upstream.lastReq.Header.Get(openaiAttestationHeader),
				"应转发客户端设备证明")
			// 2) routing-hint 必须与出站 body 的 model 一致（三个真实客户端都发它）。
			require.Equal(t, "model="+gjson.GetBytes(upstream.lastBody, "model").String(),
				upstream.lastReq.Header.Get("x-codex-routing-hint"))
			// 3) 下划线兼容头被删除。
			require.Empty(t, upstream.lastReq.Header.Get("session_id"))
			// 4) off 模式并非纯透传：thread/session 被隔离为另一值。
			if mode == "off" || mode == "device" {
				require.NotEqual(t, clientThread, upstream.lastReq.Header.Get("thread-id"),
					"off/device 模式下 header thread-id 应已被隔离")
			}
			// 5) session/full 模式：头 turn-metadata 与体 client_metadata 的派生身份必须一致。
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

// realCodexClientRequestHeaders 是真实 Codex 客户端 /responses 的**全量**请求头，
// 来自 2026-09-22 的 mitm 全量抓包（无白名单）。
//
// 记录在案的两个坑：
//   - 客户端**不发** `session_id`(下划线) 也**不发** `version`。此前用白名单 addon 得到的
//     「客户端会发下划线 session_id」是 addon 里 `name.replace("_","-")` 回退造成的假象。
//   - authorization / cookie 用占位值：本测试只验证网关**是否**透传它们，不验内容。
//
// shortHeaderValueForLog 截断用于日志的头值，避免把长令牌刷进测试输出。
func shortHeaderValueForLog(v string) string {
	if len(v) > 48 {
		return v[:48] + "…"
	}
	return v
}

func realCodexClientRequestHeaders(thread, install, turnMetadata string) map[string]string {
	return map[string]string{
		"accept":                                 "text/event-stream",
		"authorization":                          "Bearer client-token-placeholder",
		"chatgpt-account-id":                     "d8ee690b-9098-43d7-a525-79044f324b8e",
		"content-encoding":                       "zstd",
		"content-type":                           "application/json",
		"cookie":                                 "__Secure-next-auth.session-token=client-cookie-placeholder",
		"originator":                             "codex_exec",
		"session-id":                             thread,
		"thread-id":                              thread,
		"user-agent":                             "codex_exec/0.155.1 (Mac OS 27.0.0; arm64) unknown (codex_exec; 0.155.1)",
		"x-client-request-id":                    thread,
		"x-codex-beta-features":                  "prevent_idle_sleep,remote_compaction_v2",
		"x-codex-routing-hint":                   "model=gpt-5.6-terra",
		"x-codex-turn-metadata":                  turnMetadata,
		"x-codex-window-id":                      thread + ":0",
		"x-openai-internal-codex-responses-lite": "true",
		// ChatGPT.app 独有：设备证明（密码学可验证，内含 locale/时区/bundle id）。
		// CLI 两个入口都不发这个头。
		"x-oai-attestation": `{"v":1,"s":0,"t":"v1.client-attestation-placeholder"}`,
	}
}

// --- 按入站客户端类型镜像出站身份 ---

func TestCodexIdentityCandidateUA_MirrorsOfficialClientFamily(t *testing.T) {
	svc := newOverloadRotationTestService(false)
	account := newTestOAuthAccount(7001, nil)

	cases := []struct {
		name    string
		inbound string
		wantMir bool
	}{
		{"TUI", "codex-tui/0.155.1 (Mac OS 27.0.0; arm64) xterm-256color (codex-tui; 0.155.1)", true},
		{"exec", "codex_exec/0.155.1 (Mac OS 27.0.0; arm64) unknown (codex_exec; 0.155.1)", true},
		{"Desktop", "Codex Desktop/0.155.0-alpha.9.2 (Mac OS 27.0.0; arm64) unknown (Codex Desktop; 26.915.31945)", true},
		{"非官方客户端", "MyCustomClient/1.2.3 (Linux; x86_64)", false},
		{"空", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newFingerprintTestContext("sess")
			if tc.inbound != "" {
				c.Request.Header.Set("User-Agent", tc.inbound)
			}
			got := svc.codexIdentityCandidateUA(c, account)
			if tc.wantMir {
				require.Equal(t, tc.inbound, got, "官方家族应镜像入站 UA")
			} else {
				require.NotEqual(t, tc.inbound, got, "非官方客户端不得被镜像")
				require.True(t, strings.HasPrefix(got, "codex_cli_rs/"), "应回退到账号派生 UA")
			}
		})
	}
}

// 管理员显式配置的账号级 UA 优先于自动镜像（显式意图不被覆盖）。
func TestCodexIdentityCandidateUA_ExplicitAccountUAPrecedence(t *testing.T) {
	svc := newOverloadRotationTestService(false)
	account := newTestOAuthAccount(7002, nil)
	// GetOpenAIUserAgent 读的是 Credentials.user_agent（不是 extra）。
	account.Credentials = map[string]any{"user_agent": "PinnedAgent/9.9.9 (Plan9; mips)"}
	c := newFingerprintTestContext("sess")
	c.Request.Header.Set("User-Agent", "codex-tui/0.155.1 (Mac OS 27.0.0; arm64) xterm-256color (codex-tui; 0.155.1)")
	require.Equal(t, "PinnedAgent/9.9.9 (Plan9; mips)", svc.codexIdentityCandidateUA(c, account))
}

// 端到端：镜像后出站 originator 与 UA 首段一致，且都跟随客户端家族。
func TestCodexIdentityCandidateUA_EndToEndIdentityIsSelfConsistent(t *testing.T) {
	cfg := &config.Config{}
	identity := newCodexIdentityFromConfig(cfg, nil)
	for _, inbound := range []string{
		"codex-tui/0.155.1 (Mac OS 27.0.0; arm64) xterm-256color (codex-tui; 0.155.1)",
		"codex_exec/0.155.1 (Mac OS 27.0.0; arm64) unknown (codex_exec; 0.155.1)",
	} {
		resolved := identity.resolveOutboundIdentity(inbound)
		require.True(t, strings.HasPrefix(resolved.userAgent, resolved.originator+"/"),
			"UA 首段必须与 originator 配套（上游会校验，issue #3901）：ua=%q og=%q", resolved.userAgent, resolved.originator)
	}
}

// 残缺的官方 UA（无 OS 组）不得被镜像：镜像会产出不存在的 UA 形态。
func TestCodexIdentityCandidateUA_RejectsIncompleteOfficialUA(t *testing.T) {
	svc := newOverloadRotationTestService(false)
	account := newTestOAuthAccount(7003, nil)
	for _, inbound := range []string{
		"codex_cli_rs/0.1.0",
		"codex_cli_rs/0.144.1",
		"codex_cli_rs/",
	} {
		c := newFingerprintTestContext("sess")
		c.Request.Header.Set("User-Agent", inbound)
		got := svc.codexIdentityCandidateUA(c, account)
		require.NotEqual(t, inbound, got, "残缺 UA 不得被镜像：%q", inbound)
		require.True(t, strings.HasPrefix(got, "codex_cli_rs/"), "应回退账号派生 UA")
		require.Contains(t, got, "(", "回退 UA 必须带 OS 组")
		require.Contains(t, got, ")", "回退 UA 必须带 OS 组")
	}
}

// --- 请求体 zstd 压缩 ---

func TestCompressOpenAIUpstreamBodyZstd(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-terra","input":"` + strings.Repeat("hello world ", 400) + `"}`)

	compressed, enc := compressOpenAIUpstreamBodyZstd(body)
	require.Equal(t, "zstd", enc, "应声明 zstd 压缩")
	require.Less(t, len(compressed), len(body), "压缩后应更小")

	// 必须能被标准 zstd 解出原始内容（防止写出损坏的帧）。
	dec, err := zstd.NewReader(strings.NewReader(string(compressed)))
	require.NoError(t, err)
	defer dec.Close()
	roundTrip, err := io.ReadAll(dec)
	require.NoError(t, err)
	require.Equal(t, body, roundTrip, "压缩必须可逆")
}

func TestCompressOpenAIUpstreamBodyZstd_EmptyBody(t *testing.T) {
	compressed, enc := compressOpenAIUpstreamBodyZstd(nil)
	require.Empty(t, enc, "空体不压缩")
	require.Nil(t, compressed)
}

// 端到端：OAuth /responses 出站必须带 content-encoding: zstd，且请求体可解回原 JSON。
func TestGatewayOutboundCompressesRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_zstd","model":"gpt-5.6-terra","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_exec/0.155.1 (Mac OS 27.0.0; arm64) unknown (codex_exec; 0.155.1)")
	c.Request.Header.Set("originator", "codex_exec")

	account := newCapacityShedTestAccount(7004)
	account.Credentials = map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"}

	body := []byte(`{"model":"gpt-5.6-terra","input":"hi","stream":false}`)
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "zstd", upstream.lastReq.Header.Get("Content-Encoding"),
		"OAuth /responses 出站必须声明 zstd 压缩（官方客户端一律压缩）")

	// recorder 抓到的就是线上发出的原始字节：断言它是可解回的 zstd 帧。
	dec, err := zstd.NewReader(strings.NewReader(string(upstream.lastBodyRaw)))
	require.NoError(t, err)
	defer dec.Close()
	plain, err := io.ReadAll(dec)
	require.NoError(t, err)
	// 出站体会被 Codex OAuth 变换改写（input → messages、注入 instructions），
	// 所以这里断言的是「压缩帧解出的仍是合法 JSON，且模型正确」，而不是与入参逐字相等。
	require.True(t, json.Valid(plain), "解压后必须是合法 JSON")
	require.Equal(t, "gpt-5.6-terra", gjson.GetBytes(plain, "model").String())
	require.NotEmpty(t, gjson.GetBytes(plain, "instructions").String(), "Codex 变换应已生效")
}

// --- 采票探针的出口与身份必须与业务出站同源 ---

// 同一个账号不该存在两种请求形态：此前探针是「6 个头的极简 CLI 请求」，
// 业务请求是「19 个头、镜像真实客户端」。于是同一张票由极简请求采到、
// 由全副身份的请求回放。这里用**业务测试同一份真实客户端 fixture** 当尺子，
// 直接量探针的出站差异；并断言探针确实走了账号代理（出口一致）。
func TestCodexTurnStateProbeOutboundFingerprint(t *testing.T) {
	const (
		clientThread  = "01a0c48d-5710-7c03-9065-ced0ad8c374f"
		clientInstall = "6b7282e1-b32c-4c3a-99ea-c2f025eeb2d7"
		probeModel    = "gpt-6-astra"
	)
	clientTurnMetadata := `{"installation_id":"` + clientInstall + `","session_id":"` + clientThread + `",` +
		`"thread_id":"` + clientThread + `","agent_name":"/root",` +
		`"turn_id":"01a0c4af-90e0-7031-9d80-a5753e2a20df","window_id":"` + clientThread + `:0",` +
		`"window_number":0,"context_window_id":"01a0c48d-5710-7c03-9065-ceee6bbbc9fb",` +
		`"request_kind":"turn","root_turn_id":"01a0c4af-90e0-7031-9d80-a5753e2a20df",` +
		`"thread_source":"user","turn_trigger":"composer","sandbox":"none",` +
		`"sandbox_mode":"danger-full-access","auto_review_enabled":false,` +
		`"node_repl_auto_review_required":false,"node_repl_disabled":false,` +
		`"turn_started_at_unix_ms":1790006235372,"analytics_enabled":true,"model":"` + probeModel + `",` +
		`"reasoning_effort":"high","workspace_kind":"project"}`

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				openAICodexTurnStateHeader: []string{"gAAAAA" + strings.Repeat("P", 250)},
				"Content-Type":             []string{"text/event-stream"},
			},
			Body: io.NopCloser(strings.NewReader(
				`data: {"type":"response.completed","response":{"model":"gpt-6-astra"}}` + "\n\n")),
		},
	}
	cfg := &config.Config{}
	svc := &OpenAIGatewayService{
		cfg:           cfg,
		httpUpstream:  upstream,
		codexIdentity: newCodexIdentityFromConfig(cfg, nil),
	}
	acc := &Account{
		ID:          5151,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Proxy:       &Proxy{Protocol: "http", Host: "proxy.internal", Port: 8080},
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
	}

	result := svc.probeOpenAICodexTurnState(context.Background(), acc, "oauth-token", "chatgpt-acc", probeModel, nil)
	require.True(t, result.completed)
	require.NotNil(t, upstream.lastReq, "探针必须真的发出了上游请求")

	// ---- 出口：必须与业务请求相同（账号代理），而不是服务器直连 ----
	require.Equal(t, acc.Proxy.URL(), upstream.lastProxyURL,
		"采票出口必须与业务出口一致；此前探针用裸 http.Client 直连服务器 IP")
	// 传输 profile 必须与业务相同：同一个 httpUpstream 下不同 profile 走不同传输策略。
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()),
		"采票必须使用与业务出站相同的 openai 传输 profile")

	// ---- 形态：与真实客户端逐头差异 ----
	inbound := realCodexClientRequestHeaders(clientThread, clientInstall, clientTurnMetadata)
	outbound := map[string]string{}
	for k := range upstream.lastReq.Header {
		outbound[strings.ToLower(k)] = upstream.lastReq.Header.Get(k)
	}
	var added, removed []string
	for k := range outbound {
		if _, had := inbound[k]; !had {
			added = append(added, k)
		}
	}
	for k := range inbound {
		if _, ok := outbound[k]; !ok {
			removed = append(removed, k)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	t.Logf("探针出站相对真实客户端的差异：[新增]=%v [丢失]=%v", added, removed)

	// 新增必须为空：探针头集合是真实客户端头集合的子集，一条不多。
	// （authorization 在 fixture 里也存在，只是值被替换；content-length 由 Go 在
	//  真正写出时填充，不在 Header map 里。）
	require.Empty(t, added, "探针不得引入真实客户端之外的任何头")
	// 允许的丢失只有三项，均为结构性或不适用（本用例没有采集到 cookie）：
	//   cookie                                  —— 网关用账号 OAuth 鉴权，LB cookie 注入是独立的一件事；
	//   x-oai-attestation                       —— 设备证明只能转发、无法生成；
	//   x-openai-internal-codex-responses-lite  —— 该头声明本体是 Lite 形态，探针体不是；
	//                                             实测带上它上游回 HTTP 400。
	require.Equal(t, []string{"cookie", "x-oai-attestation", "x-openai-internal-codex-responses-lite"}, removed)
}

// 采集到上游签发的 cookie 后，探针必须把它带上：真实客户端 123/123 都发 cookie，
// 票由探针采、由业务回放，两边带不同的亲和状态就等于又造了两种会话形态。
func TestCodexTurnStateProbeSendsCapturedCookies(t *testing.T) {
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				openAICodexTurnStateHeader: []string{"gAAAAA" + strings.Repeat("Q", 250)},
				"Set-Cookie":               []string{"__oailb=fresh-lb; Max-Age=3600; Path=/"},
			},
			Body: io.NopCloser(strings.NewReader(
				`data: {"type":"response.completed","response":{"model":"gpt-6-astra"}}` + "\n\n")),
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAICodexTurnStateReuseEnabled = true
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, codexIdentity: newCodexIdentityFromConfig(cfg, nil)}
	acc := &Account{
		ID: 5152, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Concurrency: 1,
		Extra:       map[string]any{},
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
	}

	// 第一轮：响应下发 cookie → 被采下来。
	first := svc.probeOpenAICodexTurnState(context.Background(), acc, "oauth-token", "chatgpt-acc", "gpt-6-astra", nil)
	require.True(t, first.completed)
	require.Contains(t, codexTurnStateCookieHeaderValue(acc, time.Now()), "__oailb=fresh-lb")

	// 第二轮：探针必须带上它。
	svc.probeOpenAICodexTurnState(context.Background(), acc, "oauth-token", "chatgpt-acc", "gpt-6-astra", nil)
	require.Equal(t, "__oailb=fresh-lb", upstream.lastReq.Header.Get("Cookie"))
}
