package lifecycle

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRegistryStopsParallelThenSequential(t *testing.T) {
	var order []string
	var mu atomic.Bool
	reg := New()
	reg.AddParallel("a", func() error {
		time.Sleep(20 * time.Millisecond)
		if !mu.Swap(true) {
			order = append(order, "parallel")
		}
		return nil
	})
	reg.AddParallel("b", func() error {
		if !mu.Swap(true) {
			order = append(order, "parallel")
		}
		return nil
	})
	reg.AddSequential("infra", func() error {
		order = append(order, "sequential")
		return nil
	})

	reg.Stop(context.Background())
	require.Equal(t, []string{"parallel", "sequential"}, order)
}
