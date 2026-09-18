//go:build !headless && !windows

package nativeui

import "syscall"

func sysProcAttrDetached() *syscall.SysProcAttr {
	// New process group so the UI window is not tightly coupled as a job object,
	// but still a child we can Wait on.
	return &syscall.SysProcAttr{Setpgid: true}
}
