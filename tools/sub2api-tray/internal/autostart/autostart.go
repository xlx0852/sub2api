package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	macLabel = "com.sub2api.tray"
	appName  = "Sub2API Tray"
)

// Spec describes how the tray should be launched at login.
type Spec struct {
	// Executable is absolute path to sub2api-tray binary.
	Executable string
	// Args are extra args after the binary (should include stable --addr etc).
	Args []string
	// StateDir is ~/.sub2api-tray (used for logs on macOS).
	StateDir string
}

func DefaultExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(exe)
}

func Enabled() (bool, error) {
	switch runtime.GOOS {
	case "darwin":
		return macEnabled()
	case "linux":
		return linuxEnabled()
	case "windows":
		return windowsEnabled()
	default:
		return false, fmt.Errorf("autostart unsupported on %s", runtime.GOOS)
	}
}

func Enable(spec Spec) error {
	if strings.TrimSpace(spec.Executable) == "" {
		exe, err := DefaultExecutable()
		if err != nil {
			return err
		}
		spec.Executable = exe
	}
	switch runtime.GOOS {
	case "darwin":
		return macEnable(spec)
	case "linux":
		return linuxEnable(spec)
	case "windows":
		return windowsEnable(spec)
	default:
		return fmt.Errorf("autostart unsupported on %s", runtime.GOOS)
	}
}

func Disable() error {
	switch runtime.GOOS {
	case "darwin":
		return macDisable()
	case "linux":
		return linuxDisable()
	case "windows":
		return windowsDisable()
	default:
		return fmt.Errorf("autostart unsupported on %s", runtime.GOOS)
	}
}

func macPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", macLabel+".plist"), nil
}

func macEnabled() (bool, error) {
	path, err := macPlistPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func macEnable(spec Spec) error {
	path, err := macPlistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	logDir := spec.StateDir
	if logDir == "" {
		home, _ := os.UserHomeDir()
		logDir = filepath.Join(home, ".sub2api-tray")
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	args := append([]string{spec.Executable}, spec.Args...)
	var progArgs strings.Builder
	for _, a := range args {
		progArgs.WriteString(fmt.Sprintf("    <string>%s</string>\n", xmlEscape(a)))
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
%s  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <false/>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, macLabel, progArgs.String(),
		xmlEscape(filepath.Join(logDir, "launchd.out.log")),
		xmlEscape(filepath.Join(logDir, "launchd.err.log")),
	)
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	// Best-effort load for current session.
	_ = exec.Command("launchctl", "unload", path).Run()
	_ = exec.Command("launchctl", "load", path).Run()
	return nil
}

func macDisable() error {
	path, err := macPlistPath()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func linuxDesktopPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "autostart", "sub2api-tray.desktop"), nil
}

func linuxEnabled() (bool, error) {
	path, err := linuxDesktopPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func linuxEnable(spec Spec) error {
	path, err := linuxDesktopPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	execLine := shellJoin(append([]string{spec.Executable}, spec.Args...))
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Version=1.0
Name=%s
Comment=Sub2API Codex tray companion
Exec=%s
X-GNOME-Autostart-enabled=true
Terminal=false
Categories=Utility;
`, appName, execLine)
	return os.WriteFile(path, []byte(content), 0o644)
}

func linuxDisable() error {
	path, err := linuxDesktopPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func windowsEnabled() (bool, error) {
	path, err := windowsShortcutPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func windowsShortcutPath() (string, error) {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		appdata = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(appdata, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "Sub2API Tray.lnk"), nil
}

func windowsEnable(spec Spec) error {
	path, err := windowsShortcutPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Create .lnk via PowerShell (no extra deps).
	args := ""
	if len(spec.Args) > 0 {
		args = strings.Join(spec.Args, " ")
	}
	ps := fmt.Sprintf(
		`$ws = New-Object -ComObject WScript.Shell; $s = $ws.CreateShortcut('%s'); $s.TargetPath = '%s'; $s.Arguments = '%s'; $s.WorkingDirectory = '%s'; $s.Save()`,
		psEscape(path),
		psEscape(spec.Executable),
		psEscape(args),
		psEscape(filepath.Dir(spec.Executable)),
	)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create startup shortcut: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func windowsDisable() error {
	path, err := windowsShortcutPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		`&`, `&amp;`,
		`<`, `&lt;`,
		`>`, `&gt;`,
		`"`, `&quot;`,
		`'`, `&apos;`,
	)
	return r.Replace(s)
}

func psEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func shellJoin(parts []string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.ContainsAny(p, " \t\"'") {
			out = append(out, "'"+strings.ReplaceAll(p, `'`, `'\''`)+"'")
		} else {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}
