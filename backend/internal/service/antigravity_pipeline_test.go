package service

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAntigravityGeminiForwardPipelineStageOrder(t *testing.T) {
	svc := &AntigravityGatewayService{}
	stages := svc.geminiForwardStages()
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

func TestAntigravityGeminiParseRejectsNonTextModels(t *testing.T) {
	tests := []string{
		"claude-sonnet-4-6",
		"gemini-3.1-flash-image",
		"imagen-4",
	}
	for _, model := range tests {
		t.Run(model, func(t *testing.T) {
			st := &antigravityForwardState{c: &gin.Context{}, originalModel: model, action: "generateContent"}
			ex := &pipeline.Exchange{Inbound: []byte(`{"contents":[]}`)}
			ex.Set(antigravityForwardStateKey, st)
			err := (&AntigravityGatewayService{}).forwardGeminiParse(ex)
			var clientErr *antigravityClientError
			require.ErrorAs(t, err, &clientErr)
			require.Equal(t, http.StatusBadRequest, clientErr.Status)
			require.Contains(t, clientErr.Message, "text Gemini models only")
		})
	}
}

func TestAntigravityGeminiParseAcceptsTextGemini(t *testing.T) {
	st := &antigravityForwardState{c: &gin.Context{}, originalModel: "gemini-3-flash", action: "generateContent"}
	ex := &pipeline.Exchange{Inbound: []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)}
	ex.Set(antigravityForwardStateKey, st)
	require.NoError(t, (&AntigravityGatewayService{}).forwardGeminiParse(ex))
}
