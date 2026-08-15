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

func TestScoreBreakdownComponentTotal(t *testing.T) {
	breakdown := ScoreBreakdown{
		Priority:         1,
		Load:             2,
		Queue:            3,
		ErrorRate:        4,
		TTFT:             5,
		Reset:            6,
		QuotaHeadroom:    7,
		CompactError:     8,
		CompactLatency:   9,
		PreviousResponse: 10,
		SessionSticky:    11,
	}
	require.Equal(t, float64(66), breakdown.ComponentTotal())
}

func TestNoAffinityDecisionIsExplicit(t *testing.T) {
	require.Equal(t, AffinityDecision{Kind: AffinityNone}, NoAffinityDecision())
}
