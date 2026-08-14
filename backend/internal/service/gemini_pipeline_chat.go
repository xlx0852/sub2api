package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

func (s *GeminiMessagesCompatService) forwardGeminiChatParse(ex *pipeline.Exchange) error {
	st := geminiChatForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse gemini chat completions request: empty state")
	}
	c := st.c
	var ccReq apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(st.body, &ccReq); err != nil {
		return s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
	}
	if strings.TrimSpace(ccReq.Model) == "" {
		return s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
	}
	st.originalModel = ccReq.Model
	st.clientStream = ccReq.Stream
	st.includeUsage = ccReq.StreamOptions != nil && ccReq.StreamOptions.IncludeUsage
	st.originalChatBody = st.body

	responsesReq, err := apicompat.ChatCompletionsToResponses(&ccReq)
	if err != nil {
		return s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(responsesReq)
	if err != nil {
		return s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	anthropicReq.Stream = st.clientStream
	claudeBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return fmt.Errorf("marshal chat completions compat request: %w", err)
	}

	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(claudeBody, &req); err != nil {
		return s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
	}
	if strings.TrimSpace(req.Model) == "" {
		return s.writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
	}
	st.claudeBody = claudeBody
	st.claudeModel = req.Model
	ex.RequestedModel = st.originalModel
	ex.Inbound = st.body
	return nil
}

func (s *GeminiMessagesCompatService) forwardGeminiChatRewrite(ex *pipeline.Exchange) error {
	st := geminiChatForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("rewrite gemini chat completions request: empty state")
	}
	account := ex.Account
	st.mappedModel = st.claudeModel
	if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
		st.mappedModel = account.GetMappedModel(st.claudeModel)
	}
	geminiReq, err := convertClaudeMessagesToGeminiGenerateContent(st.claudeBody)
	if err != nil {
		return s.writeChatCompletionsError(st.c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	st.geminiReq = ensureGeminiFunctionCallThoughtSignatures(geminiReq)
	if account.ProxyID != nil && account.Proxy != nil {
		st.proxyURL = account.Proxy.URL()
	}
	st.useUpstreamStream = st.clientStream
	if account.Type == AccountTypeOAuth && !st.clientStream && strings.TrimSpace(account.GetCredential("project_id")) != "" {
		st.useUpstreamStream = true
	}
	ex.UpstreamModel = st.mappedModel
	ex.Outbound = st.geminiReq
	return nil
}

func (s *GeminiMessagesCompatService) forwardGeminiChatTransport(ex *pipeline.Exchange) error {
	st := geminiChatForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("transport gemini chat completions request: empty state")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	buildReq, requestIDHeader := s.buildGeminiChatCompletionsUpstreamRequestFunc(
		account,
		st.mappedModel,
		st.geminiReq,
		st.clientStream,
		st.useUpstreamStream,
	)
	st.requestIDHeader = requestIDHeader

	var resp *http.Response
	for attempt := 1; attempt <= geminiMaxRetries; attempt++ {
		upstreamReq, idHeader, err := buildReq(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", err.Error())
		}
		st.requestIDHeader = idHeader

		resp, err = s.httpUpstream.Do(upstreamReq, st.proxyURL, account.ID, account.Concurrency)
		if err != nil {
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: 0,
				Kind:               "request_error",
				Message:            safeErr,
			})
			if attempt < geminiMaxRetries {
				logger.LegacyPrintf("service.gemini_chat_completions", "Gemini account %d: upstream request failed, retry %d/%d: %v", account.ID, attempt, geminiMaxRetries, err)
				sleepGeminiBackoff(ctx, attempt)
				continue
			}
			setOpsUpstreamError(c, 0, safeErr, "")
			return s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries: "+safeErr)
		}

		if matched, rebuilt := s.checkErrorPolicyInLoop(ctx, account, resp); matched {
			resp = rebuilt
			break
		} else {
			resp = rebuilt
		}

		if resp.StatusCode >= 400 && s.shouldRetryGeminiUpstreamError(account, resp.StatusCode) {
			respBody := s.readUpstreamErrorBody(resp)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusForbidden && isGeminiInsufficientScope(resp.Header, respBody) {
				resp = &http.Response{
					StatusCode: resp.StatusCode,
					Header:     resp.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(respBody)),
				}
				break
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			}
			if attempt < geminiMaxRetries {
				upstreamReqID := resp.Header.Get(st.requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  upstreamReqID,
					Kind:               "retry",
					Message:            upstreamMsg,
				})
				logger.LegacyPrintf("service.gemini_chat_completions", "Gemini account %d: upstream status %d, retry %d/%d", account.ID, resp.StatusCode, attempt, geminiMaxRetries)
				sleepGeminiBackoff(ctx, attempt)
				continue
			}
			resp = &http.Response{
				StatusCode: resp.StatusCode,
				Header:     resp.Header.Clone(),
				Body:       io.NopCloser(bytes.NewReader(respBody)),
			}
			break
		}

		break
	}
	st.resp = resp
	if st.resp == nil {
		return s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
	}

	requestID := resp.Header.Get(st.requestIDHeader)
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
	}
	if requestID != "" {
		c.Header("x-request-id", requestID)
	}

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		evBody := unwrapIfNeeded(account.Type == AccountTypeOAuth, respBody)
		if s.shouldFailoverGeminiUpstreamError(resp.StatusCode) {
			upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(evBody)))
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  requestID,
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: evBody}
		}
		return s.writeGeminiChatCompletionsMappedError(c, account, resp.StatusCode, requestID, evBody)
	}
	return nil
}

func (s *GeminiMessagesCompatService) forwardGeminiChatRelay(ex *pipeline.Exchange) error {
	st := geminiChatForwardStateFrom(ex)
	if st == nil {
		return nil
	}
	if st.resp == nil {
		return fmt.Errorf("relay gemini chat completions request: empty response")
	}
	account := ex.Account
	c := st.c
	resp := st.resp

	requestID := resp.Header.Get(st.requestIDHeader)
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
	}
	reasoningEffort := extractCCReasoningEffortFromBody(st.originalChatBody)
	reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, st.originalChatBody, st.mappedModel)

	var usage *ClaudeUsage
	var firstTokenMs *int
	if st.clientStream {
		streamRes, err := s.handleChatCompletionsStreamingResponseFromGemini(c, resp, st.startTime, st.originalModel, account.Type == AccountTypeOAuth, st.includeUsage)
		if err != nil {
			return err
		}
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
	} else if st.useUpstreamStream {
		collected, usageObj, err := collectGeminiSSE(resp.Body, account.Type == AccountTypeOAuth)
		if err != nil {
			return s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Failed to read upstream stream")
		}
		collectedBytes, _ := json.Marshal(collected)
		chatResp, usageObj2, err := geminiResponseToChatCompletions(collected, st.originalModel, collectedBytes, usageObj)
		if err != nil {
			return s.writeChatCompletionsError(c, http.StatusBadGateway, "upstream_error", "Failed to parse upstream response")
		}
		c.JSON(http.StatusOK, chatResp)
		usage = usageObj2
	} else {
		usageResp, err := s.handleChatCompletionsNonStreamingResponseFromGemini(c, resp, st.originalModel, account.Type == AccountTypeOAuth)
		if err != nil {
			return err
		}
		usage = usageResp
	}
	if usage == nil {
		usage = &ClaudeUsage{}
	}

	imageCount := 0
	imageInputSize := s.extractImageInputSize(st.claudeBody)
	imageSize := normalizeOpenAIImageSizeTier(imageInputSize)
	if isImageGenerationModel(st.originalModel) {
		imageCount = 1
	}

	st.result = &ForwardResult{
		RequestID:        requestID,
		Usage:            *usage,
		Model:            st.originalModel,
		UpstreamModel:    st.mappedModel,
		Stream:           st.clientStream,
		Duration:         time.Since(st.startTime),
		FirstTokenMs:     firstTokenMs,
		ReasoningEffort:  reasoningEffort,
		ImageCount:       imageCount,
		ImageSize:        imageSize,
		ImageInputSize:   imageInputSize,
		ClientDisconnect: false,
	}
	return nil
}

func (s *GeminiMessagesCompatService) forwardGeminiChatSettle(ex *pipeline.Exchange) error {
	return nil
}
