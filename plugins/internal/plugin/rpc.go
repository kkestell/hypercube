package plugin

import (
	"bufio"
	"bytes"
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
// speaks on its stdin and stdout.
type RPC struct {
	w     io.Writer
	wmu   sync.Mutex
	done  chan struct{}
	err   error // why the connection ended; set before done is closed
	mu    sync.Mutex
	next  int64
	calls map[int64]chan response
}

type response struct {
	result json.RawMessage
	err    *RPCError
}

// NewRPC reads messages from r and writes to w. It calls handle for each
// request and notification, in order, on the reading goroutine: handle may
// write but must not wait for a response. A line that is not JSON is
// reported on stderr and skipped. The connection ends when r does.
func NewRPC(r io.Reader, w io.Writer, handle func(Message)) *RPC {
	c := &RPC{w: w, done: make(chan struct{}), calls: map[int64]chan response{}}
	go c.read(r, handle)
	return c
}

func (c *RPC) read(r io.Reader, handle func(Message)) {
	reader := bufio.NewReaderSize(r, 1<<20)
	var err error
	for {
		var line []byte
		line, err = reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			c.line(line, handle)
		}
		if err != nil {
			break
		}
	}
	if errors.Is(err, io.EOF) {
		err = errors.New("connection closed")
	}
	c.err = err
	close(c.done)
}

func (c *RPC) line(line []byte, handle func(Message)) {
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
		handle(Message{ID: m.ID, Method: m.Method, Params: m.Params})
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

// Done is closed when the connection has ended.
func (c *RPC) Done() <-chan struct{} { return c.done }

// Err says why the connection ended. It is valid once Done is closed.
func (c *RPC) Err() error { return c.err }

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
