//go:build !headless && darwin

package nativeui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

void nativeuiActivateApp(void) {
  @autoreleasepool {
    [NSApplication sharedApplication];
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    [NSApp activateIgnoringOtherApps:YES];
  }
}
*/
import "C"
import "runtime"

func activateApp() {
	runtime.LockOSThread()
	C.nativeuiActivateApp()
}
