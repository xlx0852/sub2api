//go:build !headless && windows

package nativeui

import "syscall"

func sysProcAttrDetached() *syscall.SysProcAttr {
	const createNewProcessGroup = 0x00000200
	const detachProcess = 0x00000008
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachProcess,
		HideWindow:    false,
	}
}
