//go:build unit

package modelcatalog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 用真实 catalog 做底，验证注册表覆盖后的模型/别名/默认价，以及 ui_presets 不受影响。
func TestApplyRegistryOverlaysModelsAliasesAndPrices(t *testing.T) {
	base := Get()
	require.NotNil(t, base)
	presets := base.UIPresets

	snap := &RegistrySnapshot{
		Models: []RegistryModel{
			{ID: "custom-model-a", Platform: "openai", DisplayName: "Custom A", ContextWindow: 4096, Enabled: true, IsDefaultChat: true, IsDefaultTest: true},
			{ID: "custom-model-b", Platform: "openai", DisplayName: "Custom B", Enabled: true, UpstreamID: "custom-model-b-upstream"},
			{ID: "custom-retired", Platform: "openai", Retired: true},
			{ID: "price-only-id", Platform: "", Enabled: true},
		},
		Aliases: []RegistryAlias{
			{Alias: "alias-a", Platform: "openai", ModelID: "custom-model-a", Kind: "alias"},
			{Alias: "short-id", Platform: "anthropic", ModelID: "claude-sonnet-4-5-20250929", Kind: "id_override"},
		},
		DefaultPrices: map[string]PriceEntry{
			"custom-model-a": {Currency: "CNY", InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6},
			"price-only-id":  {InputCostPerToken: 5e-7},
		},
	}

	diffs, err := ApplyRegistry(snap)
	require.NoError(t, err)

	after := Get()
	openai := after.Platforms["openai"]
	require.Len(t, openai.Models, 2, "只剩注册表里 enabled 且非 retired 的 openai 模型")
	require.Equal(t, "custom-model-a", openai.Models[0].ID)
	require.Equal(t, "Custom A", openai.Models[0].DisplayName)
	require.Equal(t, 4096, openai.Models[0].ContextWindow)
	require.Equal(t, "custom-model-b-upstream", openai.DefaultMapping["custom-model-b"])
	require.Equal(t, "custom-model-a", openai.DefaultChatModel)
	require.Equal(t, "custom-model-a", openai.DefaultTestModel)
	require.Equal(t, []string{"custom-retired"}, openai.RetiredIDs)
	require.Equal(t, "custom-model-a", openai.Aliases["alias-a"])
	require.NotContains(t, openai.Aliases, "custom-retired", "retired id 不应伪装成别名")
	require.Equal(t, "claude-sonnet-4-5-20250929", after.Platforms["anthropic"].IDOverrides["short-id"])
	require.Equal(t, 2.0e-6, after.FallbackPricing["custom-model-a"].OutputCostPerToken)
	require.Contains(t, after.FallbackPricing, "price-only-id", "价格专用 id 也要保留默认价")
	require.Equal(t, presets, after.UIPresets, "ui_presets 保持文件值")

	// 差异里必须能看到"文件有、注册表没有"的模型
	joined := strings.Join(diffs, "\n")
	require.Contains(t, joined, "openai")

	// 恢复文件版，避免影响同包其它用例
	require.NoError(t, Load(nil))
}

func TestDiffRegistryReportsModelSetDifference(t *testing.T) {
	t.Cleanup(func() { _ = Load(nil) })
	snap := &RegistrySnapshot{
		Models: []RegistryModel{
			{ID: "only-in-registry", Platform: "kimi", Enabled: true},
		},
	}
	diffs, err := DiffRegistry(snap)
	require.NoError(t, err)
	joined := strings.Join(diffs, "\n")
	require.Contains(t, joined, "only-in-registry")
	require.Contains(t, joined, "kimi")

	// DiffRegistry 不能改动全局 catalog
	require.NotEmpty(t, Get().Platforms["kimi"].Models)
}

func TestDecodeMediaFlags(t *testing.T) {
	got := DecodeMediaFlags([]byte(`{"image": true, "video": false, "image_input": true}`))
	require.True(t, got["image"])
	require.True(t, got["image_input"])
	require.False(t, got["video"])
	require.Empty(t, DecodeMediaFlags(nil))
	require.Empty(t, DecodeMediaFlags([]byte(`not-json`)), "坏数据不应 panic，返回空能力集")
}
