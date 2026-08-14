package service

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

func (s *AntigravityGatewayService) forwardClaudeTransport(ex *pipeline.Exchange) error {
	st := antigravityForwardStateFrom(ex)
	if st == nil || ex == nil || ex.Account == nil {
		return &antigravityClientError{
			Status:  http.StatusBadRequest,
			Type:    "invalid_request_error",
			Message: "Invalid request body",
		}
	}
	claudeReqPtr := antigravityClaudeRequestFromExchange(ex)
	if claudeReqPtr == nil {
		return &antigravityClientError{
			Status:  http.StatusBadRequest,
			Type:    "invalid_request_error",
			Message: "Invalid request body",
		}
	}
	st.claudeReq = *claudeReqPtr
	st.originalModel = ex.RequestedModel
	st.mappedModel = ex.UpstreamModel
	st.billingModel = ex.UpstreamModel

	ctx := ex.Ctx
	account := ex.Account
	c := st.c

	if s.tokenProvider == nil {
		return s.writeClaudeError(c, http.StatusBadGateway, "api_error", "Antigravity token provider not configured")
	}
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return &UpstreamFailoverError{
			StatusCode:   http.StatusBadGateway,
			ResponseBody: []byte(`{"error":{"type":"authentication_error","message":"Failed to get upstream access token"},"type":"error"}`),
		}
	}
	st.accessToken = accessToken

	projectID, err := resolveAntigravityProjectID(account)
	if err != nil {
		_ = s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return err
	}
	st.projectID = projectID

	if account.ProxyID != nil && account.Proxy != nil {
		st.proxyURL = account.Proxy.URL()
	}

	// Antigravity 上游要求必须包含身份提示词，否则会返回 429
	transformOpts := s.getClaudeTransformOptions(ctx)
	transformOpts.EnableIdentityPatch = true
	st.transformOpts = transformOpts

	geminiBody, err := antigravity.TransformClaudeToGeminiWithOptions(&st.claudeReq, projectID, st.mappedModel, transformOpts)
	if err != nil {
		return s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", "Invalid request")
	}
	st.geminiBody = geminiBody
	st.action = "streamGenerateContent"

	result, err := s.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:             ctx,
		prefix:          st.prefix,
		account:         account,
		proxyURL:        st.proxyURL,
		accessToken:     accessToken,
		action:          st.action,
		body:            geminiBody,
		c:               c,
		httpUpstream:    s.httpUpstream,
		settingService:  s.settingService,
		accountRepo:     s.accountRepo,
		handleError:     s.handleUpstreamError,
		requestedModel:  st.originalModel,
		isStickySession: st.isStickySession,
		groupID:         0,
		sessionHash:     "",
	})
	if err != nil {
		if switchErr, ok := IsAntigravityAccountSwitchError(err); ok {
			return &UpstreamFailoverError{
				StatusCode:        http.StatusServiceUnavailable,
				ForceCacheBilling: switchErr.IsStickySession,
			}
		}
		if c.Request.Context().Err() != nil {
			return s.writeClaudeError(c, http.StatusBadGateway, "client_disconnected", "Client disconnected before upstream response")
		}
		return s.writeClaudeError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed after retries")
	}

	resp := result.resp
	closeResp := true
	defer func() {
		if closeResp && resp != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)

		if resp.StatusCode == http.StatusBadRequest && isSignatureRelatedError(respBody) && s.settingService.IsSignatureRectifierEnabled(ctx) {
			upstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(respBody))
			upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
			logBody, maxBytes := s.getLogConfig()
			upstreamDetail := s.getUpstreamErrorDetail(respBody)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "signature_error",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})

			retryStages := []struct {
				name  string
				strip func(*antigravity.ClaudeRequest) (bool, error)
			}{
				{name: "thinking-only", strip: stripThinkingFromClaudeRequest},
				{name: "thinking+tools", strip: stripSignatureSensitiveBlocksFromClaudeRequest},
			}

			for _, stage := range retryStages {
				retryClaudeReq := st.claudeReq
				retryClaudeReq.Messages = append([]antigravity.ClaudeMessage(nil), st.claudeReq.Messages...)

				stripped, stripErr := stage.strip(&retryClaudeReq)
				if stripErr != nil || !stripped {
					continue
				}

				logger.LegacyPrintf("service.antigravity_gateway", "Antigravity account %d: detected signature-related 400, retrying once (%s)", account.ID, stage.name)

				retryGeminiBody, txErr := antigravity.TransformClaudeToGeminiWithOptions(&retryClaudeReq, projectID, st.mappedModel, s.getClaudeTransformOptions(ctx))
				if txErr != nil {
					continue
				}
				retryResult, retryErr := s.antigravityRetryLoop(antigravityRetryLoopParams{
					ctx:             ctx,
					prefix:          st.prefix,
					account:         account,
					proxyURL:        st.proxyURL,
					accessToken:     accessToken,
					action:          st.action,
					body:            retryGeminiBody,
					c:               c,
					httpUpstream:    s.httpUpstream,
					settingService:  s.settingService,
					accountRepo:     s.accountRepo,
					handleError:     s.handleUpstreamError,
					requestedModel:  st.originalModel,
					isStickySession: st.isStickySession,
					groupID:         0,
					sessionHash:     "",
				})
				if retryErr != nil {
					appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
						Platform:           account.Platform,
						AccountID:          account.ID,
						AccountName:        account.Name,
						UpstreamStatusCode: 0,
						Kind:               "signature_retry_request_error",
						Message:            sanitizeUpstreamErrorMessage(retryErr.Error()),
					})
					logger.LegacyPrintf("service.antigravity_gateway", "Antigravity account %d: signature retry request failed (%s): %v", account.ID, stage.name, retryErr)
					continue
				}

				retryResp := retryResult.resp
				if retryResp.StatusCode < 400 {
					_ = resp.Body.Close()
					resp = retryResp
					respBody = nil
					break
				}

				retryBody, _ := io.ReadAll(io.LimitReader(retryResp.Body, 8<<10))
				_ = retryResp.Body.Close()
				if retryResp.StatusCode == http.StatusTooManyRequests {
					retryBaseURL := ""
					if retryResp.Request != nil && retryResp.Request.URL != nil {
						retryBaseURL = retryResp.Request.URL.Scheme + "://" + retryResp.Request.URL.Host
					}
					logger.LegacyPrintf("service.antigravity_gateway", "%s status=429 rate_limited base_url=%s retry_stage=%s body=%s", st.prefix, retryBaseURL, stage.name, truncateForLog(retryBody, 200))
				}
				kind := "signature_retry"
				if strings.TrimSpace(stage.name) != "" {
					kind = "signature_retry_" + strings.ReplaceAll(stage.name, "+", "_")
				}
				retryUpstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(retryBody))
				retryUpstreamMsg = sanitizeUpstreamErrorMessage(retryUpstreamMsg)
				retryUpstreamDetail := ""
				if logBody {
					retryUpstreamDetail = truncateString(string(retryBody), maxBytes)
				}
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: retryResp.StatusCode,
					UpstreamRequestID:  retryResp.Header.Get("x-request-id"),
					Kind:               kind,
					Message:            retryUpstreamMsg,
					Detail:             retryUpstreamDetail,
				})

				if retryResp.StatusCode != http.StatusBadRequest || !isSignatureRelatedError(retryBody) {
					respBody = retryBody
					resp = &http.Response{
						StatusCode: retryResp.StatusCode,
						Header:     retryResp.Header.Clone(),
						Body:       io.NopCloser(bytes.NewReader(retryBody)),
					}
					break
				}

				respBody = retryBody
				resp = &http.Response{
					StatusCode: retryResp.StatusCode,
					Header:     retryResp.Header.Clone(),
					Body:       io.NopCloser(bytes.NewReader(retryBody)),
				}
			}
		}

		if resp.StatusCode == http.StatusBadRequest && respBody != nil && !isSignatureRelatedError(respBody) {
			errMsg := strings.TrimSpace(extractAntigravityErrorMessage(respBody))
			if isThinkingBudgetConstraintError(errMsg) && s.settingService.IsBudgetRectifierEnabled(ctx) {
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					Kind:               "budget_constraint_error",
					Message:            errMsg,
					Detail:             s.getUpstreamErrorDetail(respBody),
				})

				if st.claudeReq.Thinking == nil || st.claudeReq.Thinking.Type != "adaptive" {
					retryClaudeReq := st.claudeReq
					retryClaudeReq.Messages = append([]antigravity.ClaudeMessage(nil), st.claudeReq.Messages...)
					retryClaudeReq.Thinking = &antigravity.ThinkingConfig{
						Type:         "enabled",
						BudgetTokens: BudgetRectifyBudgetTokens,
					}
					if retryClaudeReq.MaxTokens < BudgetRectifyMinMaxTokens {
						retryClaudeReq.MaxTokens = BudgetRectifyMaxTokens
					}

					logger.LegacyPrintf("service.antigravity_gateway", "Antigravity account %d: detected budget_tokens constraint error, retrying with rectified budget (budget_tokens=%d, max_tokens=%d)", account.ID, BudgetRectifyBudgetTokens, BudgetRectifyMaxTokens)

					retryGeminiBody, txErr := antigravity.TransformClaudeToGeminiWithOptions(&retryClaudeReq, projectID, st.mappedModel, transformOpts)
					if txErr == nil {
						retryResult, retryErr := s.antigravityRetryLoop(antigravityRetryLoopParams{
							ctx:             ctx,
							prefix:          st.prefix,
							account:         account,
							proxyURL:        st.proxyURL,
							accessToken:     accessToken,
							action:          st.action,
							body:            retryGeminiBody,
							c:               c,
							httpUpstream:    s.httpUpstream,
							settingService:  s.settingService,
							accountRepo:     s.accountRepo,
							handleError:     s.handleUpstreamError,
							requestedModel:  st.originalModel,
							isStickySession: st.isStickySession,
							groupID:         0,
							sessionHash:     "",
						})
						if retryErr == nil {
							retryResp := retryResult.resp
							if retryResp.StatusCode < 400 {
								_ = resp.Body.Close()
								resp = retryResp
								respBody = nil
							} else {
								retryBody := s.readUpstreamErrorBody(retryResp)
								_ = retryResp.Body.Close()
								respBody = retryBody
								resp = &http.Response{
									StatusCode: retryResp.StatusCode,
									Header:     retryResp.Header.Clone(),
									Body:       io.NopCloser(bytes.NewReader(retryBody)),
								}
							}
						} else {
							logger.LegacyPrintf("service.antigravity_gateway", "Antigravity account %d: budget rectifier retry failed: %v", account.ID, retryErr)
						}
					}
				}
			}
		}

		if resp.StatusCode >= 400 {
			if resp.StatusCode == http.StatusBadRequest && isPromptTooLongError(respBody) {
				upstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
				upstreamDetail := s.getUpstreamErrorDetail(respBody)
				logBody, maxBytes := s.getLogConfig()
				if logBody {
					logger.LegacyPrintf("service.antigravity_gateway", "%s status=400 prompt_too_long=true upstream_message=%q request_id=%s body=%s", st.prefix, upstreamMsg, resp.Header.Get("x-request-id"), truncateForLog(respBody, maxBytes))
				}
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					Kind:               "prompt_too_long",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				return &PromptTooLongError{
					StatusCode: resp.StatusCode,
					RequestID:  resp.Header.Get("x-request-id"),
					Body:       respBody,
				}
			}

			s.handleUpstreamError(ctx, st.prefix, account, resp.StatusCode, resp.Header, respBody, st.originalModel, 0, "", st.isStickySession)

			if resp.StatusCode == http.StatusBadRequest {
				msg := strings.ToLower(strings.TrimSpace(extractAntigravityErrorMessage(respBody)))
				if isGoogleProjectConfigError(msg) {
					upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractAntigravityErrorMessage(respBody)))
					upstreamDetail := s.getUpstreamErrorDetail(respBody)
					log.Printf("%s status=400 google_config_error failover=true upstream_message=%q account=%d", st.prefix, upstreamMsg, account.ID)
					appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
						Platform:           account.Platform,
						AccountID:          account.ID,
						AccountName:        account.Name,
						UpstreamStatusCode: resp.StatusCode,
						UpstreamRequestID:  resp.Header.Get("x-request-id"),
						Kind:               "failover",
						Message:            upstreamMsg,
						Detail:             upstreamDetail,
					})
					return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody, RetryableOnSameAccount: true}
				}
			}

			if s.shouldFailoverUpstreamError(resp.StatusCode) {
				upstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(respBody))
				upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
				upstreamDetail := s.getUpstreamErrorDetail(respBody)
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					Kind:               "failover",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})
				return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
			}

			return s.writeMappedClaudeError(c, account, resp.StatusCode, resp.Header.Get("x-request-id"), respBody)
		}
	}

	st.resp = resp
	closeResp = false
	return nil
}

func (s *AntigravityGatewayService) forwardClaudeRelay(ex *pipeline.Exchange) error {
	st := antigravityForwardStateFrom(ex)
	if st == nil || st.resp == nil {
		return &antigravityClientError{
			Status:  http.StatusBadGateway,
			Type:    "upstream_error",
			Message: "Upstream request failed after retries",
		}
	}
	resp := st.resp
	defer func() { _ = resp.Body.Close() }()

	requestID := resp.Header.Get("x-request-id")
	if requestID != "" {
		st.c.Header("x-request-id", requestID)
	}
	st.requestID = requestID

	if st.claudeReq.Stream {
		streamRes, err := s.handleClaudeStreamingResponse(st.c, resp, st.startTime, st.originalModel)
		if err != nil {
			logger.LegacyPrintf("service.antigravity_gateway", "%s status=stream_error error=%v", st.prefix, err)
			return err
		}
		st.usage = streamRes.usage
		st.firstTokenMs = streamRes.firstTokenMs
		st.clientDisconnect = streamRes.clientDisconnect
		return nil
	}

	streamRes, err := s.handleClaudeStreamToNonStreaming(st.c, resp, st.startTime, st.originalModel)
	if err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=stream_collect_error error=%v", st.prefix, err)
		return err
	}
	st.usage = streamRes.usage
	st.firstTokenMs = streamRes.firstTokenMs
	return nil
}

func (s *AntigravityGatewayService) forwardClaudeSettle(ex *pipeline.Exchange) error {
	st := antigravityForwardStateFrom(ex)
	if st == nil || st.usage == nil {
		return &antigravityClientError{
			Status:  http.StatusBadGateway,
			Type:    "upstream_error",
			Message: "Upstream request failed after retries",
		}
	}
	st.result = &ForwardResult{
		RequestID:        st.requestID,
		Usage:            *st.usage,
		Model:            st.originalModel,
		UpstreamModel:    st.billingModel,
		Stream:           st.claudeReq.Stream,
		Duration:         time.Since(st.startTime),
		FirstTokenMs:     st.firstTokenMs,
		ClientDisconnect: st.clientDisconnect,
	}
	return nil
}
