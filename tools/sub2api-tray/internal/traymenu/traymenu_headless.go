//go:build headless

package traymenu

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/control"
	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/state"
)

// App is a no-op tray shell used for headless builds (no CGO / systray).
type App struct {
	Store  *state.Store
	Server *control.Server
	OnQuit func()
}

func (a *App) Run() {
	fmt.Println("sub2api-tray: built with -tags headless; tray UI disabled, control server only")
	if a.OnQuit != nil {
		// Run returns immediately in headless stub; main should block on signals instead.
	}
}
