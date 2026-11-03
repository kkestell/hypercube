package desktop

/*
#include <stdlib.h>
void hypercubeNotify(const char *key, const char *session, const char *title, const char *subtitle, const char *text);
*/
import "C"

import (
	"unsafe"

	"github.com/kkestell/hypercube/internal/session"
)

// Notify shows a macOS notification for the alert unless the window has
// keyboard focus. Clicking it shows the window on the alert's session.
func Notify(a session.Alert) {
	var args [5]*C.char
	for i, s := range []string{a.Key, a.Session, a.Title, a.Subtitle, a.Text} {
		args[i] = C.CString(s)
		defer C.free(unsafe.Pointer(args[i]))
	}
	C.hypercubeNotify(args[0], args[1], args[2], args[3], args[4])
}
