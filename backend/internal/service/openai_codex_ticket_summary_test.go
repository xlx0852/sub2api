package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testTicketValue = "gAAAAABqq_20UDF7SECRETTICKETVALUE" + "0123456789"
	testCookieValue = "SECRETCOOKIEVALUE"
)

func newTicketSummaryAccount(now time.Time) *Account {
	tokKey, modelKey, capturedKey := openAICodexTurnStatePoolKeys("gpt-5.6-luna")
	astraKey, _, astraCaptured := openAICodexTurnStatePoolKeys("gpt-6-astra")
	return &Account{
		ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Extra: map[string]any{
			tokKey:        testTicketValue,
			modelKey:      "gpt-5.6-luna",
			capturedKey:   now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
			astraKey:      testTicketValue,
			astraCaptured: now.Add(-90 * time.Minute).Format(time.RFC3339Nano),
			openAICodexTurnStateCookieExtra: encodeCodexTurnStateCookies([]codexTurnStateCookie{
				{Name: "__oailb", Value: testCookieValue, SetAt: now.Add(-2 * time.Minute).Unix(), Expires: now.Add(time.Hour).Unix()},
			}),
			openAICodexTurnStateCookieEgressExtra: "5ee7a93798f061c8",
			"codex_fingerprint_mode":              "session",
		},
	}
}

func TestBuildCodexTicketSummaryDerivesWithoutLeaking(t *testing.T) {
	now := time.Now()
	acc := newTicketSummaryAccount(now)

	sum := BuildCodexTicketSummary(acc, now)
	require.NotNil(t, sum)
	require.Len(t, sum.Pools, 2)

	byPool := map[string]int{}
	for _, p := range sum.Pools {
		byPool[p.Pool] = p.Shape
	}
	// 形态 = 票长；两个池都指向同一张测试票，所以长度相同。
	require.Equal(t, len(testTicketValue), byPool["gpt-5.6-luna"])
	require.Equal(t, len(testTicketValue), byPool["gpt-6-astra"])

	for _, p := range sum.Pools {
		switch p.Pool {
		case "gpt-5.6-luna":
			require.True(t, p.Fresh, "10 分钟票应仍在回放窗口内")
			require.Equal(t, int64(600), p.AgeSeconds)
			require.Equal(t, "gpt-5.6-luna", p.Model)
		case "gpt-6-astra":
			require.False(t, p.Fresh, "90 分钟票已超过 60 分钟回放窗口")
		}
	}
	require.NotNil(t, sum.Cookies)
	require.Equal(t, 1, sum.Cookies.Count)
	require.True(t, sum.Cookies.Fresh)
	require.Equal(t, "5ee7a93798f061c8", sum.Cookies.Egress)
	require.Equal(t, int64(3600), sum.ReplayTTLSeconds)

	// 最关键的一条：摘要里绝不能出现票或 cookie 的值。
	raw, err := json.Marshal(sum)
	require.NoError(t, err)
	require.NotContains(t, string(raw), testTicketValue)
	require.NotContains(t, string(raw), testCookieValue)
	require.NotContains(t, string(raw), "gAAAAA")
}

func TestRedactCodexTicketExtraDropsOnlyTicketMaterial(t *testing.T) {
	tokKey, _, capturedKey := openAICodexTurnStatePoolKeys("gpt-5.6-luna")
	in := map[string]any{
		openAICodexTurnStateExtraToken:        testTicketValue,
		tokKey:                                testTicketValue,
		capturedKey:                           "2026-09-22T15:00:00Z",
		openAICodexTurnStateCookieExtra:       `[{"n":"__oailb","v":"` + testCookieValue + `"}]`,
		openAICodexTurnStateCookieEgressExtra: "5ee7a93798f061c8",
		"codex_fingerprint_mode":              "session",
		"codex_overload_identity_rotation":    "on",
	}

	out := RedactCodexTicketExtra(in)

	for _, key := range []string{
		openAICodexTurnStateExtraToken, tokKey, capturedKey,
		openAICodexTurnStateCookieExtra, openAICodexTurnStateCookieEgressExtra,
	} {
		_, present := out[key]
		require.False(t, present, "敏感键 %s 必须被剥离", key)
	}
	// 其它 extra 键必须原样保留：前端大量设置项都从这里读。
	require.Equal(t, "session", out["codex_fingerprint_mode"])
	require.Equal(t, "on", out["codex_overload_identity_rotation"])
	// 不能修改入参（原 map 仍被调度器使用）。
	require.Contains(t, in, openAICodexTurnStateExtraToken)
	require.True(t, strings.HasPrefix(in[tokKey].(string), "gAAAAA"))
}

// 只有兼容单值槽位、没有池槽位时，摘要要回退到单值，且不重复计数。
func TestBuildCodexTicketSummaryFallsBackToLegacySlot(t *testing.T) {
	now := time.Now()
	acc := &Account{
		ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Extra: map[string]any{
			openAICodexTurnStateExtraToken:      testTicketValue,
			openAICodexTurnStateExtraModel:      "gpt-5.5",
			openAICodexTurnStateExtraCapturedAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
		},
	}
	sum := BuildCodexTicketSummary(acc, now)
	require.NotNil(t, sum)
	require.Len(t, sum.Pools, 1)
	require.Equal(t, "(legacy)", sum.Pools[0].Pool)
	require.Equal(t, "gpt-5.5", sum.Pools[0].Model)
	require.True(t, sum.Pools[0].Fresh)
	require.Nil(t, sum.Cookies)
}

// 非 OAuth 账号（API-key 等）不涉及票，摘要应为 nil，避免前端出现空面板。
func TestBuildCodexTicketSummarySkipsNonOAuth(t *testing.T) {
	require.Nil(t, BuildCodexTicketSummary(&Account{ID: 1, Type: AccountTypeAPIKey, Extra: map[string]any{openAICodexTurnStateExtraToken: testTicketValue}}, time.Now()))
	require.Nil(t, BuildCodexTicketSummary(nil, time.Now()))
}

// 失效清理会把票值写成 null 但保留键。那种状态必须与"该池从未采到票"区分开，
// 否则面板会把"刚被清理、正在等待重采"报成"从未打过票"。
func TestBuildCodexTicketSummaryReportsClearedSlots(t *testing.T) {
	now := time.Now()
	clearedKey, modelKey, capturedKey := openAICodexTurnStatePoolKeys("gpt-5.6-luna")
	acc := &Account{
		ID: 9, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Extra: map[string]any{
			clearedKey:  nil, // 被清理：键在、值为 null
			modelKey:    "gpt-5.6-luna",
			capturedKey: nil,
		},
	}
	sum := BuildCodexTicketSummary(acc, now)
	require.NotNil(t, sum, "有槽位就要出摘要，否则面板无法区分'已清理'与'从没有过'")
	require.Len(t, sum.Pools, 1)
	require.Equal(t, "gpt-5.6-luna", sum.Pools[0].Pool)
	require.Zero(t, sum.Pools[0].Shape, "已清理的槽位形态为 0")
	require.False(t, sum.Pools[0].Fresh)
}
