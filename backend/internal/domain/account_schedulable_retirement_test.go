package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountIsSchedulableForModel_RejectsRetiredAnthropicOAuthSupply(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		typ      string
		want     bool
	}{
		{name: "anthropic oauth", platform: PlatformAnthropic, typ: AccountTypeOAuth, want: false},
		{name: "anthropic setup token", platform: PlatformAnthropic, typ: AccountTypeSetupToken, want: false},
		{name: "anthropic api key remains schedulable", platform: PlatformAnthropic, typ: AccountTypeAPIKey, want: true},
		{name: "antigravity setup token remains schedulable", platform: PlatformAntigravity, typ: AccountTypeSetupToken, want: true},
		{name: "openai oauth remains schedulable", platform: PlatformOpenAI, typ: AccountTypeOAuth, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    tt.platform,
				Type:        tt.typ,
				Status:      StatusActive,
				Schedulable: true,
			}
			require.Equal(t, tt.want, account.IsSchedulableForModel("model"))
		})
	}
}
