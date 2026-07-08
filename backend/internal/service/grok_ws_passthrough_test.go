package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAccountResolveGrokXAIWSPassthroughMode(t *testing.T) {
	account := &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}
	require.Equal(t, GrokXAIWSPassthroughModeOff, account.ResolveGrokXAIWSPassthroughMode(""))
	require.Equal(t, GrokXAIWSPassthroughModeForce, account.ResolveGrokXAIWSPassthroughMode(GrokXAIWSPassthroughModeForce))

	account.Extra = map[string]any{"grok_xai_ws_passthrough_enabled": true}
	require.Equal(t, GrokXAIWSPassthroughModeAuto, account.ResolveGrokXAIWSPassthroughMode(GrokXAIWSPassthroughModeOff))

	account.Extra = map[string]any{"grok_xai_ws_passthrough_mode": "force"}
	require.Equal(t, GrokXAIWSPassthroughModeForce, account.ResolveGrokXAIWSPassthroughMode(GrokXAIWSPassthroughModeOff))
}

func TestBuildGrokResponsesWebSocketURL(t *testing.T) {
	t.Run("api.x.ai", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformGrok,
			Credentials: map[string]any{"base_url": "https://api.x.ai/v1/"},
		}
		wsURL, err := buildGrokResponsesWebSocketURL(account)
		require.NoError(t, err)
		require.Equal(t, "wss://api.x.ai/v1/responses", wsURL)
	})

	t.Run("cli-chat-proxy maps to api.x.ai", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformGrok,
			Type:        AccountTypeOAuth,
			Credentials: map[string]any{"base_url": "https://cli-chat-proxy.grok.com/v1"},
		}
		wsURL, err := buildGrokResponsesWebSocketURL(account)
		require.NoError(t, err)
		require.Equal(t, "wss://api.x.ai/v1/responses", wsURL)
	})
}

func TestPrepareGrokWebSocketUpstreamPayloadStripsReasoningEffort(t *testing.T) {
	account := &Account{
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"base_url": "https://cli-chat-proxy.grok.com/v1"},
	}
	payload := []byte(`{"model":"gpt-5.4-mini","stream":true,"reasoningEffort":"high","reasoning":{"effort":"high"},"input":[{"type":"input_text","text":"hi"}]}`)
	updated, err := prepareGrokWebSocketUpstreamPayload(payload, "grok-composer-2.5-fast", account, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "response.create", gjson.GetBytes(updated, "type").String())
	require.Equal(t, "grok-composer-2.5-fast", gjson.GetBytes(updated, "model").String())
	require.False(t, gjson.GetBytes(updated, "reasoningEffort").Exists())
	require.False(t, gjson.GetBytes(updated, "reasoning").Exists())
	require.False(t, gjson.GetBytes(updated, "stream").Exists())
}

func TestShouldUseGrokHTTPBridgeIDChain(t *testing.T) {
	require.False(t, shouldUseGrokHTTPBridgeIDChain(&Account{Platform: PlatformGrok, Type: AccountTypeOAuth}))
	require.False(t, shouldUseGrokHTTPBridgeIDChain(&Account{Platform: PlatformGrok, Type: AccountTypeAPIKey}))
	require.False(t, shouldUseGrokHTTPBridgeIDChain(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
}

func TestIsGrokWSAutoDialFallbackEligible(t *testing.T) {
	require.True(t, isGrokWSAutoDialFallbackEligible(&openAIWSDialError{StatusCode: 405, Err: errors.New("expected 101 but got 405")}))
	require.True(t, isGrokWSAutoDialFallbackEligible(&openAIWSDialError{StatusCode: 502, Err: errors.New("bad gateway")}))
	require.False(t, isGrokWSAutoDialFallbackEligible(&openAIWSDialError{StatusCode: 401, Err: errors.New("unauthorized")}))
	require.False(t, isGrokWSAutoDialFallbackEligible(&openAIWSDialError{StatusCode: 429, Err: errors.New("rate limited")}))
}

func TestBuildGrokResponsesWebSocketPayload(t *testing.T) {
	oauthAccount := &Account{
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"base_url": "https://cli-chat-proxy.grok.com/v1"},
	}
	apiKeyAccount := &Account{
		Platform:    PlatformGrok,
		Credentials: map[string]any{"base_url": "https://cli-chat-proxy.grok.com/v1"},
	}

	t.Run("native chain forces store for oauth inference api", func(t *testing.T) {
		payload := []byte(`{"model":"grok-composer-2.5-fast","store":false,"stream":true,"instructions":"old","previous_response_id":"resp_1","input":[{"type":"input_text","text":"continue"}]}`)
		updated, err := buildGrokResponsesWebSocketPayload(payload, oauthAccount)
		require.NoError(t, err)
		require.Equal(t, "response.create", gjson.GetBytes(updated, "type").String())
		require.True(t, gjson.GetBytes(updated, "store").Bool())
		require.Equal(t, "resp_1", gjson.GetBytes(updated, "previous_response_id").String())
		require.False(t, gjson.GetBytes(updated, "instructions").Exists())
	})

	t.Run("non-native preserves explicit store false", func(t *testing.T) {
		payload := []byte(`{"model":"grok-composer-2.5-fast","store":false,"stream":true,"background":false,"stream_options":{"include_usage":true}}`)
		updated, err := buildGrokResponsesWebSocketPayload(payload, apiKeyAccount)
		require.NoError(t, err)
		require.False(t, gjson.GetBytes(updated, "store").Bool())
		require.False(t, gjson.GetBytes(updated, "stream").Exists())
		require.False(t, gjson.GetBytes(updated, "stream_options").Exists())
		require.False(t, gjson.GetBytes(updated, "background").Exists())
	})
}

func TestPrepareGrokWebSocketUpstreamPayloadCollapsesContinuationInput(t *testing.T) {
	account := &Account{
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"base_url": "https://cli-chat-proxy.grok.com/v1"},
	}
	payload := []byte(`{
		"model":"grok-composer-2.5-fast",
		"store":false,
		"previous_response_id":"resp_1",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"hello"}]},
			{"role":"assistant","content":[{"type":"output_text","text":"hi"}]},
			{"role":"user","content":[{"type":"input_text","text":"continue"}]}
		]
	}`)
	updated, err := prepareGrokWebSocketUpstreamPayload(payload, "grok-composer-2.5-fast", account, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "resp_1", gjson.GetBytes(updated, "previous_response_id").String())
	require.True(t, gjson.GetBytes(updated, "store").Bool())
	require.Len(t, gjson.GetBytes(updated, "input").Array(), 1)
	require.Equal(t, "continue", gjson.GetBytes(updated, "input.0.content").String())
}

func TestCollapseGrokWSIngressInputWithPreviousResponseID(t *testing.T) {
	payload := []byte(`{
		"model":"grok-composer-2.5-fast",
		"previous_response_id":"resp_1",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"hello"}]},
			{"role":"assistant","content":[{"type":"output_text","text":"hi"}]},
			{"role":"user","content":[{"type":"input_text","text":"continue"}]}
		]
	}`)
	collapsed, changed, err := collapseGrokWSIngressInput(payload, 15*1024*1024)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, gjson.GetBytes(collapsed, "input").Array(), 1)
	require.Equal(t, "continue", gjson.GetBytes(collapsed, "input.0.content.0.text").String())
	require.Equal(t, "previous_response_id", grokWSIngressCollapseReason(payload, 15*1024*1024))
}

func TestCollapseGrokWSIngressInputLargePayloadWithoutPreviousResponseID(t *testing.T) {
	largeText := strings.Repeat("x", 200)
	payload := []byte(fmt.Sprintf(`{
		"model":"grok-composer-2.5-fast",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"%s"}]},
			{"role":"assistant","content":[{"type":"output_text","text":"ok"}]},
			{"role":"user","content":[{"type":"input_text","text":"next"}]}
		]
	}`, largeText))
	collapsed, changed, err := collapseGrokWSIngressInput(payload, 128)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, gjson.GetBytes(collapsed, "input").Array(), 1)
	require.Equal(t, "next", gjson.GetBytes(collapsed, "input.0.content.0.text").String())
	require.Equal(t, "large_payload", grokWSIngressCollapseReason(payload, 128))
}

func TestCollapseGrokWSIngressInputKeepsAllToolPairsSinceLastUser(t *testing.T) {
	payload := []byte(`{
		"model":"grok-composer-2.5-fast",
		"previous_response_id":"resp_1",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"old task"}]},
			{"role":"assistant","content":[{"type":"output_text","text":"working"}]},
			{"role":"user","content":[{"type":"input_text","text":"重启一下前后端"}]},
			{"type":"local_shell_call","call_id":"call_1","name":"shell","input":{"command":"ls"}},
			{"type":"mcp_tool_call_output","call_id":"call_1","output":"README.md"},
			{"type":"local_shell_call","call_id":"call_2","name":"shell","input":{"command":"cat README.md"}},
			{"type":"mcp_tool_call_output","call_id":"call_2","output":"make dev"}
		]
	}`)
	collapsed, changed, err := collapseGrokWSIngressInput(payload, 15*1024*1024)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, gjson.GetBytes(collapsed, "input").Array(), 5)
	require.Equal(t, "重启一下前后端", gjson.GetBytes(collapsed, "input.0.content.0.text").String())
	require.Equal(t, "call_2", gjson.GetBytes(collapsed, "input.3.call_id").String())
}

func TestCollapseGrokWSIngressInputKeepsUserRequestOnToolContinuation(t *testing.T) {
	payload := []byte(`{
		"model":"grok-composer-2.5-fast",
		"previous_response_id":"resp_1",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"重启一下前后端"}]},
			{"role":"assistant","content":[{"type":"output_text","text":"我先确认项目启动方式"}]},
			{"type":"local_shell_call","call_id":"call_1","name":"shell","input":{"command":"ls"}},
			{"type":"mcp_tool_call_output","call_id":"call_1","output":"README.md"}
		]
	}`)
	collapsed, changed, err := collapseGrokWSIngressInput(payload, 15*1024*1024)
	require.NoError(t, err)
	require.True(t, changed)
	items := gjson.GetBytes(collapsed, "input").Array()
	require.Len(t, items, 3)
	require.Equal(t, "重启一下前后端", gjson.GetBytes(collapsed, "input.0.content.0.text").String())
	require.Equal(t, "local_shell_call", gjson.GetBytes(collapsed, "input.1.type").String())
	require.Equal(t, "mcp_tool_call_output", gjson.GetBytes(collapsed, "input.2.type").String())
}

func TestCollapseGrokWSIngressInputSkipsSmallSingleItem(t *testing.T) {
	payload := []byte(`{"model":"grok-composer-2.5-fast","input":[{"role":"user","content":"hello"}]}`)
	collapsed, changed, err := collapseGrokWSIngressInput(payload, 15*1024*1024)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, payload, collapsed)
}

func TestShouldUseGrokWSNativeResponseChain(t *testing.T) {
	require.True(t, shouldUseGrokWSNativeResponseChain(&Account{
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"base_url": "https://cli-chat-proxy.grok.com/v1"},
	}))
	require.False(t, shouldUseGrokWSNativeResponseChain(&Account{
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://cli-chat-proxy.grok.com/v1"},
	}))
}

func TestBuildGrokResponsesWebSocketHeadersUsesPromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)

	t.Run("oauth cli-chat-proxy uses minimal api.x.ai headers", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformGrok,
			Type:        AccountTypeOAuth,
			Credentials: map[string]any{"base_url": "https://cli-chat-proxy.grok.com/v1"},
		}
		headers := buildGrokResponsesWebSocketHeaders(c, account, "tok", []byte(`{"model":"grok-composer-2.5-fast","prompt_cache_key":"session-1"}`), "")

		require.Equal(t, "Bearer tok", headers.Get("Authorization"))
		require.Equal(t, "application/json", headers.Get("Accept"))
		require.Empty(t, headers.Get("x-grok-conv-id"))
		require.Empty(t, headers.Get("x-grok-client-version"))
		require.Empty(t, headers.Get("x-grok-model-override"))
	})

	t.Run("api key cli-chat-proxy keeps grok cli headers", func(t *testing.T) {
		account := &Account{Platform: PlatformGrok, Credentials: map[string]any{"base_url": "https://cli-chat-proxy.grok.com/v1"}}
		headers := buildGrokResponsesWebSocketHeaders(c, account, "tok", []byte(`{"model":"grok-composer-2.5-fast","prompt_cache_key":"session-1"}`), "")

		require.Equal(t, "Bearer tok", headers.Get("Authorization"))
		require.Equal(t, "session-1", headers.Get("x-grok-conv-id"))
		require.NotEmpty(t, headers.Get("x-grok-client-version"))
		require.Equal(t, "grok-composer-2.5-fast", headers.Get("x-grok-model-override"))
	})
}

func TestShouldBridgeGrokWSPayloadForImageInput(t *testing.T) {
	require.True(t, shouldBridgeGrokWSPayload([]byte(`{"input":[{"type":"input_image","image_url":"data:image/png;base64,abc"}]}`)))
	require.True(t, shouldBridgeGrokWSPayload([]byte(`{"input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,abc"}]}]}`)))
	require.True(t, shouldBridgeGrokWSPayload([]byte(`{"input":[{"role":"user","content":[{"type":"image","mimeType":"image/png","data":"abc"}]}]}`)))
	require.False(t, shouldBridgeGrokWSPayload([]byte(`{"input":[{"type":"function_call_output","call_id":"call_1","output":[{"type":"image","mimeType":"image/png","data":"abc"}]}]}`)))
	require.False(t, shouldBridgeGrokWSPayload([]byte(`{"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)))
}

func TestShouldPreprocessGrokComposerImagesAtWSIngress(t *testing.T) {
	oauthAccount := &Account{
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"base_url": "https://cli-chat-proxy.grok.com/v1"},
	}
	payload := []byte(`{"model":"grok-composer-2.5-fast","input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,abc"}]}]}`)

	require.True(t, shouldPreprocessGrokComposerImagesAtWSIngress(oauthAccount, "grok-composer-2.5-fast", payload))
	require.False(t, shouldPreprocessGrokComposerImagesAtWSIngress(oauthAccount, "grok-4.3", payload))

	toolTurn := []byte(`{"model":"grok-composer-2.5-fast","previous_response_id":"resp_prev","input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,old"}]},{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	require.False(t, shouldPreprocessGrokComposerImagesAtWSIngress(oauthAccount, "grok-composer-2.5-fast", toolTurn))
	require.False(t, shouldPreprocessGrokComposerImagesAtWSIngress(&Account{Platform: PlatformGrok}, "grok-composer-2.5-fast", payload))
}
