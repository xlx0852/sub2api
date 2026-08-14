package pipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/app/gateway/pipeline"
	"github.com/stretchr/testify/require"
)

type recordingStage struct {
	name     pipeline.StageName
	calls    *[]pipeline.StageName
	err      error
	setExErr error
}

func (s recordingStage) Name() pipeline.StageName { return s.name }

func (s recordingStage) Run(ex *pipeline.Exchange) error {
	*s.calls = append(*s.calls, s.name)
	if s.setExErr != nil {
		ex.Err = s.setExErr
	}
	return s.err
}

func TestPipelineRecordsStageDuration(t *testing.T) {
	p := pipeline.New(recordingStage{name: pipeline.StageParse, calls: new([]pipeline.StageName)})
	ex := &pipeline.Exchange{Ctx: context.Background()}
	require.NoError(t, p.Run(ex))
	require.Contains(t, ex.StageDuration, pipeline.StageParse)
}

func TestPipelineRunsStagesInOrder(t *testing.T) {
	var calls []pipeline.StageName
	p := pipeline.New(
		recordingStage{name: pipeline.StageParse, calls: &calls},
		recordingStage{name: pipeline.StageAdmission, calls: &calls},
		recordingStage{name: pipeline.StageSelect, calls: &calls},
		recordingStage{name: pipeline.StageRewrite, calls: &calls},
		recordingStage{name: pipeline.StageTransport, calls: &calls},
		recordingStage{name: pipeline.StageRelay, calls: &calls},
		recordingStage{name: pipeline.StageSettle, calls: &calls},
	)

	err := p.Run(&pipeline.Exchange{Ctx: context.Background(), Platform: "antigravity"})
	require.NoError(t, err)
	require.Equal(t, []pipeline.StageName{
		pipeline.StageParse,
		pipeline.StageAdmission,
		pipeline.StageSelect,
		pipeline.StageRewrite,
		pipeline.StageTransport,
		pipeline.StageRelay,
		pipeline.StageSettle,
	}, calls)
}

func TestPipelineStopsOnStageError(t *testing.T) {
	var calls []pipeline.StageName
	boom := errors.New("admission denied")
	p := pipeline.New(
		recordingStage{name: pipeline.StageParse, calls: &calls},
		recordingStage{name: pipeline.StageAdmission, calls: &calls, err: boom},
		recordingStage{name: pipeline.StageSelect, calls: &calls},
	)

	ex := &pipeline.Exchange{Ctx: context.Background()}
	err := p.Run(ex)
	require.ErrorIs(t, err, boom)
	require.ErrorIs(t, ex.Err, boom)
	require.Equal(t, []pipeline.StageName{pipeline.StageParse, pipeline.StageAdmission}, calls)
}

func TestPipelineStopsOnExchangeErr(t *testing.T) {
	var calls []pipeline.StageName
	boom := errors.New("select failed")
	p := pipeline.New(
		recordingStage{name: pipeline.StageParse, calls: &calls},
		recordingStage{name: pipeline.StageSelect, calls: &calls, setExErr: boom},
		recordingStage{name: pipeline.StageRewrite, calls: &calls},
	)

	ex := &pipeline.Exchange{Ctx: context.Background()}
	err := p.Run(ex)
	require.ErrorIs(t, err, boom)
	require.Equal(t, []pipeline.StageName{pipeline.StageParse, pipeline.StageSelect}, calls)
}

func TestPipelineNilExchange(t *testing.T) {
	p := pipeline.New(recordingStage{name: pipeline.StageParse, calls: new([]pipeline.StageName)})
	require.Error(t, p.Run(nil))
}

type stubProvider struct {
	parsed    bool
	rewritten bool
}

func (p *stubProvider) Platform() string { return "antigravity" }

func (p *stubProvider) Parse(ex *pipeline.Exchange) error {
	p.parsed = true
	ex.RequestedModel = "claude-sonnet-4-5"
	return nil
}

func (p *stubProvider) Rewrite(ex *pipeline.Exchange) error {
	p.rewritten = true
	ex.UpstreamModel = ex.RequestedModel + "-thinking"
	return nil
}

func (p *stubProvider) ExtractUsage(*pipeline.Exchange) error { return nil }

func TestFuncStageUsesGivenName(t *testing.T) {
	called := false
	stage := pipeline.Func(pipeline.StageTransport, func(ex *pipeline.Exchange) error {
		called = true
		ex.UpstreamModel = "ok"
		return nil
	})
	require.Equal(t, pipeline.StageTransport, stage.Name())
	ex := &pipeline.Exchange{}
	require.NoError(t, pipeline.New(stage).Run(ex))
	require.True(t, called)
	require.Equal(t, "ok", ex.UpstreamModel)
}

func TestProviderStagesDelegateToAdapter(t *testing.T) {
	provider := &stubProvider{}
	ex := &pipeline.Exchange{Ctx: context.Background()}
	err := pipeline.New(pipeline.ParseStage(provider), pipeline.RewriteStage(provider)).Run(ex)
	require.NoError(t, err)
	require.True(t, provider.parsed)
	require.True(t, provider.rewritten)
	require.Equal(t, "claude-sonnet-4-5", ex.RequestedModel)
	require.Equal(t, "claude-sonnet-4-5-thinking", ex.UpstreamModel)
}
