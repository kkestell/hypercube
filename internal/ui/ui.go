// Package ui serves a browser view of the running workspaces. Each page drives
// its own tmux control-mode client: the server streams the client's output to
// the page and writes the page's commands to the client.
package ui

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

//go:embed static
var static embed.FS

type server struct {
	mu      sync.Mutex
	clients map[string]io.Writer
}

// Handler serves the page, its event stream, and its command endpoint.
func Handler() http.Handler {
	s := &server{clients: map[string]io.Writer{}}
	files, _ := fs.Sub(static, "static")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(files))
	mux.HandleFunc("GET /events", s.events)
	mux.HandleFunc("POST /send", s.send)
	mux.HandleFunc("POST /window", window)
	mux.HandleFunc("POST /workspace", workspaceAction)
	return localOnly(mux)
}

// localOnly refuses other hosts' names, so a rebound DNS name cannot reach the
// server, and other origins' commands, so another page cannot type into a pane.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || host != "localhost" && host != "127.0.0.1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("Origin") != "http://"+r.Host {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// events attaches a control-mode client to the requested workspace session, or
// to the first one, and streams its output lines. Lines are base64 encoded
// because pane output can split a UTF-8 sequence. The first event names the
// client for send.
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	session, err := pick(r.URL.Query().Get("session"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	cmd := exec.CommandContext(r.Context(), "tmux", "-C", "attach-session", "-t", "="+session)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := cmd.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id := rand.Text()
	s.mu.Lock()
	s.clients[id] = stdin
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, id)
		s.mu.Unlock()
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	flusher := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "event: id\ndata: %s\n\n", id)
	if flusher.Flush() != nil {
		return
	}
	lines := bufio.NewReader(stdout)
	for {
		line, err := lines.ReadBytes('\n')
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", base64.StdEncoding.EncodeToString(bytes.TrimSuffix(line, []byte("\n"))))
		if lines.Buffered() == 0 && flusher.Flush() != nil {
			return
		}
	}
}

// send writes the body's tmux commands, one per line, to the named client.
func (s *server) send(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	client := s.clients[r.URL.Query().Get("id")]
	if client == nil {
		http.Error(w, "no such client", http.StatusNotFound)
		return
	}
	if _, err := client.Write(append(bytes.TrimSuffix(body, []byte("\n")), '\n')); err != nil {
		http.Error(w, err.Error(), http.StatusGone)
	}
}

// window creates, renames, or deletes a Hypercube session.
func window(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	args := []string{"window", q.Get("action"), q.Get("state")}
	switch q.Get("action") {
	case "new":
	case "delete":
		args = append(args, q.Get("team"), q.Get("window"))
	case "rename":
		args = append(args, q.Get("team"), q.Get("window"), q.Get("title"))
	default:
		http.Error(w, "unknown window action", http.StatusBadRequest)
		return
	}
	hypercube(w, args...)
}

// workspace starts a workspace in a folder the user picks and returns its tmux
// session's name, or deletes the workspace of a state file, and reports why the
// command refused.
func workspaceAction(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch q.Get("action") {
	case "new":
		// activate brings the picker in front of the browser.
		out, err := exec.Command("osascript", "-e", "activate", "-e", `POSIX path of (choose folder with prompt "Choose the workspace's folder:")`).Output()
		var exit *exec.ExitError
		if errors.As(err, &exit) && strings.Contains(string(exit.Stderr), "(-128)") {
			return // The user cancelled.
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dir := strings.TrimSpace(string(out))
		_, name, err := workspace.At("/", dir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if hypercube(w, "start", dir) {
			fmt.Fprint(w, name)
		}
	case "delete":
		s, err := store.Load(q.Get("state"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, a := range s.Agents {
			if a.Role == store.Lead {
				hypercube(w, "delete", a.Dir)
				return
			}
		}
		http.Error(w, "no such workspace", http.StatusNotFound)
	default:
		http.Error(w, "unknown workspace action", http.StatusBadRequest)
	}
}

// hypercube runs Hypercube's own command, reports why it refused, and returns
// whether it succeeded.
func hypercube(w http.ResponseWriter, args ...string) bool {
	exe, err := os.Executable()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	if out, err := exec.Command(exe, args...).CombinedOutput(); err != nil {
		http.Error(w, strings.TrimPrefix(strings.TrimSpace(string(out)), "hypercube: "), http.StatusConflict)
		return false
	}
	return true
}

// pick returns the wanted workspace session if it is running, or else the
// first running one.
func pick(want string) (string, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}\t#{@hypercube_state}").Output()
	if err != nil {
		return "", errors.New("no running workspaces")
	}
	var first string
	for row := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		name, state, _ := strings.Cut(row, "\t")
		if state == "" {
			continue
		}
		if name == want {
			return name, nil
		}
		if first == "" {
			first = name
		}
	}
	if first == "" {
		return "", errors.New("no running workspaces")
	}
	return first, nil
}
