//go:build unit

package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountIsOpenAILongContextBillingEnabled(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "nil account", account: nil, want: false},
		{name: "non OpenAI account", account: &Account{Platform: PlatformAnthropic}, want: false},
		{name: "OpenAI missing extra", account: &Account{Platform: PlatformOpenAI}, want: true},
		{
			name: "OpenAI missing setting",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{"other": true},
			},
			want: true,
		},
		{
			name: "OpenAI invalid setting defaults enabled",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{openAILongContextBillingEnabledKey: "false"},
			},
			want: true,
		},
		{
			name: "OpenAI explicitly enabled",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{openAILongContextBillingEnabledKey: true},
			},
			want: true,
		},
		{
			name: "OpenAI explicitly disabled",
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{openAILongContextBillingEnabledKey: false},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsOpenAILongContextBillingEnabled())
		})
	}
}
