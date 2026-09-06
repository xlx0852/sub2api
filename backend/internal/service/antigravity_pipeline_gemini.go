package service

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

func (s *AntigravityGatewayService) forwardGeminiParse(ex *pipeline.Exchange) error {
	st := antigravityForwardStateFrom(ex)
	if st == nil || st.c == nil {
		return googleClientError(http.StatusBadRequest, "Invalid request body")
	}
	if strings.TrimSpace(st.originalModel) == "" {
		return googleClientError(http.StatusBadRequest, "Missing model in URL")
	}
	// This deployment intentionally supports text Gemini models only.  Keep the
	// boundary check here so image models cannot reach mapping, billing, or the
	// upstream retry path.
	modelLower := strings.ToLower(strings.TrimSpace(st.originalModel))
	if !strings.HasPrefix(modelLower, "gemini-") || strings.Contains(modelLower, "image") || strings.Contains(modelLower, "imagen") {
		return googleClientError(http.StatusBadRequest, "Unsupported Antigravity model: text Gemini models only")
	}
	if strings.TrimSpace(st.action) == "" {
		return googleClientError(http.StatusBadRequest, "Missing action in URL")
	}
	if len(ex.Inbound) == 0 {
		return googleClientError(http.StatusBadRequest, "Request body is empty")
	}

	ex.RequestedModel = st.originalModel

	switch st.action {
	case "generateContent", "streamGenerateContent":
		return nil
	case "countTokens":
		st.c.JSON(http.StatusOK, map[string]any{"totalTokens": 0})
		st.result = &ForwardResult{
			RequestID:    "",
			Usage:        ClaudeUsage{},
			Model:        st.originalModel,
			Stream:       false,
			Duration:     time.Since(st.startTime),
			FirstTokenMs: nil,
		}
		st.skipRemaining = true
		return nil
	default:
		return googleClientError(http.StatusNotFound, "Unsupported action: "+st.action)
	}
}

func (s *AntigravityGatewayService) forwardGeminiRewrite(ex *pipeline.Exchange) error {
	st := antigravityForwardStateFrom(ex)
	if st == nil {
		return googleClientError(http.StatusBadRequest, "Invalid request body")
	}
	if st.skipRemaining {
		return nil
	}
	mappedModel := s.getMappedModel(ex.Account, st.originalModel)
	if mappedModel == "" {
		return &antigravityClientError{
			Status:  http.StatusForbidden,
			Message: fmt.Sprintf("model %s not in whitelist", st.originalModel),
			MarkOps: true,
		}
	}
	st.mappedModel = mappedModel
	st.billingModel = mappedModel
	ex.UpstreamModel = mappedModel

	injectedBody, err := injectIdentityPatchToGeminiRequest(ex.Inbound)
	if err != nil {
		return googleClientError(http.StatusBadRequest, "Invalid request body")
	}
	if cleanedBody, cleanErr := cleanGeminiRequest(injectedBody); cleanErr == nil {
		injectedBody = cleanedBody
		logger.LegacyPrintf("service.antigravity_gateway", "[Antigravity] Cleaned request schema in forwarded request for account %s", ex.Account.Name)
	} else {
		logger.LegacyPrintf("service.antigravity_gateway", "[Antigravity] Failed to clean schema: %v", cleanErr)
	}
	st.injectedBody = injectedBody
	return nil
}

func (s *AntigravityGatewayService) forwardGeminiTransport(ex *pipeline.Exchange) error {
	st := antigravityForwardStateFrom(ex)
	if st == nil || ex == nil || ex.Account == nil {
		return googleClientError(http.StatusBadRequest, "Invalid request body")
	}
	if st.skipRemaining {
		return nil
	}

	ctx := ex.Ctx
	account := ex.Account
	c := st.c

	if s.tokenProvider == nil {
		return s.writeGoogleError(c, http.StatusBadGateway, "Antigravity token provider not configured")
	}
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return &UpstreamFailoverError{
			StatusCode:   http.StatusBadGateway,
			ResponseBody: []byte(`{"error":{"message":"Failed to get upstream access token","status":"UNAVAILABLE"}}`),
		}
	}
	st.accessToken = accessToken

	projectID, err := resolveAntigravityProjectID(account)
	if err != nil {
		_ = s.writeGoogleError(c, http.StatusBadRequest, err.Error())
		return err
	}
	st.projectID = projectID

	if account.ProxyID != nil && account.Proxy != nil {
		st.proxyURL = account.Proxy.URL()
	}

	wrappedBody, err := s.wrapV1InternalRequest(projectID, st.mappedModel, st.injectedBody)
	if err != nil {
		return s.writeGoogleError(c, http.StatusInternalServerError, "Failed to build upstream request")
	}
	st.wrappedBody = wrappedBody
	st.upstreamAction = "streamGenerateContent"

	result, err := s.antigravityRetryLoop(antigravityRetryLoopParams{
		ctx:             ctx,
		prefix:          st.prefix,
		account:         account,
		proxyURL:        st.proxyURL,
		accessToken:     accessToken,
		action:          st.upstreamAction,
		body:            wrappedBody,
		c:               c,
		httpUpstream:    s.httpUpstream,
		settingService:  s.settingService,
		accountRepo:     s.accountRepo,
		handleError:     s.handleUpstreamError,
		requestedModel:  st.originalModel,
		isStickySession: st.isStickySession,
		groupID:         st.groupID,
		sessionHash:     st.sessionHash,
	})
	if err != nil {
		if switchErr, ok := IsAntigravityAccountSwitchError(err); ok {
			return &UpstreamFailoverError{
				StatusCode:        http.StatusServiceUnavailable,
				ForceCacheBilling: switchErr.IsStickySession,
			}
		}
		if c.Request.Context().Err() != nil {
			return s.writeGoogleError(c, http.StatusBadGateway, "Client disconnected before upstream response")
		}
		return s.writeGoogleError(c, http.StatusBadGateway, "Upstream request failed after retries")
	}

	resp := result.resp
	closeResp := true
	defer func() {
		if closeResp && resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		contentType := resp.Header.Get("Content-Type")
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		if s.settingService != nil && s.settingService.IsModelFallbackEnabled(ctx) &&
			isModelNotFoundError(resp.StatusCode, respBody) {
			fallbackModel := s.settingService.GetFallbackModel(ctx, PlatformAntigravity)
			if fallbackModel != "" && fallbackModel != st.mappedModel {
				logger.LegacyPrintf("service.antigravity_gateway", "[Antigravity] Model not found (%s), retrying with fallback model %s (account: %s)", st.mappedModel, fallbackModel, account.Name)

				fallbackWrapped, wrapErr := s.wrapV1InternalRequest(projectID, fallbackModel, st.injectedBody)
				if wrapErr == nil {
					fallbackReq, reqErr := antigravity.NewAPIRequest(ctx, st.upstreamAction, accessToken, fallbackWrapped)
					if reqErr == nil {
						fallbackResp, doErr := s.httpUpstream.Do(fallbackReq, st.proxyURL, account.ID, account.Concurrency)
						if doErr == nil && fallbackResp.StatusCode < 400 {
							_ = resp.Body.Close()
							resp = fallbackResp
						} else if fallbackResp != nil {
							_ = fallbackResp.Body.Close()
						}
					}
				}
			}
		}

		signatureCheckBody := respBody
		if unwrapped, unwrapErr := s.unwrapV1InternalResponse(respBody); unwrapErr == nil && len(unwrapped) > 0 {
			signatureCheckBody = unwrapped
		}
		if resp.StatusCode == http.StatusBadRequest &&
			s.settingService != nil &&
			s.settingService.IsSignatureRectifierEnabled(ctx) &&
			isSignatureRelatedError(signatureCheckBody) &&
			bytes.Contains(st.injectedBody, []byte(`"thoughtSignature"`)) {
			upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractAntigravityErrorMessage(signatureCheckBody)))
			upstreamDetail := s.getUpstreamErrorDetail(signatureCheckBody)
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

			logger.LegacyPrintf("service.antigravity_gateway", "Antigravity Gemini account %d: detected signature-related 400, retrying with cleaned thought signatures", account.ID)

			cleanedInjectedBody := CleanGeminiNativeThoughtSignatures(st.injectedBody)
			retryWrappedBody, wrapErr := s.wrapV1InternalRequest(projectID, st.mappedModel, cleanedInjectedBody)
			if wrapErr == nil {
				retryResult, retryErr := s.antigravityRetryLoop(antigravityRetryLoopParams{
					ctx:             ctx,
					prefix:          st.prefix,
					account:         account,
					proxyURL:        st.proxyURL,
					accessToken:     accessToken,
					action:          st.upstreamAction,
					body:            retryWrappedBody,
					c:               c,
					httpUpstream:    s.httpUpstream,
					settingService:  s.settingService,
					accountRepo:     s.accountRepo,
					handleError:     s.handleUpstreamError,
					requestedModel:  st.originalModel,
					isStickySession: st.isStickySession,
					groupID:         st.groupID,
					sessionHash:     st.sessionHash,
				})
				if retryErr == nil {
					retryResp := retryResult.resp
					if retryResp.StatusCode < 400 {
						resp = retryResp
					} else {
						retryRespBody := s.readUpstreamErrorBody(retryResp)
						_ = retryResp.Body.Close()
						retryOpsBody := retryRespBody
						if retryUnwrapped, unwrapErr := s.unwrapV1InternalResponse(retryRespBody); unwrapErr == nil && len(retryUnwrapped) > 0 {
							retryOpsBody = retryUnwrapped
						}
						appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
							Platform:           account.Platform,
							AccountID:          account.ID,
							AccountName:        account.Name,
							UpstreamStatusCode: retryResp.StatusCode,
							UpstreamRequestID:  retryResp.Header.Get("x-request-id"),
							Kind:               "signature_retry",
							Message:            sanitizeUpstreamErrorMessage(strings.TrimSpace(extractAntigravityErrorMessage(retryOpsBody))),
							Detail:             s.getUpstreamErrorDetail(retryOpsBody),
						})
						respBody = retryRespBody
						resp = &http.Response{
							StatusCode: retryResp.StatusCode,
							Header:     retryResp.Header.Clone(),
							Body:       io.NopCloser(bytes.NewReader(retryRespBody)),
						}
						contentType = resp.Header.Get("Content-Type")
					}
				} else {
					if switchErr, ok := IsAntigravityAccountSwitchError(retryErr); ok {
						appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
							Platform:           account.Platform,
							AccountID:          account.ID,
							AccountName:        account.Name,
							UpstreamStatusCode: http.StatusServiceUnavailable,
							Kind:               "failover",
							Message:            sanitizeUpstreamErrorMessage(retryErr.Error()),
						})
						return &UpstreamFailoverError{
							StatusCode:        http.StatusServiceUnavailable,
							ForceCacheBilling: switchErr.IsStickySession,
						}
					}
					appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
						Platform:           account.Platform,
						AccountID:          account.ID,
						AccountName:        account.Name,
						UpstreamStatusCode: 0,
						Kind:               "signature_retry_request_error",
						Message:            sanitizeUpstreamErrorMessage(retryErr.Error()),
					})
					logger.LegacyPrintf("service.antigravity_gateway", "Antigravity Gemini account %d: signature retry request failed: %v", account.ID, retryErr)
				}
			} else {
				logger.LegacyPrintf("service.antigravity_gateway", "Antigravity Gemini account %d: signature retry wrap failed: %v", account.ID, wrapErr)
			}
		}

		if resp.StatusCode < 400 {
			st.resp = resp
			closeResp = false
			return nil
		}

		requestID := resp.Header.Get("x-request-id")
		if requestID != "" {
			c.Header("x-request-id", requestID)
		}

		unwrapped, unwrapErr := s.unwrapV1InternalResponse(respBody)
		unwrappedForOps := unwrapped
		if unwrapErr != nil || len(unwrappedForOps) == 0 {
			unwrappedForOps = respBody
		}
		s.handleUpstreamError(ctx, st.prefix, account, resp.StatusCode, resp.Header, respBody, st.originalModel, st.groupID, st.sessionHash, st.isStickySession)
		upstreamMsg := strings.TrimSpace(extractAntigravityErrorMessage(unwrappedForOps))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		upstreamDetail := s.getUpstreamErrorDetail(unwrappedForOps)

		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

		if resp.StatusCode == http.StatusBadRequest && isGoogleProjectConfigError(strings.ToLower(upstreamMsg)) {
			log.Printf("%s status=400 google_config_error failover=true upstream_message=%q account=%d", st.prefix, upstreamMsg, account.ID)
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
			return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: unwrappedForOps, RetryableOnSameAccount: true}
		}

		if s.shouldFailoverUpstreamError(resp.StatusCode) {
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
			return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: unwrappedForOps}
		}
		if contentType == "" {
			contentType = "application/json"
		}
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
		logger.LegacyPrintf("service.antigravity_gateway", "[antigravity-Forward] upstream error status=%d body=%s", resp.StatusCode, truncateForLog(unwrappedForOps, 500))
		MarkResponseCommitted(c)
		c.Data(resp.StatusCode, contentType, unwrappedForOps)
		return fmt.Errorf("antigravity upstream error: %d", resp.StatusCode)
	}

	st.resp = resp
	closeResp = false
	return nil
}

func (s *AntigravityGatewayService) forwardGeminiRelay(ex *pipeline.Exchange) error {
	st := antigravityForwardStateFrom(ex)
	if st == nil {
		return googleClientError(http.StatusBadGateway, "Upstream request failed after retries")
	}
	if st.skipRemaining {
		return nil
	}
	if st.resp == nil {
		return googleClientError(http.StatusBadGateway, "Upstream request failed after retries")
	}
	resp := st.resp
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	requestID := resp.Header.Get("x-request-id")
	if requestID != "" {
		st.c.Header("x-request-id", requestID)
	}
	st.requestID = requestID

	if st.stream {
		streamRes, err := s.handleGeminiStreamingResponse(st.c, resp, st.startTime)
		if err != nil {
			logger.LegacyPrintf("service.antigravity_gateway", "%s status=stream_error error=%v", st.prefix, err)
			return err
		}
		st.usage = streamRes.usage
		st.firstTokenMs = streamRes.firstTokenMs
		st.clientDisconnect = streamRes.clientDisconnect
		return nil
	}

	streamRes, err := s.handleGeminiStreamToNonStreaming(st.c, resp, st.startTime)
	if err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=stream_collect_error error=%v", st.prefix, err)
		return err
	}
	st.usage = streamRes.usage
	st.firstTokenMs = streamRes.firstTokenMs
	return nil
}

func (s *AntigravityGatewayService) forwardGeminiSettle(ex *pipeline.Exchange) error {
	st := antigravityForwardStateFrom(ex)
	if st == nil {
		return googleClientError(http.StatusBadGateway, "Upstream request failed after retries")
	}
	if st.skipRemaining {
		return nil
	}
	if st.usage == nil {
		st.usage = &ClaudeUsage{}
	}
	st.result = &ForwardResult{
		RequestID:        st.requestID,
		Usage:            *st.usage,
		Model:            st.originalModel,
		UpstreamModel:    st.billingModel,
		Stream:           st.stream,
		Duration:         time.Since(st.startTime),
		FirstTokenMs:     st.firstTokenMs,
		ClientDisconnect: st.clientDisconnect,
	}
	return nil
}

func applyForwardGeminiOptions(options []ForwardGeminiOption) forwardGeminiOptions {
	forwardOpts := forwardGeminiOptions{}
	for _, apply := range options {
		if apply != nil {
			apply(&forwardOpts)
		}
	}
	return forwardOpts
}

func newGeminiForwardState(c *gin.Context, account *Account, originalModel, action string, stream, isStickySession bool, options []ForwardGeminiOption) *antigravityForwardState {
	opts := applyForwardGeminiOptions(options)
	return &antigravityForwardState{
		c:               c,
		isStickySession: isStickySession,
		startTime:       time.Now(),
		prefix:          logPrefix(getSessionID(c), account.Name),
		originalModel:   originalModel,
		action:          action,
		stream:          stream,
		groupID:         opts.groupID,
		sessionHash:     opts.sessionHash,
	}
}
