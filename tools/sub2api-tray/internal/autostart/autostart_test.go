package autostart

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnableDisableRoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("round-trip file checks covered on darwin/linux")
	}
	exe := filepath.Join(t.TempDir(), "sub2api-tray-fake")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	spec := Spec{Executable: exe, Args: []string{"--addr", "127.0.0.1:8787"}, StateDir: state}

	// Isolate HOME so we don't touch the developer's real autostart entries.
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "darwin" {
		_ = os.MkdirAll(filepath.Join(home, "Library", "LaunchAgents"), 0o755)
	} else {
		_ = os.MkdirAll(filepath.Join(home, ".config", "autostart"), 0o755)
	}

	if err := Enable(spec); err != nil {
		t.Fatalf("enable: %v", err)
	}
	on, err := Enabled()
	if err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("expected enabled")
	}
	if err := Disable(); err != nil {
		t.Fatalf("disable: %v", err)
	}
	on, err = Enabled()
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Fatal("expected disabled")
	}
}
