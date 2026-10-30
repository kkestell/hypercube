// Package server holds the mux's sessions, windows, and panes and serves them
// over a Unix socket.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/kkestell/hypercube/mux/client"
)

type session struct {
	id      string
	windows []*window
}

type window struct {
	id    string
	panes []*pane
}

// Server guards the session tree and every terminal with one mutex.
type Server struct {
	mu       sync.Mutex
	counters map[byte]int // one per kind of ID: 's', 'w', 'p'
	sessions []*session
	attached map[*attachment]struct{}

	path     string // the socket, which kill removes with the last session
	listener net.Listener
	exited   chan struct{} // closed once the last session's kill has been answered
}

func New() *Server {
	return &Server{
		counters: map[byte]int{},
		attached: map[*attachment]struct{}{},
		exited:   make(chan struct{}),
	}
}

// ListenAndServe serves the socket at path until the listener fails, or until
// kill removes the last session, which returns nil. It fails if another server
// is already listening there.
func (s *Server) ListenAndServe(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if c, err := client.Dial(path); err == nil {
		c.Close()
		return fmt.Errorf("a server is already listening on %s", path)
	}
	os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer l.Close()
	s.path, s.listener = path, l
	for {
		conn, err := l.Accept()
		if errors.Is(err, net.ErrClosed) {
			<-s.exited
			return nil
		}
		if err != nil {
			return err
		}
		go s.serve(conn)
	}
}

func (s *Server) serve(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	for {
		var req client.Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		if req.Type == "attach" {
			s.attach(conn, dec, enc, req)
			return
		}
		reply := s.handle(req)
		err := enc.Encode(reply)
		// The server exits even if the reply could not be written. A client
		// that connected before the listener closed could spawn between the
		// kill and this check, and its pane would die with the server, or
		// keep the server from exiting; that is unlikely. Two kills that
		// finish at once and leave no session both see it empty and close
		// exited twice, which panics; that is unlikely too.
		if req.Type == "kill" && reply.OK && s.empty() {
			close(s.exited)
		}
		if err != nil {
			return
		}
	}
}

func (s *Server) handle(req client.Request) client.Reply {
	var reply client.Reply
	var err error
	switch req.Type {
	case "spawn":
		reply, err = s.spawn(req)
	case "capture":
		reply, err = s.capture(req)
	case "list":
		reply = s.list()
	case "kill":
		err = s.kill(req)
	case "respawn":
		err = s.respawn(req)
	default:
		err = fmt.Errorf("unknown request type %q", req.Type)
	}
	if err != nil {
		return client.Reply{Error: err.Error()}
	}
	reply.OK = true
	return reply
}

func (s *Server) empty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions) == 0
}

// nextID must be called with the mutex held.
func (s *Server) nextID(kind byte) string {
	s.counters[kind]++
	return fmt.Sprintf("%c%d", kind, s.counters[kind])
}

// spawn starts a pane in a new session, or in a new window of req.Session, or in
// an existing window req.Window.
func (s *Server) spawn(req client.Request) (client.Reply, error) {
	if len(req.Argv) == 0 {
		return client.Reply{}, fmt.Errorf("spawn needs a command")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var sess *session
	var w *window
	switch {
	case req.Window != "":
		sess, w = s.findWindow(req.Window)
		if w == nil {
			return client.Reply{}, fmt.Errorf("unknown window %q", req.Window)
		}
		if req.Session != "" && req.Session != sess.id {
			return client.Reply{}, fmt.Errorf("window %s is not in session %s", w.id, req.Session)
		}
	case req.Session != "":
		if sess = s.findSession(req.Session); sess == nil {
			return client.Reply{}, fmt.Errorf("unknown session %q", req.Session)
		}
	}

	p, err := s.startPane(s.nextID('p'), req.Dir, req.Argv, req.Env, defaultCols, defaultRows)
	if err != nil {
		return client.Reply{}, err
	}
	if w != nil {
		w.panes = append(w.panes, p)
	} else {
		w = &window{id: s.nextID('w'), panes: []*pane{p}}
		if sess == nil {
			sess = &session{id: s.nextID('s')}
			s.sessions = append(s.sessions, sess)
		}
		sess.windows = append(sess.windows, w)
	}
	return client.Reply{Session: sess.id, Window: w.id, Pane: p.id}, nil
}

func (s *Server) list() client.Reply {
	s.mu.Lock()
	defer s.mu.Unlock()
	var reply client.Reply
	for _, sess := range s.sessions {
		cs := client.Session{ID: sess.id}
		for _, w := range sess.windows {
			cw := client.Window{ID: w.id}
			for _, p := range w.panes {
				cw.Panes = append(cw.Panes, client.Pane{ID: p.id, Alive: p.alive, Status: p.status})
			}
			cs.Windows = append(cs.Windows, cw)
		}
		reply.Sessions = append(reply.Sessions, cs)
	}
	return reply
}

func (s *Server) capture(req client.Request) (client.Reply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.findPane(req.Pane)
	if p == nil {
		return client.Reply{}, fmt.Errorf("unknown pane %q", req.Pane)
	}
	text, err := p.capture()
	return client.Reply{Text: text}, err
}

// kill removes the one pane, window, or session req names, then any window or
// session that has no panes left. When no session remains it removes the socket
// and closes the listener before the reply, so no new client can connect.
func (s *Server) kill(req client.Request) error {
	targets := 0
	for _, id := range []string{req.Pane, req.Window, req.Session} {
		if id != "" {
			targets++
		}
	}
	if targets != 1 {
		return fmt.Errorf("kill needs exactly one of pane, window, or session")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case req.Pane != "" && s.findPane(req.Pane) == nil:
		return fmt.Errorf("unknown pane %q", req.Pane)
	case req.Window != "":
		if _, w := s.findWindow(req.Window); w == nil {
			return fmt.Errorf("unknown window %q", req.Window)
		}
	case req.Session != "" && s.findSession(req.Session) == nil:
		return fmt.Errorf("unknown session %q", req.Session)
	}

	// IDs are never empty, so the two fields req leaves empty match nothing.
	for _, sess := range s.sessions {
		for _, w := range sess.windows {
			w.panes = slices.DeleteFunc(w.panes, func(p *pane) bool {
				if sess.id != req.Session && w.id != req.Window && p.id != req.Pane {
					return false
				}
				s.removePane(p)
				return true
			})
		}
		sess.windows = slices.DeleteFunc(sess.windows, func(w *window) bool { return len(w.panes) == 0 })
	}
	s.sessions = slices.DeleteFunc(s.sessions, func(sess *session) bool { return len(sess.windows) == 0 })

	if len(s.sessions) == 0 {
		os.Remove(s.path)
		s.listener.Close()
	}
	return nil
}

// respawn replaces the pane's process with a new one of the same size, keeping
// the pane's ID and place in its window. If the new process cannot start, the
// pane keeps the old one.
func (s *Server) respawn(req client.Request) error {
	if len(req.Argv) == 0 {
		return fmt.Errorf("respawn needs a command")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.findPane(req.Pane)
	if old == nil {
		return fmt.Errorf("unknown pane %q", req.Pane)
	}
	p, err := s.startPane(old.id, req.Dir, req.Argv, req.Env, old.cols, old.rows)
	if err != nil {
		return err
	}
	for _, sess := range s.sessions {
		for _, w := range sess.windows {
			if i := slices.Index(w.panes, old); i >= 0 {
				w.panes[i] = p
			}
		}
	}
	for a := range s.attached {
		if a.pane == old {
			a.pane = p
			a.signal()
		}
	}
	old.hangUp()
	return nil
}

// removePane ends the attachments that show p and hangs up its process. It
// must be called with the mutex held.
func (s *Server) removePane(p *pane) {
	for a := range s.attached {
		if a.pane == p {
			delete(s.attached, a)
			a.conn.Close()
		}
	}
	p.hangUp()
}

// findPane must be called with the mutex held.
func (s *Server) findPane(id string) *pane {
	for _, sess := range s.sessions {
		for _, w := range sess.windows {
			for _, p := range w.panes {
				if p.id == id {
					return p
				}
			}
		}
	}
	return nil
}

// findSession must be called with the mutex held.
func (s *Server) findSession(id string) *session {
	for _, sess := range s.sessions {
		if sess.id == id {
			return sess
		}
	}
	return nil
}

// findWindow returns the window and its session. It must be called with the
// mutex held.
func (s *Server) findWindow(id string) (*session, *window) {
	for _, sess := range s.sessions {
		for _, w := range sess.windows {
			if w.id == id {
				return sess, w
			}
		}
	}
	return nil, nil
}
