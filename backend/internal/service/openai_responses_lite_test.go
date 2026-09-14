package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIModelUsesResponsesLite(t *testing.T) {
	t.Parallel()
	require.True(t, openAIModelUsesResponsesLite("gpt-6-astra"))
	require.True(t, openAIModelUsesResponsesLite("gpt-5.6-sol"))
	require.True(t, openAIModelUsesResponsesLite("GPT-5.6-Terra"))
	require.True(t, openAIModelUsesResponsesLite("gpt-5.6-luna"))
	require.False(t, openAIModelUsesResponsesLite("gpt-5.5"))
	require.False(t, openAIModelUsesResponsesLite("gpt-5.4"))
	require.False(t, openAIModelUsesResponsesLite(""))
	require.False(t, openAIModelUsesResponsesLite("not-a-real-model"))
}

func TestShouldDemoteOpenAIResponsesLite(t *testing.T) {
	t.Parallel()
	require.False(t, shouldDemoteOpenAIResponsesLite(false, "gpt-5.5"))
	require.False(t, shouldDemoteOpenAIResponsesLite(true, "gpt-5.6-sol"))
	require.False(t, shouldDemoteOpenAIResponsesLite(true, "gpt-6-astra"))
	require.True(t, shouldDemoteOpenAIResponsesLite(true, "gpt-5.5"))
	require.True(t, shouldDemoteOpenAIResponsesLite(true, "gpt-5.4"))
}

func TestDemoteOpenAIResponsesLiteBody(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model":"gpt-5.6-sol",
		"instructions":"",
		"tools":[{"type":"function","name":"already"}],
		"reasoning":{"effort":"high","context":"all_turns"},
		"client_metadata":{
			"ws_request_header_x_openai_internal_codex_responses_lite":"true",
			"x-codex-window-id":"win-1"
		},
		"input":[
			{
				"type":"additional_tools",
				"role":"developer",
				"id":"at_1",
				"tools":[
					{"type":"namespace","name":"functions","tools":[{"type":"function","name":"exec"}]},
					{"type":"function","name":"wait"}
				]
			},
			{
				"type":"message",
				"role":"developer",
				"id":"msg_1",
				"content":[{"type":"input_text","text":"You are Codex."}],
				"internal_chat_message_metadata_passthrough":{
					"content_item_kinds":["model.base_instructions"]
				}
			},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
		]
	}`)

	out, changed, err := demoteOpenAIResponsesLiteBody(body)
	require.NoError(t, err)
	require.True(t, changed)

	require.Equal(t, "You are Codex.", gjson.GetBytes(out, "instructions").String())
	require.Equal(t, 3, int(gjson.GetBytes(out, "tools.#").Int()))
	require.Equal(t, "already", gjson.GetBytes(out, "tools.0.name").String())
	require.Equal(t, "namespace", gjson.GetBytes(out, "tools.1.type").String())
	require.Equal(t, "functions", gjson.GetBytes(out, "tools.1.name").String())
	require.Equal(t, "wait", gjson.GetBytes(out, "tools.2.name").String())

	require.Equal(t, 1, int(gjson.GetBytes(out, "input.#").Int()))
	require.Equal(t, "user", gjson.GetBytes(out, "input.0.role").String())
	require.False(t, gjson.GetBytes(out, "input.#(type==\"additional_tools\")").Exists())
	require.False(t, gjson.GetBytes(out, "reasoning.context").Exists())
	require.Equal(t, "high", gjson.GetBytes(out, "reasoning.effort").String())
	require.False(t, gjson.GetBytes(out, "client_metadata."+responsesLiteWSMetadataKey).Exists())
	require.Equal(t, "win-1", gjson.GetBytes(out, "client_metadata.x-codex-window-id").String())
}

func TestDemoteOpenAIResponsesLiteBodyNoop(t *testing.T) {
	t.Parallel()
	body := []byte(`{"model":"gpt-5.5","instructions":"hi","input":[{"type":"message","role":"user","content":"x"}]}`)
	out, changed, err := demoteOpenAIResponsesLiteBody(body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, out)
}

func TestApplyOpenAIResponsesLiteForUpstream(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	t.Run("demote lite to non-lite", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set(responsesLiteHeader, "true")

		body := []byte(`{
			"model":"gpt-5.6-sol",
			"reasoning":{"context":"all_turns"},
			"input":[{
				"type":"additional_tools",
				"tools":[{"type":"function","name":"exec"}]
			},{
				"type":"message",
				"role":"developer",
				"content":[{"type":"input_text","text":"base"}],
				"internal_chat_message_metadata_passthrough":{"content_item_kinds":["model.base_instructions"]}
			}]
		}`)
		out, demoted, err := applyOpenAIResponsesLiteForUpstream(c, body, "gpt-5.5")
		require.NoError(t, err)
		require.True(t, demoted)
		require.True(t, isOpenAIResponsesLiteDemoted(c))
		require.Equal(t, "base", gjson.GetBytes(out, "instructions").String())
		require.Equal(t, "exec", gjson.GetBytes(out, "tools.0.name").String())
		require.False(t, gjson.GetBytes(out, "reasoning.context").Exists())
	})

	t.Run("lite to lite keeps body", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set(responsesLiteHeader, "true")

		body := []byte(`{
			"model":"gpt-5.6-sol",
			"input":[{"type":"additional_tools","tools":[{"type":"function","name":"exec"}]}],
			"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true"}
		}`)
		out, demoted, err := applyOpenAIResponsesLiteForUpstream(c, body, "gpt-6-astra")
		require.NoError(t, err)
		require.False(t, demoted)
		require.False(t, isOpenAIResponsesLiteDemoted(c))
		require.True(t, gjson.GetBytes(out, `input.#(type=="additional_tools")`).Exists())
		require.True(t, isOpenAIResponsesLiteWebSocketPayload(out))
	})

	t.Run("external bridge force demotes lite even for astra", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set(responsesLiteHeader, "true")

		body := []byte(`{
			"model":"gpt-6-astra",
			"input":[
				{"type":"additional_tools","tools":[{"type":"function","name":"shell","parameters":{"type":"object"}}]},
				{"type":"message","role":"user","content":[{"type":"input_text","text":"ls"}]}
			]
		}`)
		out, demoted, err := applyOpenAIResponsesLiteForUpstream(c, body, "gpt-6-astra", true)
		require.NoError(t, err)
		require.True(t, demoted)
		require.True(t, isOpenAIResponsesLiteDemoted(c))
		require.Equal(t, "shell", gjson.GetBytes(out, "tools.0.name").String())
		require.False(t, gjson.GetBytes(out, `input.#(type=="additional_tools")`).Exists())
	})

	t.Run("non-lite inbound never promotes", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		body := []byte(`{"model":"gpt-5.5","instructions":"x","tools":[{"type":"function","name":"exec"}]}`)
		out, demoted, err := applyOpenAIResponsesLiteForUpstream(c, body, "gpt-6-astra")
		require.NoError(t, err)
		require.False(t, demoted)
		require.Equal(t, body, out)
	})
}
