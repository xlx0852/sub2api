package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// mirasim 空映射标记是**管理端可见性**的一部分，属于事故护栏（2026-09-25 账号 140）。
//
// 关键约束（改动前请先读）：
//   - 该标记必须由服务层派生（IsMirasimRelayAccount + GetModelMapping），
//     不允许 handler/前端自行猜「空映射 = 有问题」：普通 anthropic apikey 账号
//     的空映射是正常配置，误报会诱导管理员乱写映射。
//   - 标记只暴露布尔值，响应里绝不能出现 mirasim_refresh_token 的值。
func TestAccountFromServiceShallow_MirasimModelMappingMissingFlag(t *testing.T) {
	t.Run("relay 账号空映射被标记", func(t *testing.T) {
		src := mirasimRelayFixture()
		got := AccountFromServiceShallow(src)
		require.NotNil(t, got)
		require.True(t, got.MirasimModelMappingMissing)

		// 凭据就地脱敏：值不再出现在响应里，只留存在性标记
		require.NotContains(t, got.Credentials, "mirasim_refresh_token")
		require.True(t, got.CredentialsStatus["has_mirasim_refresh_token"])
		raw, err := json.Marshal(got)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "rt-mirasim-secret")
		require.Contains(t, string(raw), "mirasim_model_mapping_missing")
	})

	t.Run("relay 账号已有映射不再标记", func(t *testing.T) {
		src := mirasimRelayFixture()
		src.Credentials["model_mapping"] = map[string]any{
			"kimi-k3":        "kimi-k3",
			"deepseek-flash": "deepseek-flash",
		}
		got := AccountFromServiceShallow(src)
		require.False(t, got.MirasimModelMappingMissing)
	})

	t.Run("kill switch 关闭时不标记", func(t *testing.T) {
		src := mirasimRelayFixture()
		src.Extra = map[string]any{"mirasim_relay_enabled": false}
		got := AccountFromServiceShallow(src)
		require.False(t, got.MirasimModelMappingMissing)
	})

	t.Run("普通 anthropic apikey 空映射不标记", func(t *testing.T) {
		src := mirasimRelayFixture()
		delete(src.Credentials, "mirasim_refresh_token")
		src.Credentials["api_key"] = "sk-plain"
		got := AccountFromServiceShallow(src)
		require.False(t, got.MirasimModelMappingMissing)
	})

	t.Run("其它平台不标记", func(t *testing.T) {
		src := mirasimRelayFixture()
		src.Platform = "openai"
		got := AccountFromServiceShallow(src)
		require.False(t, got.MirasimModelMappingMissing)
	})

	t.Run("详情映射继承该标记", func(t *testing.T) {
		got := AccountFromService(mirasimRelayFixture())
		require.NotNil(t, got)
		require.True(t, got.MirasimModelMappingMissing)
	})
}

// 字段在列表与详情里都必须**无条件**序列化（不带 omitempty），
// 否则前端只能靠 `undefined` 猜，一旦后端升级顺序错位就会静默漏提示。
func TestAccountFromServiceShallow_MirasimFlagSerializedWhenFalse(t *testing.T) {
	src := mirasimRelayFixture()
	src.Credentials["model_mapping"] = map[string]any{"kimi-k3": "kimi-k3"}

	raw, err := json.Marshal(AccountFromServiceShallow(src))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"mirasim_model_mapping_missing":false`)
}

func mirasimRelayFixture() *service.Account {
	return &service.Account{
		ID:       140,
		Name:     "mirasim-relay",
		Platform: "anthropic",
		Type:     "apikey",
		Credentials: map[string]any{
			"mirasim_refresh_token": "rt-mirasim-secret",
			"mirasim_device_key":    "pem-mirasim-secret",
			"base_url":              "https://relay.example.com",
		},
		Extra:  map[string]any{"mirasim_relay_enabled": true},
		Status: "active",
	}
}
