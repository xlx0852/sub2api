package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/stretchr/testify/require"
)

func TestClaudeForwardPipelineStageOrder(t *testing.T) {
	svc := &GatewayService{}
	stages := svc.claudeForwardStages()
	require.Len(t, stages, 7)
	names := make([]pipeline.StageName, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name())
	}
	require.Equal(t, []pipeline.StageName{
		pipeline.StageParse,
		pipeline.StageAdmission,
		pipeline.StageSelect,
		pipeline.StageRewrite,
		pipeline.StageTransport,
		pipeline.StageRelay,
		pipeline.StageSettle,
	}, names)
}

func TestClaudeForwardParseRejectsEmptyRequest(t *testing.T) {
	svc := &GatewayService{}
	ex := &pipeline.Exchange{Ctx: context.Background()}
	err := svc.forwardClaudeParse(ex)
	require.EqualError(t, err, "parse request: empty request")
}

func TestClaudeForwardParseBindsParsedRequest(t *testing.T) {
	svc := &GatewayService{}
	parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(`{"model":"claude-sonnet-4-5","messages":[]}`)), PlatformAnthropic)
	require.NoError(t, err)
	st := &claudeForwardState{parsed: parsed}
	ex := &pipeline.Exchange{Ctx: context.Background()}
	ex.Set(claudeForwardStateKey, st)

	require.NoError(t, svc.forwardClaudeParse(ex))
	require.Equal(t, "claude-sonnet-4-5", ex.RequestedModel)
	require.Equal(t, "claude-sonnet-4-5", st.reqModel)
	require.Equal(t, "claude-sonnet-4-5", st.originalModel)
	require.False(t, st.reqStream)
	require.NotEmpty(t, st.body)
}

func TestClaudeAsResponsesPipelineStageOrder(t *testing.T) {
	svc := &GatewayService{}
	stages := svc.claudeAsResponsesStages()
	require.Len(t, stages, 5)
	names := make([]pipeline.StageName, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name())
	}
	require.Equal(t, []pipeline.StageName{
		pipeline.StageParse,
		pipeline.StageRewrite,
		pipeline.StageTransport,
		pipeline.StageRelay,
		pipeline.StageSettle,
	}, names)
}

func TestClaudeAsChatPipelineStageOrder(t *testing.T) {
	svc := &GatewayService{}
	stages := svc.claudeAsChatStages()
	require.Len(t, stages, 5)
	names := make([]pipeline.StageName, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name())
	}
	require.Equal(t, []pipeline.StageName{
		pipeline.StageParse,
		pipeline.StageRewrite,
		pipeline.StageTransport,
		pipeline.StageRelay,
		pipeline.StageSettle,
	}, names)
}

func TestClaudeCountTokensPipelineStageOrder(t *testing.T) {
	svc := &GatewayService{}
	stages := svc.claudeCountTokensStages()
	require.Len(t, stages, 5)
	names := make([]pipeline.StageName, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name())
	}
	require.Equal(t, []pipeline.StageName{
		pipeline.StageParse,
		pipeline.StageRewrite,
		pipeline.StageTransport,
		pipeline.StageRelay,
		pipeline.StageSettle,
	}, names)
}

func TestClaudeCountTokensParseRejectsEmptyRequest(t *testing.T) {
	svc := &GatewayService{}
	ex := &pipeline.Exchange{Ctx: context.Background()}
	err := svc.forwardClaudeCountTokensParse(ex)
	require.EqualError(t, err, "parse request: empty request")
}

func TestMapClaudeCompatUpstreamModelUsesAccountMapping(t *testing.T) {
	account := &Account{
		Type:     AccountTypeAPIKey,
		Platform: PlatformAnthropic,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"claude-sonnet-4-5": "claude-sonnet-4-5-mapped",
			},
		},
	}
	require.Equal(t, "claude-sonnet-4-5-mapped", mapClaudeCompatUpstreamModel(account, "claude-sonnet-4-5"))
}

func TestClaudeForwardFinishPreservesResultAndError(t *testing.T) {
	svc := &GatewayService{}
	st := &claudeForwardState{
		result: &ForwardResult{RequestID: "rid-1"},
		resp:   &http.Response{Body: http.NoBody},
	}
	result, err := svc.finishClaudeForward(st, context.Canceled)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, "rid-1", result.RequestID)
}
