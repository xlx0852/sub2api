package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/gin-gonic/gin"
)

const (
	antigravityClaudeRequestKey = "antigravity.claude_request"
	antigravityForwardStateKey  = "antigravity.forward_state"
)

var _ pipeline.Provider = (*AntigravityGatewayService)(nil)

type antigravityClientError struct {
	Status  int
	Type    string
	Message string
	MarkOps bool
}

func (e *antigravityClientError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (s *AntigravityGatewayService) Platform() string {
	return PlatformAntigravity
}

func (s *AntigravityGatewayService) Parse(ex *pipeline.Exchange) error {
	if ex == nil {
		return &antigravityClientError{
			Status:  http.StatusBadRequest,
			Type:    "invalid_request_error",
			Message: "Invalid request body",
		}
	}
	var claudeReq antigravity.ClaudeRequest
	if err := json.Unmarshal(ex.Inbound, &claudeReq); err != nil {
		return &antigravityClientError{
			Status:  http.StatusBadRequest,
			Type:    "invalid_request_error",
			Message: "Invalid request body",
		}
	}
	if strings.TrimSpace(claudeReq.Model) == "" {
		return &antigravityClientError{
			Status:  http.StatusBadRequest,
			Type:    "invalid_request_error",
			Message: "Missing model",
		}
	}
	ex.RequestedModel = claudeReq.Model
	ex.Set(antigravityClaudeRequestKey, &claudeReq)
	return nil
}

func (s *AntigravityGatewayService) Rewrite(ex *pipeline.Exchange) error {
	req, _ := ex.Get(antigravityClaudeRequestKey).(*antigravity.ClaudeRequest)
	if req == nil {
		return &antigravityClientError{
			Status:  http.StatusBadRequest,
			Type:    "invalid_request_error",
			Message: "Invalid request body",
		}
	}
	mappedModel := s.getMappedModel(ex.Account, req.Model)
	if mappedModel == "" {
		return &antigravityClientError{
			Status:  http.StatusForbidden,
			Type:    "permission_error",
			Message: fmt.Sprintf("model %s not in whitelist", req.Model),
			MarkOps: true,
		}
	}
	thinkingEnabled := req.Thinking != nil && (req.Thinking.Type == "enabled" || req.Thinking.Type == "adaptive")
	ex.UpstreamModel = applyThinkingModelSuffix(mappedModel, thinkingEnabled)
	return nil
}

func (s *AntigravityGatewayService) ExtractUsage(ex *pipeline.Exchange) error {
	return nil
}

func antigravityClaudeRequestFromExchange(ex *pipeline.Exchange) *antigravity.ClaudeRequest {
	if ex == nil {
		return nil
	}
	req, _ := ex.Get(antigravityClaudeRequestKey).(*antigravity.ClaudeRequest)
	return req
}

type antigravityForwardState struct {
	c                *gin.Context
	isStickySession  bool
	startTime        time.Time
	prefix           string
	claudeReq        antigravity.ClaudeRequest
	originalModel    string
	mappedModel      string
	billingModel     string
	accessToken      string
	projectID        string
	proxyURL         string
	geminiBody       []byte
	action           string
	transformOpts    antigravity.TransformOptions
	resp             *http.Response
	requestID        string
	usage            *ClaudeUsage
	firstTokenMs     *int
	clientDisconnect bool
	result           *ForwardResult
	skipRemaining    bool
	stream           bool
	groupID          int64
	sessionHash      string
	imageInputSize   string
	imageSize        string
	imageCount       int
	injectedBody     []byte
	wrappedBody      []byte
	upstreamAction   string
}

func antigravityForwardStateFrom(ex *pipeline.Exchange) *antigravityForwardState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(antigravityForwardStateKey).(*antigravityForwardState)
	return st
}

func (s *AntigravityGatewayService) claudeForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.claudeForwardStages()...)
}

func (s *AntigravityGatewayService) claudeForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.ParseStage(s),
		pipeline.RewriteStage(s),
		pipeline.Func(pipeline.StageTransport, s.forwardClaudeTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardClaudeRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardClaudeSettle),
	}
}

func (s *AntigravityGatewayService) finishClaudeForward(c *gin.Context, st *antigravityForwardState, err error) (*ForwardResult, error) {
	if err == nil {
		if st == nil {
			return nil, nil
		}
		return st.result, nil
	}
	var clientErr *antigravityClientError
	if errors.As(err, &clientErr) {
		if clientErr.MarkOps {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
		}
		return nil, s.writeClaudeError(c, clientErr.Status, clientErr.Type, clientErr.Message)
	}
	return nil, err
}

func (s *AntigravityGatewayService) geminiForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.geminiForwardStages()...)
}

func (s *AntigravityGatewayService) geminiForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardGeminiParse),
		pipeline.Func(pipeline.StageRewrite, s.forwardGeminiRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardGeminiTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardGeminiRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardGeminiSettle),
	}
}

func (s *AntigravityGatewayService) finishGeminiForward(c *gin.Context, st *antigravityForwardState, err error) (*ForwardResult, error) {
	if err == nil {
		if st == nil {
			return nil, nil
		}
		return st.result, nil
	}
	var clientErr *antigravityClientError
	if errors.As(err, &clientErr) {
		if clientErr.MarkOps {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
		}
		return nil, s.writeGoogleError(c, clientErr.Status, clientErr.Message)
	}
	return nil, err
}

func googleClientError(status int, message string) *antigravityClientError {
	return &antigravityClientError{Status: status, Message: message}
}
