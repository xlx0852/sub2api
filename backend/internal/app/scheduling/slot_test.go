package scheduling

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubSlotAcquirer struct {
	result *SlotResult
	err    error
	called bool
}

func (s *stubSlotAcquirer) AcquireModelAwareAccountSlot(context.Context, string, int64, int) (*SlotResult, error) {
	s.called = true
	return s.result, s.err
}

func TestTryAcquireAccountSlotNilAcquirerFailOpen(t *testing.T) {
	got, err := TryAcquireAccountSlot(context.Background(), nil, "gpt-4", 7, 3)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.True(t, got.Acquired)
	require.False(t, got.ModelLimited)
	require.NotNil(t, got.Release)
	got.Release()
}

func TestTryAcquireAccountSlotDelegatesToAcquirer(t *testing.T) {
	released := false
	stub := &stubSlotAcquirer{
		result: &SlotResult{
			Acquired:     true,
			Release:      func() { released = true },
			ModelLimited: false,
		},
	}
	got, err := TryAcquireAccountSlot(context.Background(), stub, "gpt-4", 7, 3)
	require.NoError(t, err)
	require.True(t, stub.called)
	require.Equal(t, stub.result, got)
	got.Release()
	require.True(t, released)
}

func TestTryAcquireAccountSlotPropagatesAcquirerError(t *testing.T) {
	want := errors.New("redis down")
	stub := &stubSlotAcquirer{err: want}
	got, err := TryAcquireAccountSlot(context.Background(), stub, "gpt-4", 7, 3)
	require.ErrorIs(t, err, want)
	require.Nil(t, got)
}

func TestExcludedSetCloneAndAdd(t *testing.T) {
	original := map[int64]struct{}{7: {}}
	cloned := CloneExcluded(original)
	cloned = AddExcluded(cloned, 8)

	require.True(t, IsExcluded(cloned, 7))
	require.True(t, IsExcluded(cloned, 8))
	require.False(t, IsExcluded(original, 8))
	require.Nil(t, CloneExcluded(nil))
	require.True(t, IsExcluded(AddExcluded(nil, 9), 9))
}

func TestIsExcluded(t *testing.T) {
	require.False(t, IsExcluded(nil, 1))
	require.False(t, IsExcluded(map[int64]struct{}{}, 1))
	require.True(t, IsExcluded(map[int64]struct{}{1: {}}, 1))
	require.False(t, IsExcluded(map[int64]struct{}{1: {}}, 2))
}
