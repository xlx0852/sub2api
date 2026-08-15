package scheduling

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRuntimeStatsStoreReportsEWMAAndKeepsAvailability(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	store := NewRuntimeStatsStore(time.Hour, 10, 0.2, func() time.Time { return now })
	errorOne, errorZero := 1.0, 0.0
	ttftOne, ttftTwo := 1000.0, 2000.0
	loadOne, loadTwo := 20.0, 40.0
	waitOne, waitTwo := 1.0, 3.0
	quotaOne, quotaTwo := 1.0, 0.5
	windowOne, windowTwo := 0.8, 0.4

	require.True(t, store.Report(RuntimeFeedback{AccountID: 1, LoadRate: &loadOne, QueueDepth: &waitOne, ErrorSample: &errorOne, TTFTMillis: &ttftOne, QuotaHeadroom: &quotaOne, WindowHeadroom: &windowOne}))
	now = now.Add(time.Minute)
	require.True(t, store.Report(RuntimeFeedback{AccountID: 1, LoadRate: &loadTwo, QueueDepth: &waitTwo, ErrorSample: &errorZero, TTFTMillis: &ttftTwo, QuotaHeadroom: &quotaTwo, WindowHeadroom: &windowTwo}))
	snapshot := store.Snapshot(1, now)

	require.InDelta(t, 24, snapshot.LoadRate.Value, 1e-12)
	require.InDelta(t, 1.4, snapshot.QueueDepth.Value, 1e-12)
	require.True(t, snapshot.ErrorRate.Available)
	require.InDelta(t, 0.8, snapshot.ErrorRate.Value, 1e-12)
	require.InDelta(t, 1200, snapshot.TTFTMillis.Value, 1e-12)
	require.InDelta(t, 0.9, snapshot.QuotaHeadroom.Value, 1e-12)
	require.InDelta(t, 0.72, snapshot.WindowHeadroom.Value, 1e-12)
	require.False(t, snapshot.CompactError.Available)
}

func TestRuntimeStatsStoreExpiresAndRemovesEntries(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	store := NewRuntimeStatsStore(time.Minute, 10, 0.2, func() time.Time { return now })
	sample := 0.0
	require.True(t, store.Report(RuntimeFeedback{AccountID: 1, ErrorSample: &sample}))

	now = now.Add(2 * time.Minute)
	snapshot := store.Snapshot(1, now)
	require.False(t, snapshot.ErrorRate.Available)
	require.Zero(t, store.Len())
}

func TestRuntimeStatsStoreEvictsOldestAtCapacity(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	store := NewRuntimeStatsStore(time.Hour, 2, 0.2, func() time.Time { return now })
	sample := 0.0
	require.True(t, store.Report(RuntimeFeedback{AccountID: 1, ErrorSample: &sample}))
	now = now.Add(time.Second)
	require.True(t, store.Report(RuntimeFeedback{AccountID: 2, ErrorSample: &sample}))
	now = now.Add(time.Second)
	require.True(t, store.Report(RuntimeFeedback{AccountID: 3, ErrorSample: &sample}))

	require.Equal(t, 2, store.Len())
	require.False(t, store.Snapshot(1, now).ErrorRate.Available)
	require.True(t, store.Snapshot(2, now).ErrorRate.Available)
	require.True(t, store.Snapshot(3, now).ErrorRate.Available)
}

func TestRuntimeStatsStoreRejectsInvalidFeedbackAndSupportsExplicitRemove(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	store := NewRuntimeStatsStore(time.Hour, 2, 0.2, func() time.Time { return now })
	nan := math.NaN()
	require.False(t, store.Report(RuntimeFeedback{AccountID: 1, ErrorSample: &nan}))
	negativeTTFT := -1.0
	require.False(t, store.Report(RuntimeFeedback{AccountID: 1, TTFTMillis: &negativeTTFT}))
	negativeLoad := -1.0
	require.False(t, store.Report(RuntimeFeedback{AccountID: 1, LoadRate: &negativeLoad}))
	valid := 10.0
	require.True(t, store.Report(RuntimeFeedback{AccountID: 1, TTFTMillis: &valid}))
	store.Remove(1)
	require.Zero(t, store.Len())
}
