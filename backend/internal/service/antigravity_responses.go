package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const antigravityResponsesContextKey = "antigravity.responses"

type antigravityResponseHistory struct {
	scope    string
	model    string
	contents []antigravity.GeminiContent
	expires  time.Time
	bytes    int
}

// Explicitly bounded process-local continuation storage. Missing/expired IDs
// fail closed; callers can always replay their full input with store=false.
type antigravityResponseStore struct {
	mu      sync.Mutex
	entries map[string]antigravityResponseHistory
	bytes   int
}

func (s *AntigravityGatewayService) ResponsesContinuationAccount(id string, apiKeyID, groupID int64) (int64, bool) {
	s.responsesStore.mu.Lock()
	defer s.responsesStore.mu.Unlock()
	entry, ok := s.responsesStore.entries[id]
	prefix := fmt.Sprintf("%d:%d:", apiKeyID, groupID)
	if !ok || !strings.HasPrefix(entry.scope, prefix) || time.Now().After(entry.expires) {
		return 0, false
	}
	account, err := strconv.ParseInt(strings.TrimPrefix(entry.scope, prefix), 10, 64)
	return account, err == nil && account > 0
}

func (s *antigravityResponseStore) get(id, scope, model string) ([]antigravity.GeminiContent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.entries[id]
	if !ok || v.scope != scope || v.model != model || time.Now().After(v.expires) {
		return nil, false
	}
	return append([]antigravity.GeminiContent(nil), v.contents...), true
}

func (s *antigravityResponseStore) put(id, scope, model string, contents []antigravity.GeminiContent) {
	raw, err := json.Marshal(contents)
	if err != nil || len(raw) > 8<<20 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]antigravityResponseHistory)
	}
	for key, v := range s.entries {
		if time.Now().After(v.expires) {
			delete(s.entries, key)
			s.bytes -= v.bytes
		}
	}
	for len(s.entries) >= 128 || s.bytes+len(raw) > 64<<20 {
		oldest := ""
		var expiry time.Time
		for key, v := range s.entries {
			if oldest == "" || v.expires.Before(expiry) {
				oldest = key
				expiry = v.expires
			}
		}
		if oldest == "" {
			break
		}
		s.bytes -= s.entries[oldest].bytes
		delete(s.entries, oldest)
	}
	s.entries[id] = antigravityResponseHistory{scope: scope, model: model, contents: contents, expires: time.Now().Add(30 * time.Minute), bytes: len(raw)}
	s.bytes += len(raw)
}

type antigravityResponsesState struct {
	request     apicompat.ResponsesRequest
	customTools map[string]bool
	scope       string
	contents    []antigravity.GeminiContent
}

func (s *AntigravityGatewayService) ForwardResponses(ctx context.Context, c *gin.Context, account *Account, body []byte, apiKeyID int64, groupID int64, session string) (*ForwardResult, error) {
	c.Set(antigravityResponsesContextKey, true)
	path := strings.TrimRight(c.Request.URL.Path, "/")
	if path != "/v1/responses" && path != "/responses" {
		return nil, s.writeGoogleError(c, 400, "Unsupported Antigravity Responses operation")
	}
	var req apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, s.writeGoogleError(c, 400, "Invalid Responses request")
	}
	for _, key := range []string{"background", "conversation", "context_management"} {
		v := gjson.GetBytes(body, key)
		if v.Exists() && v.Type != gjson.Null && v.Raw != "false" {
			return nil, s.writeGoogleError(c, 400, key+" is not supported for Antigravity Responses")
		}
	}
	if apiKeyID <= 0 || account == nil {
		return nil, s.writeGoogleError(c, 400, "Missing Responses owner or account")
	}
	mapped := s.getMappedModel(account, req.Model)
	effort := ""
	if req.Reasoning != nil {
		effort = req.Reasoning.Effort
	}
	mapped = domain.ApplyAntigravityEffortModel(mapped, effort)
	if !s.IsModelSupported(mapped) {
		return nil, s.writeGoogleError(c, 400, "Unsupported Antigravity model: text Gemini models only")
	}
	scope := fmt.Sprintf("%d:%d:%d", apiKeyID, groupID, account.ID)
	var previous []antigravity.GeminiContent
	if req.PreviousResponseID != "" {
		var ok bool
		previous, ok = s.responsesStore.get(req.PreviousResponseID, scope, mapped)
		if !ok {
			return nil, s.writeGoogleError(c, 400, "previous_response_id not found or expired; resend full input without previous_response_id")
		}
	}
	geminiBody, custom, err := convertAntigravityResponsesRequest(&req, previous)
	if err != nil {
		return nil, s.writeGoogleError(c, 400, err.Error())
	}
	var native antigravity.GeminiRequest
	if err = json.Unmarshal(geminiBody, &native); err != nil {
		return nil, err
	}
	st := newGeminiForwardState(c, account, req.Model, "streamGenerateContent", req.Stream, false, []ForwardGeminiOption{WithForwardGeminiSession(groupID, "responses:"+scope+":"+session)})
	st.responses = &antigravityResponsesState{request: req, customTools: custom, scope: scope, contents: native.Contents}
	ex := &pipeline.Exchange{Ctx: ctx, Platform: PlatformAntigravity, RequestedModel: req.Model, Inbound: geminiBody, Account: account}
	ex.Set(antigravityForwardStateKey, st)
	result, err := s.finishGeminiForward(c, st, s.geminiForwardPipeline().Run(ex))
	if result != nil {
		result.ReasoningEffort = ExtractResponsesReasoningEffortFromBody(body)
	}
	return result, err
}

// relayResponses reuses the Responses event state machine, but consumes Gemini
// directly. Only a real upstream finishReason can produce response.completed.
func (s *AntigravityGatewayService) relayResponses(st *antigravityForwardState) error {
	c := st.c
	stopCancel := context.AfterFunc(c.Request.Context(), func() { _ = st.resp.Body.Close() })
	defer stopCancel()
	interval := 300 * time.Second
	if s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		interval = time.Duration(s.settingService.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	idleTimer := time.AfterFunc(interval, func() { _ = st.resp.Body.Close() })
	defer idleTimer.Stop()
	opts := st.responses
	state := apicompat.NewChatCompletionsToResponsesStreamState(opts.request.Model)
	state.CustomTools = opts.customTools
	state.ResponseID = "resp_ag_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	scanner := bufio.NewScanner(st.resp.Body)
	scanner.Buffer(make([]byte, 64<<10), defaultMaxLineSize)
	if s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.MaxLineSize > 0 {
		scanner.Buffer(make([]byte, 64<<10), s.settingService.cfg.Gateway.MaxLineSize)
	}
	started := false
	finished := false
	disconnected := false
	toolIndex := 0
	var final *apicompat.ResponsesResponse
	var nativeParts []antigravity.GeminiPart
	responseBytes := 0
	tools, _ := apicompat.EffectiveResponsesTools(&opts.request)
	namespaceTools := map[string][2]string{}
	for _, tool := range tools {
		if tool.Type == "namespace" {
			children := tool.Tools
			if len(children) == 0 {
				children = tool.Children
			}
			for _, child := range children {
				namespaceTools[apicompat.FlattenNamespaceToolName(tool.Name, child.Name)] = [2]string{tool.Name, child.Name}
			}
		}
	}
	restoreNamespace := func(item *apicompat.ResponsesOutput) {
		if item != nil {
			if pair, ok := namespaceTools[item.Name]; ok {
				item.Namespace = pair[0]
				item.Name = pair[1]
			}
		}
	}
	emit := func(events []apicompat.ResponsesStreamEvent) {
		for _, event := range events {
			if event.Response != nil && event.Response.Status == "incomplete" && event.Type == "response.completed" {
				event.Type = "response.incomplete"
			}
			restoreNamespace(event.Item)
			if event.Response != nil && (event.Type == "response.completed" || event.Type == "response.incomplete") {
				for i := range event.Response.Output {
					restoreNamespace(&event.Response.Output[i])
				}
				final = event.Response
				if opts.request.Store == nil || *opts.request.Store {
					history := append(opts.contents, antigravity.GeminiContent{Role: "model", Parts: nativeParts})
					s.responsesStore.put(final.ID, opts.scope, st.mappedModel, history)
				}
			}
			if !opts.request.Stream || disconnected {
				continue
			}
			if !started {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("X-Accel-Buffering", "no")
				c.Status(200)
				started = true
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err = c.Writer.Write([]byte("event: " + event.Type + "\ndata: " + string(data) + "\n\n")); err != nil {
				disconnected = true
				continue
			}
			c.Writer.Flush()
		}
	}
	for scanner.Scan() {
		idleTimer.Reset(interval)
		responseBytes += len(scanner.Bytes())
		if responseBytes > 32<<20 {
			return s.responsesRelayError(st, state, started, "upstream_response_too_large", "Antigravity response exceeds 32 MiB")
		}
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		if string(raw) == "[DONE]" {
			continue
		}
		inner, err := s.unwrapV1InternalResponse(raw)
		if err != nil {
			return s.responsesRelayError(st, state, started, "upstream_error", "Invalid Antigravity stream envelope")
		}
		if gjson.GetBytes(inner, "error").Exists() {
			return s.responsesRelayError(st, state, started, "upstream_error", "Antigravity stream returned an error")
		}
		var response antigravity.GeminiResponse
		if json.Unmarshal(inner, &response) != nil {
			return s.responsesRelayError(st, state, started, "upstream_error", "Invalid Antigravity stream JSON")
		}
		if response.UsageMetadata != nil {
			u := response.UsageMetadata
			// Use shared usage conversion to retain cached and reasoning token details.
			state.Usage = apicompat.ChatUsageToResponsesUsage(&apicompat.ChatUsage{PromptTokens: u.PromptTokenCount, CompletionTokens: u.CandidatesTokenCount + u.ThoughtsTokenCount, TotalTokens: u.PromptTokenCount + u.CandidatesTokenCount + u.ThoughtsTokenCount, PromptTokensDetails: &apicompat.ChatTokenDetails{CachedTokens: u.CachedContentTokenCount}, CompletionTokensDetails: &apicompat.ChatTokenDetails{ReasoningTokens: u.ThoughtsTokenCount}})
			st.usage = &ClaudeUsage{InputTokens: max(0, u.PromptTokenCount-u.CachedContentTokenCount), OutputTokens: u.CandidatesTokenCount + u.ThoughtsTokenCount, CacheReadInputTokens: u.CachedContentTokenCount}
		}
		if len(response.Candidates) == 0 {
			continue
		}
		candidate := response.Candidates[0]
		if finished {
			continue
		}
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				delta := apicompat.ChatDelta{}
				if part.InlineData != nil {
					return s.responsesRelayError(st, state, started, "unsupported_output", "Image output is not supported")
				}
				if part.Text != "" {
					text := part.Text
					if part.Thought {
						delta.ReasoningContent = &text
					} else {
						delta.Content = &text
					}
				}
				if part.FunctionCall != nil {
					fc := part.FunctionCall
					if fc.ID == "" {
						fc.ID = "call_" + strings.ReplaceAll(uuid.NewString(), "-", "")
					}
					args, _ := json.Marshal(fc.Args)
					idx := toolIndex
					toolIndex++
					delta.ToolCalls = []apicompat.ChatToolCall{{Index: &idx, ID: fc.ID, Type: "function", Function: apicompat.ChatFunctionCall{Name: fc.Name, Arguments: string(args)}}}
					if part.ThoughtSignature != "" {
						s.getReasoningReplayCache().RecordSignature(st.sessionHash, fc.ID, fc.Name, part.ThoughtSignature)
					}
				}
				if part.FunctionCall == nil && part.FunctionResponse == nil && part.InlineData == nil && strings.TrimSpace(part.Text) == "" {
					if part.ThoughtSignature != "" {
						attached := false
						for i := len(nativeParts) - 1; i >= 0; i-- {
							if nativeParts[i].FunctionCall != nil {
								if nativeParts[i].ThoughtSignature == "" {
									nativeParts[i].ThoughtSignature = part.ThoughtSignature
								}
								s.getReasoningReplayCache().RecordSignature(st.sessionHash, nativeParts[i].FunctionCall.ID, nativeParts[i].FunctionCall.Name, part.ThoughtSignature)
								attached = true
								break
							}
						}
						if !attached {
							s.getReasoningReplayCache().RecordSignature(st.sessionHash, "", "", part.ThoughtSignature)
						}
					}
					continue
				}
				nativeParts = append(nativeParts, part)
				if st.firstTokenMs == nil && (part.Text != "" || part.FunctionCall != nil) {
					ms := int(time.Since(st.startTime).Milliseconds())
					st.firstTokenMs = &ms
				}
				emit(apicompat.ChatCompletionsChunkToResponsesEvents(&apicompat.ChatCompletionsChunk{Choices: []apicompat.ChatChunkChoice{{Delta: delta}}}, state))
			}
		}
		if candidate.FinishReason != "" {
			finished = true
			switch candidate.FinishReason {
			case "STOP":
				state.FinishReason = "stop"
				if toolIndex > 0 {
					state.FinishReason = "tool_calls"
				}
			case "MAX_TOKENS":
				state.FinishReason = "length"
			default:
				return s.responsesRelayError(st, state, started, "upstream_error", "Antigravity finished with "+candidate.FinishReason)
			}
		}
	}
	if scanner.Err() != nil || !finished {
		return s.responsesRelayError(st, state, started, "upstream_stream_incomplete", "Antigravity stream ended before completion")
	}
	emit(apicompat.FinalizeChatCompletionsResponsesStream(state))
	if final == nil {
		return s.responsesRelayError(st, state, started, "upstream_stream_incomplete", "Missing final response")
	}
	if !opts.request.Stream {
		c.Header("Content-Type", "application/json")
		c.JSON(200, final)
	}
	st.requestID = final.ID
	st.clientDisconnect = disconnected
	return nil
}

func (s *AntigravityGatewayService) responsesRelayError(st *antigravityForwardState, state *apicompat.ChatCompletionsToResponsesStreamState, started bool, code, message string) error {
	if !started {
		return s.writeGoogleError(st.c, http.StatusBadGateway, message)
	}
	data, _ := json.Marshal(map[string]any{"type": "response.failed", "sequence_number": state.SequenceNumber, "response": map[string]any{"id": state.ResponseID, "object": "response", "status": "failed", "output": []any{}, "error": map[string]string{"code": code, "message": message}}})
	_, _ = st.c.Writer.Write([]byte("event: response.failed\ndata: " + string(data) + "\n\n"))
	st.c.Writer.Flush()
	return fmt.Errorf("%s", message)
}
