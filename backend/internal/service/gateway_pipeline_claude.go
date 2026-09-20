package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

func (s *GatewayService) forwardClaudeParse(ex *pipeline.Exchange) error {
	st := claudeForwardStateFrom(ex)
	if st == nil || st.parsed == nil {
		return fmt.Errorf("parse request: empty request")
	}
	st.body = st.parsed.Body.Bytes()
	st.reqModel = st.parsed.Model
	st.reqStream = st.parsed.Stream
	st.originalModel = st.reqModel
	st.mappedModel = st.reqModel
	ex.RequestedModel = st.reqModel
	ex.Inbound = st.body
	return nil
}

func (s *GatewayService) forwardClaudeRewrite(ex *pipeline.Exchange) error {
	st := claudeForwardStateFrom(ex)
	if st == nil || st.parsed == nil {
		return fmt.Errorf("parse request: empty request")
	}
	account := ex.Account
	c := st.c

	// Beta policy: evaluate once; block check + cache filter set for buildUpstreamRequest.
	// Always overwrite the cache to prevent stale values from a previous retry with a different account.
	if account.Platform == PlatformAnthropic && c != nil {
		policy := s.evaluateBetaPolicy(ex.Ctx, c.GetHeader("anthropic-beta"), account, st.parsed.Model)
		if policy.blockErr != nil {
			return policy.blockErr
		}
		filterSet := policy.filterSet
		if filterSet == nil {
			filterSet = map[string]struct{}{}
		}
		c.Set(betaPolicyFilterSetKey, filterSet)
	}

	if c != nil {
		s.debugLogGatewaySnapshot("CLIENT_ORIGINAL", c.Request.Header, st.body, map[string]string{
			"account":      fmt.Sprintf("%d(%s)", account.ID, account.Name),
			"account_type": string(account.Type),
			"model":        st.reqModel,
			"stream":       strconv.FormatBool(st.reqStream),
		})
	}

	// Claude OAuth CLI 拟态已退役：不再为第三方客户端伪造 Claude Code system/billing block。
	// API Key 透传、模型映射、cache_control 上限和 dateline 归一化仍然生效。
	st.shouldMimicClaudeCode = false

	// 客户端 dateline 归一化：仅对 Anthropic OAuth/SetupToken 账号生效。
	// 抹除 "Today's date is …" 语句里可能被注入的隐写指纹（4 种撇号 × 2 种日期
	// 分隔符），还原为 ASCII 撇号 + "-" 分隔符。运行在 mimicry 分支之外，
	// 保证真实 Claude Code 客户端注入的指纹同样被清洗。
	if next, ok := s.normalizeClientDatelineIfEnabled(ex.Ctx, account, st.body); ok {
		if err := st.replaceBody(next); err != nil {
			return err
		}
	}

	// 强制执行 cache_control 块数量限制（最多 4 个）
	if err := st.replaceBody(enforceCacheControlLimit(st.body)); err != nil {
		return err
	}

	// 应用模型映射：
	// - APIKey 账号：使用账号级别的显式映射（如果配置），否则透传原始模型名
	// - OAuth/SetupToken 账号：使用 Anthropic 标准映射（短ID → 长ID）
	st.mappedModel = st.reqModel
	mappingSource := ""
	if account.Type == AccountTypeAPIKey {
		st.mappedModel = account.GetMappedModel(st.reqModel)
		if st.mappedModel != st.reqModel {
			mappingSource = "account"
		}
	}
	if mappingSource == "" && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		normalized := claude.NormalizeModelID(st.reqModel)
		if normalized != st.reqModel {
			st.mappedModel = normalized
			mappingSource = "prefix"
		}
	}
	if st.mappedModel != st.reqModel {
		if err := st.replaceBody(s.replaceModelInBody(st.body, st.mappedModel)); err != nil {
			return err
		}
		st.reqModel = st.mappedModel
		st.parsed.Model = st.mappedModel
		logger.LegacyPrintf("service.gateway", "Model mapping applied: %s -> %s (account: %s, source=%s)", st.originalModel, st.mappedModel, account.Name, mappingSource)
	}

	if s.shouldInjectAnthropicCacheTTL1h(ex.Ctx, account) {
		if err := st.replaceBody(injectAnthropicCacheControlTTL1h(st.body)); err != nil {
			return err
		}
	}

	// Pre-filter: strip empty text blocks (including nested in tool_result) to prevent upstream 400.
	if err := st.replaceBody(StripEmptyTextBlocks(st.body)); err != nil {
		return err
	}
	// Pre-filter: strip web-search history blocks the upstream cannot accept
	// (emulation-synthesized server_tool_use / web_search_tool_result always;
	// genuine ones additionally for passback-required upstreams). See
	// FilterWebSearchHistoryBlocks. reqModel 此时已是映射后的模型 ID。
	if err := st.replaceBody(FilterWebSearchHistoryBlocks(st.body, st.reqModel)); err != nil {
		return err
	}
	// Pre-filter: remove thinking blocks with missing/invalid signatures before forwarding.
	// Clients (e.g. Claude Code) sometimes send multi-turn conversations where a historical
	// assistant message contains a thinking block that is missing the required "signature" field,
	// causing upstream to reject the request with 400 "thinking.signature: Field required".
	// FilterThinkingBlocks removes only the invalid blocks; thinking blocks with valid signatures
	// are preserved. This avoids relying solely on the post-error retry path, which can time out
	// (maxRetryElapsed = 10s) for long conversations before the retry budget is exhausted.
	//
	// 仅 anthropic-strict 模型族执行此过滤；passback-required 上游 (DeepSeek/Kimi/GLM 等)
	// 要求历史 thinking block 原样回传，过滤反而制造 400。reqModel 此时已是映射后的模型 ID。
	if err := st.replaceBody(FilterThinkingBlocks(st.body, st.reqModel)); err != nil {
		return err
	}
	// Chinese LLM thinking.type 协议差异补正（如 MiniMax 只接受 adaptive；Anthropic-SDK
	// 客户端默认发 enabled）。仅对 passback-required 上游生效（claude-* 不会进来）。
	if ResolveThinkingProtocol(st.reqModel) == ThinkingProtocolPassbackRequired {
		if rewritten, applied := NormalizeChineseLLMThinking(st.body, st.reqModel); applied {
			if err := st.replaceBody(rewritten); err != nil {
				return err
			}
			logger.LegacyPrintf("service.gateway", "Account %d: rewrote thinking.type for %s (Anthropic-SDK default 'enabled' -> vendor-specific)", account.ID, st.reqModel)
		}
	}

	ex.UpstreamModel = st.mappedModel
	ex.Inbound = st.body
	return nil
}

func (s *GatewayService) forwardClaudeTransport(ex *pipeline.Exchange) error {
	st := claudeForwardStateFrom(ex)
	if st == nil || st.parsed == nil {
		return fmt.Errorf("parse request: empty request")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return err
	}
	st.token = token
	st.tokenType = tokenType

	st.proxyURL = ""
	if account.ProxyID != nil && account.Proxy != nil {
		if !account.IsCustomBaseURLEnabled() || account.GetCustomBaseURL() == "" {
			st.proxyURL = account.Proxy.URL()
		}
	}

	st.tlsProfile = s.tlsFPProfileService.ResolveTLSProfile(account)

	logger.LegacyPrintf("service.gateway", "[Forward] Using account: ID=%d Name=%s Platform=%s Type=%s TLSFingerprint=%v Proxy=%s",
		account.ID, account.Name, account.Platform, account.Type, st.tlsProfile, st.proxyURL)

	var resp *http.Response
	st.lastWireBody = st.body
	retryStart := time.Now()
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, st.reqStream)
		upstreamReq, wireBody, buildErr := s.buildUpstreamRequest(upstreamCtx, c, account, st.body, st.token, st.tokenType, st.reqModel, st.reqStream, st.shouldMimicClaudeCode)
		releaseUpstreamCtx()
		if buildErr != nil {
			return buildErr
		}
		st.lastWireBody = wireBody

		resp, err = s.httpUpstream.DoWithTLS(upstreamReq, st.proxyURL, account.ID, account.Concurrency, st.tlsProfile)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			if errors.Is(err, context.Canceled) || (c != nil && c.Request != nil && c.Request.Context().Err() != nil) {
				return err
			}
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			setOpsUpstreamError(c, 0, safeErr, "")
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: 0,
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "request_error",
				Message:            safeErr,
			})
			c.JSON(http.StatusBadGateway, gin.H{
				"type": "error",
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream request failed",
				},
			})
			return fmt.Errorf("upstream request failed: %s", safeErr)
		}

		if resp.StatusCode == 400 {
			respBody, readErr := s.readUpstreamErrorBody(resp)
			if readErr == nil {
				_ = resp.Body.Close()

				if s.shouldRectifySignatureError(ctx, account, respBody, st.reqModel) {
					appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
						Platform:           account.Platform,
						AccountID:          account.ID,
						AccountName:        account.Name,
						UpstreamStatusCode: resp.StatusCode,
						UpstreamRequestID:  resp.Header.Get("x-request-id"),
						UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
						Kind:               "signature_error",
						Message:            extractUpstreamErrorMessage(respBody),
						Detail: func() string {
							if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
								return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
							}
							return ""
						}(),
					})

					looksLikeToolSignatureError := func(msg string) bool {
						m := strings.ToLower(msg)
						return strings.Contains(m, "tool_use") ||
							strings.Contains(m, "tool_result") ||
							strings.Contains(m, "functioncall") ||
							strings.Contains(m, "function_call") ||
							strings.Contains(m, "functionresponse") ||
							strings.Contains(m, "function_response")
					}

					if time.Since(retryStart) >= maxRetryElapsed {
						resp.Body = io.NopCloser(bytes.NewReader(respBody))
						break
					}
					logger.LegacyPrintf("service.gateway", "[warn] Account %d: thinking blocks have invalid signature, retrying with filtered blocks", account.ID)

					filteredBody := FilterThinkingBlocksForRetry(st.body, st.reqModel)
					retryCtx, releaseRetryCtx := detachStreamUpstreamContext(ctx, st.reqStream)
					retryReq, retryWireBody, buildErr := s.buildUpstreamRequest(retryCtx, c, account, filteredBody, st.token, st.tokenType, st.reqModel, st.reqStream, st.shouldMimicClaudeCode)
					releaseRetryCtx()
					if buildErr == nil {
						retryResp, retryErr := s.httpUpstream.DoWithTLS(retryReq, st.proxyURL, account.ID, account.Concurrency, st.tlsProfile)
						if retryErr == nil {
							if retryResp.StatusCode < 400 {
								st.lastWireBody = retryWireBody
								if err := st.replaceBody(retryWireBody); err != nil {
									_ = retryResp.Body.Close()
									return err
								}
								logger.LegacyPrintf("service.gateway", "Account %d: thinking block retry succeeded (blocks downgraded)", account.ID)
								resp = retryResp
								break
							}

							retryRespBody, retryReadErr := s.readUpstreamErrorBody(retryResp)
							_ = retryResp.Body.Close()
							if retryReadErr == nil && retryResp.StatusCode == 400 && s.isSignatureErrorPattern(ctx, account, retryRespBody) {
								appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
									Platform:           account.Platform,
									AccountID:          account.ID,
									AccountName:        account.Name,
									UpstreamStatusCode: retryResp.StatusCode,
									UpstreamRequestID:  retryResp.Header.Get("x-request-id"),
									UpstreamURL:        safeUpstreamURL(retryReq.URL.String()),
									Kind:               "signature_retry_thinking",
									Message:            extractUpstreamErrorMessage(retryRespBody),
									Detail: func() string {
										if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
											return truncateString(string(retryRespBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
										}
										return ""
									}(),
								})
								msg2 := extractUpstreamErrorMessage(retryRespBody)
								if looksLikeToolSignatureError(msg2) && time.Since(retryStart) < maxRetryElapsed {
									logger.LegacyPrintf("service.gateway", "Account %d: signature retry still failing and looks tool-related, retrying with tool blocks downgraded", account.ID)
									filteredBody2 := FilterSignatureSensitiveBlocksForRetry(st.body, st.reqModel)
									retryCtx2, releaseRetryCtx2 := detachStreamUpstreamContext(ctx, st.reqStream)
									retryReq2, retryWireBody2, buildErr2 := s.buildUpstreamRequest(retryCtx2, c, account, filteredBody2, st.token, st.tokenType, st.reqModel, st.reqStream, st.shouldMimicClaudeCode)
									releaseRetryCtx2()
									if buildErr2 == nil {
										retryResp2, retryErr2 := s.httpUpstream.DoWithTLS(retryReq2, st.proxyURL, account.ID, account.Concurrency, st.tlsProfile)
										if retryErr2 == nil {
											if retryResp2.StatusCode < 400 {
												st.lastWireBody = retryWireBody2
												if err := st.replaceBody(retryWireBody2); err != nil {
													_ = retryResp2.Body.Close()
													return err
												}
											}
											resp = retryResp2
											break
										}
										if retryResp2 != nil && retryResp2.Body != nil {
											_ = retryResp2.Body.Close()
										}
										appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
											Platform:           account.Platform,
											AccountID:          account.ID,
											AccountName:        account.Name,
											UpstreamStatusCode: 0,
											UpstreamURL:        safeUpstreamURL(retryReq2.URL.String()),
											Kind:               "signature_retry_tools_request_error",
											Message:            sanitizeUpstreamErrorMessage(retryErr2.Error()),
										})
										logger.LegacyPrintf("service.gateway", "Account %d: tool-downgrade signature retry failed: %v", account.ID, retryErr2)
									} else {
										logger.LegacyPrintf("service.gateway", "Account %d: tool-downgrade signature retry build failed: %v", account.ID, buildErr2)
									}
								}
							}

							resp = &http.Response{
								StatusCode: retryResp.StatusCode,
								Header:     retryResp.Header.Clone(),
								Body:       io.NopCloser(bytes.NewReader(retryRespBody)),
							}
							break
						}
						if retryResp != nil && retryResp.Body != nil {
							_ = retryResp.Body.Close()
						}
						logger.LegacyPrintf("service.gateway", "Account %d: signature error retry failed: %v", account.ID, retryErr)
					} else {
						logger.LegacyPrintf("service.gateway", "Account %d: signature error retry build request failed: %v", account.ID, buildErr)
					}

					resp.Body = io.NopCloser(bytes.NewReader(respBody))
					break
				}
				errMsg := extractUpstreamErrorMessage(respBody)
				if isThinkingBudgetConstraintError(errMsg) && s.settingService.IsBudgetRectifierEnabled(ctx) {
					appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
						Platform:           account.Platform,
						AccountID:          account.ID,
						AccountName:        account.Name,
						UpstreamStatusCode: resp.StatusCode,
						UpstreamRequestID:  resp.Header.Get("x-request-id"),
						UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
						Kind:               "budget_constraint_error",
						Message:            errMsg,
						Detail: func() string {
							if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
								return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
							}
							return ""
						}(),
					})

					rectifiedBody, applied := RectifyThinkingBudget(st.body)
					if applied && time.Since(retryStart) < maxRetryElapsed {
						logger.LegacyPrintf("service.gateway", "Account %d: detected budget_tokens constraint error, retrying with rectified budget (budget_tokens=%d, max_tokens=%d)", account.ID, BudgetRectifyBudgetTokens, BudgetRectifyMaxTokens)
						budgetRetryCtx, releaseBudgetRetryCtx := detachStreamUpstreamContext(ctx, st.reqStream)
						budgetRetryReq, budgetWireBody, buildErr := s.buildUpstreamRequest(budgetRetryCtx, c, account, rectifiedBody, st.token, st.tokenType, st.reqModel, st.reqStream, st.shouldMimicClaudeCode)
						releaseBudgetRetryCtx()
						if buildErr == nil {
							budgetRetryResp, retryErr := s.httpUpstream.DoWithTLS(budgetRetryReq, st.proxyURL, account.ID, account.Concurrency, st.tlsProfile)
							if retryErr == nil {
								if budgetRetryResp.StatusCode < 400 {
									st.lastWireBody = budgetWireBody
									if err := st.replaceBody(budgetWireBody); err != nil {
										_ = budgetRetryResp.Body.Close()
										return err
									}
								}
								resp = budgetRetryResp
								break
							}
							if budgetRetryResp != nil && budgetRetryResp.Body != nil {
								_ = budgetRetryResp.Body.Close()
							}
							logger.LegacyPrintf("service.gateway", "Account %d: budget rectifier retry failed: %v", account.ID, retryErr)
						} else {
							logger.LegacyPrintf("service.gateway", "Account %d: budget rectifier retry build failed: %v", account.ID, buildErr)
						}
					}
				}

				resp.Body = io.NopCloser(bytes.NewReader(respBody))
			}
		}

		if resp.StatusCode >= 400 && resp.StatusCode != 400 && s.shouldRetryUpstreamError(account, resp.StatusCode) {
			if attempt < maxRetryAttempts {
				elapsed := time.Since(retryStart)
				if elapsed >= maxRetryElapsed {
					break
				}

				delay := retryBackoffDelay(attempt)
				remaining := maxRetryElapsed - elapsed
				if delay > remaining {
					delay = remaining
				}
				if delay <= 0 {
					break
				}

				respBody, _ := s.readUpstreamErrorBody(resp)
				_ = resp.Body.Close()
				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: resp.StatusCode,
					UpstreamRequestID:  resp.Header.Get("x-request-id"),
					UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
					Kind:               "retry",
					Message:            extractUpstreamErrorMessage(respBody),
					Detail: func() string {
						if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
							return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
						}
						return ""
					}(),
				})
				logger.LegacyPrintf("service.gateway", "Account %d: upstream error %d, retry %d/%d after %v (elapsed=%v/%v)",
					account.ID, resp.StatusCode, attempt, maxRetryAttempts, delay, elapsed, maxRetryElapsed)
				if err := sleepWithContext(ctx, delay); err != nil {
					return err
				}
				continue
			}
			break
		}

		if account.Platform == PlatformGemini && resp.StatusCode < 400 && s.cfg != nil && s.cfg.Gateway.GeminiDebugResponseHeaders {
			logger.LegacyPrintf("service.gateway", "[DEBUG] Gemini API Response Headers for account %d:", account.ID)
			for k, v := range resp.Header {
				logger.LegacyPrintf("service.gateway", "[DEBUG]   %s: %v", k, v)
			}
		}
		break
	}
	st.resp = resp
	if st.resp == nil || st.resp.Body == nil {
		return errors.New("upstream request failed: empty response")
	}

	if st.resp.StatusCode >= 400 && s.shouldRetryUpstreamError(account, st.resp.StatusCode) {
		if s.shouldFailoverUpstreamError(st.resp.StatusCode) {
			respBody, _ := s.readUpstreamErrorBody(st.resp)
			_ = st.resp.Body.Close()
			st.resp.Body = io.NopCloser(bytes.NewReader(respBody))

			logger.LegacyPrintf("service.gateway", "[Forward] Upstream error (retry exhausted, failover): Account=%d(%s) Status=%d RequestID=%s Body=%s",
				account.ID, account.Name, st.resp.StatusCode, st.resp.Header.Get("x-request-id"), truncateString(string(respBody), 1000))

			s.handleRetryExhaustedSideEffects(ctx, st.resp, account)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: st.resp.StatusCode,
				UpstreamRequestID:  st.resp.Header.Get("x-request-id"),
				Kind:               "retry_exhausted_failover",
				Message:            extractUpstreamErrorMessage(respBody),
				Detail: func() string {
					if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
						return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
					}
					return ""
				}(),
			})
			return &UpstreamFailoverError{
				StatusCode:             st.resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(st.resp.StatusCode),
			}
		}
		st.result, err = s.handleRetryExhaustedError(ctx, st.resp, c, account)
		return err
	}

	if st.resp.StatusCode >= 400 && s.shouldFailoverUpstreamError(st.resp.StatusCode) {
		respBody, _ := s.readUpstreamErrorBody(st.resp)
		_ = st.resp.Body.Close()
		st.resp.Body = io.NopCloser(bytes.NewReader(respBody))

		logger.LegacyPrintf("service.gateway", "[Forward] Upstream error (failover): Account=%d(%s) Status=%d RequestID=%s Body=%s",
			account.ID, account.Name, st.resp.StatusCode, st.resp.Header.Get("x-request-id"), truncateString(string(respBody), 1000))

		s.handleFailoverSideEffects(ctx, st.resp, account, st.reqModel)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			UpstreamStatusCode: st.resp.StatusCode,
			UpstreamRequestID:  st.resp.Header.Get("x-request-id"),
			Kind:               "failover",
			Message:            extractUpstreamErrorMessage(respBody),
			Detail: func() string {
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					return truncateString(string(respBody), s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes)
				}
				return ""
			}(),
		})
		return &UpstreamFailoverError{
			StatusCode:             st.resp.StatusCode,
			ResponseBody:           respBody,
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(st.resp.StatusCode),
		}
	}
	if st.resp.StatusCode >= 400 {
		if st.resp.StatusCode == 400 && s.cfg != nil && s.cfg.Gateway.FailoverOn400 {
			respBody, readErr := s.readUpstreamErrorBody(st.resp)
			if readErr != nil {
				st.result, err = s.handleErrorResponse(ctx, st.resp, c, account, st.reqModel)
				return err
			}
			_ = st.resp.Body.Close()
			st.resp.Body = io.NopCloser(bytes.NewReader(respBody))

			if s.shouldFailoverOn400(respBody) {
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
					UpstreamStatusCode: st.resp.StatusCode,
					UpstreamRequestID:  st.resp.Header.Get("x-request-id"),
					Kind:               "failover_on_400",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})

				if s.cfg.Gateway.LogUpstreamErrorBody {
					logger.LegacyPrintf("service.gateway",
						"Account %d: 400 error, attempting failover: %s",
						account.ID,
						truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
					)
				} else {
					logger.LegacyPrintf("service.gateway", "Account %d: 400 error, attempting failover", account.ID)
				}
				s.handleFailoverSideEffects(ctx, st.resp, account, st.reqModel)
				return &UpstreamFailoverError{StatusCode: st.resp.StatusCode, ResponseBody: respBody}
			}
		}
		st.result, err = s.handleErrorResponse(ctx, st.resp, c, account, st.reqModel)
		return err
	}
	return nil
}

func (s *GatewayService) forwardClaudeRelay(ex *pipeline.Exchange) error {
	st := claudeForwardStateFrom(ex)
	if st == nil || st.parsed == nil {
		return fmt.Errorf("parse request: empty request")
	}
	if st.resp == nil || st.resp.Body == nil {
		return errors.New("upstream request failed: empty response")
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	if !bytes.Equal(st.lastWireBody, st.body) {
		if err := st.replaceBody(st.lastWireBody); err != nil {
			return err
		}
	}

	if st.parsed.OnUpstreamAccepted != nil {
		st.parsed.OnUpstreamAccepted()
	}

	if st.reqStream {
		streamResult, err := s.handleStreamingResponse(ctx, st.resp, c, account, st.startTime, st.originalModel, st.reqModel, st.shouldMimicClaudeCode)
		if err != nil {
			var sseErr *sseStreamErrorEventError
			if errors.As(err, &sseErr) {
				body := []byte(sseErr.RawData)

				upstreamMsg := sanitizeUpstreamErrorMessage(
					strings.TrimSpace(extractUpstreamErrorMessage(body)),
				)

				upstreamDetail := ""
				if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
					maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
					if maxBytes <= 0 {
						maxBytes = 2048
					}
					upstreamDetail = redactAndTruncateUpstreamDetail(sseErr.RawData, maxBytes)
				}

				appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
					Platform:           account.Platform,
					AccountID:          account.ID,
					AccountName:        account.Name,
					UpstreamStatusCode: 403,
					UpstreamRequestID:  st.resp.Header.Get("x-request-id"),
					Kind:               "stream_error",
					Message:            upstreamMsg,
					Detail:             upstreamDetail,
				})

				logger.LegacyPrintf("service.gateway",
					"[Forward] SSE error event in stream: Account=%d(%s) RequestID=%s Body=%s",
					account.ID, account.Name, st.resp.Header.Get("x-request-id"),
					truncateString(sseErr.RawData, 1000),
				)

				return &UpstreamFailoverError{
					StatusCode:   403,
					ResponseBody: body,
				}
			}
			return err
		}
		st.usage = streamResult.usage
		st.firstTokenMs = streamResult.firstTokenMs
		st.clientDisconnect = streamResult.clientDisconnect
		return nil
	}

	usage, err := s.handleNonStreamingResponse(ctx, st.resp, c, account, st.originalModel, st.reqModel)
	if err != nil {
		return err
	}
	st.usage = usage
	return nil
}

func (s *GatewayService) forwardClaudeSettle(ex *pipeline.Exchange) error {
	st := claudeForwardStateFrom(ex)
	if st == nil || st.usage == nil || st.resp == nil {
		return errors.New("upstream request failed: empty response")
	}
	st.result = &ForwardResult{
		RequestID:        st.resp.Header.Get("x-request-id"),
		Usage:            *st.usage,
		Model:            st.originalModel,
		UpstreamModel:    st.mappedModel,
		Stream:           st.reqStream,
		Duration:         time.Since(st.startTime),
		FirstTokenMs:     st.firstTokenMs,
		ClientDisconnect: st.clientDisconnect,
	}
	ex.UpstreamModel = st.mappedModel
	return nil
}
