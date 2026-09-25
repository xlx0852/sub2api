package service

import (
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/app/gateway/stream"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
)

const (
	claudeAsResponsesStateKey = "claude.as_responses_state"
	claudeAsChatStateKey      = "claude.as_chat_state"
)

type claudeAsResponsesState struct {
	c               *gin.Context
	body            []byte
	parsed          *ParsedRequest
	startTime       time.Time
	originalModel   string
	mappedModel     string
	clientStream    bool
	reqStream       bool
	reasoningEffort *string
	anthropicReq    *apicompat.AnthropicRequest
	anthropicBody   []byte
	shouldMimic     bool
	token           string
	tokenType       string
	proxyURL        string
	resp            *http.Response
	result          *ForwardResult
}

func claudeAsResponsesStateFrom(ex *pipeline.Exchange) *claudeAsResponsesState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(claudeAsResponsesStateKey).(*claudeAsResponsesState)
	return st
}

type claudeAsChatState struct {
	c               *gin.Context
	sink            stream.Sink
	body            []byte
	parsed          *ParsedRequest
	startTime       time.Time
	originalModel   string
	mappedModel     string
	clientStream    bool
	reqStream       bool
	includeUsage    bool
	reasoningEffort *string
	anthropicReq    *apicompat.AnthropicRequest
	anthropicBody   []byte
	shouldMimic     bool
	token           string
	tokenType       string
	proxyURL        string
	resp            *http.Response
	result          *ForwardResult
}

func claudeAsChatStateFrom(ex *pipeline.Exchange) *claudeAsChatState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(claudeAsChatStateKey).(*claudeAsChatState)
	return st
}

func mapClaudeCompatUpstreamModel(account *Account, originalModel string) string {
	mappedModel := originalModel
	if account.Type == AccountTypeAPIKey {
		mappedModel = account.GetMappedModel(originalModel)
	}
	if mappedModel == originalModel && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		normalized := claude.NormalizeModelID(originalModel)
		if normalized != originalModel {
			mappedModel = normalized
		}
	}
	return mappedModel
}

func (s *GatewayService) claudeAsResponsesPipeline() *pipeline.Pipeline {
	return pipeline.New(s.claudeAsResponsesStages()...)
}

func (s *GatewayService) claudeAsResponsesStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardClaudeAsResponsesParse),
		pipeline.Func(pipeline.StageRewrite, s.forwardClaudeAsResponsesRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardClaudeAsResponsesTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardClaudeAsResponsesRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardClaudeAsResponsesSettle),
	}
}

func (s *GatewayService) finishClaudeAsResponses(st *claudeAsResponsesState, err error) (*ForwardResult, error) {
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}

func (s *GatewayService) claudeAsChatPipeline() *pipeline.Pipeline {
	return pipeline.New(s.claudeAsChatStages()...)
}

func (s *GatewayService) claudeAsChatStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardClaudeAsChatParse),
		pipeline.Func(pipeline.StageRewrite, s.forwardClaudeAsChatRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardClaudeAsChatTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardClaudeAsChatRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardClaudeAsChatSettle),
	}
}

func (s *GatewayService) finishClaudeAsChat(st *claudeAsChatState, err error) (*ForwardResult, error) {
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}
