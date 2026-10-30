// Package mux keeps Hypercube's panes, windows, labels, and attention in the
// mux, through its socket.
package mux

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/mux/client"
)

// exe is the mux installed beside this executable. Both come from the same
// build.
func exe() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(self), "mux"), nil
}

func dial() (*client.Conn, error) { return client.Dial(client.SocketPath()) }

// target sets the field of req that names id, by the ID's first letter.
func target(req *client.Request, id string) {
	switch id[0] {
	case 'p':
		req.Pane = id
	case 'w':
		req.Window = id
	default:
		req.Session = id
	}
}

// Spawn starts a pane in a new window of session, in an existing window, or in
// a new session when both are empty. It starts the mux server if none is
// running.
func Spawn(session, window, dir string, argv []string, env map[string]string) (client.Reply, error) {
	mux, err := exe()
	if err != nil {
		return client.Reply{}, err
	}
	c, err := client.DialOrStart(mux)
	if err != nil {
		return client.Reply{}, err
	}
	defer c.Close()
	req := client.Request{Type: "spawn", Session: session, Window: window, Dir: dir, Argv: argv}
	for key, value := range env {
		req.Env = append(req.Env, key+"="+value)
	}
	return c.Call(req)
}

// Kill removes the pane, window, or session id names. An ID that names nothing
// is already gone, as is one the server never had.
func Kill(id string) error {
	if id == "" {
		return nil
	}
	c, err := dial()
	if errors.Is(err, client.ErrNoServer) {
		return nil
	}
	if err != nil {
		return err
	}
	defer c.Close()
	req := client.Request{Type: "kill"}
	target(&req, id)
	if _, err := c.Call(req); err != nil && !strings.HasPrefix(err.Error(), "unknown ") {
		return err
	}
	return nil
}

// Label sets the plain-text label of the pane, window, or session id names.
func Label(id, text string) error {
	c, err := dial()
	if err != nil {
		return err
	}
	defer c.Close()
	req := client.Request{Type: "label", Spans: []client.Span{{Text: text}}}
	target(&req, id)
	_, err = c.Call(req)
	return err
}

// Select makes the window or pane id names the one its session shows.
func Select(id string) error {
	c, err := dial()
	if err != nil {
		return err
	}
	defer c.Close()
	req := client.Request{Type: "select"}
	target(&req, id)
	_, err = c.Call(req)
	return err
}

// Attach runs a mux client on the terminal until the user detaches.
func Attach(session string) error {
	mux, err := exe()
	if err != nil {
		return err
	}
	cmd := exec.Command(mux, "attach", session)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func list() ([]client.Session, error) {
	c, err := dial()
	if errors.Is(err, client.ErrNoServer) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer c.Close()
	reply, err := c.Call(client.Request{Type: "list"})
	return reply.Sessions, err
}

// find returns the session and window that hold pane, or empty strings.
func find(sessions []client.Session, pane string) (session, window string) {
	for _, s := range sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.ID == pane {
					return s.ID, w.ID
				}
			}
		}
	}
	return "", ""
}

// Workspace returns the mux session that holds any of the workspace's agents,
// or "" when none is running.
func Workspace(s store.State) (string, error) {
	sessions, err := list()
	if err != nil {
		return "", err
	}
	for _, a := range s.Agents {
		if session, _ := find(sessions, a.Target); session != "" {
			return session, nil
		}
	}
	return "", nil
}

// Team returns the window that holds the team's lead, or "" when the team is
// not running. If a user kills a lead's pane by hand while its workers live,
// the team counts as not running and the next hypercube starts it again in a
// new window.
func Team(s store.State, team string) (string, error) {
	lead := s.TeamLead(team)
	if lead == nil {
		return "", nil
	}
	sessions, err := list()
	if err != nil {
		return "", err
	}
	_, window := find(sessions, lead.Target)
	return window, nil
}

// RefreshAttention is best effort: a closed pane must not make a delivered
// message fail. It reads the newest state after the hook or wake-up commits it.
func RefreshAttention(path, id string) {
	s, err := store.Load(path)
	if err != nil {
		return
	}
	a, err := s.Agent(id)
	if err != nil || a.Target == "" {
		return
	}
	c, err := dial()
	if err != nil {
		return
	}
	defer c.Close()
	_, _ = c.Call(client.Request{Type: "attention", Pane: a.Target, On: a.Attention})
}
