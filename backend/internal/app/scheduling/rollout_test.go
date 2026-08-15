package scheduling

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRolloutConfigResolveUsesGroupPlatformDefaultPrecedence(t *testing.T) {
	groupID := int64(25)
	config := ParseRolloutConfig(`{
		"default":"off",
		"platforms":{"openai":"shadow"},
		"groups":{"openai":{"25":"enforce"}}
	}`)

	require.Equal(t, RolloutEnforce, config.Resolve(" OpenAI ", &groupID))
	require.Equal(t, RolloutShadow, config.Resolve("openai", nil))
	require.Equal(t, RolloutOff, config.Resolve("grok", &groupID))
}

func TestRolloutConfigInvalidInputAndModeFailClosed(t *testing.T) {
	require.Equal(t, RolloutOff, ParseRolloutConfig("not-json").Resolve("openai", nil))
	require.Equal(t, RolloutOff, ParseRolloutConfig(`{"default":"unexpected"}`).Resolve("openai", nil))
}

func TestRolloutConfigValidateRejectsInvalidScopeAndCloneIsIndependent(t *testing.T) {
	valid := ParseRolloutConfig(`{"default":"off","platforms":{"openai":"shadow"},"groups":{"openai":{"25":"enforce"}}}`)
	require.NoError(t, valid.Validate())
	cloned := valid.Clone()
	cloned.Platforms["openai"] = RolloutOff
	cloned.Groups["openai"]["25"] = RolloutOff
	require.Equal(t, RolloutShadow, valid.Platforms["openai"])
	require.Equal(t, RolloutEnforce, valid.Groups["openai"]["25"])

	require.Error(t, (RolloutConfig{Default: "invalid"}).Validate())
	require.Error(t, (RolloutConfig{Default: RolloutOff, Groups: map[string]map[string]RolloutMode{"openai": {"bad": RolloutShadow}}}).Validate())
}
