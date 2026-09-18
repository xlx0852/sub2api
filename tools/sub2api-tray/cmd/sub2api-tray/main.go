package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/autostart"
	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/control"
	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/nativeui"
	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/state"
	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/traymenu"
)

func main() {
	var (
		headless  = flag.Bool("headless", false, "run control server without system tray")
		addr      = flag.String("addr", control.DefaultAddr, "loopback listen address")
		stateDir  = flag.String("state-dir", "", "state directory (default ~/.sub2api-tray)")
		codexHome = flag.String("codex-home", "", "CODEX_HOME override")
		openPanel = flag.Bool("open", false, "open native panel window after start")
		uiURL     = flag.String("ui", "", "run native panel window only (internal)")
		install   = flag.Bool("install-autostart", false, "enable login autostart and exit")
		uninstall = flag.Bool("uninstall-autostart", false, "disable login autostart and exit")
	)
	flag.Parse()

	if strings.TrimSpace(*uiURL) != "" {
		if err := nativeui.RunWindow(*uiURL, "Sub2API Tray"); err != nil {
			fatal(err)
		}
		return
	}

	store, err := state.Open(*stateDir)
	if err != nil {
		fatal(err)
	}

	autoArgs := buildAutoArgs(*addr, *stateDir, *codexHome, *headless)

	if *install {
		exe, err := autostart.DefaultExecutable()
		if err != nil {
			fatal(err)
		}
		if err := autostart.Enable(autostart.Spec{
			Executable: exe,
			Args:       autoArgs,
			StateDir:   store.Dir(),
		}); err != nil {
			fatal(err)
		}
		fmt.Println("autostart enabled")
		return
	}
	if *uninstall {
		if err := autostart.Disable(); err != nil {
			fatal(err)
		}
		fmt.Println("autostart disabled")
		return
	}

	srv, err := control.Start(control.Options{
		Store:     store,
		CodexHome: *codexHome,
		Addr:      *addr,
		AutoArgs:  autoArgs,
	})
	if err != nil {
		fatal(err)
	}

	fmt.Fprintf(os.Stderr, "sub2api-tray listening on %s\n", srv.Addr())
	fmt.Fprintf(os.Stderr, "panel: %s\n", srv.PanelURL())
	fmt.Fprintf(os.Stderr, "panel file: %s\n", filepath.Join(store.Dir(), "panel.url"))
	fmt.Fprintf(os.Stderr, "state: %s\n", store.Dir())

	if *openPanel {
		// Headless builds fall back inside OpenInProcessWindow error path via browser helper.
		if err := nativeui.OpenInProcessWindow(srv.PanelURL()); err != nil {
			_ = openBrowser(srv.PanelURL())
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *headless {
		<-ctx.Done()
		shutdown(srv)
		return
	}

	app := &traymenu.App{
		Store:  store,
		Server: srv,
		OnQuit: func() { shutdown(srv) },
	}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdown(srv)
		close(done)
	}()
	app.Run()
	// Real tray blocks inside Run until Quit. Headless stub returns immediately.
	select {
	case <-done:
	default:
		<-ctx.Done()
		shutdown(srv)
	}
}

func buildAutoArgs(addr, stateDir, codexHome string, headless bool) []string {
	args := make([]string, 0, 8)
	if strings.TrimSpace(addr) != "" {
		args = append(args, "--addr", addr)
	}
	if strings.TrimSpace(stateDir) != "" {
		args = append(args, "--state-dir", stateDir)
	}
	if strings.TrimSpace(codexHome) != "" {
		args = append(args, "--codex-home", codexHome)
	}
	if headless {
		args = append(args, "--headless")
	}
	return args
}

func shutdown(srv *control.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "sub2api-tray: %v\n", err)
	os.Exit(1)
}
