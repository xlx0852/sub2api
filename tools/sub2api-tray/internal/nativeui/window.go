//go:build !headless

package nativeui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	webview "github.com/webview/webview_go"
)

const (
	DefaultTitle  = "Sub2API Tray"
	DefaultWidth  = 720
	DefaultHeight = 860
)

// RunWindow blocks on the current thread and shows a native OS window hosting the panel URL.
// Call this from a dedicated process main thread (e.g. sub2api-tray --ui <url>).
func RunWindow(panelURL, title string) error {
	panelURL = strings.TrimSpace(panelURL)
	if panelURL == "" {
		return fmt.Errorf("panel url is required")
	}
	if title == "" {
		title = DefaultTitle
	}
	// webview must live on the OS main thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	activateApp()

	w := webview.New(false)
	if w == nil {
		return fmt.Errorf("failed to create native webview window")
	}
	defer w.Destroy()
	w.SetTitle(title)
	w.SetSize(DefaultWidth, DefaultHeight, webview.HintNone)
	w.Navigate(panelURL)
	activateApp()
	w.Run()
	return nil
}

var (
	spawnMu   sync.Mutex
	spawnedPID int
)

// OpenInProcessWindow starts (or focuses) a separate native UI process for the panel.
// Using a child process avoids fighting systray for the platform main thread.
func OpenInProcessWindow(panelURL string) error {
	panelURL = strings.TrimSpace(panelURL)
	if panelURL == "" {
		return fmt.Errorf("panel url is required")
	}
	spawnMu.Lock()
	defer spawnMu.Unlock()

	if spawnedPID > 0 && processAlive(spawnedPID) {
		// Already open — best-effort activate on macOS.
		_ = activatePID(spawnedPID)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, "--ui", panelURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Detach from tray lifetime grouping lightly; tray exit still can reap via process group on unix if desired.
	cmd.SysProcAttr = sysProcAttrDetached()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start native ui: %w", err)
	}
	spawnedPID = cmd.Process.Pid
	// Reap in background so we don't leave zombies.
	go func(p *os.Process) {
		_, _ = p.Wait()
		spawnMu.Lock()
		if spawnedPID == p.Pid {
			spawnedPID = 0
		}
		spawnMu.Unlock()
	}(cmd.Process)
	// Give the window a moment to appear before returning to tray callback.
	time.Sleep(150 * time.Millisecond)
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, signal 0 checks existence.
	if runtime.GOOS == "windows" {
		return true
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func activatePID(pid int) error {
	if runtime.GOOS != "darwin" || pid <= 0 {
		return nil
	}
	script := fmt.Sprintf(`tell application "System Events" to set frontmost of (first process whose unix id is %d) to true`, pid)
	return exec.Command("osascript", "-e", script).Start()
}
