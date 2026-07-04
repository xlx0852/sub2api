package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPatchGrokResponsesBodyStripsInstructionsWhenPreviousResponseIDPresent(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"instructions": "system prompt from codex",
		"previous_response_id": "resp_prev",
		"input": [{"role": "user", "content": "hello"}]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-composer-2.5-fast")
	require.NoError(t, err)
	require.Equal(t, "resp_prev", gjson.GetBytes(patched, "previous_response_id").String())
	require.False(t, gjson.GetBytes(patched, "instructions").Exists())
}

func TestPatchGrokResponsesBodyNormalizesCodexToolInputItems(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"input": [
			{"type": "item_reference", "id": "call_shell_1"},
			{"type": "local_shell_call", "call_id": "call_shell_1", "name": "shell", "input": "{\"command\":\"ls\"}"},
			{"type": "mcp_tool_call_output", "call_id": "call_mcp_1", "output": "listed files"},
			{"type": "reasoning", "id": "rs_1", "encrypted_content": "opaque", "summary": [{"type": "summary_text", "text": "thinking"}]},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "描述", "nonce": 123}]}
		]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-composer-2.5-fast")
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(patched, "input").Array(), 3)
	require.Equal(t, "function_call", gjson.GetBytes(patched, "input.0.type").String())
	require.Equal(t, "call_shell_1", gjson.GetBytes(patched, "input.0.call_id").String())
	require.Equal(t, "shell", gjson.GetBytes(patched, "input.0.name").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(patched, "input.1.type").String())
	require.Equal(t, "user", gjson.GetBytes(patched, "input.2.role").String())
	require.Equal(t, "描述", gjson.GetBytes(patched, "input.2.content").String())
	require.False(t, gjson.GetBytes(patched, "input.2.type").Exists())
}

func TestPatchGrokResponsesBodyStripsCodexCompactionItems(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"include": ["reasoning.encrypted_content", "output_text"],
		"input": [
			{"type": "compaction_summary", "id": "cmp_openai_1", "encrypted_content": "opaque-openai-blob"},
			{"type": "compaction", "id": "cmp_xai_1", "encrypted_content": "opaque-xai-blob"},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "你好"}]}
		]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-composer-2.5-fast")
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(patched, "input").Array(), 1)
	require.Equal(t, "user", gjson.GetBytes(patched, "input.0.role").String())
	require.Equal(t, "你好", gjson.GetBytes(patched, "input.0.content").String())
	require.False(t, gjson.GetBytes(patched, "input.0.type").Exists())
	require.Equal(t, "output_text", gjson.GetBytes(patched, "include.0").String())
	require.False(t, gjson.GetBytes(patched, "include.1").Exists())
}

func TestPatchGrokResponsesBodyRelocatesDeveloperMessagesToInstructions(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"instructions": "base prompt",
		"input": [
			{"type": "message", "role": "developer", "content": [{"type": "input_text", "text": "Be concise."}]},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "hello"}]}
		]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-composer-2.5-fast")
	require.NoError(t, err)
	require.Contains(t, gjson.GetBytes(patched, "instructions").String(), "base prompt")
	require.Contains(t, gjson.GetBytes(patched, "instructions").String(), "Be concise.")
	require.Len(t, gjson.GetBytes(patched, "input").Array(), 1)
	require.Equal(t, "hello", gjson.GetBytes(patched, "input.0.content").String())
}

func TestPatchGrokResponsesBodyDropsDeveloperMessagesWhenPreviousResponseIDPresent(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"previous_response_id": "resp_prev",
		"input": [
			{"type": "message", "role": "developer", "content": [{"type": "input_text", "text": "Be concise."}]},
			{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "Sure."}]},
			{"type": "local_shell_call", "call_id": "call_shell_1", "name": "shell", "input": "{\"command\":\"ls\"}"},
			{"type": "mcp_tool_call_output", "call_id": "call_mcp_1", "output": "listed files"},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "描述"}]}
		]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-composer-2.5-fast")
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(patched, "instructions").Exists())
	require.False(t, gjson.GetBytes(patched, "input.#(role=developer)").Exists())
	require.False(t, gjson.GetBytes(patched, "input.#(role=system)").Exists())
	require.False(t, gjson.GetBytes(patched, "input.#(role=assistant)").Exists())
	require.Len(t, gjson.GetBytes(patched, "input").Array(), 3)
	require.Equal(t, "function_call", gjson.GetBytes(patched, "input.0.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(patched, "input.1.type").String())
	require.Equal(t, "描述", gjson.GetBytes(patched, "input.2.content").String())
}

func TestPatchGrokResponsesBodyCollapsesReplayedHistoryForContinuation(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"previous_response_id": "resp_prev",
		"input": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "等哈建行卡"}]},
			{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "收到。"}]},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "你是什么模型"}]}
		]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-composer-2.5-fast")
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(patched, "input").Array(), 1)
	require.Equal(t, "user", gjson.GetBytes(patched, "input.0.role").String())
	require.Equal(t, "你是什么模型", gjson.GetBytes(patched, "input.0.content").String())
}

func TestPatchGrokResponsesBodyWrapsStandaloneInputTextItems(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"previous_response_id": "resp_prev",
		"input": [{"type": "input_text", "text": "continue"}]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-composer-2.5-fast")
	require.NoError(t, err)
	require.Equal(t, "user", gjson.GetBytes(patched, "input.0.role").String())
	require.Equal(t, "continue", gjson.GetBytes(patched, "input.0.content").String())
}

func TestPatchGrokResponsesBodyStripsReasoningForGrokComposer(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"reasoning": {"effort": "high", "summary": "detailed"},
		"reasoning_effort": "high",
		"reasoningEffort": "high",
		"input": [{"role": "user", "content": "hello"}]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-composer-2.5-fast")
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(patched, "reasoning").Exists())
	require.False(t, gjson.GetBytes(patched, "reasoning_effort").Exists())
	require.False(t, gjson.GetBytes(patched, "reasoningEffort").Exists())
}

func TestPatchGrokResponsesBodyPreservesReasoningForGrok43(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"reasoning": {"effort": "high", "summary": "detailed"},
		"input": [{"role": "user", "content": "hello"}]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-4.3")
	require.NoError(t, err)
	require.Equal(t, "high", gjson.GetBytes(patched, "reasoning.effort").String())
}

func TestPatchGrokResponsesBodyPreservesInputImagesForGrokComposer(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"input": [{
			"role": "user",
			"content": [
				{"type": "input_text", "text": "describe this"},
				{"type": "input_image", "image_url": "data:image/png;base64,abc"}
			]
		}]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-composer-2.5-fast")
	require.NoError(t, err)
	require.True(t, json.Valid(patched))
	require.Equal(t, "grok-composer-2.5-fast", gjson.GetBytes(patched, "model").String())
	require.Len(t, gjson.GetBytes(patched, "input.0.content").Array(), 2)
	require.Equal(t, "input_text", gjson.GetBytes(patched, "input.0.content.0.type").String())
	require.Equal(t, "input_image", gjson.GetBytes(patched, "input.0.content.1.type").String())
	require.Equal(t, "data:image/png;base64,abc", gjson.GetBytes(patched, "input.0.content.1.image_url").String())
}

func TestPatchGrokResponsesBodyNormalizesInputImageDetailAuto(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"input": [{
			"role": "user",
			"content": [
				{"type": "input_image", "image_url": "data:image/png;base64,abc"}
			]
		}]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-4.3")
	require.NoError(t, err)
	require.Equal(t, "auto", gjson.GetBytes(patched, "input.0.content.0.detail").String())
}

func TestPatchGrokResponsesBodyFlattensNestedImageURLObject(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"input": [{
			"role": "user",
			"content": [
				{"type": "image_url", "image_url": {"url": "https://example.com/a.png", "detail": "high"}}
			]
		}]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-4.3")
	require.NoError(t, err)
	require.Equal(t, "input_image", gjson.GetBytes(patched, "input.0.content.0.type").String())
	require.Equal(t, "https://example.com/a.png", gjson.GetBytes(patched, "input.0.content.0.image_url").String())
	require.Equal(t, "high", gjson.GetBytes(patched, "input.0.content.0.detail").String())
}

func TestPatchGrokResponsesBodyRewritesFunctionCallOutputImages(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"input": [{
			"type": "function_call_output",
			"call_id": "call_img_1",
			"output": [
				{"type": "input_text", "text": "screenshot attached"},
				{"type": "input_image", "image_url": "data:image/png;base64,abc"}
			]
		}]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-4.3")
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(patched, "input").Array(), 2)
	require.Equal(t, "function_call_output", gjson.GetBytes(patched, "input.0.type").String())
	require.Equal(t, "screenshot attached", gjson.GetBytes(patched, "input.0.output").String())
	require.Equal(t, "user", gjson.GetBytes(patched, "input.1.role").String())
	require.Equal(t, "input_image", gjson.GetBytes(patched, "input.1.content.1.type").String())
	require.Equal(t, "auto", gjson.GetBytes(patched, "input.1.content.1.detail").String())
	require.Contains(t, gjson.GetBytes(patched, "input.1.content.0.text").String(), "call_img_1")
}

func TestPatchGrokResponsesBodyPreservesInputImagesForGrokBuild(t *testing.T) {
	body := []byte(`{
		"model": "grok",
		"input": [{
			"role": "user",
			"content": [
				{"type": "input_text", "text": "describe this"},
				{"type": "input_image", "image_url": "data:image/png;base64,abc"}
			]
		}]
	}`)

	patched, err := patchGrokResponsesBody(body, "grok-build")
	require.NoError(t, err)
	require.True(t, json.Valid(patched))
	require.Equal(t, "grok-build", gjson.GetBytes(patched, "model").String())
	require.Len(t, gjson.GetBytes(patched, "input.0.content").Array(), 2)
	require.Equal(t, "input_text", gjson.GetBytes(patched, "input.0.content.0.type").String())
	require.Equal(t, "input_image", gjson.GetBytes(patched, "input.0.content.1.type").String())
	require.Equal(t, "data:image/png;base64,abc", gjson.GetBytes(patched, "input.0.content.1.image_url").String())
}

func TestForwardGrokResponsesPreprocessesComposerImages(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestBody := []byte(`{
		"model": "grok-composer-2.5-fast",
		"stream": false,
		"input": [{
			"role": "user",
			"content": [
				{"type": "input_text", "text": "explain the screenshot"},
				{"type": "input_image", "image_url": "data:image/png;base64,abc"}
			]
		}]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"vision-rid"}},
			Body: io.NopCloser(strings.NewReader(`{
				"id": "resp_vision",
				"model": "grok-build",
				"output": [{"type":"message","content":[{"type":"output_text","text":"The image shows a registration form with an invalid invitation code error."}]}],
				"usage": {"input_tokens": 10, "output_tokens": 5}
			}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"final-rid"}},
			Body: io.NopCloser(strings.NewReader(`{
				"id": "resp_final",
				"model": "grok-composer-2.5-fast",
				"output": [{"type":"message","content":[{"type":"output_text","text":"done"}]}],
				"usage": {"input_tokens": 20, "output_tokens": 7}
			}`)),
		},
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:          88,
		Name:        "grok-oauth",
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
		},
	}

	result, err := svc.forwardGrokResponses(context.Background(), c, account, requestBody, "grok-composer-2.5-fast", false, time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 2, len(upstream.bodies))
	require.Equal(t, "grok-build", upstream.requests[0].Header.Get("x-grok-model-override"))
	require.Equal(t, "grok-pager", upstream.requests[0].Header.Get("x-grok-client-identifier"))
	require.Contains(t, upstream.requests[0].Header.Get("User-Agent"), "grok-pager/")
	require.Equal(t, "grok-build", gjson.GetBytes(upstream.bodies[0], "model").String())
	require.Contains(t, string(upstream.bodies[0]), `"input_image"`)
	require.Equal(t, "data:image/png;base64,abc", gjson.GetBytes(upstream.bodies[0], "input.0.content.1.image_url").String())
	require.NotContains(t, gjson.GetBytes(upstream.bodies[0], "input.0.content.0.text").String(), "data:image/png;base64,abc")

	finalBody := upstream.bodies[1]
	require.Equal(t, "grok-composer-2.5-fast", upstream.requests[1].Header.Get("x-grok-model-override"))
	require.Equal(t, "grok-composer-2.5-fast", gjson.GetBytes(finalBody, "model").String())
	require.False(t, strings.Contains(string(finalBody), `"input_image"`))
	require.False(t, strings.Contains(string(finalBody), `"image_url"`))
	require.Contains(t, gjson.GetBytes(finalBody, "input.0.content").String(), "invalid invitation code error")
	require.Equal(t, 30, result.Usage.InputTokens)
	require.Equal(t, 12, result.Usage.OutputTokens)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "done")
}
