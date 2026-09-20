package scheduling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSignalAvailabilityAndFreshnessKeepUnknownSeparateFromZero(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	unknown := UnavailableSignal()
	zero := AvailableSignal(0, now.Add(-time.Minute))

	require.False(t, unknown.Available)
	require.False(t, unknown.Fresh(now, 5*time.Minute))
	require.True(t, zero.Available)
	require.Zero(t, zero.Value)
	require.True(t, zero.Fresh(now, 5*time.Minute))
	require.False(t, zero.Fresh(now, 30*time.Second))
}

func TestSignalWithFutureTimestampIsNotFresh(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	signal := AvailableSignal(1, now.Add(time.Second))
	require.False(t, signal.Fresh(now, time.Minute))
}

func TestNewCandidateSnapshotCopiesCapabilities(t *testing.T) {
	capabilities := []string{"responses", "compact"}
	snapshot := NewCandidateSnapshot(CandidateSnapshot{
		AccountID:    7,
		Capabilities: capabilities,
	})
	capabilities[0] = "mutated"

	require.Equal(t, []string{"responses", "compact"}, snapshot.Capabilities)
}

func TestNoAffinityDecisionIsExplicit(t *testing.T) {
	require.Equal(t, AffinityDecision{Kind: AffinityNone}, NoAffinityDecision())
}
