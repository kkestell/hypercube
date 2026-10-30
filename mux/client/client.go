// Package client is the mux wire protocol: JSON lines over the server's socket.
package client

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// Request is one flat struct for every request type. Each field is omitted
// when empty.
type Request struct {
	Type    string   `json:"type"`
	Session string   `json:"session,omitempty"`
	Window  string   `json:"window,omitempty"`
	Pane    string   `json:"pane,omitempty"`
	Dir     string   `json:"dir,omitempty"`
	Argv    []string `json:"argv,omitempty"`
	Env     []string `json:"env,omitempty"`
	Cols    int      `json:"cols,omitempty"`
	Rows    int      `json:"rows,omitempty"`
	Data    []byte   `json:"data,omitempty"`
	On      bool     `json:"on,omitempty"`
	Spans   []Span   `json:"spans,omitempty"`
}

// Span is a run of label text. Color and bold come later.
type Span struct {
	Text string `json:"text"`
}

// Reply answers one request.
type Reply struct {
	OK      bool   `json:"ok,omitempty"`
	Error   string `json:"error,omitempty"`
	Session string `json:"session,omitempty"`
	Window  string `json:"window,omitempty"`
	Pane    string `json:"pane,omitempty"`
	Text    string `json:"text,omitempty"`

	Sessions []Session `json:"sessions,omitempty"`
}

// Session, Window, and Pane are the tree a list reply returns, in creation
// order. A pane's status is 0 while it is alive.
type Session struct {
	ID      string   `json:"id"`
	Windows []Window `json:"windows"`
}

type Window struct {
	ID    string `json:"id"`
	Panes []Pane `json:"panes"`
}

type Pane struct {
	ID     string `json:"id"`
	Alive  bool   `json:"alive"`
	Status int    `json:"status"`
}

// Output is one line of an attached client's stream: a frame to write to the
// terminal, or the notice that the server detached the client.
type Output struct {
	Frame    []byte `json:"frame,omitempty"`
	Detached bool   `json:"detached,omitempty"`
}

// SocketPath returns $XDG_STATE_HOME/mux/socket, or ~/.local/state/mux/socket.
func SocketPath() string {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, _ := os.UserHomeDir()
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "mux", "socket")
}

// Conn is a connection to the server. Send is safe for concurrent use, so one
// goroutine may read while others send. Reads are not.
type Conn struct {
	conn net.Conn
	r    *bufio.Reader
	mu   sync.Mutex // guards enc
	enc  *json.Encoder
}

func Dial(path string) (*Conn, error) {
	c, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	return &Conn{conn: c, r: bufio.NewReader(c), enc: json.NewEncoder(c)}, nil
}

func (c *Conn) Close() error { return c.conn.Close() }

// Send writes a request without waiting for a reply.
func (c *Conn) Send(req Request) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(req)
}

// Call sends a request and returns its reply. A reply with an error becomes a
// Go error.
func (c *Conn) Call(req Request) (Reply, error) {
	if err := c.Send(req); err != nil {
		return Reply{}, err
	}
	var reply Reply
	if err := c.read(&reply); err != nil {
		return Reply{}, err
	}
	if reply.Error != "" {
		return reply, errors.New(reply.Error)
	}
	return reply, nil
}

// ReadOutput reads the next line of an attached client's stream.
func (c *Conn) ReadOutput() (Output, error) {
	var out Output
	err := c.read(&out)
	return out, err
}

func (c *Conn) read(v any) error {
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, v)
}
