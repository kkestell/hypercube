// Package menu runs Hypercube's macOS menu bar icon.
package menu

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
void hypercubeMenuRun(const char *url);
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

// Run shows the menu bar icon and runs the application until the user quits.
func Run(url string) {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))
	C.hypercubeMenuRun(cURL)
}
