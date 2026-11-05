// Package desktop runs Hypercube's macOS application: the menu bar icon, the
// window that shows the Hypercube UI, and each session's browser in that
// window. Cocoa runs on the main thread; Go reaches it through C functions
// that dispatch to the main queue.
package desktop

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications -framework WebKit
#include <pwd.h>
#include <stdlib.h>
#include <unistd.h>
void hypercubeRun(const char *url, const void *icon, size_t iconLen);
*/
import "C"

import (
	_ "embed"
	"errors"
	"runtime"
	"unsafe"
)

// LoginShell looks up the account's shell when Finder supplies no SHELL.
// It is called before starting the application or its background work.
func LoginShell() (string, error) {
	account := C.getpwuid(C.getuid())
	if account == nil || account.pw_shell == nil || C.GoString(account.pw_shell) == "" {
		return "", errors.New("cannot find the current account's login shell")
	}
	return C.GoString(account.pw_shell), nil
}

// menuBarIcon is drawn as a template image, so only its shape matters.
//
//go:embed menubar.svg
var menuBarIcon []byte

func init() {
	// Cocoa's application loop must stay on the main thread.
	runtime.LockOSThread()
}

// Run shows the menu bar icon, loads the UI at url into the window, and runs
// the application until the user quits, then returns. The window opens from
// the menu bar icon or the Dock.
func Run(url string) {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))
	cIcon := C.CBytes(menuBarIcon)
	defer C.free(cIcon)
	C.hypercubeRun(cURL, cIcon, C.size_t(len(menuBarIcon)))
}
