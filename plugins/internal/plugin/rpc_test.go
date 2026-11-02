package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// peer is the far end of an RPC connection.
type peer struct {
	in  *bufio.Reader // what the RPC wrote
	out io.WriteCloser
}

func connect(t *testing.T, handle func(Message)) (*RPC, *peer) {
	t.Helper()
	clientIn, peerOut := io.Pipe()
	peerIn, clientOut := io.Pipe()
	c := NewRPC(clientOut, handle)
	go func() { c.End(ReadLines(clientIn, c.Line)) }()
	t.Cleanup(func() { peerOut.Close(); peerIn.Close() })
	return c, &peer{in: bufio.NewReader(peerIn), out: peerOut}
}

func (p *peer) read(t *testing.T) map[string]any {
	t.Helper()
	line, err := p.in.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(line, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func (p *peer) send(t *testing.T, line string) {
	t.Helper()
	if _, err := io.WriteString(p.out, line+"\n"); err != nil {
		t.Fatal(err)
	}
}

func TestCallMatchesResponsesByID(t *testing.T) {
	c, p := connect(t, func(Message) {})
	type result struct {
		raw json.RawMessage
		err error
	}
	first, second := make(chan result, 1), make(chan result, 1)
	call := func(out chan result, method string) {
		raw, err := c.Call(context.Background(), method, map[string]any{"n": 1})
		out <- result{raw, err}
	}
	go call(first, "one")
	a := p.read(t)
	go call(second, "two")
	b := p.read(t)
	if a["method"] != "one" || b["method"] != "two" || a["jsonrpc"] != "2.0" || a["id"] == b["id"] {
		t.Fatal(a, b)
	}
	// Responses arrive in the opposite order.
	idB, _ := json.Marshal(b["id"])
	idA, _ := json.Marshal(a["id"])
	p.send(t, `{"jsonrpc":"2.0","id":`+string(idB)+`,"result":{"which":"two"}}`)
	if r := <-second; r.err != nil || string(r.raw) != `{"which":"two"}` {
		t.Fatal(r)
	}
	p.send(t, `{"jsonrpc":"2.0","id":`+string(idA)+`,"error":{"code":-32602,"message":"bad params"}}`)
	r := <-first
	var rpcErr *RPCError
	if !errors.As(r.err, &rpcErr) || rpcErr.Code != -32602 || rpcErr.Message != "bad params" {
		t.Fatal(r)
	}
}

func TestIncomingMessages(t *testing.T) {
	got := make(chan Message, 4)
	c, p := connect(t, func(m Message) { got <- m })
	p.send(t, "not json")
	p.send(t, `{"method":"note","params":{"a":1}}`)
	p.send(t, `{"jsonrpc":"2.0","id":7,"method":"ask","params":{"b":2}}`)
	p.send(t, `{"jsonrpc":"2.0","id":"s","method":"ask2"}`)
	note, ask, ask2 := <-got, <-got, <-got
	if note.Method != "note" || len(note.ID) != 0 || string(note.Params) != `{"a":1}` {
		t.Fatal(note)
	}
	if ask.Method != "ask" || string(ask.ID) != "7" || string(ask.Params) != `{"b":2}` {
		t.Fatal(ask)
	}
	go func() {
		_ = c.Respond(ask.ID, map[string]string{"ok": "yes"})
		_ = c.RespondError(ask2.ID, -32601, "no such method")
		_ = c.Notify("done", map[string]int{"n": 1})
	}()
	r := p.read(t)
	if r["id"] != float64(7) || r["result"].(map[string]any)["ok"] != "yes" {
		t.Fatal(r)
	}
	r = p.read(t)
	if r["id"] != "s" || r["error"].(map[string]any)["message"] != "no such method" {
		t.Fatal(r)
	}
	if r = p.read(t); r["method"] != "done" || r["id"] != nil {
		t.Fatal(r)
	}
}

// Messages reach the handler in the order they were written, and a response
// follows the notifications written before it.
func TestHandlerOrder(t *testing.T) {
	var mu sync.Mutex
	var order []string
	c, p := connect(t, func(m Message) { mu.Lock(); order = append(order, m.Method); mu.Unlock() })
	done := make(chan error, 1)
	go func() {
		_, err := c.Call(context.Background(), "go", nil)
		mu.Lock()
		order = append(order, "response")
		mu.Unlock()
		done <- err
	}()
	p.read(t)
	p.send(t, `{"method":"a"}`)
	p.send(t, `{"method":"b"}`)
	p.send(t, `{"id":1,"result":{}}`)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(order, ",") != "a,b,response" {
		t.Fatal(order)
	}
}

func TestCallEnds(t *testing.T) {
	t.Run("context", func(t *testing.T) {
		c, p := connect(t, func(Message) {})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { _, err := c.Call(ctx, "slow", nil); done <- err }()
		p.read(t)
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		// The answer to an abandoned call is dropped.
		p.send(t, `{"id":1,"result":{}}`)
		go func() { _, _ = c.Call(context.Background(), "next", nil) }()
		if m := p.read(t); m["method"] != "next" {
			t.Fatal(m)
		}
	})
	t.Run("closed connection", func(t *testing.T) {
		c, p := connect(t, func(Message) {})
		done := make(chan error, 1)
		go func() { _, err := c.Call(context.Background(), "x", nil); done <- err }()
		p.read(t)
		p.out.Close()
		select {
		case err := <-done:
			if err == nil || err.Error() != "connection closed" {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("call did not end")
		}
		<-c.Done()
		if c.Err() == nil {
			t.Fatal("no error")
		}
		if _, err := c.Call(context.Background(), "y", nil); err == nil {
			t.Fatal("call on a closed connection succeeded")
		}
	})
}

// A response on the last line of the output, without a newline, still reaches
// its call.
func TestFinalResponseWithoutNewline(t *testing.T) {
	c, p := connect(t, func(Message) {})
	done := make(chan error, 1)
	go func() { _, err := c.Call(context.Background(), "x", nil); done <- err }()
	p.read(t)
	if _, err := io.WriteString(p.out, `{"id":1,"result":{}}`); err != nil {
		t.Fatal(err)
	}
	p.out.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
