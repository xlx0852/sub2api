package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAntigravityProviderParseAndRewrite(t *testing.T) {
	svc := &AntigravityGatewayService{}
	account := &Account{Platform: PlatformAntigravity}
	body, err := json.Marshal(antigravity.ClaudeRequest{
		Model: "claude-sonnet-4-5",
		Thinking: &antigravity.ThinkingConfig{
			Type: "enabled",
		},
	})
	require.NoError(t, err)

	ex := &pipeline.Exchange{
		Ctx:     context.Background(),
		Inbound: body,
		Account: account,
	}
	require.NoError(t, pipeline.New(pipeline.ParseStage(svc), pipeline.RewriteStage(svc)).Run(ex))
	require.Equal(t, "claude-sonnet-4-5", ex.RequestedModel)
	require.Equal(t, "claude-sonnet-4-5-thinking", ex.UpstreamModel)
	require.NotNil(t, antigravityClaudeRequestFromExchange(ex))
}

func TestAntigravityProviderParseRejectsMissingModel(t *testing.T) {
	svc := &AntigravityGatewayService{}
	ex := &pipeline.Exchange{Inbound: []byte(`{"messages":[]}`)}
	err := svc.Parse(ex)
	var clientErr *antigravityClientError
	require.ErrorAs(t, err, &clientErr)
	require.Equal(t, http.StatusBadRequest, clientErr.Status)
	require.Equal(t, "Missing model", clientErr.Message)
}

func TestAntigravityProviderRewriteRejectsUnmappedModel(t *testing.T) {
	svc := &AntigravityGatewayService{}
	ex := &pipeline.Exchange{Account: &Account{Platform: PlatformAntigravity}}
	ex.Set(antigravityClaudeRequestKey, &antigravity.ClaudeRequest{Model: "gpt-4"})
	err := svc.Rewrite(ex)
	var clientErr *antigravityClientError
	require.ErrorAs(t, err, &clientErr)
	require.Equal(t, http.StatusForbidden, clientErr.Status)
	require.True(t, clientErr.MarkOps)
	require.Contains(t, clientErr.Message, "not in whitelist")
}

func TestAntigravityProviderPlatform(t *testing.T) {
	require.Equal(t, PlatformAntigravity, (&AntigravityGatewayService{}).Platform())
}

func TestAntigravityGeminiForwardPipelineStageOrder(t *testing.T) {
	svc := &AntigravityGatewayService{}
	stages := svc.geminiForwardStages()
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

func TestAntigravityGeminiParseRejectsMissingModel(t *testing.T) {
	svc := &AntigravityGatewayService{}
	st := &antigravityForwardState{c: &gin.Context{}, originalModel: "", action: "generateContent"}
	ex := &pipeline.Exchange{Inbound: []byte(`{}`)}
	ex.Set(antigravityForwardStateKey, st)
	err := svc.forwardGeminiParse(ex)
	var clientErr *antigravityClientError
	require.ErrorAs(t, err, &clientErr)
	require.Equal(t, http.StatusBadRequest, clientErr.Status)
	require.Equal(t, "Missing model in URL", clientErr.Message)
}

func TestAntigravityClaudeForwardPipelineStageOrder(t *testing.T) {
	svc := &AntigravityGatewayService{}
	stages := svc.claudeForwardStages()
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
