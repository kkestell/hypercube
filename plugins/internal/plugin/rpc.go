package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
)

// Message is a request or notification from the other side of an RPC
// connection. ID is empty for a notification; a request is answered with
// Respond or RespondError using the same ID.
type Message struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
}

// RPCError is the error of a JSON-RPC response.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("%s (code %d)", e.Message, e.Code) }

// RPC is a JSON-RPC 2.0 connection over newline-delimited JSON, as a harness
// speaks on its stdin and stdout. Its owner feeds it the lines it reads with
// Line and ends it with End.
type RPC struct {
	w      io.Writer
	handle func(Message)
	wmu    sync.Mutex
	done   chan struct{}
	err    error // why the connection ended; set before done is closed
	mu     sync.Mutex
	next   int64
	calls  map[int64]chan response
}

type response struct {
	result json.RawMessage
	err    *RPCError
}

// NewRPC writes to w. Line calls handle for each request and notification:
// handle may write but must not wait for a response.
func NewRPC(w io.Writer, handle func(Message)) *RPC {
	return &RPC{w: w, handle: handle, done: make(chan struct{}), calls: map[int64]chan response{}}
}

// Line handles one line of input: a response for a call, or a request or
// notification for handle. A line that is not JSON is reported on stderr and
// skipped.
func (c *RPC) Line(line []byte) {
	var m struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		Result json.RawMessage `json:"result"`
		Error  *RPCError       `json:"error"`
	}
	if err := json.Unmarshal(line, &m); err != nil {
		fmt.Fprintf(os.Stderr, "unreadable JSON-RPC message: %v\n", err)
		return
	}
	if string(m.ID) == "null" {
		m.ID = nil
	}
	if m.Method != "" {
		c.handle(Message{ID: m.ID, Method: m.Method, Params: m.Params})
		return
	}
	id, err := strconv.ParseInt(string(m.ID), 10, 64)
	if err != nil {
		return
	}
	c.mu.Lock()
	ch := c.calls[id]
	delete(c.calls, id)
	c.mu.Unlock()
	if ch != nil { // a call that gave up on its response has no channel
		ch <- response{m.Result, m.Error}
	}
}

// End ends the connection: pending and later calls fail with err. io.EOF
// becomes "connection closed".
func (c *RPC) End(err error) {
	if errors.Is(err, io.EOF) {
		err = errors.New("connection closed")
	}
	c.err = err
	close(c.done)
}

// Done is closed when the connection has ended.
func (c *RPC) Done() <-chan struct{} { return c.done }

func (c *RPC) write(v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.w.Write(append(line, '\n'))
	return err
}

// Call sends a request and waits for its response, which is the error of a
// failed call. An error response is an *RPCError.
func (c *RPC) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	select {
	case <-c.done:
		return nil, c.err
	default:
	}
	ch := make(chan response, 1)
	c.mu.Lock()
	c.next++
	id := c.next
	c.calls[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.calls, id)
		c.mu.Unlock()
	}()
	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		return r.unwrap()
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		select {
		case r := <-ch:
			return r.unwrap()
		default:
			return nil, c.err
		}
	}
}

func (r response) unwrap() (json.RawMessage, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.result, nil
}

// Notify sends a notification.
func (c *RPC) Notify(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// Respond answers a request with a result.
func (c *RPC) Respond(id json.RawMessage, result any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

// RespondError answers a request with an error.
func (c *RPC) RespondError(id json.RawMessage, code int, message string) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": RPCError{Code: code, Message: message}})
}
