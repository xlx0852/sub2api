package scheduling

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

type selectorStrategyStub struct {
	name     string
	platform string
	result   *Outcome
	calls    int
}

func (s *selectorStrategyStub) Name() string { return s.name }

func (s *selectorStrategyStub) Supports(req Request) bool { return req.Platform == s.platform }

func (s *selectorStrategyStub) Select(_ context.Context, _ Request) (*Outcome, error) {
	s.calls++
	return s.result, nil
}

func TestEngineDispatchesToFirstSupportingStrategy(t *testing.T) {
	claude := &selectorStrategyStub{name: "gateway", platform: "anthropic"}
	openAI := &selectorStrategyStub{
		name:     "openai",
		platform: "openai",
		result: &Outcome{
			Result:   &Result{Account: &domain.Account{ID: 7}},
			Decision: "load_balance",
		},
	}
	engine := NewEngine(claude, openAI)

	outcome, err := engine.SelectOutcome(context.Background(), Request{Platform: "openai"})
	require.NoError(t, err)
	require.Equal(t, int64(7), outcome.Result.Account.ID)
	require.Equal(t, "load_balance", outcome.Decision)
	require.Zero(t, claude.calls)
	require.Equal(t, 1, openAI.calls)
}

func TestEngineReturnsNoStrategy(t *testing.T) {
	engine := NewEngine(&selectorStrategyStub{name: "openai", platform: "openai"})
	result, err := engine.Select(context.Background(), Request{Platform: "anthropic"})
	require.ErrorIs(t, err, ErrNoStrategy)
	require.Nil(t, result)
}
