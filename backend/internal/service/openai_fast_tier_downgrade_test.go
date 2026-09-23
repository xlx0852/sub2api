//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// OpenAI 在 ramp rate 限流下会把 Fast mode 请求降级为标准速度并按标准价计费，
// 此时响应携带 service_tier: "default"。计费必须跟随上游报告的实际档位。
func TestExtractOpenAIUpstreamServiceTier_JSONShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"responses response object", `{"type":"response.completed","response":{"service_tier":"default"}}`, "default"},
		{"responses priority", `{"response":{"service_tier":"priority"}}`, "priority"},
		{"chat completions chunk", `{"service_tier":"default","choices":[]}`, "default"},
		{"wrapped payload", `{"data":{"service_tier":"flex"}}`, "flex"},
		{"fast alias normalizes to priority", `{"response":{"service_tier":"fast"}}`, "priority"},
		{"absent", `{"usage":{"input_tokens":1}}`, ""},
		{"unknown value ignored", `{"response":{"service_tier":"turbo"}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractOpenAIUpstreamServiceTier([]byte(tc.body))
			if tc.want == "" {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, tc.want, *got)
		})
	}
}

func TestExtractOpenAIUpstreamServiceTier_SSEFrames(t *testing.T) {
	// response.created 也会带 service_tier，但只有终止帧反映实际服务的档位，
	// 因此 SSE 形态必须取最后一个帧的取值（降级场景下即 default）。
	body := "event: response.created\n" +
		`data: {"type":"response.created","response":{"service_tier":"priority"}}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","response":{"service_tier":"default"}}` + "\n\n" +
		"data: [DONE]\n"

	got := extractOpenAIUpstreamServiceTier([]byte(body))
	require.NotNil(t, got)
	require.Equal(t, "default", *got)
}

// 流式热路径只在终止事件上抓取，避免每帧都做 gjson 路径查找。
func TestCaptureOpenAIUpstreamServiceTier_TerminalEventsOnly(t *testing.T) {
	terminal := []string{"response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled"}
	for _, eventType := range terminal {
		body := `{"type":"` + eventType + `","response":{"service_tier":"default"}}`
		got := captureOpenAIUpstreamServiceTier([]byte(body))
		require.NotNilf(t, got, "%s must be captured", eventType)
		require.Equal(t, "default", *got)
	}

	nonTerminal := []string{"response.created", "response.in_progress", "response.output_text.delta"}
	for _, eventType := range nonTerminal {
		body := `{"type":"` + eventType + `","response":{"service_tier":"default"}}`
		require.Nilf(t, captureOpenAIUpstreamServiceTier([]byte(body)), "%s must not be captured", eventType)
	}
}

func TestBilledServiceTierPrefersReportedTier(t *testing.T) {
	requested := ptrString("priority")

	// 降级：上游报告 default → 按标准价计费，而不是请求的 priority。
	downgraded := ptrString("default")
	require.Equal(t, "default", *billedServiceTier(requested, downgraded))

	// 上游确认：按上游档位计费。
	honored := ptrString("priority")
	require.Equal(t, "priority", *billedServiceTier(requested, honored))

	// 上游未报告：回退请求档位，保持既有行为。
	require.Equal(t, "priority", *billedServiceTier(requested, nil))
	require.Nil(t, billedServiceTier(nil, nil))
}

// 端到端口径：被降级的 Fast 请求按标准价结算。
func TestCalculateCostWithServiceTier_DowngradedFastBillsStandard(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:         50_000,
		OutputTokens:        50_000,
		CacheCreationTokens: 50_000,
		CacheReadTokens:     50_000,
	}

	std, err := svc.CalculateCostWithServiceTier("gpt-6-sol", tokens, 1.0, "")
	require.NoError(t, err)
	fast, err := svc.CalculateCostWithServiceTier("gpt-6-sol", tokens, 1.0, "priority")
	require.NoError(t, err)
	require.NotEqual(t, std.TotalCost, fast.TotalCost, "priority must differ from standard")

	// 上游报告 default 时 billedServiceTier 产出 "default"，计费必须等于标准档。
	billed := billedServiceTier(ptrString("priority"), ptrString("default"))
	downgraded, err := svc.CalculateCostWithServiceTier("gpt-6-sol", tokens, 1.0, *billed)
	require.NoError(t, err)
	require.InDelta(t, std.TotalCost, downgraded.TotalCost, 1e-12, "downgraded fast request must bill standard")
}
