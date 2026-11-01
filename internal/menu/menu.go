// Package menu runs Hypercube's macOS menu bar icon.
package menu

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
void hypercubeMenuRun(const char *url, const void *icon, size_t iconLen);
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

// Run shows the menu bar icon and runs the application until the user chooses
// Quit, then returns.
func Run(url string, icon []byte) {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))
	cIcon := C.CBytes(icon)
	defer C.free(cIcon)
	C.hypercubeMenuRun(cURL, cIcon, C.size_t(len(icon)))
}
