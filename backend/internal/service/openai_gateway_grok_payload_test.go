package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOptimizeGrokResponsesPayloadDeduplicatesOnlyExactHistoricalDataImages(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model":"grok-4.5",
		"store":true,
		"input":[
			{"type":"message","role":"user","content":[
				{"type":"input_image","image_url":"data:image/png;base64,SAME"},
				{"type":"input_image","image_url":"https://example.com/image.png"},
				{"type":"input_image","image_url":"data:image/png;base64,OTHER"}
			]},
			{"type":"message","role":"assistant","content":[
				{"type":"input_image","image_url":"data:image/png;base64,SAME"},
				{"type":"input_image","image_url":"https://example.com/image.png"}
			]},
			{"type":"message","role":"user","content":[
				{"type":"input_text","text":"current"},
				{"type":"input_image","image_url":"data:image/png;base64,SAME"},
				{"type":"input_image","image_url":"data:image/png;base64,SAME"}
			]}
		]
	}`)

	got, err := optimizeGrokPayload(body, grokPayloadResponses, config.GrokPayloadConfig{
		DeduplicateImages:    true,
		DisableStoreOnImages: true,
	})
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(got, "store").Bool())
	require.Equal(t, 1, strings.Count(string(got), "data:image/png;base64,OTHER"))
	require.Equal(t, 2, strings.Count(string(got), "https://example.com/image.png"))
	require.Equal(t, 2, strings.Count(string(got), "data:image/png;base64,SAME"), "current input images must not be deduplicated")
	require.Equal(t, "current", gjson.GetBytes(got, "input.2.content.0.text").String())
}

func TestOptimizeGrokChatPayloadSetsStoreFalseForImage(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"grok","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,AA"}}]}]}`)
	got, err := optimizeGrokPayload(body, grokPayloadChat, config.GrokPayloadConfig{
		DisableStoreOnImages: true,
	})
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(got, "store").Bool())
}

func TestOptimizeGrokPayloadTruncatesHistoricalToolOutputUTF8Safely(t *testing.T) {
	t.Parallel()

	oldOutput := strings.Repeat("历史输出🙂", 40)
	latestOutput := strings.Repeat("latest🙂", 20)
	payload := map[string]any{
		"model": "grok",
		"input": []any{
			map[string]any{"type": "function_call", "call_id": "call-old", "name": "exec", "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": "call-old", "output": oldOutput},
			map[string]any{"type": "function_call_output", "call_id": "call-latest", "output": latestOutput},
			map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "continue"}}},
		},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	got, err := optimizeGrokPayload(body, grokPayloadResponses, config.GrokPayloadConfig{
		SoftLimitBytes:     1,
		ToolOutputMaxBytes: 96,
	})
	require.NoError(t, err)
	historical := gjson.GetBytes(got, "input.1.output").String()
	require.LessOrEqual(t, len(historical), 96)
	require.True(t, utf8.ValidString(historical))
	require.Contains(t, historical, grokToolOutputTruncationMarker)
	require.Equal(t, "call-old", gjson.GetBytes(got, "input.1.call_id").String())
	require.Equal(t, latestOutput, gjson.GetBytes(got, "input.2.output").String())
	require.Equal(t, "call-latest", gjson.GetBytes(got, "input.2.call_id").String())
}

func TestOptimizeGrokPayloadDoesNotTruncateBelowSoftLimit(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[{"type":"function_call_output","call_id":"old","output":"abcdefghijklmnopqrstuvwxyz"},{"type":"message","role":"user","content":"next"}]}`)
	got, err := optimizeGrokPayload(body, grokPayloadResponses, config.GrokPayloadConfig{
		SoftLimitBytes:     int64(len(body) + 1),
		ToolOutputMaxBytes: 10,
	})
	require.NoError(t, err)
	require.JSONEq(t, string(body), string(got))
}

func TestOptimizeGrokPayloadRejectsUnsafeHardLimitOverflow(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"grok","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"current input must remain intact"}]}]}`)
	_, err := optimizeGrokPayload(body, grokPayloadResponses, config.GrokPayloadConfig{HardLimitBytes: 32})
	var tooLarge *grokPayloadTooLargeError
	require.ErrorAs(t, err, &tooLarge)
	require.Equal(t, int64(32), tooLarge.Limit)
}

func TestOpenAIGatewayOptimizeGrokPayloadLeavesNonGrokUnchanged(t *testing.T) {
	t.Parallel()

	body := []byte(`{"store":true,"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA"}}]}]}`)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		GrokPayload: config.GrokPayloadConfig{
			DeduplicateImages:    true,
			DisableStoreOnImages: true,
			HardLimitBytes:       1,
		},
	}}}
	got, err := svc.optimizeGrokPayload(&Account{Platform: PlatformOpenAI}, body, grokPayloadChat)
	require.NoError(t, err)
	require.Equal(t, body, got)
}

func TestOptimizeGrokPayloadBeforeImageBridgeDefersHardLimitCheck(t *testing.T) {
	t.Parallel()

	body := []byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA"}}]}]}`)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		GrokPayload: config.GrokPayloadConfig{
			DisableStoreOnImages: true,
			HardLimitBytes:       1,
		},
	}}}
	account := &Account{Platform: PlatformGrok}

	got, err := svc.optimizeGrokPayloadBeforeImageBridge(account, body)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(got, "store").Bool())

	_, err = svc.optimizeGrokPayload(account, got, grokPayloadChat)
	var tooLarge *grokPayloadTooLargeError
	require.ErrorAs(t, err, &tooLarge)
}

func TestWriteGrokPayloadTooLargeUsesProtocolErrorShape(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	err := &grokPayloadTooLargeError{Size: 100, Limit: 50}

	t.Run("responses", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		writeGrokPayloadTooLarge(ctx, grokPayloadResponses, err)
		require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
		require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.code").String())
	})
	t.Run("chat", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		writeGrokPayloadTooLarge(ctx, grokPayloadChat, err)
		require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
		require.Equal(t, "invalid_request_error", gjson.Get(recorder.Body.String(), "error.type").String())
	})
}
