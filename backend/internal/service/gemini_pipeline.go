package service

import (
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/gin-gonic/gin"
)

const (
	geminiMessagesForwardStateKey = "gemini.messages_forward_state"
	geminiNativeForwardStateKey   = "gemini.native_forward_state"
	geminiChatForwardStateKey     = "gemini.chat_forward_state"
)

type geminiMessagesForwardState struct {
	c                  *gin.Context
	startTime          time.Time
	body               []byte
	originalClaudeBody []byte
	originalModel      string
	mappedModel        string
	clientStream       bool
	useUpstreamStream  bool
	geminiReq          []byte
	proxyURL           string
	requestIDHeader    string
	resp               *http.Response
	result             *ForwardResult
}

func geminiMessagesForwardStateFrom(ex *pipeline.Exchange) *geminiMessagesForwardState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(geminiMessagesForwardStateKey).(*geminiMessagesForwardState)
	return st
}

func (s *GeminiMessagesCompatService) geminiMessagesForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.geminiMessagesForwardStages()...)
}

func (s *GeminiMessagesCompatService) geminiMessagesForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardGeminiMessagesParse),
		pipeline.Func(pipeline.StageRewrite, s.forwardGeminiMessagesRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardGeminiMessagesTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardGeminiMessagesRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardGeminiMessagesSettle),
	}
}

type geminiNativeForwardState struct {
	c                 *gin.Context
	startTime         time.Time
	body              []byte
	originalModel     string
	mappedModel       string
	action            string
	upstreamAction    string
	clientStream      bool
	useUpstreamStream bool
	forceAIStudio     bool
	proxyURL          string
	requestIDHeader   string
	resp              *http.Response
	result            *ForwardResult
	skipRemaining     bool
}

func geminiNativeForwardStateFrom(ex *pipeline.Exchange) *geminiNativeForwardState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(geminiNativeForwardStateKey).(*geminiNativeForwardState)
	return st
}

func (s *GeminiMessagesCompatService) geminiNativeForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.geminiNativeForwardStages()...)
}

func (s *GeminiMessagesCompatService) geminiNativeForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardGeminiNativeParse),
		pipeline.Func(pipeline.StageRewrite, s.forwardGeminiNativeRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardGeminiNativeTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardGeminiNativeRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardGeminiNativeSettle),
	}
}

type geminiChatForwardState struct {
	c                 *gin.Context
	startTime         time.Time
	body              []byte
	claudeBody        []byte
	originalChatBody  []byte
	originalModel     string
	claudeModel       string
	mappedModel       string
	geminiReq         []byte
	clientStream      bool
	includeUsage      bool
	useUpstreamStream bool
	proxyURL          string
	requestIDHeader   string
	resp              *http.Response
	result            *ForwardResult
}

func geminiChatForwardStateFrom(ex *pipeline.Exchange) *geminiChatForwardState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(geminiChatForwardStateKey).(*geminiChatForwardState)
	return st
}

func (s *GeminiMessagesCompatService) geminiChatForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.geminiChatForwardStages()...)
}

func (s *GeminiMessagesCompatService) geminiChatForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardGeminiChatParse),
		pipeline.Func(pipeline.StageRewrite, s.forwardGeminiChatRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardGeminiChatTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardGeminiChatRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardGeminiChatSettle),
	}
}

func (s *GeminiMessagesCompatService) finishGeminiChatForward(st *geminiChatForwardState, err error) (*ForwardResult, error) {
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}

func (s *GeminiMessagesCompatService) finishGeminiNativeForward(st *geminiNativeForwardState, err error) (*ForwardResult, error) {
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}

func (s *GeminiMessagesCompatService) finishGeminiMessagesForward(st *geminiMessagesForwardState, err error) (*ForwardResult, error) {
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}
