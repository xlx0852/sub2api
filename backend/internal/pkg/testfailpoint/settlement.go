package testfailpoint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	SettlementTerminalParsed   = "terminal_parsed"
	SettlementEventPersisted   = "event_persisted"
	SettlementBillingPreCommit = "billing_before_commit"
	SettlementCommitPreAck     = "commit_before_ack"
)

var settlementFailpoint struct {
	mu      sync.Mutex
	stage   string
	dir     string
	reached bool
}

func ConfigureSettlement(stage, dir string) error {
	stage = strings.TrimSpace(stage)
	dir = strings.TrimSpace(dir)
	settlementFailpoint.mu.Lock()
	defer settlementFailpoint.mu.Unlock()
	settlementFailpoint.stage = ""
	settlementFailpoint.dir = ""
	settlementFailpoint.reached = false
	if stage == "" && dir == "" {
		return nil
	}
	if !isSettlementStage(stage) {
		return fmt.Errorf("invalid settlement test failpoint stage %q", stage)
	}
	if !filepath.IsAbs(dir) || !strings.Contains(filepath.Clean(dir), "sub2api-settlement") {
		return errors.New("settlement test failpoint dir must be an absolute test namespace path containing sub2api-settlement")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create settlement test failpoint dir: %w", err)
	}
	for _, suffix := range []string{".reached", ".release"} {
		if err := os.Remove(filepath.Join(dir, stage+suffix)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear settlement test failpoint marker: %w", err)
		}
	}
	settlementFailpoint.stage = stage
	settlementFailpoint.dir = dir
	return nil
}

func ReachSettlement(stage string) {
	settlementFailpoint.mu.Lock()
	if settlementFailpoint.stage != stage || settlementFailpoint.reached || settlementFailpoint.dir == "" {
		settlementFailpoint.mu.Unlock()
		return
	}
	settlementFailpoint.reached = true
	dir := settlementFailpoint.dir
	settlementFailpoint.mu.Unlock()

	reachedPath := filepath.Join(dir, stage+".reached")
	releasePath := filepath.Join(dir, stage+".release")
	if err := os.WriteFile(reachedPath, []byte(fmt.Sprintf("pid=%d\nstage=%s\n", os.Getpid(), stage)), 0o600); err != nil {
		panic(fmt.Sprintf("write settlement test failpoint marker: %v", err))
	}
	for {
		if _, err := os.Stat(releasePath); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			panic(fmt.Sprintf("read settlement test failpoint release: %v", err))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func isSettlementStage(stage string) bool {
	switch stage {
	case SettlementTerminalParsed, SettlementEventPersisted, SettlementBillingPreCommit, SettlementCommitPreAck:
		return true
	default:
		return false
	}
}
