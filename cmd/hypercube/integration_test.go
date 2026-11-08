package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

func TestStandIn(t *testing.T) { harnesstest.Run() }

// world is an isolated state directory, configuration, and Git repository, with
// the stand-in plugin installed in plugins.
type world struct {
	t       *testing.T
	dir     string
	plugins string
	logs    string
	root    string
	db      *store.DB
}

func newWorld(t *testing.T) *world {
	t.Helper()
	dir, err := os.MkdirTemp("", "hc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	w := &world{t: t, dir: dir, plugins: filepath.Join(dir, "plugins"), logs: filepath.Join(dir, "logs"), root: filepath.Join(dir, "repo")}
	for _, d := range []string{w.plugins, w.logs, w.root, filepath.Join(dir, "config/hypercube")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	harnesstest.Install(t, w.plugins, "test")
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("HYPERCUBE_TEST_LOGS", w.logs)
	// Panel shells run $SHELL, which a bare container leaves unset.
	t.Setenv("SHELL", "/bin/sh")
	// A config from before agent profiles were removed, which the app upgrades.
	config := `{"profiles":[
		{"name":"first","description":"First lead","harness":"test","model":"haiku","effort":"low"},
		{"name":"worker","description":"test","harness":"test","model":"p/m","effort":"high"}
	]}`
	if err := os.WriteFile(filepath.Join(dir, "config/hypercube/config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", append([]string{"-C", w.root, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}
	if w.root, err = filepath.EvalSymlinks(w.root); err != nil {
		t.Fatal(err)
	}
	if w.db, err = store.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.db.Close() })
	return w
}

func (w *world) read(fn func(*store.Tx) error) {
	w.t.Helper()
	if err := w.db.Read(fn); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) family(session string) (list []store.Session) {
	w.t.Helper()
	w.read(func(tx *store.Tx) error {
		var err error
		list, err = tx.Children(session)
		if err != nil {
			return err
		}
		root, err := tx.Session(session)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		list = append([]store.Session{root}, list...)
		return nil
	})
	return list
}

func (w *world) session(id string) (s store.Session) {
	w.t.Helper()
	w.read(func(tx *store.Tx) error { var err error; s, err = tx.Session(id); return err })
	return s
}

func (w *world) until(what string, check func() bool) {
	w.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	entries, _ := os.ReadDir(w.logs)
	for _, entry := range entries {
		b, _ := os.ReadFile(filepath.Join(w.logs, entry.Name()))
		w.t.Logf("%s:\n%s", entry.Name(), b)
	}
	var list []store.Session
	_ = w.db.Read(func(tx *store.Tx) error { var err error; list, err = tx.Sessions(""); return err })
	b, _ := json.MarshalIndent(list, "", "  ")
	w.t.Logf("database: %s", b)
	w.t.Fatalf("timed out waiting for %s", what)
}

func (w *world) log(id, kind string) []harnesstest.Entry {
	w.t.Helper()
	return harnesstest.Log(w.t, w.logs, id, kind)
}

// got reports whether a prompt containing text reached the agent's plugin.
func (w *world) got(id, text string) bool {
	for _, e := range w.log(id, "input") {
		if strings.Contains(e.Text, text) {
			return true
		}
	}
	return false
}

// starts counts how many times the agent's plugin started.
func (w *world) starts(id string) int { return len(w.log(id, "start")) }

// lastPid is the process ID of the agent's most recent plugin.
func (w *world) lastPid(id string) int {
	w.t.Helper()
	starts := w.log(id, "start")
	if len(starts) == 0 {
		w.t.Fatalf("agent %s has no plugin process", id)
	}
	return starts[len(starts)-1].Pid
}

func gone(pid int) bool { return syscall.Kill(pid, 0) == syscall.ESRCH }

// client drives an app over its HTTP address.
type client struct {
	t    *testing.T
	base string
}

func (c client) post(path string, values map[string]string, body string) (int, string) {
	c.t.Helper()
	q := url.Values{}
	for k, v := range values {
		q.Set(k, v)
	}
	req, err := http.NewRequest("POST", c.base+path+"?"+q.Encode(), strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Origin", c.base)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return res.StatusCode, strings.TrimSpace(string(b))
}

// newSession starts a session with the settings and returns its ID.
func (c client) newSession(workspace string, settings harness.Settings) string {
	c.t.Helper()
	code, body := c.post("/session", map[string]string{"action": "new", "workspace": workspace, "harness": settings.Harness, "model": settings.Model, "effort": settings.Effort}, `{"text":"start"}`)
	if code != http.StatusOK {
		c.t.Fatalf("new session: %d %s", code, body)
	}
	var created struct{ Session string }
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		c.t.Fatal(err)
	}
	return created.Session
}

// freePort returns a local port that nothing is listening on.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// ready reports whether the agent's plugin has started and the agent is idle.
func (w *world) ready(id string) bool {
	return w.starts(id) > 0 && w.session(id).Activity == store.Idle
}

// TestAppProcess exercises real app exits against isolated state and plugins.
func TestAppProcess(t *testing.T) {
	w := newWorld(t)
	binary := filepath.Join(w.plugins, "hypercube")
	if out, err := exec.Command("go", "build", "-o", w.plugins+string(os.PathSeparator), ".", "../hypercube-mcp", "../hypercube-hold").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	bridgeBinary, err := filepath.EvalSymlinks(filepath.Join(w.plugins, "hypercube-mcp"))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(w.dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(bin, "hypercube")
	if err := os.Symlink(binary, linked); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bridgeBinary, "unknown").CombinedOutput(); err == nil || !strings.Contains(string(out), "usage: hypercube-mcp") {
		t.Fatalf("unknown bridge arguments should print the usage: %v %s", err, out)
	}
	if out, err := exec.Command(binary, "unknown").CombinedOutput(); err == nil || !strings.Contains(string(out), "usage: hypercube") {
		t.Fatalf("unknown commands should print the usage: %v %s", err, out)
	}
	version, err := exec.Command(binary, "version").CombinedOutput()
	if err != nil || !strings.HasPrefix(string(version), "Hypercube v") && strings.TrimSpace(string(version)) != "Hypercube devel" {
		t.Fatalf("version: %v %s", err, version)
	}
	ws, err := session.New(w.db, "", session.Programs{}).AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	var app *exec.Cmd
	var c client
	start := func() {
		t.Helper()
		port := freePort(t)
		app = exec.Command(linked, "ui", fmt.Sprint(port))
		app.Stderr = os.Stderr
		if err := app.Start(); err != nil {
			t.Fatal(err)
		}
		c = client{t, fmt.Sprintf("http://127.0.0.1:%d", port)}
		w.until("the app serving", func() bool {
			res, err := http.Get(c.base + "/config")
			if err == nil {
				res.Body.Close()
			}
			return err == nil
		})
	}
	t.Cleanup(func() {
		if app != nil && app.ProcessState == nil {
			_ = app.Process.Signal(syscall.SIGINT)
			_ = app.Wait()
		}
	})
	start()
	res, err := http.Get(c.base + "/version")
	if err != nil {
		t.Fatal(err)
	}
	serverVersion, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || !bytes.Equal(serverVersion, version) {
		t.Fatalf("running server version = %q, %v; CLI = %q", serverVersion, err, version)
	}
	res, err = http.Get(c.base + "/config")
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	err = json.NewDecoder(res.Body).Decode(&cfg)
	res.Body.Close()
	if err != nil || cfg.DefaultSelection == nil || !strings.Contains(cfg.ChildGuidance, "model p/m, effort high: test") {
		t.Fatalf("upgraded config = %+v, %v", cfg, err)
	}
	parent := w.session(c.newSession(ws.ID, *cfg.DefaultSelection))
	if parent.Model != "haiku" {
		t.Fatalf("parent %+v", parent)
	}
	w.until("the parent", func() bool { return w.starts(parent.ID) == 1 && w.session(parent.ID).Activity == store.Idle })
	if got := w.log(parent.ID, "start")[0].MCP; got != bridgeBinary {
		t.Fatalf("parent MCP executable = %q, want %q", got, bridgeBinary)
	}
	// Both the bridge executable and the legacy command reach the backend.
	bridge := func(legacy bool, tool string, args map[string]string) *exec.Cmd {
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
		cmd := exec.Command(bridgeBinary)
		if legacy {
			cmd = exec.Command(binary, "mcp")
		}
		cmd.Env = append(os.Environ(), "HYPERCUBE_SESSION_ID="+parent.ID, "HYPERCUBE_URL="+c.base)
		cmd.Stdin = bytes.NewReader(append(body, '\n'))
		return cmd
	}
	out, err := bridge(false, "create_session", map[string]string{"initiation": "own_initiative", "name": "w1", "title": "Title", "model": "p/m", "effort": "high", "message": "brief"}).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Proposed w1") {
		t.Fatalf("mcp bridge: %v %s", err, out)
	}
	if len(w.family(parent.ID)) != 1 {
		t.Fatal("a proposed child started without a click")
	}
	var proposal string
	w.read(func(tx *store.Tx) error {
		entries, err := tx.Transcript(parent.ID)
		if err != nil {
			return err
		}
		for _, e := range entries {
			var item struct{ ID, Kind string }
			if err := json.Unmarshal(e.Data, &item); err != nil {
				return err
			}
			if item.Kind == "proposal" {
				proposal = item.ID
			}
		}
		return nil
	})
	if code, body := c.post("/session", map[string]string{"action": "start-proposal", "id": parent.ID, "proposal": proposal}, ""); code != http.StatusOK {
		t.Fatalf("start proposal: %d %s", code, body)
	}
	child := w.family(parent.ID)[1]
	w.until("the child's brief", func() bool {
		return w.got(child.ID, "brief") && w.session(child.ID).Activity == store.Idle
	})
	out, err = bridge(true, "list_sessions", nil).CombinedOutput()
	if err != nil || !strings.Contains(string(out), `w1`) {
		t.Fatalf("legacy mcp command: %v %s", err, out)
	}
	pids := []int{w.lastPid(parent.ID), w.lastPid(child.ID)}

	// The desktop app connects and disconnects without affecting the backend or
	// its agents.
	desktop, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(c.base, "http")+"/desktop", nil)
	if err != nil {
		t.Fatal(err)
	}
	desktop.Close(websocket.StatusNormalClosure, "")
	if w.session(parent.ID).Activity != store.Idle || gone(w.lastPid(parent.ID)) || gone(w.lastPid(child.ID)) {
		t.Fatal("desktop disconnect disturbed the agents")
	}

	panelURL := func() string { return "ws" + strings.TrimPrefix(c.base, "http") + "/terminal?session=" + parent.ID }
	panel, _, err := websocket.Dial(t.Context(), panelURL(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { panel.CloseNow() }()
	shellPIDFile := filepath.Join(w.dir, "panel.pid")
	if err = panel.Write(t.Context(), websocket.MessageBinary, []byte("echo $$ > '"+shellPIDFile+"'; echo shell-before\n")); err != nil {
		t.Fatal(err)
	}
	w.until("panel shell PID", func() bool { b, _ := os.ReadFile(shellPIDFile); return len(b) > 0 })
	b, _ := os.ReadFile(shellPIDFile)
	shellPID, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	pids = append(pids, shellPID)
	readPanel := func(want string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		var got string
		for !strings.Contains(got, want) {
			_, b, err := panel.Read(ctx)
			if err != nil {
				t.Fatalf("panel %q: %v", got, err)
			}
			got += string(b)
		}
	}
	readPanel("shell-before")
	// Agent plugins and the panel shell run under the separate holder command.
	holders, err := filepath.Glob(filepath.Join(w.dir, "state", "hypercube", "run", "*", "pid"))
	if err != nil || len(holders) == 0 {
		t.Fatalf("holder PID files: %v, %v", holders, err)
	}
	holderBinary, err := filepath.EvalSymlinks(filepath.Join(w.plugins, "hypercube-hold"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range holders {
		pid, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		command, err := exec.Command("ps", "-p", strings.TrimSpace(string(pid)), "-o", "command=").CombinedOutput()
		if err != nil || !strings.HasPrefix(strings.TrimSpace(string(command)), holderBinary+" ") {
			t.Fatalf("holder %s command = %q: %v", pid, command, err)
		}
	}
	// SIGTERM and a crash preserve process identity and transcript continuity.
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		if err := app.Process.Signal(sig); err != nil {
			t.Fatal(err)
		}
		waitErr := app.Wait()
		if sig == syscall.SIGTERM && waitErr != nil {
			t.Fatal(waitErr)
		}
		for _, pid := range pids {
			if gone(pid) {
				t.Fatalf("agent %d died with app", pid)
			}
		}
		start()
		panel.CloseNow()
		panel, _, err = websocket.Dial(t.Context(), panelURL(), nil)
		if err != nil {
			t.Fatal(err)
		}
		readPanel("shell-before")
		if err = panel.Write(t.Context(), websocket.MessageBinary, []byte("echo shell-after\n")); err != nil {
			t.Fatal(err)
		}
		readPanel("shell-after")
		status, body := c.post("/session", map[string]string{"action": "input", "id": parent.ID}, `{"text":"after restart"}`)
		if status != http.StatusOK {
			t.Fatalf("input: %d %s", status, body)
		}
		w.until("input after restart", func() bool { return w.session(parent.ID).Activity == store.Idle })
		if w.starts(parent.ID) != 1 || w.starts(child.ID) != 1 {
			t.Fatal("restart replaced agents")
		}
	}
	if err := app.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if err := app.Wait(); err != nil {
		t.Fatal(err)
	}
	for _, pid := range pids {
		if !gone(pid) {
			t.Fatalf("agent %d survived Quit", pid)
		}
	}
	// After Quit the sessions stay parked until used; a prompt wakes only its
	// session.
	start()
	if w.starts(parent.ID) != 1 || w.starts(child.ID) != 1 {
		t.Fatal("restore after Quit started agents")
	}
	status, body := c.post("/session", map[string]string{"action": "input", "id": parent.ID}, `{"text":"after quit"}`)
	if status != http.StatusOK {
		t.Fatalf("input after Quit: %d %s", status, body)
	}
	w.until("the parent after Quit", func() bool { return w.starts(parent.ID) == 2 && w.ready(parent.ID) })
	if w.starts(child.ID) != 1 {
		t.Fatal("a prompt to the parent woke its child")
	}
}
