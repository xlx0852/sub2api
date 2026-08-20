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

func (s *GeminiMessagesCompatService) forwardGeminiNativeParse(ex *pipeline.Exchange) error {
	st := geminiNativeForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("parse native gemini request: empty state")
	}
	c := st.c
	if strings.TrimSpace(st.originalModel) == "" {
		return s.writeGoogleError(c, http.StatusBadRequest, "Missing model in URL")
	}
	if strings.TrimSpace(st.action) == "" {
		return s.writeGoogleError(c, http.StatusBadRequest, "Missing action in URL")
	}
	if len(st.body) == 0 {
		return s.writeGoogleError(c, http.StatusBadRequest, "Request body is empty")
	}
	if filteredBody, err := filterEmptyPartsFromGeminiRequest(st.body); err == nil {
		st.body = filteredBody
	}
	switch st.action {
	case "generateContent", "streamGenerateContent", "countTokens":
	default:
		return s.writeGoogleError(c, http.StatusNotFound, "Unsupported action: "+st.action)
	}
	st.body = ensureGeminiFunctionCallThoughtSignatures(st.body)
	ex.RequestedModel = st.originalModel
	ex.Inbound = st.body
	return nil
}

func (s *GeminiMessagesCompatService) forwardGeminiNativeRewrite(ex *pipeline.Exchange) error {
	st := geminiNativeForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("rewrite native gemini request: empty state")
	}
	account := ex.Account
	st.mappedModel = st.originalModel
	if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
		st.mappedModel = account.GetMappedModel(st.originalModel)
	}
	if account.ProxyID != nil && account.Proxy != nil {
		st.proxyURL = account.Proxy.URL()
	}
	st.useUpstreamStream = st.clientStream
	st.upstreamAction = st.action
	if account.Type == AccountTypeOAuth && !st.clientStream && st.action == "generateContent" && strings.TrimSpace(account.GetCredential("project_id")) != "" {
		st.useUpstreamStream = true
		st.upstreamAction = "streamGenerateContent"
	}
	st.forceAIStudio = st.action == "countTokens"
	ex.UpstreamModel = st.mappedModel
	ex.Outbound = st.body
	return nil
}

func (s *GeminiMessagesCompatService) buildGeminiNativeUpstreamRequest(ctx context.Context, st *geminiNativeForwardState, account *Account) (*http.Request, string, error) {
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
		fullURL, err := buildGeminiAIStudioModelURL(normalizedBaseURL, st.mappedModel, st.upstreamAction, st.useUpstreamStream)
		if err != nil {
			return nil, "", err
		}
		upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(st.body))
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
		if projectID != "" && !st.forceAIStudio {
			baseURL, err := s.validateUpstreamBaseURL(GeminiCliBaseURL)
			if err != nil {
				return nil, "", err
			}
			fullURL := fmt.Sprintf("%s/v1internal:%s", strings.TrimRight(baseURL, "/"), st.upstreamAction)
			if st.useUpstreamStream {
				fullURL += "?alt=sse"
			}
			wrapped := map[string]any{
				"model":   st.mappedModel,
				"project": projectID,
			}
			var inner any
			if err := json.Unmarshal(st.body, &inner); err != nil {
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
		fullURL, err := buildGeminiAIStudioModelURL(normalizedBaseURL, st.mappedModel, st.upstreamAction, st.useUpstreamStream)
		if err != nil {
			return nil, "", err
		}
		upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(st.body))
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
		fullURL, err := buildVertexGeminiURL(account.VertexProjectID(), account.VertexLocation(st.mappedModel), st.mappedModel, st.upstreamAction, st.useUpstreamStream)
		if err != nil {
			return nil, "", err
		}
		upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(st.body))
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

func (s *GeminiMessagesCompatService) estimateNativeCountTokensResult(st *geminiNativeForwardState, requestID string) *ForwardResult {
	estimated := estimateGeminiCountTokens(st.body)
	st.c.JSON(http.StatusOK, map[string]any{"totalTokens": estimated})
	return &ForwardResult{
		RequestID:     requestID,
		Usage:         ClaudeUsage{},
		Model:         st.originalModel,
		UpstreamModel: st.mappedModel,
		Stream:        false,
		Duration:      time.Since(st.startTime),
		FirstTokenMs:  nil,
	}
}

func (s *GeminiMessagesCompatService) forwardGeminiNativeTransport(ex *pipeline.Exchange) error {
	st := geminiNativeForwardStateFrom(ex)
	if st == nil {
		return fmt.Errorf("transport native gemini request: empty state")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	var resp *http.Response
	for attempt := 1; attempt <= geminiMaxRetries; attempt++ {
		upstreamReq, idHeader, err := s.buildGeminiNativeUpstreamRequest(ctx, st, account)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			if strings.Contains(err.Error(), "unsupported account type") {
				return s.writeGoogleError(c, http.StatusBadGateway, "Unsupported account type: "+account.Type)
			}
			if strings.Contains(err.Error(), "missing project_id") {
				return s.writeGoogleError(c, http.StatusBadRequest, err.Error())
			}
			return s.writeGoogleError(c, http.StatusBadGateway, err.Error())
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
			if st.action == "countTokens" {
				st.result = s.estimateNativeCountTokensResult(st, "")
				st.skipRemaining = true
				return nil
			}
			setOpsUpstreamError(c, 0, safeErr, "")
			return s.writeGoogleError(c, http.StatusBadGateway, "Upstream request failed after retries: "+safeErr)
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
					upstreamDetail = redactAndTruncateUpstreamDetail(string(respBody), maxBytes)
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
			if st.action == "countTokens" {
				st.result = s.estimateNativeCountTokensResult(st, "")
				st.skipRemaining = true
				return nil
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
		return s.writeGoogleError(c, http.StatusBadGateway, "Upstream request failed")
	}

	requestID := resp.Header.Get(st.requestIDHeader)
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
	}
	if requestID != "" {
		c.Header("x-request-id", requestID)
	}
	isOAuth := account.Type == AccountTypeOAuth

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		if st.action == "countTokens" && isOAuth && isGeminiInsufficientScope(resp.Header, respBody) {
			st.result = s.estimateNativeCountTokensResult(st, requestID)
			st.skipRemaining = true
			return nil
		}

		if s.rateLimitService != nil {
			switch s.rateLimitService.CheckErrorPolicy(ctx, account, resp.StatusCode, respBody) {
			case ErrorPolicySkipped:
				respBody = unwrapIfNeeded(isOAuth, respBody)
				contentType := resp.Header.Get("Content-Type")
				if contentType == "" {
					contentType = "application/json"
				}
				MarkResponseCommitted(c)
				c.Data(http.StatusInternalServerError, contentType, respBody)
				return fmt.Errorf("gemini upstream error: %d (skipped by error policy)", resp.StatusCode)
			case ErrorPolicyMatched, ErrorPolicyTempUnscheduled:
				s.handleGeminiUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
				evBody := unwrapIfNeeded(isOAuth, respBody)
				upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(evBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = redactAndTruncateUpstreamDetail(string(evBody), maxBytes)
				}
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  requestID,
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
				evBody := unwrapIfNeeded(isOAuth, respBody)
				upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(evBody)))
				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = redactAndTruncateUpstreamDetail(string(evBody), maxBytes)
				}
				log.Printf("[Gemini] status=400 google_config_error failover=true upstream_message=%q account=%d", upstreamMsg, account.ID)
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  requestID,
					Kind:               "failover",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: evBody, RetryableOnSameAccount: true}
			}
		}
		if s.shouldFailoverGeminiUpstreamError(resp.StatusCode) {
			evBody := unwrapIfNeeded(isOAuth, respBody)
			upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(evBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
				}
				upstreamDetail = redactAndTruncateUpstreamDetail(string(evBody), maxBytes)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  requestID,
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: evBody}
		}

		respBody = unwrapIfNeeded(isOAuth, respBody)
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		upstreamDetail := ""
		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
			if maxBytes <= 0 {
				maxBytes = 2048
			}
			upstreamDetail = redactAndTruncateUpstreamDetail(string(respBody), maxBytes)
			logger.LegacyPrintf("service.gemini_messages_compat", "[Gemini] native upstream error %d: %s", resp.StatusCode, truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes))
		}
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestID,
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})

		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
		}
		MarkResponseCommitted(c)
		c.Data(resp.StatusCode, contentType, respBody)
		if upstreamMsg == "" {
			return fmt.Errorf("gemini upstream error: %d", resp.StatusCode)
		}
		return fmt.Errorf("gemini upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
	}
	return nil
}

func (s *GeminiMessagesCompatService) forwardGeminiNativeRelay(ex *pipeline.Exchange) error {
	st := geminiNativeForwardStateFrom(ex)
	if st == nil || st.skipRemaining {
		return nil
	}
	if st.resp == nil {
		return fmt.Errorf("relay native gemini request: empty response")
	}
	account := ex.Account
	c := st.c
	resp := st.resp
	isOAuth := account.Type == AccountTypeOAuth

	requestID := resp.Header.Get(st.requestIDHeader)
	if requestID == "" {
		requestID = resp.Header.Get("x-goog-request-id")
	}

	var usage *ClaudeUsage
	var firstTokenMs *int
	if st.clientStream {
		streamRes, err := s.handleNativeStreamingResponse(c, resp, st.startTime, isOAuth)
		if err != nil {
			return err
		}
		usage = streamRes.usage
		firstTokenMs = streamRes.firstTokenMs
	} else if st.useUpstreamStream {
		collected, usageObj, err := collectGeminiSSE(resp.Body, isOAuth)
		if err != nil {
			return s.writeGoogleError(c, http.StatusBadGateway, "Failed to read upstream stream")
		}
		b, _ := json.Marshal(collected)
		c.Data(http.StatusOK, "application/json", b)
		usage = usageObj
	} else {
		usageResp, err := s.handleNativeNonStreamingResponse(c, resp, isOAuth)
		if err != nil {
			return err
		}
		usage = usageResp
	}
	if usage == nil {
		usage = &ClaudeUsage{}
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

func (s *GeminiMessagesCompatService) forwardGeminiNativeSettle(ex *pipeline.Exchange) error {
	return nil
}
