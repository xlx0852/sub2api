//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestAccountGrokUpstreamRouting(t *testing.T) {
	tests := []struct {
		name         string
		account      *Account
		wantUsingAPI bool
		wantChat     string
		wantOfficial string
	}{
		{
			name: "oauth defaults to cli chat and official media",
			account: &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeOAuth,
			},
			wantChat:     xai.DefaultCLIBaseURL,
			wantOfficial: xai.DefaultBaseURL,
		},
		{
			name: "oauth persisted official base is honored for chat",
			account: &Account{
				Platform:    PlatformGrok,
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"base_url": xai.DefaultBaseURL},
			},
			wantUsingAPI: true,
			wantChat:     xai.DefaultBaseURL,
			wantOfficial: xai.DefaultBaseURL,
		},
		{
			name: "oauth regional api host is honored",
			account: &Account{
				Platform:    PlatformGrok,
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"base_url": "https://us-east-1.api.x.ai/v1"},
			},
			wantUsingAPI: true,
			wantChat:     "https://us-east-1.api.x.ai/v1",
			wantOfficial: "https://us-east-1.api.x.ai/v1",
		},
		{
			name: "boolean using api selects official",
			account: &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"base_url":  xai.DefaultBaseURL,
					"using_api": true,
				},
			},
			wantUsingAPI: true,
			wantChat:     xai.DefaultBaseURL,
			wantOfficial: xai.DefaultBaseURL,
		},
		{
			name: "string using api rewrites legacy cli base to official",
			account: &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"base_url":  xai.DefaultCLIBaseURL,
					"using_api": "true",
				},
			},
			wantUsingAPI: true,
			wantChat:     xai.DefaultBaseURL,
			wantOfficial: xai.DefaultBaseURL,
		},
		{
			name: "using_api false keeps explicit cli host",
			account: &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"base_url":  xai.DefaultCLIBaseURL,
					"using_api": false,
				},
			},
			wantUsingAPI: false,
			wantChat:     xai.DefaultCLIBaseURL,
			wantOfficial: xai.DefaultBaseURL,
		},
		{
			name: "oauth custom gateway is preserved",
			account: &Account{
				Platform:    PlatformGrok,
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"base_url": "https://grok.example.test/v1/"},
			},
			wantChat:     "https://grok.example.test/v1",
			wantOfficial: "https://grok.example.test/v1",
		},
		{
			name: "api key defaults to official",
			account: &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeAPIKey,
			},
			wantUsingAPI: true,
			wantChat:     xai.DefaultBaseURL,
			wantOfficial: xai.DefaultBaseURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantUsingAPI, tt.account.GrokUsesOfficialAPI())
			require.Equal(t, tt.wantChat, tt.account.GetGrokChatBaseURL())
			require.Equal(t, tt.wantOfficial, tt.account.GetGrokOfficialBaseURL())
		})
	}
}

func TestBuildOpenAIResponsesWSURLRejectsGrok(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"base_url": xai.DefaultCLIBaseURL},
	}

	got, err := svc.buildOpenAIResponsesWSURL(account)
	require.Error(t, err)
	require.Empty(t, got)
	require.Contains(t, err.Error(), "not supported")
}
