// Package server holds the mux's sessions, windows, and panes and serves them
// over a Unix socket.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/kkestell/hypercube/mux/client"
)

// A session's active window and a window's active pane are what the attached
// client sees and types into. They stay with the session and window, so a
// client that attaches again finds them as it left them.
type session struct {
	id      string
	windows []*window
	active  *window
	marks
}

type window struct {
	id     string
	panes  []*pane
	active *pane
	split  float64 // the main pane's share of the width
	marks
}

// marks is the label a client set on a session, window, or pane. The server's
// mutex guards it.
type marks struct {
	spans []client.Span // empty shows the default
}

// attention reports whether any pane of w is marked.
func (w *window) attention() bool {
	return slices.ContainsFunc(w.panes, func(p *pane) bool { return p.attention })
}

// attention reports whether any pane of sess is marked.
func (sess *session) attention() bool {
	return slices.ContainsFunc(sess.windows, (*window).attention)
}

// Server guards the session tree and every terminal with one mutex.
type Server struct {
	mu       sync.Mutex
	counters map[byte]int // one per kind of ID: 's', 'w', 'p'
	tag      string       // the random part of every ID, so an ID from an earlier server names nothing
	sessions []*session
	attached map[*attachment]struct{}

	path     string // the socket, which kill removes with the last session
	listener net.Listener
	exited   chan struct{} // closed once the last session's kill has been answered
}

func New() *Server {
	return &Server{
		counters: map[byte]int{},
		tag:      fmt.Sprintf("%04x", rand.IntN(1<<16)),
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
	case "attention":
		err = s.attention(req)
	case "collapse":
		err = s.collapse(req)
	case "label":
		err = s.label(req)
	case "select":
		err = s.selectTarget(req)
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

// nextID must be called with the mutex held. Two servers that draw the same tag
// and counter give the same ID, a 1 in 65,536 chance; that is unlikely.
func (s *Server) nextID(kind byte) string {
	s.counters[kind]++
	return fmt.Sprintf("%c%d-%s", kind, s.counters[kind], s.tag)
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
		w = &window{id: s.nextID('w'), panes: []*pane{p}, active: p, split: 0.4}
		if sess == nil {
			sess = &session{id: s.nextID('s'), active: w}
			s.sessions = append(s.sessions, sess)
		}
		sess.windows = append(sess.windows, w)
	}
	s.layoutSession(sess)
	s.signalAll() // every status bar lists the session's label
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
				p.hangUp()
				return true
			})
		}
		sess.windows = slices.DeleteFunc(sess.windows, func(w *window) bool { return len(w.panes) == 0 })
		for _, w := range sess.windows {
			if !slices.Contains(w.panes, w.active) {
				w.active = w.panes[0]
			}
			w.panes[0].collapsed = false // a stacked pane may have become the main pane
		}
		if len(sess.windows) > 0 && !slices.Contains(sess.windows, sess.active) {
			sess.active = sess.windows[0]
		}
	}
	s.sessions = slices.DeleteFunc(s.sessions, func(sess *session) bool { return len(sess.windows) == 0 })
	for a := range s.attached {
		if slices.Contains(s.sessions, a.session) {
			s.layoutSession(a.session) // the window may have lost panes
		} else {
			delete(s.attached, a)
			a.conn.Close()
		}
	}
	s.signalAll() // every status bar lists the labels

	if len(s.sessions) == 0 {
		os.Remove(s.path)
		s.listener.Close()
	}
	return nil
}

// collapse collapses a stacked pane to its title row, or opens it when req.On
// is false.
func (s *Server) collapse(req client.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		for _, w := range sess.windows {
			i := slices.IndexFunc(w.panes, func(p *pane) bool { return p.id == req.Pane })
			if i < 0 {
				continue
			}
			if i == 0 {
				return fmt.Errorf("pane %s is its window's main pane, which cannot collapse", req.Pane)
			}
			w.setCollapsed(w.panes[i], req.On)
			s.layoutSession(sess)
			return nil
		}
	}
	return fmt.Errorf("unknown pane %q", req.Pane)
}

// setCollapsed collapses or opens a stacked pane of w. A collapsed pane gives
// the focus to the main pane. It must be called with the server's mutex held.
func (w *window) setCollapsed(p *pane, on bool) {
	p.collapsed = on
	if on && w.active == p {
		w.active = w.panes[0]
	}
}

// attention marks the pane req names as needing attention, or clears the mark
// when req.On is false. A window or session shows the bell while any of its
// panes is marked.
func (s *Server) attention(req client.Request) error {
	if req.Pane == "" || req.Window != "" || req.Session != "" {
		return fmt.Errorf("attention needs a pane")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.findPane(req.Pane)
	if p == nil {
		return fmt.Errorf("unknown pane %q", req.Pane)
	}
	if req.On && !p.attention {
		s.ring()
	}
	p.attention = req.On
	s.signalAll() // every status bar lists the sessions
	return nil
}

// selectTarget makes the window or pane req names the one its session shows
// and, for a pane, the one its window types into. A collapsed pane opens first,
// as a click on its title does. It does not move an attached client to another
// session.
func (s *Server) selectTarget(req client.Request) error {
	if req.Session != "" || (req.Window == "") == (req.Pane == "") {
		return fmt.Errorf("select needs exactly one of window or pane")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var sess *session
	var w *window
	if req.Window != "" {
		if sess, w = s.findWindow(req.Window); w == nil {
			return fmt.Errorf("unknown window %q", req.Window)
		}
	} else {
		for _, candidate := range s.sessions {
			for _, cw := range candidate.windows {
				if i := slices.IndexFunc(cw.panes, func(p *pane) bool { return p.id == req.Pane }); i >= 0 {
					sess, w = candidate, cw
					w.panes[i].collapsed = false
					w.active = w.panes[i]
				}
			}
		}
		if w == nil {
			return fmt.Errorf("unknown pane %q", req.Pane)
		}
	}
	sess.active = w
	s.layoutSession(sess)
	return nil
}

// label sets the label of the one pane, window, or session req names, or
// restores the default when req.Spans is empty.
func (s *Server) label(req client.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.marks(req)
	if err != nil {
		return err
	}
	m.spans = req.Spans
	s.signalAll() // every status bar lists the sessions
	return nil
}

// marks finds the marks of the one pane, window, or session req names. It must
// be called with the mutex held.
func (s *Server) marks(req client.Request) (*marks, error) {
	switch {
	case req.Pane != "" && req.Window == "" && req.Session == "":
		if p := s.findPane(req.Pane); p != nil {
			return &p.marks, nil
		}
		return nil, fmt.Errorf("unknown pane %q", req.Pane)
	case req.Window != "" && req.Pane == "" && req.Session == "":
		if _, w := s.findWindow(req.Window); w != nil {
			return &w.marks, nil
		}
		return nil, fmt.Errorf("unknown window %q", req.Window)
	case req.Session != "" && req.Pane == "" && req.Window == "":
		if sess := s.findSession(req.Session); sess != nil {
			return &sess.marks, nil
		}
		return nil, fmt.Errorf("unknown session %q", req.Session)
	}
	return nil, fmt.Errorf("%s needs exactly one of pane, window, or session", req.Type)
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
	p.collapsed, p.attention, p.marks = old.collapsed, old.attention, old.marks
	for _, sess := range s.sessions {
		for _, w := range sess.windows {
			if i := slices.Index(w.panes, old); i >= 0 {
				w.panes[i] = p
				if w.active == old {
					w.active = p
				}
			}
		}
	}
	s.signalAll() // the command name may differ
	old.hangUp()
	return nil
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
