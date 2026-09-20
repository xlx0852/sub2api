package service

import (
	"context"
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

// Select through the new profile, then assert the actual upstream request. The
// scheduler must never strip tool pairing or continuation state to move a turn.
func TestSimpleSelection_HTTPAndSSEPreserveToolContinuation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			a := simpleTestAccount(132, AccountTypeAPIKey)
			a.Credentials["api_key"] = "synthetic-test-key"
			a.Extra = map[string]any{"openai_passthrough": true, "openai_responses_mode": "force_responses"}
			svc := simpleTestService([]Account{a}, schedulerTestConcurrencyCache{})
			svc.cfg.Gateway.OpenAIWS.Enabled = false
			require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(context.Background(), 4, "resp_original", 132, time.Hour))
			sel, decision, err := svc.SelectAccountWithScheduler(context.Background(), int64PtrForTest(4), "resp_original", "thread", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.True(t, decision.StickyPreviousHit)
			defer sel.ReleaseFunc()
			response := `{"id":"resp_next","object":"response","status":"completed","model":"gpt-6-astra","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`
			contentType := "application/json"
			if stream {
				response = "data: {\"type\":\"response.completed\",\"response\":" + response + "}\n\ndata: [DONE]\n\n"
				contentType = "text/event-stream"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(response))}}
			svc.httpUpstream = upstream
			streamValue := "false"
			if stream {
				streamValue = "true"
			}
			body := []byte(`{"model":"gpt-6-astra","stream":` + streamValue + `,"previous_response_id":"resp_original","input":[{"type":"function_call","id":"fc_keep","call_id":"call_keep","name":"test","arguments":"{}"},{"type":"function_call_output","call_id":"call_keep","output":"result"}]}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(string(body)))
			result, err := svc.Forward(context.Background(), c, sel.Account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "resp_original", gjson.GetBytes(upstream.lastBody, "previous_response_id").String())
			require.Equal(t, "call_keep", gjson.GetBytes(upstream.lastBody, "input.0.call_id").String())
			require.Equal(t, "call_keep", gjson.GetBytes(upstream.lastBody, "input.1.call_id").String())
			require.Equal(t, "result", gjson.GetBytes(upstream.lastBody, "input.1.output").String())
		})
	}
}

func TestSimpleSelection_WSHTTPBridgePreservesCompleteToolHistory(t *testing.T) {
	a := simpleTestAccount(132, AccountTypeOAuth)
	a.Credentials["access_token"] = "synthetic-test-token"
	a.Credentials["chatgpt_account_id"] = "synthetic-account"
	a.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": "http_bridge"}
	svc := simpleTestService([]Account{a}, schedulerTestConcurrencyCache{})
	svc.cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	sel, _, err := svc.SelectAccountWithScheduler(context.Background(), int64PtrForTest(4), "", "new", "gpt-6-astra", nil, OpenAIUpstreamTransportResponsesWebsocketV2Ingress, false)
	require.NoError(t, err)
	defer sel.ReleaseFunc()
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_done\",\"status\":\"completed\",\"model\":\"gpt-6-astra\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\ndata: [DONE]\n\n"))}}
	svc.httpUpstream = upstream
	payload := []byte(`{"type":"response.create","model":"gpt-6-astra","input":[{"type":"function_call","call_id":"call_keep","name":"test","arguments":"{}"},{"type":"function_call_output","call_id":"call_keep","output":"result"}]}`)
	var events [][]byte
	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), nil, sel.Account, "synthetic-test-token", payload, len(payload), "gpt-6-astra", "", "", "", 1, func(b []byte) error { events = append(events, append([]byte(nil), b...)); return nil })
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, events)
	require.Equal(t, "call_keep", gjson.GetBytes(upstream.lastBody, "input.0.call_id").String())
	require.Equal(t, "call_keep", gjson.GetBytes(upstream.lastBody, "input.1.call_id").String())
	require.Equal(t, "result", gjson.GetBytes(upstream.lastBody, "input.1.output").String())
}
