package service

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

const claudeCountTokensStateKey = "claude.count_tokens_state"

type claudeCountTokensState struct {
	c                     *gin.Context
	parsed                *ParsedRequest
	body                  []byte
	reqModel              string
	shouldMimicClaudeCode bool
	token                 string
	tokenType             string
	proxyURL              string
	acceptedWireBody      []byte
	respStatus            int
	respBody              []byte
	skipRemaining         bool
}

func claudeCountTokensStateFrom(ex *pipeline.Exchange) *claudeCountTokensState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(claudeCountTokensStateKey).(*claudeCountTokensState)
	return st
}

func (st *claudeCountTokensState) replaceBody(next []byte) error {
	if st == nil || st.parsed == nil {
		return fmt.Errorf("rewrite count_tokens body: empty request")
	}
	if err := st.parsed.ReplaceBody(next); err != nil {
		return fmt.Errorf("rewrite count_tokens body: %w", err)
	}
	st.body = st.parsed.Body.Bytes()
	return nil
}

func (s *GatewayService) claudeCountTokensPipeline() *pipeline.Pipeline {
	return pipeline.New(s.claudeCountTokensStages()...)
}

func (s *GatewayService) claudeCountTokensStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardClaudeCountTokensParse),
		pipeline.Func(pipeline.StageRewrite, s.forwardClaudeCountTokensRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardClaudeCountTokensTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardClaudeCountTokensRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardClaudeCountTokensSettle),
	}
}

func (s *GatewayService) finishClaudeCountTokens(st *claudeCountTokensState, err error) error {
	return err
}

func (s *GatewayService) forwardClaudeCountTokensParse(ex *pipeline.Exchange) error {
	st := claudeCountTokensStateFrom(ex)
	if st == nil || st.parsed == nil {
		return fmt.Errorf("parse request: empty request")
	}
	st.body = st.parsed.Body.Bytes()
	st.reqModel = st.parsed.Model
	ex.RequestedModel = st.reqModel
	ex.Inbound = st.body
	return nil
}

func (s *GatewayService) forwardClaudeCountTokensRewrite(ex *pipeline.Exchange) error {
	st := claudeCountTokensStateFrom(ex)
	if st == nil || st.parsed == nil {
		return fmt.Errorf("rewrite count_tokens body: empty request")
	}
	account := ex.Account
	c := st.c

	if err := st.replaceBody(StripEmptyTextBlocks(st.body)); err != nil {
		return err
	}

	st.shouldMimicClaudeCode = false

	// Antigravity 账户不支持 count_tokens，返回 404 让客户端 fallback 到本地估算。
	// 返回 nil 避免 handler 层记录为错误，也不设置 ops 上游错误上下文。
	if account.Platform == PlatformAntigravity {
		s.countTokensError(c, http.StatusNotFound, "not_found_error", "count_tokens endpoint is not supported for this platform")
		st.skipRemaining = true
		return nil
	}

	if st.reqModel != "" {
		mappedModel := st.reqModel
		mappingSource := ""
		if account.Type == AccountTypeAPIKey {
			mappedModel = account.GetMappedModel(st.reqModel)
			if mappedModel != st.reqModel {
				mappingSource = "account"
			}
		}
		if mappingSource == "" && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
			normalized := claude.NormalizeModelID(st.reqModel)
			if normalized != st.reqModel {
				mappedModel = normalized
				mappingSource = "prefix"
			}
		}
		if mappedModel != st.reqModel {
			originalReqModel := st.reqModel
			if err := st.replaceBody(s.replaceModelInBody(st.body, mappedModel)); err != nil {
				return err
			}
			st.reqModel = mappedModel
			st.parsed.Model = mappedModel
			logger.LegacyPrintf("service.gateway", "CountTokens model mapping applied: %s -> %s (account: %s, source=%s)", originalReqModel, mappedModel, account.Name, mappingSource)
		}
	}
	ex.RequestedModel = st.reqModel
	ex.UpstreamModel = st.reqModel
	ex.Outbound = st.body
	return nil
}

func (s *GatewayService) forwardClaudeCountTokensTransport(ex *pipeline.Exchange) error {
	st := claudeCountTokensStateFrom(ex)
	if st == nil {
		return fmt.Errorf("transport count_tokens: empty state")
	}
	if st.skipRemaining {
		return nil
	}
	account := ex.Account
	c := st.c
	ctx := ex.Ctx

	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to get access token")
		return err
	}
	st.token = token
	st.tokenType = tokenType

	upstreamReq, wireBody, err := s.buildCountTokensRequest(ctx, c, account, st.body, token, tokenType, st.reqModel, st.shouldMimicClaudeCode)
	if err != nil {
		s.countTokensError(c, http.StatusInternalServerError, "api_error", "Failed to build request")
		return err
	}
	st.acceptedWireBody = wireBody

	if account.ProxyID != nil && account.Proxy != nil {
		if !account.IsCustomBaseURLEnabled() || account.GetCustomBaseURL() == "" {
			st.proxyURL = account.Proxy.URL()
		}
	}

	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, st.proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		setOpsUpstreamError(c, 0, sanitizeUpstreamErrorMessage(err.Error()), "")
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Request failed")
		return fmt.Errorf("upstream request failed: %w", err)
	}

	countTokensTooLarge := func(c *gin.Context) {
		s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Upstream response too large")
	}
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, countTokensTooLarge)
	_ = resp.Body.Close()
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
		}
		return err
	}

	if resp.StatusCode == 400 && s.shouldRectifySignatureError(ctx, account, respBody, st.reqModel) {
		logger.LegacyPrintf("service.gateway", "Account %d: detected thinking block signature error on count_tokens, retrying with filtered thinking blocks", account.ID)

		filteredBody := FilterThinkingBlocksForRetry(st.body, st.reqModel)
		retryReq, retryWireBody, buildErr := s.buildCountTokensRequest(ctx, c, account, filteredBody, token, tokenType, st.reqModel, st.shouldMimicClaudeCode)
		if buildErr == nil {
			retryResp, retryErr := s.httpUpstream.DoWithTLS(retryReq, st.proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
			if retryErr == nil {
				if retryResp.StatusCode < 400 {
					st.acceptedWireBody = retryWireBody
				}
				resp = retryResp
				respBody, err = ReadUpstreamResponseBody(resp.Body, s.cfg, c, countTokensTooLarge)
				_ = resp.Body.Close()
				if err != nil {
					if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
						s.countTokensError(c, http.StatusBadGateway, "upstream_error", "Failed to read response")
					}
					return err
				}
			}
		}
	}

	if resp.StatusCode < 400 && !bytes.Equal(st.acceptedWireBody, st.body) {
		if err := st.replaceBody(st.acceptedWireBody); err != nil {
			return err
		}
	}

	if resp.StatusCode >= 400 {
		s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)

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
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

		if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
			logger.LegacyPrintf("service.gateway",
				"count_tokens upstream error %d (account=%d platform=%s type=%s): %s",
				resp.StatusCode,
				account.ID,
				account.Platform,
				account.Type,
				truncateForLog(respBody, s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes),
			)
		}

		errMsg := "Upstream request failed"
		switch resp.StatusCode {
		case 429:
			errMsg = "Rate limit exceeded"
		case 529:
			errMsg = "Service overloaded"
		}
		s.countTokensError(c, resp.StatusCode, "upstream_error", errMsg)
		if upstreamMsg == "" {
			return fmt.Errorf("upstream error: %d", resp.StatusCode)
		}
		return fmt.Errorf("upstream error: %d message=%s", resp.StatusCode, upstreamMsg)
	}

	st.respStatus = resp.StatusCode
	st.respBody = respBody
	return nil
}

func (s *GatewayService) forwardClaudeCountTokensRelay(ex *pipeline.Exchange) error {
	st := claudeCountTokensStateFrom(ex)
	if st == nil || st.skipRemaining {
		return nil
	}
	st.c.Data(st.respStatus, "application/json", st.respBody)
	return nil
}

func (s *GatewayService) forwardClaudeCountTokensSettle(ex *pipeline.Exchange) error {
	return nil
}
