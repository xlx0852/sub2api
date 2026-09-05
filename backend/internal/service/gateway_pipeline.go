package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
)

const claudeForwardStateKey = "claude.forward_state"

type claudeForwardState struct {
	c                     *gin.Context
	parsed                *ParsedRequest
	startTime             time.Time
	body                  []byte
	reqModel              string
	reqStream             bool
	originalModel         string
	mappedModel           string
	shouldMimicClaudeCode bool
	token                 string
	tokenType             string
	proxyURL              string
	tlsProfile            *tlsfingerprint.Profile
	resp                  *http.Response
	lastWireBody          []byte
	usage                 *ClaudeUsage
	firstTokenMs          *int
	clientDisconnect      bool
	result                *ForwardResult
}

func claudeForwardStateFrom(ex *pipeline.Exchange) *claudeForwardState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(claudeForwardStateKey).(*claudeForwardState)
	return st
}

func (st *claudeForwardState) replaceBody(next []byte) error {
	if st == nil || st.parsed == nil {
		return fmt.Errorf("rewrite request body: empty request")
	}
	if err := st.parsed.ReplaceBody(next); err != nil {
		return fmt.Errorf("rewrite request body: %w", err)
	}
	st.body = st.parsed.Body.Bytes()
	return nil
}

func (s *GatewayService) claudeForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.claudeForwardStages()...)
}

func (s *GatewayService) claudeForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardClaudeParse),
		pipeline.Func(pipeline.StageAdmission, admitAccount),
		pipeline.Func(pipeline.StageSelect, selectAccount),
		pipeline.Func(pipeline.StageRewrite, s.forwardClaudeRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardClaudeTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardClaudeRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardClaudeSettle),
	}
}

func (s *GatewayService) finishClaudeForward(st *claudeForwardState, err error) (*ForwardResult, error) {
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}
