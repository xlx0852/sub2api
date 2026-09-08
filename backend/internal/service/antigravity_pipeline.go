package service

import (
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/gin-gonic/gin"
)

const antigravityForwardStateKey = "antigravity.forward_state"

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

type antigravityForwardState struct {
	responses        *antigravityResponsesState
	c                *gin.Context
	isStickySession  bool
	startTime        time.Time
	prefix           string
	originalModel    string
	mappedModel      string
	billingModel     string
	accessToken      string
	projectID        string
	proxyURL         string
	action           string
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

func (s *AntigravityGatewayService) geminiForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.geminiForwardStages()...)
}

func (s *AntigravityGatewayService) geminiForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardGeminiParse),
		pipeline.Func(pipeline.StageAdmission, admitAccount),
		pipeline.Func(pipeline.StageSelect, selectAccount),
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
