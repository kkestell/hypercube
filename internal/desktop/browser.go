package desktop

/*
#include <stdint.h>
#include <stdlib.h>
void hypercubeNavigate(const char *session, const char *url, uint64_t request);
void hypercubeEval(const char *session, const char *script, uint64_t request);
void hypercubeClose(const char *session, uint64_t request);
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"unsafe"
)

// Browser drives the sessions' browsers in the window. Its calls finish only
// while Run's application loop runs.
type Browser struct{}

// Navigate loads url in the session's browser, creating it, and returns the
// page's URL and title once it has loaded.
func (Browser) Navigate(ctx context.Context, session, url string) (string, string, error) {
	text, err := call(ctx, func(request C.uint64_t) {
		cSession, cURL := C.CString(session), C.CString(url)
		defer C.free(unsafe.Pointer(cSession))
		defer C.free(unsafe.Pointer(cURL))
		C.hypercubeNavigate(cSession, cURL, request)
	})
	if err != nil {
		return "", "", err
	}
	var page struct{ URL, Title string }
	if err := json.Unmarshal([]byte(text), &page); err != nil {
		return "", "", err
	}
	return page.URL, page.Title, nil
}

// Eval runs script as the body of an async function in the session's page,
// and returns JSON.stringify of its result.
func (Browser) Eval(ctx context.Context, session, script string) (string, error) {
	return call(ctx, func(request C.uint64_t) {
		cSession, cScript := C.CString(session), C.CString(script)
		defer C.free(unsafe.Pointer(cSession))
		defer C.free(unsafe.Pointer(cScript))
		C.hypercubeEval(cSession, cScript, request)
	})
}

// Close closes the session's browser and its tab, if it has one.
func (Browser) Close(ctx context.Context, session string) error {
	_, err := call(ctx, func(request C.uint64_t) {
		cSession := C.CString(session)
		defer C.free(unsafe.Pointer(cSession))
		C.hypercubeClose(cSession, request)
	})
	return err
}

type reply struct {
	text string
	err  error
}

// waiting holds the calls the main thread has yet to finish, by request ID.
var waiting = struct {
	sync.Mutex
	next  uint64
	calls map[uint64]chan reply
}{calls: map[uint64]chan reply{}}

// call starts a request on the main thread and waits for hypercubeDone to
// finish it, or for ctx to end. A request finished after ctx ends is dropped.
func call(ctx context.Context, start func(C.uint64_t)) (string, error) {
	done := make(chan reply, 1)
	waiting.Lock()
	waiting.next++
	id := waiting.next
	waiting.calls[id] = done
	waiting.Unlock()
	start(C.uint64_t(id))
	select {
	case r := <-done:
		return r.text, r.err
	case <-ctx.Done():
		waiting.Lock()
		delete(waiting.calls, id)
		waiting.Unlock()
		return "", ctx.Err()
	}
}

// hypercubeDone finishes a request with its result, or with an error when
// failure is not NULL.
//
//export hypercubeDone
func hypercubeDone(request C.uint64_t, result, failure *C.char) {
	waiting.Lock()
	done, ok := waiting.calls[uint64(request)]
	delete(waiting.calls, uint64(request))
	waiting.Unlock()
	if !ok {
		return
	}
	if failure != nil {
		done <- reply{err: errors.New(C.GoString(failure))}
		return
	}
	done <- reply{text: C.GoString(result)}
}
