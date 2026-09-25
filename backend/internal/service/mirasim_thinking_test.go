//go:build unit

package service

// Mirasim thinking 形态与 effort 阶梯单测（对齐官方客户端 0.0.354 语义）。
//
// 关键约束（改动前请先读）：这些断言同时是「本地 400」边界的守卫——
// 一旦有人把某些档位误判成可用，请求就会带着上游必拒的形态发出去。

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func mirasimThinkingBody(t *testing.T, raw string) []byte {
	t.Helper()
	require.True(t, gjson.Valid(raw), "测试体必须是合法 JSON")
	return []byte(raw)
}

func TestMirasimThinkingAdaptiveFormEffortLadder(t *testing.T) {
	cases := []struct {
		name    string
		model   string
		body    string
		wantErr bool
		effort  string
	}{
		{"claude medium", "claude-sonnet-5", `{"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"}}`, false, "medium"},
		{"claude xhigh", "claude-opus-5", `{"output_config":{"effort":"xhigh"}}`, false, "xhigh"},
		{"claude ultra folds to max", "claude-sonnet-5", `{"output_config":{"effort":"ultra"}}`, false, "max"},
		// reasoning_effort 不参与 Claude 形态推导，只需把 ultra 折成 max（单独断言见下）。
		{"claude ultra in reasoning_effort", "claude-sonnet-5", `{"output_config":{"effort":"ultra"}}`, false, "max"},
		{"claude off rejected", "claude-sonnet-5", `{"output_config":{"effort":"off"}}`, true, ""},
		{"claude minimal rejected", "claude-sonnet-5", `{"output_config":{"effort":"minimal"}}`, true, ""},
		{"deepseek low ok", "deepseek-flash", `{"output_config":{"effort":"low"}}`, false, "low"},
		{"deepseek high ok", "deepseek-flash", `{"output_config":{"effort":"high"}}`, false, "high"},
		{"deepseek medium rejected", "deepseek-flash", `{"output_config":{"effort":"medium"}}`, true, ""},
		{"deepseek ultra folds to max then ok", "deepseek-flash", `{"output_config":{"effort":"ultra"}}`, false, "max"},
		{"glm low ok", "glm-5.3-flash", `{"output_config":{"effort":"low"}}`, false, "low"},
		{"glm xhigh rejected", "glm-5.3-flash", `{"output_config":{"effort":"xhigh"}}`, true, ""},
		{"kimi max ok", "kimi-k3", `{"output_config":{"effort":"max"}}`, false, "max"},
		{"kimi medium rejected", "kimi-k3", `{"output_config":{"effort":"medium"}}`, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := mirasimThinkingBody(t, tc.body)
			out, err := normalizeMirasimRelayThinkingBody(body, tc.model, mirasimShapeAdaptive)
			if tc.wantErr {
				require.Error(t, err)
				var cfgErr *MirasimThinkingConfigError
				require.ErrorAs(t, err, &cfgErr)
				require.Equal(t, 400, cfgErr.StatusCode())
				return
			}
			require.NoError(t, err)
			require.Equal(t, "adaptive", gjson.GetBytes(out, "thinking.type").String())
			require.False(t, gjson.GetBytes(out, "thinking.budget_tokens").Exists(),
				"自适应型不得携带 budget_tokens")
			require.Equal(t, tc.effort, gjson.GetBytes(out, "output_config.effort").String())
		})
	}
}

func TestMirasimThinkingDeepSeekOffAndGlmKimiOff(t *testing.T) {
	// DeepSeek 的 off 是合法档位：删掉 thinking，写 effort=off。
	out, err := normalizeMirasimRelayThinkingBody(
		mirasimThinkingBody(t, `{"thinking":{"type":"disabled"}}`),
		"deepseek-flash", mirasimShapeAdaptive)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(out, "thinking").Exists())
	require.Equal(t, "off", gjson.GetBytes(out, "output_config.effort").String(),
		"DeepSeek 的 off 必须走 effort=off 而不是 thinking 字段")

	out2, err := normalizeMirasimRelayThinkingBody(
		mirasimThinkingBody(t, `{"thinking":{"type":"disabled"}}`), "glm-5.3-flash", mirasimShapeAdaptive)
	require.Error(t, err)
	require.Nil(t, out2)
	var cfgErr *MirasimThinkingConfigError
	require.ErrorAs(t, err, &cfgErr)
	require.Equal(t, "mirasim_effort_invalid", cfgErr.Code)
}

func TestMirasimThinkingBudgetFormForNonAdaptiveModel(t *testing.T) {
	out, err := normalizeMirasimRelayThinkingBody(
		mirasimThinkingBody(t, `{"max_tokens":8192,"output_config":{"effort":"high"}}`),
		"claude-legacy-budget", mirasimShapeBudget)
	require.NoError(t, err)
	require.Equal(t, "enabled", gjson.GetBytes(out, "thinking.type").String())
	require.Equal(t, int64(8191), gjson.GetBytes(out, "thinking.budget_tokens").Int(),
		"预算必须小于 max_tokens")
	require.False(t, gjson.GetBytes(out, "output_config.effort").Exists(),
		"预算型不得携带 effort")

	// 预算型模型收到固定 budget_tokens 时直接定值；但超过 max_tokens 要夹紧。
	out2, err := normalizeMirasimRelayThinkingBody(
		mirasimThinkingBody(t, `{"max_tokens":4096,"thinking":{"type":"enabled","budget_tokens":5000}}`),
		"claude-legacy-budget", mirasimShapeBudget)
	require.NoError(t, err)
	require.Equal(t, int64(4095), gjson.GetBytes(out2, "thinking.budget_tokens").Int())
}

func TestMirasimThinkingAdaptiveModelReceivesBudget(t *testing.T) {
	// 自适应型收到固定预算：折成最近档位，而不是拒绝。
	out, err := normalizeMirasimRelayThinkingBody(
		mirasimThinkingBody(t, `{"thinking":{"type":"enabled","budget_tokens":30000}}`),
		"claude-sonnet-5", mirasimShapeAdaptive)
	require.NoError(t, err)
	require.Equal(t, "adaptive", gjson.GetBytes(out, "thinking.type").String())
	require.Equal(t, "xhigh", gjson.GetBytes(out, "output_config.effort").String())
	require.False(t, gjson.GetBytes(out, "thinking.budget_tokens").Exists())

	// 自适应型 + 极小预算 → 折到最低档 low（不是 minimal）。
	out2, err := normalizeMirasimRelayThinkingBody(
		mirasimThinkingBody(t, `{"thinking":{"type":"enabled","budget_tokens":128}}`),
		"claude-sonnet-5", mirasimShapeAdaptive)
	require.NoError(t, err)
	require.Equal(t, "low", gjson.GetBytes(out2, "output_config.effort").String())
}

func TestMirasimThinkingAbsentControlsAreUntouched(t *testing.T) {
	body := `{"model":"deepseek-flash","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`
	out, err := normalizeMirasimRelayThinkingBody([]byte(body), "deepseek-flash", mirasimShapeAdaptive)
	require.NoError(t, err)
	require.JSONEq(t, body, string(out), "没有 thinking/effort 控制时必须原样放行")
}

func TestMirasimThinkingBudgetFloor(t *testing.T) {
	// max_tokens 过小 → 无法满足 ≥1024 且 < max_tokens → 400。
	_, err := normalizeMirasimRelayThinkingBody(
		mirasimThinkingBody(t, `{"max_tokens":512,"output_config":{"effort":"low"}}`),
		"claude-legacy-budget", mirasimShapeBudget)
	var cfgErr *MirasimThinkingConfigError
	require.ErrorAs(t, err, &cfgErr)
	require.Equal(t, "mirasim_claude_budget_out_of_range", cfgErr.Code)
}

func TestMirasimThinkingUnknownShapeDefaultsToAdaptive(t *testing.T) {
	out, err := normalizeMirasimRelayThinkingBody(
		mirasimThinkingBody(t, `{"thinking":{"type":"enabled","budget_tokens":8192}}`),
		"claude-sonnet-5", mirasimShapeUnknown)
	require.NoError(t, err)
	require.Equal(t, "adaptive", gjson.GetBytes(out, "thinking.type").String())
}

func TestMirasimRosterAdaptiveParsing(t *testing.T) {
	raw := []byte(`{
		"version":"v1",
		"agents":{
			"claude":[{"id":"claude-sonnet-5","adaptive":true},{"id":"claude-legacy-budget","adaptive":false},{"id":"other-model","adaptive":true}],
			"kimi":[{"id":"kimi-code/k3","adaptive":true}]
		},
		"models":{
			"claude-haiku-4-5":{"adaptive":true},
			"claude-silent":{"label":"no adaptive field"},
			"deepseek-flash":{"adaptive":true}
		}
	}`)
	adaptive, err := parseMirasimRosterAdaptive(raw)
	require.NoError(t, err)

	require.Equal(t, mirasimShapeAdaptive, mirasimThinkingShapeForModel(adaptive, "claude-sonnet-5"))
	require.Equal(t, mirasimShapeBudget, mirasimThinkingShapeForModel(adaptive, "claude-legacy-budget"),
		"显式 adaptive=false 的模型必须走预算型")
	require.Equal(t, mirasimShapeAdaptive, mirasimShapeForRosterCheck(adaptive, "kimi-k3"),
		"kimi-code/k3 必须映射到 kimi-k3")
	require.Equal(t, mirasimShapeUnknown, mirasimThinkingShapeForModel(adaptive, "claude-silent"),
		"缺 adaptive 字段不算显式 false")
	require.Equal(t, mirasimShapeUnknown, mirasimThinkingShapeForModel(adaptive, "other-model"),
		"家族前缀不匹配的条目必须被丢弃")
	require.Equal(t, mirasimShapeAdaptive, mirasimThinkingShapeForModel(adaptive, "deepseek-flash"))
	require.Equal(t, mirasimShapeUnknown, mirasimThinkingShapeForModel(adaptive, "claude-brand-new"))
}

// mirasimShapeForRosterCheck 只是让上面断言读起来更直白的别名。
func mirasimShapeForRosterCheck(adaptive map[string]bool, model string) mirasimThinkingShape {
	return mirasimThinkingShapeForModel(adaptive, model)
}

func TestMirasimRosterRejectsDocumentsWithoutVersion(t *testing.T) {
	_, err := parseMirasimRosterAdaptive([]byte(`{"models":{}}`))
	require.Error(t, err)
}

func TestMirasimRosterSerializedShapeFlag(t *testing.T) {
	// 守卫：roster 的 adaptive 只允许布尔；字符串 "false" 不得被当成显式标记。
	adaptive, err := parseMirasimRosterAdaptive([]byte(`{"version":"v1","models":{"m-1":{"adaptive":"false"}}}`))
	require.NoError(t, err)
	require.Equal(t, mirasimShapeUnknown, mirasimThinkingShapeForModel(adaptive, "m-1"))
}

var _ = json.Marshal
