// Package ui serves the browser view and the app's routes. The page's data
// WebSocket carries Hypercube's data from the database, and the transcripts of
// the session the page selects. Browsers cap a host's HTTP connections at six;
// WebSockets are outside that cap, so open pages never stall each other's
// requests. Agents reach the tools through /mcp.
package ui

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/mcp"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

//go:embed static
var static embed.FS

// Icon returns the Hypercube SVG shared by the browser and menu bar.
func Icon() []byte {
	icon, _ := static.ReadFile("static/favicon.svg")
	return icon
}

type server struct {
	db *store.DB
	rt *session.Runtime
	mu sync.Mutex
	// snapshot is the latest state message, shown the data it was made from.
	// changed is closed and replaced when the database or a transcript changes.
	snapshot []byte
	shown    []store.ShownWorkspace
	changed  chan struct{}
}

// coalesce is how long the data socket waits after a change, so a burst of
// streamed text becomes one update.
const coalesce = 50 * time.Millisecond

// maxInput is the largest input body: 10 MiB of images in base64, and text.
const maxInput = 16 << 20

// Handler serves the page, its data socket, its actions, and the agents' tools.
// It watches the database until ctx ends.
func Handler(ctx context.Context, db *store.DB, rt *session.Runtime) http.Handler {
	s := &server{db: db, rt: rt, changed: make(chan struct{})}
	go func() {
		if err := db.Watch(ctx, s.refresh); err != nil {
			fmt.Fprintln(os.Stderr, "hypercube: watching the database:", err)
		}
	}()
	go func() {
		changes := rt.Changes()
		for {
			select {
			case <-changes:
				changes = rt.Changes()
				s.wake()
			case <-ctx.Done():
				return
			}
		}
	}()
	files, _ := fs.Sub(static, "static")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(files))
	if revision := os.Getenv("HYPERCUBE_DEV_RELOAD"); revision != "" {
		devReload(mux, revision)
	}
	mux.HandleFunc("GET /state", s.state)
	mux.HandleFunc("GET /leads", leads)
	mux.Handle("POST /mcp", mcp.Handler(db, rt))
	mux.HandleFunc("POST /workspace", s.workspaceAction)
	mux.HandleFunc("POST /session", s.sessionAction)
	mux.HandleFunc("POST /agent", s.agentAction)
	mux.HandleFunc("GET /terminal", s.terminal)
	return localOnly(mux)
}

// devReload is enabled only by scripts/dev. A rebuilt server publishes its
// source revision; open pages wait through downtime and reload when it changes.
func devReload(mux *http.ServeMux, revision string) {
	mux.HandleFunc("GET /dev-reload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, revision)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		page, _ := static.ReadFile("static/index.html")
		encoded, _ := json.Marshal(revision)
		reload := fmt.Sprintf(`<script>
setInterval(async () => {
  try {
    const response = await fetch('/dev-reload', {cache: 'no-store'});
    if (response.ok && await response.text() !== %s) location.reload();
  } catch {}
}, 1000);
</script>`, encoded)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(bytes.Replace(page, []byte("</body>"), []byte(reload+"</body>"), 1))
	})
}

// wake tells the data sockets to look for changes.
func (s *server) wake() {
	s.mu.Lock()
	defer s.mu.Unlock()
	close(s.changed)
	s.changed = make(chan struct{})
}

// refresh reads the view data after a database change and wakes the pages.
func (s *server) refresh() {
	list, err := store.Snapshot(s.db)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube: reading the view data:", err)
		return
	}
	data, _ := json.Marshal(struct {
		Type       string                 `json:"type"`
		Workspaces []store.ShownWorkspace `json:"workspaces"`
	}{"state", list})
	s.mu.Lock()
	s.snapshot, s.shown = data, list
	s.mu.Unlock()
	s.wake()
}

// sentItems is what a page has of one agent's transcript: its items' IDs in
// order, and each item as it was sent.
type sentItems struct {
	ids   []string
	items map[string][]byte
}

// transcriptMessage carries items of one agent's transcript. Reset replaces
// the page's items; otherwise the items are new or replace earlier ones.
type transcriptMessage struct {
	Type  string            `json:"type"`
	Agent string            `json:"agent"`
	Reset bool              `json:"reset,omitempty"`
	Items []json.RawMessage `json:"items"`
}

// sendTranscript sends what the page lacks of the agent's transcript: its new
// and changed items, or all of it when the page's items are no longer a
// beginning of it.
func (s *server) sendTranscript(ctx context.Context, conn *websocket.Conn, agent string, have map[string]*sentItems) error {
	items, err := s.rt.Transcript(agent)
	if err != nil {
		return err
	}
	prev := have[agent]
	reset := prev == nil || len(items) < len(prev.ids)
	next := &sentItems{items: make(map[string][]byte, len(items))}
	msg := transcriptMessage{Type: "transcript", Agent: agent, Items: []json.RawMessage{}}
	for i, it := range items {
		data, _ := json.Marshal(it)
		next.ids = append(next.ids, it.ID)
		next.items[it.ID] = data
		if !reset && i < len(prev.ids) && prev.ids[i] != it.ID {
			reset = true
		}
	}
	for _, id := range next.ids {
		if reset || !bytes.Equal(prev.items[id], next.items[id]) {
			msg.Items = append(msg.Items, next.items[id])
		}
	}
	if !reset && len(msg.Items) == 0 {
		return nil
	}
	msg.Reset = reset
	data, _ := json.Marshal(msg)
	have[agent] = next
	return conn.Write(ctx, websocket.MessageText, data)
}

// state sends the view data on connect and when it changes. The page names the
// session it shows by sending {"select": id}; the socket then sends the
// transcripts of that session's agents, whole at first and then as they
// change.
func (s *server) state(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	selections := make(chan string)
	go func() {
		defer cancel()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg struct {
				Select *string `json:"select"`
			}
			if json.Unmarshal(data, &msg) != nil || msg.Select == nil {
				continue
			}
			select {
			case selections <- *msg.Select:
			case <-ctx.Done():
				return
			}
		}
	}()
	var sent []byte
	var selected string
	releaseView := func() {}
	defer func() { releaseView() }()
	have := map[string]*sentItems{}
	for {
		s.mu.Lock()
		data, shown, changed := s.snapshot, s.shown, s.changed
		s.mu.Unlock()
		if data != nil && !bytes.Equal(data, sent) {
			if conn.Write(ctx, websocket.MessageText, data) != nil {
				return
			}
			sent = data
		}
		for _, w := range shown {
			for _, sess := range w.Sessions {
				if sess.ID != selected {
					continue
				}
				for _, a := range sess.Agents {
					if err := s.sendTranscript(ctx, conn, a.ID, have); err != nil {
						fmt.Fprintln(os.Stderr, "hypercube: sending a transcript:", err)
						return
					}
				}
			}
		}
		select {
		case <-changed:
			select {
			case <-time.After(coalesce):
			case <-ctx.Done():
				return
			}
		case id := <-selections:
			nextView := s.rt.ViewSession(id)
			releaseView()
			releaseView = nextView
			selected = id
			clear(have)
		case <-ctx.Done():
			return
		}
	}
}

// leads rereads the profiles so config edits appear in the next session menu.
func leads(w http.ResponseWriter, r *http.Request) {
	cfg, err := harness.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type lead struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Harness     string `json:"harness"`
		Model       string `json:"model"`
		Effort      string `json:"effort"`
	}
	var list []lead
	for _, p := range cfg.Leads() {
		list = append(list, lead{p.Name, p.Description, p.Harness, p.Model, p.Effort})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// localOnly refuses other hosts' names, so a rebound DNS name cannot reach the
// server, and other origins' requests, so another page cannot reach the app's
// routes. Actions must name their origin. A socket's origin is checked here
// rather than by websocket.Accept, because the terminal route starts a shell
// before accepting.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || host != "localhost" && host != "127.0.0.1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "http://"+r.Host && (r.Method != http.MethodGet || origin != "") {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// workspaceAction adds a workspace in a folder the user picks and returns its
// ID, or deletes a workspace.
func (s *server) workspaceAction(w http.ResponseWriter, r *http.Request) {
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
		added, err := session.AddWorkspace(s.db, strings.TrimSpace(string(out)))
		if refused(w, err) {
			return
		}
		fmt.Fprint(w, added.ID)
	case "delete":
		refused(w, s.rt.DeleteWorkspace(q.Get("id")))
	default:
		http.Error(w, "unknown workspace action", http.StatusBadRequest)
	}
}

// input is the JSON body of a prompt: its text and images.
type input struct {
	Text   string          `json:"text"`
	Images []harness.Image `json:"images"`
}

// readInput reads and decodes a prompt's body, answering the request and
// returning false when it is too large or malformed.
func readInput(w http.ResponseWriter, r *http.Request) (input, bool) {
	var in input
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxInput))
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return in, false
	}
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, "malformed input: "+err.Error(), http.StatusBadRequest)
		return in, false
	}
	return in, true
}

// sessionAction creates a session in a workspace from its first prompt and
// returns the session and lead IDs, or starts, stops, deletes, renames, or marks
// seen a session.
func (s *server) sessionAction(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	var err error
	switch q.Get("action") {
	case "new":
		in, ok := readInput(w, r)
		if !ok {
			return
		}
		created, err := s.rt.Create(q.Get("workspace"), q.Get("lead"), in.Text, in.Images...)
		if refused(w, err) {
			return
		}
		var lead store.Agent
		err = s.db.Read(func(tx *store.Tx) error { var err error; lead, err = tx.Lead(created.ID); return err })
		if refused(w, err) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"session": created.ID, "lead": lead.ID})
		return
	case "start":
		err = s.rt.Start(id)
	case "stop":
		err = s.rt.Stop(id)
	case "delete":
		err = s.rt.Delete(id)
	case "rename":
		err = session.Rename(s.db, id, q.Get("title"))
	case "seen":
		err = session.Seen(s.db, id)
	default:
		http.Error(w, "unknown session action", http.StatusBadRequest)
		return
	}
	refused(w, err)
}

// agentAction sends the user's input to an agent, interrupts its turn, or
// answers one of its attention requests. An input's body is JSON with its text
// and images; an answer's body is its typed reply.
func (s *server) agentAction(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	var err error
	switch q.Get("action") {
	case "input":
		in, ok := readInput(w, r)
		if !ok {
			return
		}
		err = s.rt.Input(id, in.Text, in.Images...)
	case "interrupt":
		err = s.rt.Interrupt(id)
	case "answer":
		var body []byte
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		err = s.rt.Answer(id, q.Get("request"), q.Get("choice"), string(body))
	default:
		http.Error(w, "unknown agent action", http.StatusBadRequest)
		return
	}
	refused(w, err)
}

// terminal connects the page to the session's shell. Binary messages carry the
// shell's output to the page and the page's input to the shell. The page sends
// {"cols": n, "rows": n} to resize it, and the shell's end sends {"exit": n}.
func (s *server) terminal(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	size := func(name string, fallback int) int {
		n, err := strconv.Atoi(q.Get(name))
		if err != nil || n < 1 || n > 1000 {
			return fallback
		}
		return n
	}
	cols, rows := size("cols", 80), size("rows", 24)
	sh, err := s.rt.Shell(q.Get("session"), cols, rows)
	if refused(w, err) {
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	_ = sh.Resize(cols, rows)
	replay, out, release := sh.Subscribe()
	defer release()
	go func() {
		defer cancel()
		for {
			kind, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if kind == websocket.MessageBinary {
				if sh.Write(data) != nil {
					return
				}
				continue
			}
			var resize struct{ Cols, Rows int }
			if json.Unmarshal(data, &resize) == nil && resize.Cols > 0 && resize.Rows > 0 && resize.Cols <= 1000 && resize.Rows <= 1000 {
				_ = sh.Resize(resize.Cols, resize.Rows)
			}
		}
	}()
	if len(replay) > 0 && conn.Write(ctx, websocket.MessageBinary, replay) != nil {
		return
	}
	for {
		select {
		case b, ok := <-out:
			if !ok {
				select {
				case <-sh.Done():
					exit, _ := json.Marshal(map[string]int{"exit": sh.ExitCode()})
					_ = conn.Write(ctx, websocket.MessageText, exit)
					_ = conn.Close(websocket.StatusNormalClosure, "")
				default:
				}
				return
			}
			if conn.Write(ctx, websocket.MessageBinary, b) != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// refused answers with the reason an action failed, and reports whether it did.
func refused(w http.ResponseWriter, err error) bool {
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
	}
	return err != nil
}
