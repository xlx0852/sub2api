package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 本轮改动：mirasim relay 账号空 model_mapping 的护栏（生产事故 2026-09-25 账号 140）。
// 这些单测锁住三条关键约束：
//  1. relay 账号识别必须复用 isMirasimRelayAccount（不能靠 platform/type 猜）；
//  2. identity 映射必须规范化（去空白/去空串/去重），且**空清单返回空 map**，
//     让调用方能拒绝写入空映射（空映射＝认领所有模型＝422→502）；
//  3. 幂等比较必须是逐键严格比较。

func mirasimRelayTestAccount(credentials map[string]any) *Account {
	return &Account{
		ID:          140,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Credentials: credentials,
	}
}

func TestIsMirasimRelayAccount(t *testing.T) {
	refresh := map[string]any{mirasimCredentialRefreshToken: "rt"}

	t.Run("relay 账号（anthropic apikey + refresh token）", func(t *testing.T) {
		require.True(t, IsMirasimRelayAccount(mirasimRelayTestAccount(refresh)))
	})

	t.Run("缺少 refresh token 的普通 anthropic apikey 账号不算 relay", func(t *testing.T) {
		require.False(t, IsMirasimRelayAccount(mirasimRelayTestAccount(map[string]any{"api_key": "sk-x"})))
	})

	t.Run("kill switch 关闭后不算 relay", func(t *testing.T) {
		account := mirasimRelayTestAccount(refresh)
		account.Extra = map[string]any{mirasimExtraRelayEnabled: false}
		require.False(t, IsMirasimRelayAccount(account))
	})

	t.Run("nil 账号安全", func(t *testing.T) {
		require.False(t, IsMirasimRelayAccount(nil))
	})
}

func TestMirasimRelayAccountNeedsModelMapping(t *testing.T) {
	t.Run("relay 账号无映射 → 需要补", func(t *testing.T) {
		require.True(t, MirasimRelayAccountNeedsModelMapping(mirasimRelayTestAccount(map[string]any{
			mirasimCredentialRefreshToken: "rt",
		})))
	})

	t.Run("relay 账号有空映射 map → 仍视为需要补", func(t *testing.T) {
		require.True(t, MirasimRelayAccountNeedsModelMapping(mirasimRelayTestAccount(map[string]any{
			mirasimCredentialRefreshToken: "rt",
			"model_mapping":               map[string]any{},
		})))
	})

	t.Run("relay 账号已有映射 → 不需要补", func(t *testing.T) {
		require.False(t, MirasimRelayAccountNeedsModelMapping(mirasimRelayTestAccount(map[string]any{
			mirasimCredentialRefreshToken: "rt",
			"model_mapping":               map[string]any{"deepseek-flash": "deepseek-flash"},
		})))
	})

	t.Run("非 relay 账号永远不需要补（缺映射不是它的风险）", func(t *testing.T) {
		require.False(t, MirasimRelayAccountNeedsModelMapping(&Account{
			ID:       1,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
		}))
	})
}

func TestBuildMirasimIdentityModelMapping(t *testing.T) {
	t.Run("identity 且去重去空白丢弃空串", func(t *testing.T) {
		got := BuildMirasimIdentityModelMapping([]string{
			"deepseek-flash",
			" deepseek-flash ",
			"",
			"   ",
			"glm-5.3-flash",
		})
		require.Equal(t, map[string]string{
			"deepseek-flash": "deepseek-flash",
			"glm-5.3-flash":  "glm-5.3-flash",
		}, got)
	})

	t.Run("空/全空白清单 → 空 map（调用方必须据此拒绝写入）", func(t *testing.T) {
		require.Empty(t, BuildMirasimIdentityModelMapping(nil))
		require.Empty(t, BuildMirasimIdentityModelMapping([]string{}))
		require.Empty(t, BuildMirasimIdentityModelMapping([]string{"", "\t"}))
	})

	t.Run("值为 string（domain 层只接受 string 值，否则映射被判为空）", func(t *testing.T) {
		got := BuildMirasimIdentityModelMapping([]string{"kimi-k3"})
		value, ok := any(got["kimi-k3"]).(string)
		require.True(t, ok, "映射值必须是 string")
		require.Equal(t, "kimi-k3", value)
	})
}

func TestMirasimModelMappingEqual(t *testing.T) {
	identity := map[string]string{"a": "a", "b": "b"}

	require.True(t, MirasimModelMappingEqual(map[string]string{"a": "a", "b": "b"}, identity))
	require.False(t, MirasimModelMappingEqual(nil, identity), "空现值不等于目标")
	require.False(t, MirasimModelMappingEqual(map[string]string{"a": "a"}, identity), "键少")
	require.False(t, MirasimModelMappingEqual(map[string]string{"a": "a", "b": "b", "c": "c"}, identity), "键多")
	require.False(t, MirasimModelMappingEqual(map[string]string{"a": "a", "b": "c"}, identity), "值不同")
	require.True(t, MirasimModelMappingEqual(map[string]string{}, map[string]string{}), "双空相等")
}
