package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

func (s *GeminiMessagesCompatService) forwardGeminiMessagesParse(ex *pipeline.Exchange) error {
	st := geminiMessagesForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse request: empty state")
	}
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(st.body, &req); err != nil {
		return fmt.Errorf("parse request: %w", err)
	}
	if strings.TrimSpace(req.Model) == "" {
		return fmt.Errorf("missing model")
	}
	st.originalModel = req.Model
	st.clientStream = req.Stream
	ex.RequestedModel = st.originalModel
	ex.Inbound = st.body
	return nil
}

func (s *GeminiMessagesCompatService) forwardGeminiMessagesRewrite(ex *pipeline.Exchange) error {
	st := geminiMessagesForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("rewrite gemini request: empty state")
	}
	account := ex.Account
	st.mappedModel = st.originalModel
	if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
		st.mappedModel = account.GetMappedModel(st.originalModel)
	}

	geminiReq, err := convertClaudeMessagesToGeminiGenerateContent(st.body)
	if err != nil {
		return s.writeClaudeError(st.c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	st.geminiReq = ensureGeminiFunctionCallThoughtSignatures(geminiReq)
	st.originalClaudeBody = st.body
	st.useUpstreamStream = st.clientStream
	if account.Type == AccountTypeOAuth && !st.clientStream && strings.TrimSpace(account.GetCredential("project_id")) != "" {
		st.useUpstreamStream = true
	}
	if account.ProxyID != nil && account.Proxy != nil {
		st.proxyURL = account.Proxy.URL()
	}
	ex.UpstreamModel = st.mappedModel
	ex.Outbound = st.geminiReq
	return nil
}

func (s *GeminiMessagesCompatService) buildGeminiMessagesUpstreamRequest(ctx context.Context, st *geminiMessagesForwardState, account *Account) (*http.Request, string, error) {
	switch account.Type {
	case AccountTypeAPIKey:
		apiKey := account.GetCredential("api_key")
		if strings.TrimSpace(apiKey) == "" {
			return nil, "", errors.New("gemini api_key not configured")
		}
		baseURL := account.GetGeminiBaseURL(GeminiAIStudioBaseURL)
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, "", err
		}
		action := "generateContent"
		if st.clientStream {
			action = "streamGenerateContent"
		}
		fullURL, err := buildGeminiAIStudioModelURL(normalizedBaseURL, st.mappedModel, action, st.clientStream)
		if err != nil {
			return nil, "", err
		}
		restGeminiReq := normalizeGeminiRequestForAIStudio(st.geminiReq)
		upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(restGeminiReq))
		if err != nil {
			return nil, "", err
		}
		upstreamReq.Header.Set("Content-Type", "application/json")
		upstreamReq.Header.Set("x-goog-api-key", apiKey)
		return upstreamReq, "x-request-id", nil

	case AccountTypeOAuth:
		if s.tokenProvider == nil {
			return nil, "", errors.New("gemini token provider not configured")
		}
		accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
		if err != nil {
			return nil, "", err
		}
		projectID := strings.TrimSpace(account.GetCredential("project_id"))
		action := "generateContent"
		if st.useUpstreamStream {
			action = "streamGenerateContent"
		}
		if projectID != "" {
			baseURL, err := s.validateUpstreamBaseURL(GeminiCliBaseURL)
			if err != nil {
				return nil, "", err
			}
			fullURL := fmt.Sprintf("%s/v1internal:%s", strings.TrimRight(baseURL, "/"), action)
			if st.useUpstreamStream {
				fullURL += "?alt=sse"
			}
			wrapped := map[string]any{
				"model":   st.mappedModel,
				"project": projectID,
			}
			var inner any
			if err := json.Unmarshal(st.geminiReq, &inner); err != nil {
				return nil, "", fmt.Errorf("failed to parse gemini request: %w", err)
			}
			wrapped["request"] = inner
			wrappedBytes, _ := json.Marshal(wrapped)
			upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(wrappedBytes))
			if err != nil {
				return nil, "", err
			}
			upstreamReq.Header.Set("Content-Type", "application/json")
			upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
			upstreamReq.Header.Set("User-Agent", GeminiCLIUserAgent)
			return upstreamReq, "x-request-id", nil
		}
		baseURL := account.GetGeminiBaseURL(GeminiAIStudioBaseURL)
		normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, "", err
		}
		fullURL, err := buildGeminiAIStudioModelURL(normalizedBaseURL, st.mappedModel, action, st.useUpstreamStream)
		if err != nil {
			return nil, "", err
		}
		restGeminiReq := normalizeGeminiRequestForAIStudio(st.geminiReq)
		upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(restGeminiReq))
		if err != nil {
			return nil, "", err
		}
		upstreamReq.Header.Set("Content-Type", "application/json")
		upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
		return upstreamReq, "x-request-id", nil

	case AccountTypeServiceAccount:
		if s.tokenProvider == nil {
			return nil, "", errors.New("gemini token provider not configured")
		}
		accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
		if err != nil {
			return nil, "", err
		}
		action := "generateContent"
		if st.clientStream {
			action = "streamGenerateContent"
		}
		fullURL, err := buildVertexGeminiURL(account.VertexProjectID(), account.VertexLocation(st.mappedModel), st.mappedModel, action, st.clientStream)
		if err != nil {
			return nil, "", err
		}
		restGeminiReq := normalizeGeminiRequestForAIStudio(st.geminiReq)
		upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(restGeminiReq))
		if err != nil {
			return nil, "", err
		}
		upstreamReq.Header.Set("Content-Type", "application/json")
		upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
		return upstreamReq, "x-request-id", nil

	default:
		return nil, "", fmt.Errorf("unsupported account type: %s", account.Type)
	}
}

func (s *GeminiMessagesCompatService) forwardGeminiMessagesTransport(ex *pipeline.Exchange) error {
	st := geminiMessagesForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("transport gemini request: empty state")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	var resp *http.Response
	signatureRetryStage := 0
	for attempt := 1; attempt <= geminiMaxRetries; attempt++ {
		upstreamReq, idHeader, err := s.buildGeminiMessagesUpstreamRequest(ctx, st, account)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			if strings.Contains(err.Error(), "missing project_id") {
				return s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			}
			if strings.Contains(err.Error(), "unsupported account type") {
				return fmt.Errorf("unsupported account type: %s", account.Type)
			}
			return s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", err.Error())
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
				logger.LegacyPrintf("service.gemini_messages_compat", "Gemini account %d: upstream request failed, retry %d/%d: %v", account.ID, attempt, geminiMaxRetries, err)
				sleepGeminiBackoff(ctx, attempt)
				continue
			}
			setOpsUpstreamError(c, 0, safeErr, "")
			return s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries: "+safeErr)
		}

		if resp.StatusCode == http.StatusBadRequest && signatureRetryStage < 2 {
			respBody := s.readUpstreamErrorBody(resp)
			_ = resp.Body.Close()

			if isGeminiSignatureRelatedError(respBody) {
				upstreamReqID := resp.Header.Get(st.requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = truncateString(string(respBody), maxBytes)
				}
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  upstreamReqID,
					Kind:               "signature_error",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})

				var strippedClaudeBody []byte
				stageName := ""
				switch signatureRetryStage {
				case 0:
					strippedClaudeBody = FilterThinkingBlocksForRetry(st.originalClaudeBody, st.originalModel)
					stageName = "thinking-only"
					signatureRetryStage = 1
				default:
					strippedClaudeBody = FilterSignatureSensitiveBlocksForRetry(st.originalClaudeBody, st.originalModel)
					stageName = "thinking+tools"
					signatureRetryStage = 2
				}
				retryGeminiReq, txErr := convertClaudeMessagesToGeminiGenerateContent(strippedClaudeBody)
				if txErr == nil {
					logger.LegacyPrintf("service.gemini_messages_compat", "Gemini account %d: detected signature-related 400, retrying with downgraded Claude blocks (%s)", account.ID, stageName)
					st.geminiReq = retryGeminiReq
					sleepGeminiBackoff(ctx, 1)
					continue
				}
			}

			resp = &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     resp.Header.Clone(),
				Body:       io.NopCloser(bytes.NewReader(respBody)),
			}
			break
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
			if resp.StatusCode == 403 && isGeminiInsufficientScope(resp.Header, respBody) {
				resp = &http.Response{
					StatusCode: resp.StatusCode,
					Header:     resp.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(respBody)),
				}
				break
			}
			if resp.StatusCode == 429 {
				s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			}
			if attempt < geminiMaxRetries {
				upstreamReqID := resp.Header.Get(st.requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = truncateString(string(respBody), maxBytes)
				}
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  upstreamReqID,
					Kind:               "retry",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				logger.LegacyPrintf("service.gemini_messages_compat", "Gemini account %d: upstream status %d, retry %d/%d", account.ID, resp.StatusCode, attempt, geminiMaxRetries)
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
		return s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
	}

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		if s.rateLimitService != nil {
			switch s.rateLimitService.CheckErrorPolicy(ctx, account, resp.StatusCode, respBody) {
			case ErrorPolicySkipped:
				upstreamReqID := resp.Header.Get(st.requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				return s.writeGeminiMappedError(c, account, http.StatusInternalServerError, upstreamReqID, respBody)
			case ErrorPolicyMatched, ErrorPolicyTempUnscheduled:
				s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
				upstreamReqID := resp.Header.Get(st.requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = truncateString(string(respBody), maxBytes)
				}
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  upstreamReqID,
					Kind:               "failover",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
			}
		}

		s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		if resp.StatusCode == http.StatusBadRequest {
			msg400 := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
			if isGoogleProjectConfigError(msg400) {
				upstreamReqID := resp.Header.Get(st.requestIDHeader)
				if upstreamReqID == "" {
					upstreamReqID = resp.Header.Get("x-goog-request-id")
				}
				upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = truncateString(string(respBody), maxBytes)
				}
				log.Printf("[Gemini] status=400 google_config_error failover=true upstream_message=%q account=%d", upstreamMsg, account.ID)
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  upstreamReqID,
					Kind:               "failover",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody, RetryableOnSameAccount: true}
			}
		}
		if s.shouldFailoverGeminiUpstreamError(resp.StatusCode) {
			upstreamReqID := resp.Header.Get(st.requestIDHeader)
			if upstreamReqID == "" {
				upstreamReqID = resp.Header.Get("x-goog-request-id")
			}
			upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
				}
				upstreamDetail = truncateString(string(respBody), maxBytes)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  upstreamReqID,
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
		}
		upstreamReqID := resp.Header.Get(st.requestIDHeader)
		if upstreamReqID == "" {
			upstreamReqID = resp.Header.Get("x-goog-request-id")
		}
		return s.writeGeminiMappedError(c, account, resp.StatusCode, upstreamReqID, respBody)
	}
	return nil
}

func (s *GeminiMessagesCompatService) forwardGeminiMessagesRelay(ex *pipeline.Exchange) error {
	st := geminiMessagesForwardStateFrom(ex)
	if st == nil || st.resp == nil {
		return fmt.Errorf("relay gemini request: empty response")
	}
	c := st.c
	resp := st.resp

	requestID := resp.Header.Get(st.requestIDHeader)
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
	}
	if requestID != "" {
		c.Header("x-request-id", requestID)
	}

	var usage *ClaudeUsage
	var firstTokenMs *int
	if st.clientStream {
		streamRes, err := s.handleStreamingResponse(c, resp, st.startTime, st.originalModel)
		if err != nil {
			return err
		}
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
	} else if st.useUpstreamStream {
		collected, usageObj, err := collectGeminiSSE(resp.Body, true)
		if err != nil {
			return s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Failed to read upstream stream")
		}
		collectedBytes, _ := json.Marshal(collected)
		claudeResp, usageObj2 := convertGeminiToClaudeMessage(collected, st.originalModel, collectedBytes)
		c.JSON(http.StatusOK, claudeResp)
		usage = usageObj2
		if usageObj != nil && (usageObj.InputTokens > 0 || usageObj.OutputTokens > 0) {
			usage = usageObj
		}
	} else {
		var err error
		usage, err = s.handleNonStreamingResponse(c, resp, st.originalModel)
		if err != nil {
			return err
		}
	}

	imageCount := 0
	imageInputSize := s.extractImageInputSize(st.body)
	imageSize := normalizeOpenAIImageSizeTier(imageInputSize)
	if isImageGenerationModel(st.originalModel) {
		imageCount = 1
	}

	st.result = &ForwardResult{
		RequestID:      requestID,
		Usage:          *usage,
		Model:          st.originalModel,
		UpstreamModel:  st.mappedModel,
		Stream:         st.clientStream,
		Duration:       time.Since(st.startTime),
		FirstTokenMs:   firstTokenMs,
		ImageCount:     imageCount,
		ImageSize:      imageSize,
		ImageInputSize: imageInputSize,
	}
	return nil
}

func (s *GeminiMessagesCompatService) forwardGeminiMessagesSettle(ex *pipeline.Exchange) error {
	return nil
}
