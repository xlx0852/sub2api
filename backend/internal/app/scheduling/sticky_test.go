package scheduling

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluateStickyBinding(t *testing.T) {
	excluded := map[int64]struct{}{7: {}}
	tests := []struct {
		name, session string
		accountID     int64
		want          StickyDecision
	}{
		{name: "no session", accountID: 7, want: StickyNoSession},
		{name: "no binding", session: "session", want: StickyNoBinding},
		{name: "excluded", session: "session", accountID: 7, want: StickyExcluded},
		{name: "ready", session: "session", accountID: 8, want: StickyReady},
		{name: "whitespace is caller-owned", session: " ", accountID: 8, want: StickyReady},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, EvaluateStickyBinding(test.session, test.accountID, excluded).Decision)
		})
	}
}

func TestEvaluateStickyCandidate(t *testing.T) {
	binding := StickyBinding{AccountID: 8, Decision: StickyReady}
	require.Equal(t, StickyCandidateMissing, EvaluateStickyCandidate(binding, 0, false))
	require.Equal(t, StickyCandidateMismatch, EvaluateStickyCandidate(binding, 9, false))
	require.Equal(t, StickyClear, EvaluateStickyCandidate(binding, 8, true))
	require.Equal(t, StickyReady, EvaluateStickyCandidate(binding, 8, false))
	require.Equal(t, StickyExcluded, EvaluateStickyCandidate(StickyBinding{AccountID: 8, Decision: StickyExcluded}, 8, false))
}
