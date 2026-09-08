package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAntigravityResponsesContinuationExpiry(t *testing.T) {
	svc, acc, up := agResponsesFixture(agTextStream)
	w, err := agResponsesCall(t, svc, acc, `{"model":"gemini-3.8-flash","input":"hi"}`, 1)
	require.NoError(t, err)
	id := gjson.GetBytes(w.Body.Bytes(), "id").String()
	accountID, ok := svc.ResponsesContinuationAccount(id, 1, 10)
	require.True(t, ok)
	require.Equal(t, acc.ID, accountID)
	_, ok = svc.ResponsesContinuationAccount(id, 1, 11)
	require.False(t, ok)
	svc.responsesStore.mu.Lock()
	entry := svc.responsesStore.entries[id]
	entry.expires = time.Now().Add(-time.Second)
	svc.responsesStore.entries[id] = entry
	svc.responsesStore.mu.Unlock()
	_, ok = svc.ResponsesContinuationAccount(id, 1, 10)
	require.False(t, ok)
	next, _ := json.Marshal(map[string]any{"model": "gemini-3.8-flash", "input": "continue", "previous_response_id": id})
	w, err = agResponsesCall(t, svc, acc, string(next), 1)
	require.Error(t, err)
	require.Equal(t, 400, w.Code)
	require.Len(t, up.requestBodies, 1)
}

func agResponsesFixture(chunks ...string) (*AntigravityGatewayService, *Account, *queuedHTTPUpstreamStub) {
	upstream := &queuedHTTPUpstreamStub{}
	for _, chunk := range chunks {
		upstream.responses = append(upstream.responses, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(chunk))})
	}
	svc := &AntigravityGatewayService{settingService: NewSettingService(&antigravitySettingRepoStub{}, &config.Config{}), tokenProvider: &AntigravityTokenProvider{}, httpUpstream: upstream}
	account := &Account{ID: 110, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Status: StatusActive, Concurrency: 1, Credentials: map[string]any{"access_token": "test", "project_id": "project", "model_mapping": map[string]any{"gemini-3.8-flash": "gemini-3.8-flash"}}}
	return svc, account, upstream
}

func agResponsesCall(t *testing.T, svc *AntigravityGatewayService, account *Account, body string, key int64) (*httptest.ResponseRecorder, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
	_, err := svc.ForwardResponses(context.Background(), c, account, []byte(body), key, 10, "session")
	return w, err
}

const agTextStream = "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":10,\"cachedContentTokenCount\":4,\"candidatesTokenCount\":2,\"thoughtsTokenCount\":3}}}\n\n"

func TestAntigravityResponsesJSONAndSSE(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			svc, acc, up := agResponsesFixture(agTextStream)
			body, _ := json.Marshal(map[string]any{"model": "gemini-3.8-flash", "input": "hi", "instructions": "be concise", "stream": stream})
			w, err := agResponsesCall(t, svc, acc, string(body), 1)
			require.NoError(t, err)
			require.Equal(t, 200, w.Code)
			require.Equal(t, "project", gjson.GetBytes(up.requestBodies[0], "project").String())
			require.Equal(t, "hi", gjson.GetBytes(up.requestBodies[0], "request.contents.0.parts.0.text").String())
			if stream {
				require.Contains(t, w.Body.String(), "response.output_text.delta")
				require.Contains(t, w.Body.String(), "response.completed")
			} else {
				require.Equal(t, "response", gjson.GetBytes(w.Body.Bytes(), "object").String())
				require.Equal(t, int64(10), gjson.GetBytes(w.Body.Bytes(), "usage.input_tokens").Int())
				require.Equal(t, int64(5), gjson.GetBytes(w.Body.Bytes(), "usage.output_tokens").Int())
				require.Equal(t, "hello", gjson.GetBytes(w.Body.Bytes(), "output.0.content.0.text").String())
			}
		})
	}
}

func TestAntigravityResponsesToolContinuation(t *testing.T) {
	toolStream := "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"read_file\",\"args\":{\"path\":\"a.txt\"}},\"thoughtSignature\":\"native-signature\"}]},\"finishReason\":\"STOP\"}]}}\n\n"
	svc, acc, up := agResponsesFixture(toolStream, agTextStream)
	w, err := agResponsesCall(t, svc, acc, `{"model":"gemini-3.8-flash","input":"read file","tools":[{"type":"function","name":"read_file","parameters":{"type":"object","properties":{"path":{"type":"string"}}}}]}`, 1)
	require.NoError(t, err)
	id := gjson.GetBytes(w.Body.Bytes(), "id").String()
	callID := gjson.GetBytes(w.Body.Bytes(), "output.0.call_id").String()
	require.NotEmpty(t, callID)
	next, _ := json.Marshal(map[string]any{"model": "gemini-3.8-flash", "previous_response_id": id, "input": []any{map[string]any{"type": "function_call_output", "call_id": callID, "output": "file contents"}}})
	w, err = agResponsesCall(t, svc, acc, string(next), 1)
	require.NoError(t, err)
	require.Equal(t, "read_file", gjson.GetBytes(up.requestBodies[1], "request.contents.2.parts.0.functionResponse.name").String())
	require.Equal(t, "native-signature", gjson.GetBytes(up.requestBodies[1], "request.contents.1.parts.0.thoughtSignature").String())
	_, err = agResponsesCall(t, svc, acc, string(next), 2)
	require.Error(t, err)
	require.Len(t, up.requestBodies, 2)
}

func TestAntigravityResponsesCustomTool(t *testing.T) {
	stream := "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"apply_patch\",\"args\":{\"input\":\"*** Begin Patch\\n*** End Patch\"}}}]},\"finishReason\":\"STOP\"}]}}\n\n"
	svc, acc, up := agResponsesFixture(stream)
	w, err := agResponsesCall(t, svc, acc, `{"model":"gemini-3.8-flash","input":"edit","stream":true,"tools":[{"type":"custom","name":"apply_patch"}]}`, 1)
	require.NoError(t, err)
	require.Equal(t, "apply_patch", gjson.GetBytes(up.requestBodies[0], "request.tools.0.functionDeclarations.0.name").String())
	require.Contains(t, w.Body.String(), "custom_tool_call")
	require.Contains(t, w.Body.String(), "response.custom_tool_call_input.done")
	require.Contains(t, w.Body.String(), "*** Begin Patch")
}

func TestAntigravityResponsesTruncatedStreamFails(t *testing.T) {
	svc, acc, _ := agResponsesFixture("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}}\n\n")
	w, err := agResponsesCall(t, svc, acc, `{"model":"gemini-3.8-flash","input":"hi","stream":true}`, 1)
	require.Error(t, err)
	require.Contains(t, w.Body.String(), "response.failed")
	require.NotContains(t, w.Body.String(), "response.completed")
}

func TestAntigravityResponsesRejectsOrphanAndImages(t *testing.T) {
	for _, input := range []string{`[{"type":"function_call_output","call_id":"missing","output":"hi"}]`, `[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}]`} {
		req := &apicompat.ResponsesRequest{Model: "gemini-3.8-flash", Input: json.RawMessage(input)}
		_, _, err := convertAntigravityResponsesRequest(req, nil)
		require.Error(t, err)
	}
}

func TestAntigravityResponsesFullHistoryCustomTool(t *testing.T) {
	var req apicompat.ResponsesRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.8-flash","instructions":"instructions","max_output_tokens":128,"reasoning":{"effort":"high","summary":"auto"},"tools":[{"type":"custom","name":"apply_patch"}],"input":[{"role":"user","content":"edit"},{"type":"custom_tool_call","call_id":"call_1","name":"apply_patch","input":"*** Begin Patch"},{"type":"custom_tool_call_output","call_id":"call_1","output":"success"}]}`), &req))
	body, custom, err := convertAntigravityResponsesRequest(&req, nil)
	require.NoError(t, err)
	require.True(t, custom["apply_patch"])
	require.Equal(t, "*** Begin Patch", gjson.GetBytes(body, "contents.1.parts.0.functionCall.args.input").String())
	require.Equal(t, "success", gjson.GetBytes(body, "contents.2.parts.0.functionResponse.response.output").String())
	require.Equal(t, "high", gjson.GetBytes(body, "generationConfig.thinkingConfig.thinkingLevel").String())
	svc, acc, up := agResponsesFixture(agTextStream)
	_, err = agResponsesCall(t, svc, acc, `{"model":"gemini-3.8-flash","input":"hi","max_output_tokens":128,"store":false}`, 1)
	require.NoError(t, err)
	require.Equal(t, int64(128), gjson.GetBytes(up.requestBodies[0], "request.generationConfig.maxOutputTokens").Int())
	require.Empty(t, svc.responsesStore.entries)
}

func TestAntigravityResponsesNamespaceAndIncomplete(t *testing.T) {
	stream := "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"functions__read\",\"args\":{}}}]},\"finishReason\":\"STOP\"}]}}\n\n"
	svc, acc, _ := agResponsesFixture(stream)
	w, err := agResponsesCall(t, svc, acc, `{"model":"gemini-3.8-flash","input":"read","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"read","parameters":{"type":"object","properties":{}}}]}]}`, 1)
	require.NoError(t, err)
	require.Equal(t, "functions", gjson.GetBytes(w.Body.Bytes(), "output.0.namespace").String())
	require.Equal(t, "read", gjson.GetBytes(w.Body.Bytes(), "output.0.name").String())
	svc, acc, _ = agResponsesFixture(strings.ReplaceAll(agTextStream, "STOP", "MAX_TOKENS"))
	w, err = agResponsesCall(t, svc, acc, `{"model":"gemini-3.8-flash","input":"hi","stream":true}`, 1)
	require.NoError(t, err)
	require.Contains(t, w.Body.String(), "response.incomplete")
	require.NotContains(t, w.Body.String(), "response.completed")
}

func TestAntigravityResponsesToolWithConstSchema(t *testing.T) {
	svc, acc, up := agResponsesFixture(agTextStream)
	body := `{
		"model": "gemini-3.8-flash",
		"input": "test tool with const",
		"tools": [
			{
				"type": "function",
				"name": "search_files",
				"parameters": {
					"type": "object",
					"properties": {
						"filter": {
							"type": "object",
							"properties": {
								"kind": {
									"description": "the kind of target",
									"const": "file"
								}
							}
						}
					}
				}
			}
		]
	}`
	w, err := agResponsesCall(t, svc, acc, body, 1)
	require.NoError(t, err)
	require.Equal(t, 200, w.Code)
	upstreamReq := string(up.requestBodies[0])
	require.NotContains(t, upstreamReq, `"const"`)
	require.Contains(t, upstreamReq, `"enum":["file"]`)
}

func TestAntigravityResponsesClampsLargeMaxOutputTokens(t *testing.T) {
	svc, acc, up := agResponsesFixture(agTextStream)
	body := `{
		"model": "gemini-3.8-flash",
		"input": "hi",
		"max_output_tokens": 128000,
		"temperature": 2.5,
		"top_p": 1.5
	}`
	w, err := agResponsesCall(t, svc, acc, body, 1)
	require.NoError(t, err)
	require.Equal(t, 200, w.Code)
	require.Equal(t, int64(65536), gjson.GetBytes(up.requestBodies[0], "request.generationConfig.maxOutputTokens").Int())
	require.Equal(t, 2.0, gjson.GetBytes(up.requestBodies[0], "request.generationConfig.temperature").Float())
	require.Equal(t, 1.0, gjson.GetBytes(up.requestBodies[0], "request.generationConfig.topP").Float())
}


