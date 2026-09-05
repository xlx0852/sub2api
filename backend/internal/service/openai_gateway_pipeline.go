package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

const (
	grokForwardStateKey           = "grok.forward_state"
	grokChatForwardStateKey       = "grok.chat_forward_state"
	responsesChatFallbackStateKey = "openai.responses_chat_fallback_state"
	openaiForwardStateKey         = "openai.forward_state"
	openaiChatForwardStateKey     = "openai.chat_forward_state"
	openaiMessagesForwardStateKey = "openai.messages_forward_state"
)

type grokForwardState struct {
	c             *gin.Context
	startTime     time.Time
	body          []byte
	patchedBody   []byte
	originalModel string
	upstreamModel string
	reqStream     bool
	cacheIdentity string
	serviceTier   *string
	token         string
	credential    grokCredentialDescriptor
	resp          *http.Response
	usage         *OpenAIUsage
	firstTokenMs  *int
	responseID    string
	result        *OpenAIForwardResult
}

func grokForwardStateFrom(ex *pipeline.Exchange) *grokForwardState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(grokForwardStateKey).(*grokForwardState)
	return st
}

type responsesChatFallbackState struct {
	c               *gin.Context
	startTime       time.Time
	body            []byte
	chatBody        []byte
	originalModel   string
	billingModel    string
	upstreamModel   string
	clientStream    bool
	serviceTier     *string
	reasoningEffort *string
	customTools     map[string]bool
	responsesReq    *apicompat.ResponsesRequest
	resp            *http.Response
	result          *OpenAIForwardResult
}

func responsesChatFallbackStateFrom(ex *pipeline.Exchange) *responsesChatFallbackState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(responsesChatFallbackStateKey).(*responsesChatFallbackState)
	return st
}

func (s *OpenAIGatewayService) grokForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.grokForwardStages()...)
}

func (s *OpenAIGatewayService) grokForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardGrokParse),
		pipeline.Func(pipeline.StageRewrite, s.forwardGrokRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardGrokTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardGrokRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardGrokSettle),
	}
}

type grokChatForwardState struct {
	c             *gin.Context
	startTime     time.Time
	body          []byte
	chatReq       *apicompat.ChatCompletionsRequest
	originalModel string
	billingModel  string
	upstreamModel string
	clientStream  bool
	cacheIdentity string
	responsesBody []byte
	token         string
	credential    grokCredentialDescriptor
	resp          *http.Response
	result        *OpenAIForwardResult
}

func grokChatForwardStateFrom(ex *pipeline.Exchange) *grokChatForwardState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(grokChatForwardStateKey).(*grokChatForwardState)
	return st
}

func (s *OpenAIGatewayService) grokChatForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.grokChatForwardStages()...)
}

func (s *OpenAIGatewayService) grokChatForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardGrokChatParse),
		pipeline.Func(pipeline.StageRewrite, s.forwardGrokChatRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardGrokChatTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardGrokChatRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardGrokChatSettle),
	}
}

func (s *OpenAIGatewayService) finishGrokChatForward(st *grokChatForwardState, err error) (*OpenAIForwardResult, error) {
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}

func (s *OpenAIGatewayService) finishGrokForward(st *grokForwardState, err error) (*OpenAIForwardResult, error) {
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}

func (s *OpenAIGatewayService) responsesChatFallbackPipeline() *pipeline.Pipeline {
	return pipeline.New(s.responsesChatFallbackStages()...)
}

func (s *OpenAIGatewayService) responsesChatFallbackStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardResponsesChatFallbackParse),
		pipeline.Func(pipeline.StageRewrite, s.forwardResponsesChatFallbackRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardResponsesChatFallbackTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardResponsesChatFallbackRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardResponsesChatFallbackSettle),
	}
}

func (s *OpenAIGatewayService) finishResponsesChatFallback(st *responsesChatFallbackState, err error) (*OpenAIForwardResult, error) {
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}

type openaiForwardState struct {
	c                                      *gin.Context
	startTime                              time.Time
	body                                   []byte
	originalBody                           []byte
	requestView                            openAIRequestView
	reqModel                               string
	reqStream                              bool
	promptCacheKey                         string
	originalModel                          string
	billingModel                           string
	upstreamModel                          string
	isCodexCLI                             bool
	codexImageGenerationExplicitToolPolicy string
	wsDecision                             OpenAIWSProtocolDecision
	bodySignalCompactV2                    bool
	compatMessagesBridge                   bool
	bodyModified                           bool
	reqBody                                map[string]any
	imageGenerationAllowed                 bool
	codexImageGenerationBridgeEnabled      bool
	imageIntent                            bool
	imageBillingModel                      string
	imageSizeTier                          string
	imageInputSize                         string
	token                                  string
	resp                                   *http.Response
	pendingCompactCancel                   context.CancelFunc
	firstTokenMs                           *int
	responseID                             string
	responseModel                          string
	imageCount                             int
	imageOutputSizes                       []string
	reasoningEffort                        *string
	serviceTier                            *string
	isCompactRequest                       bool
	result                                 *OpenAIForwardResult
	skipRemaining                          bool
}

func openaiForwardStateFrom(ex *pipeline.Exchange) *openaiForwardState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(openaiForwardStateKey).(*openaiForwardState)
	return st
}

func (st *openaiForwardState) ensureReqBody() (map[string]any, error) {
	if st == nil {
		return nil, fmt.Errorf("parse request: empty request")
	}
	if st.requestView.HasPatches() {
		patchedBody, patchErr := st.requestView.ApplyPatches()
		if patchErr != nil {
			return nil, patchErr
		}
		st.body = patchedBody
		st.requestView = newOpenAIRequestView(st.body)
		st.reqBody = nil
		st.bodyModified = false
	}
	if st.reqBody != nil {
		return st.reqBody, nil
	}
	decoded, decodeErr := st.requestView.Decode(st.c)
	if decodeErr != nil {
		return nil, decodeErr
	}
	st.reqBody = decoded
	return st.reqBody, nil
}

func (st *openaiForwardState) markPatchSet(path string, value any) {
	if st == nil {
		return
	}
	st.bodyModified = true
	if st.requestView.patchesDisabled {
		if st.reqBody != nil {
			setOpenAIRequestMapPath(st.reqBody, path, value)
		}
		return
	}
	st.requestView.MarkPatchSet(path, value)
}

func (st *openaiForwardState) markPatchDelete(path string) {
	if st == nil {
		return
	}
	st.bodyModified = true
	if st.requestView.patchesDisabled {
		if st.reqBody != nil {
			deleteOpenAIRequestMapPath(st.reqBody, path)
		}
		return
	}
	st.requestView.MarkPatchDelete(path)
}

func (st *openaiForwardState) disablePatch() {
	if st == nil {
		return
	}
	st.requestView.DisablePatches()
}

func (st *openaiForwardState) markDecodedModified() {
	if st == nil {
		return
	}
	st.bodyModified = true
	st.disablePatch()
}

func (s *OpenAIGatewayService) openaiForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.openaiForwardStages()...)
}

func (s *OpenAIGatewayService) openaiForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardOpenAIParse),
		pipeline.Func(pipeline.StageAdmission, admitOpenAIForward),
		pipeline.Func(pipeline.StageSelect, selectOpenAIForward),
		pipeline.Func(pipeline.StageRewrite, s.forwardOpenAIRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardOpenAITransport),
		pipeline.Func(pipeline.StageRelay, s.forwardOpenAIRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardOpenAISettle),
	}
}

func admitOpenAIForward(ex *pipeline.Exchange) error {
	if ex == nil || ex.Account == nil {
		return fmt.Errorf("admission: account is required")
	}
	return nil
}

func selectOpenAIForward(ex *pipeline.Exchange) error {
	if ex == nil || ex.Account == nil {
		return fmt.Errorf("select: account is required")
	}
	return nil
}

type openaiChatForwardState struct {
	c                  *gin.Context
	startTime          time.Time
	body               []byte
	promptCacheKey     string
	defaultMappedModel string
	originalModel      string
	billingModel       string
	upstreamModel      string
	clientStream       bool
	isResponsesShape   bool
	responsesReq       *apicompat.ResponsesRequest
	responsesBody      []byte
	resp               *http.Response
	result             *OpenAIForwardResult
	skipRemaining      bool
}

func openaiChatForwardStateFrom(ex *pipeline.Exchange) *openaiChatForwardState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(openaiChatForwardStateKey).(*openaiChatForwardState)
	return st
}

func (s *OpenAIGatewayService) openaiChatForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.openaiChatForwardStages()...)
}

func (s *OpenAIGatewayService) openaiChatForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardOpenAIChatParse),
		pipeline.Func(pipeline.StageAdmission, admitOpenAIForward),
		pipeline.Func(pipeline.StageSelect, selectOpenAIForward),
		pipeline.Func(pipeline.StageRewrite, s.forwardOpenAIChatRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardOpenAIChatTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardOpenAIChatRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardOpenAIChatSettle),
	}
}

type openaiMessagesForwardState struct {
	c                           *gin.Context
	startTime                   time.Time
	body                        []byte
	promptCacheKey              string
	defaultMappedModel          string
	originalModel               string
	normalizedModel             string
	billingModel                string
	upstreamModel               string
	clientStream                bool
	isStream                    bool
	apiKeyID                    int64
	anthropicDigestChain        string
	anthropicMatchedDigestChain string
	compatReplayGuardEnabled    bool
	compatContinuationEnabled   bool
	previousResponseID          string
	compatTurnState             string
	responsesReq                *apicompat.ResponsesRequest
	responsesBody               []byte
	grokCacheIdentity           string
	resp                        *http.Response
	result                      *OpenAIForwardResult
	skipRemaining               bool
}

func openaiMessagesForwardStateFrom(ex *pipeline.Exchange) *openaiMessagesForwardState {
	if ex == nil {
		return nil
	}
	st, _ := ex.Get(openaiMessagesForwardStateKey).(*openaiMessagesForwardState)
	return st
}

func (s *OpenAIGatewayService) openaiMessagesForwardPipeline() *pipeline.Pipeline {
	return pipeline.New(s.openaiMessagesForwardStages()...)
}

func (s *OpenAIGatewayService) openaiMessagesForwardStages() []pipeline.Stage {
	return []pipeline.Stage{
		pipeline.Func(pipeline.StageParse, s.forwardOpenAIMessagesParse),
		pipeline.Func(pipeline.StageAdmission, admitOpenAIForward),
		pipeline.Func(pipeline.StageSelect, selectOpenAIForward),
		pipeline.Func(pipeline.StageRewrite, s.forwardOpenAIMessagesRewrite),
		pipeline.Func(pipeline.StageTransport, s.forwardOpenAIMessagesTransport),
		pipeline.Func(pipeline.StageRelay, s.forwardOpenAIMessagesRelay),
		pipeline.Func(pipeline.StageSettle, s.forwardOpenAIMessagesSettle),
	}
}

func (s *OpenAIGatewayService) finishOpenAIMessagesForward(st *openaiMessagesForwardState, err error) (*OpenAIForwardResult, error) {
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}

func (s *OpenAIGatewayService) finishOpenAIChatForward(st *openaiChatForwardState, err error) (*OpenAIForwardResult, error) {
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}

func (s *OpenAIGatewayService) finishOpenAIForward(st *openaiForwardState, err error) (*OpenAIForwardResult, error) {
	if st != nil && st.pendingCompactCancel != nil {
		st.pendingCompactCancel()
		st.pendingCompactCancel = nil
	}
	if st != nil && st.resp != nil && st.resp.Body != nil {
		_ = st.resp.Body.Close()
	}
	if st == nil {
		return nil, err
	}
	return st.result, err
}
