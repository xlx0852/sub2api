package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/stretchr/testify/require"
)

func TestGeminiMessagesForwardPipelineStageOrder(t *testing.T) {
	svc := &GeminiMessagesCompatService{}
	stages := svc.geminiMessagesForwardStages()
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

func TestGeminiChatForwardPipelineStageOrder(t *testing.T) {
	svc := &GeminiMessagesCompatService{}
	stages := svc.geminiChatForwardStages()
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

func TestGeminiNativeForwardPipelineStageOrder(t *testing.T) {
	svc := &GeminiMessagesCompatService{}
	stages := svc.geminiNativeForwardStages()
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
