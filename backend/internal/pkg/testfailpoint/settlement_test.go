package testfailpoint

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSettlementFailpointBlocksUntilReleased(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub2api-settlement-failpoint")
	require.NoError(t, ConfigureSettlement(SettlementEventPersisted, dir))
	t.Cleanup(func() { require.NoError(t, ConfigureSettlement("", "")) })
	done := make(chan struct{})
	go func() {
		ReachSettlement(SettlementEventPersisted)
		close(done)
	}()
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(dir, SettlementEventPersisted+".reached"))
		return err == nil
	}, time.Second, 10*time.Millisecond)
	select {
	case <-done:
		t.Fatal("failpoint returned before release")
	default:
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, SettlementEventPersisted+".release"), nil, 0o600))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("failpoint did not return after release")
	}
}

func TestSettlementFailpointRejectsNonTestDirectory(t *testing.T) {
	require.Error(t, ConfigureSettlement(SettlementTerminalParsed, "/tmp/not-a-test-dir"))
}
