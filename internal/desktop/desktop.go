// Package desktop runs Hypercube's macOS application: the menu bar icon, the
// window that shows the Hypercube UI, and each session's browser in that
// window. Cocoa runs on the main thread; Go reaches it through C functions
// that dispatch to the main queue.
package desktop

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications -framework WebKit
#include <stdlib.h>
void hypercubeRun(const char *url, const void *icon, size_t iconLen);
*/
import "C"

import (
	"runtime"
	"unsafe"
)

func init() {
	// Cocoa's application loop must stay on the main thread.
	runtime.LockOSThread()
}

// Run shows the menu bar icon, loads the UI at url into the window, and runs
// the application until the user quits, then returns. The window opens from
// the menu bar icon or the Dock.
func Run(url string, icon []byte) {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))
	cIcon := C.CBytes(icon)
	defer C.free(cIcon)
	C.hypercubeRun(cURL, cIcon, C.size_t(len(icon)))
}
