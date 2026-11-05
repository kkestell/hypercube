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

	"github.com/kkestell/hypercube/internal/harness/harnesstest"
	"github.com/kkestell/hypercube/internal/mcp"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
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
	config := `{"profiles":[
		{"name":"first","description":"First lead","harness":"test","model":"haiku","effort":"low"},
		{"name":"second","description":"Second lead","harness":"test","model":"sol","effort":"high"},
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

// newSession starts a session with the named profile and returns its ID.
func (c client) newSession(workspace, profile string) string {
	c.t.Helper()
	code, body := c.post("/session", map[string]string{"action": "new", "workspace": workspace, "profile": profile}, `{"text":"start"}`)
	if code != http.StatusOK {
		c.t.Fatalf("new session: %d %s", code, body)
	}
	var created struct{ Session string }
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		c.t.Fatal(err)
	}
	return created.Session
}

// sessionAction posts an action on a session that the app accepts.
func (c client) sessionAction(action, id string) {
	c.t.Helper()
	if code, body := c.post("/session", map[string]string{"action": action, "id": id}, ""); code != http.StatusOK {
		c.t.Fatalf("session %s: %d %s", action, code, body)
	}
}

// asAgent adds the agent's identity to every request.
type asAgent string

func (a asAgent) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set(mcp.SessionHeader, string(a))
	return http.DefaultTransport.RoundTrip(r)
}

// tool calls an MCP tool as the agent through the app's route and returns its text.
func (c client) tool(agent, name string, args map[string]string) string {
	c.t.Helper()
	text, failed := c.callTool(agent, name, args)
	if failed {
		c.t.Fatalf("MCP %s: %s", name, text)
	}
	return text
}

// callTool also returns tool refusals so tests can check failed operations.
func (c client) callTool(agent, name string, args map[string]string) (string, bool) {
	c.t.Helper()
	session, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(c.t.Context(), &sdk.StreamableClientTransport{
		Endpoint:             c.base + "/mcp",
		HTTPClient:           &http.Client{Transport: asAgent(agent)},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		c.t.Fatal(err)
	}
	defer session.Close()
	res, err := session.CallTool(c.t.Context(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		c.t.Fatalf("MCP %s: %v", name, err)
	}
	return res.Content[0].(*sdk.TextContent).Text, res.IsError
}

type shownWorkspace struct {
	Sessions []struct {
		ID      string
		Running bool
	}
}

func (c client) snapshot() []shownWorkspace {
	c.t.Helper()
	conn, _, err := websocket.Dial(c.t.Context(), "ws"+strings.TrimPrefix(c.base, "http")+"/state", nil)
	if err != nil {
		c.t.Fatal(err)
	}
	defer conn.CloseNow()
	_, data, err := conn.Read(c.t.Context())
	if err != nil {
		c.t.Fatal(err)
	}
	var msg struct{ Workspaces []shownWorkspace }
	if err := json.Unmarshal(data, &msg); err != nil {
		c.t.Fatal(err)
	}
	return msg.Workspaces
}

// listen returns a free local address.
func listen(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// runApp starts the app in this process on a free port.
func (w *world) runApp() (*app, client) {
	w.t.Helper()
	listener := listen(w.t)
	a, err := startApp(listener, w.plugins, nil, nil)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(a.shutdown)
	return a, client{w.t, "http://" + listener.Addr().String()}
}

// ready reports whether the agent's plugin has started and the agent is idle.
func (w *world) ready(id string) bool {
	return w.starts(id) > 0 && w.session(id).Activity == store.Idle
}

// newSession starts a session with the named profile and waits for its agent.
func (w *world) newSession(c client, workspace, profile string) store.Session {
	w.t.Helper()
	id := c.newSession(workspace, profile)
	w.until("session "+id, func() bool { return w.ready(id) })
	return w.session(id)
}

// createChild has the parent create a child through MCP and waits for the
// child's plugin to start and receive the brief, if any.
func (w *world) createChild(c client, parent store.Session, name, brief string) store.Session {
	w.t.Helper()
	args := map[string]string{"name": name, "title": "Title", "profile": "worker"}
	if brief != "" {
		args["message"] = brief
	}
	c.tool(parent.ID, "create_session", args)
	var child store.Session
	w.until("child "+name, func() bool {
		for _, a := range w.family(parent.ID) {
			if a.Name == name {
				child = a
			}
		}
		return child.ID != "" && w.starts(child.ID) > 0 && (brief == "" || w.got(child.ID, brief))
	})
	return child
}

func (w *world) noPath(path, what string) {
	w.t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		w.t.Fatalf("%s remains: %v", what, err)
	}
}

func TestMCPToolsManageChildren(t *testing.T) {
	w := newWorld(t)
	app, c := w.runApp()
	ws, err := app.rt.AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	parent := w.newSession(c, ws.ID, "first")
	if parent.Model != "haiku" {
		t.Fatalf("the chosen profile's model is %q", parent.Model)
	}
	child := w.createChild(c, parent, "worker", "the brief")
	if _, err := os.Stat(filepath.Join(child.Dir, ".git")); err != nil {
		t.Fatalf("the child has no worktree: %v", err)
	}
	c.tool(parent.ID, "send_message", map[string]string{"to": "worker", "text": "a later message"})
	w.until("the message", func() bool { return w.got(child.ID, "a later message") })
	if listed := c.tool(parent.ID, "list_sessions", nil); !strings.Contains(listed, child.Name) {
		t.Fatalf("list_sessions missed the child: %s", listed)
	}
	pid := w.lastPid(child.ID)
	c.tool(parent.ID, "end_session", map[string]string{"name": "worker"})
	w.noPath(child.Dir, "the ended child's worktree")
	if n := len(w.family(parent.ID)); n != 2 || !w.session(child.ID).Ended || !gone(pid) {
		t.Fatalf("the ended child is missing or running: %d sessions, process gone = %v", n, gone(pid))
	}
}

func TestSessionsAreIsolated(t *testing.T) {
	w := newWorld(t)
	app, c := w.runApp()
	ws, err := app.rt.AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	parent := w.newSession(c, ws.ID, "first")
	otherParent := w.newSession(c, ws.ID, "second")
	child := w.createChild(c, parent, "worker", "")
	otherChild := w.createChild(c, otherParent, "worker", "")
	if child.Dir == otherChild.Dir || child.Branch == otherChild.Branch {
		t.Fatal("the sessions share a worktree or branch")
	}
	for _, pair := range [][2]store.Session{{parent, otherChild}, {otherParent, child}} {
		if listed := c.tool(pair[0].ID, "list_sessions", nil); strings.Contains(listed, pair[1].ID) {
			t.Fatalf("list_sessions exposed another session's child: %s", listed)
		}
	}
	// A name reaches the sender's own session; another session's parent is
	// reached by address. The delivery to the second parent shows the first
	// message had its chance to cross.
	c.tool(otherParent.ID, "send_message", map[string]string{"to": "worker", "text": "to my child"})
	c.tool(otherParent.ID, "send_message", map[string]string{"to": parent.ID, "text": "to the other parent"})
	w.until("the deliveries", func() bool { return w.got(otherChild.ID, "to my child") && w.got(parent.ID, "to the other parent") })
	if w.got(child.ID, "to my child") {
		t.Fatal("a message crossed to another session's child")
	}
}

func TestMessagesAcrossWorkspaces(t *testing.T) {
	w := newWorld(t)
	app, c := w.runApp()
	ws, err := app.rt.AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	outside, err := app.rt.AddWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent := w.newSession(c, ws.ID, "first")
	peer := w.newSession(c, outside.ID, "second")
	child := w.createChild(c, parent, "worker", "")
	c.tool(peer.ID, "create_session", map[string]string{"name": "worker", "title": "Other child", "profile": "worker", "dir": "."})
	foreign := w.family(peer.ID)[1]
	if listed := c.tool(parent.ID, "list_sessions", nil); !strings.Contains(listed, peer.ID) || !strings.Contains(listed, outside.Path) || strings.Contains(listed, foreign.ID) {
		t.Fatal(listed)
	}
	c.tool(parent.ID, "send_message", map[string]string{"to": peer.ID, "text": "across to peer"})
	c.tool(peer.ID, "send_message", map[string]string{"to": parent.ID, "text": "back to parent"})
	w.until("cross-workspace messages", func() bool {
		return w.got(peer.ID, "across to peer") && w.got(parent.ID, "back to parent")
	})
	if !w.got(parent.ID, fmt.Sprintf("Message from %s (%q, workspace %s):", peer.ID, peer.Title, outside.Path)) {
		t.Fatal("peer message missed sender title or workspace")
	}
	for _, pair := range [][2]string{{parent.ID, foreign.ID}, {peer.ID, child.ID}, {child.ID, peer.ID}, {child.ID, foreign.ID}, {foreign.ID, parent.ID}, {foreign.ID, child.ID}} {
		if text, failed := c.callTool(pair[0], "send_message", map[string]string{"to": pair[1], "text": "forbidden"}); !failed || !strings.Contains(text, "not found") {
			t.Fatalf("out-of-scope message %v: %s", pair, text)
		}
	}
	c.sessionAction("stop", peer.ID)
	if listed := c.tool(parent.ID, "list_sessions", nil); strings.Contains(listed, peer.ID) {
		t.Fatal("stopped peer still listed:", listed)
	}
	if text, failed := c.callTool(parent.ID, "send_message", map[string]string{"to": peer.ID, "text": "stopped"}); !failed || !strings.Contains(text, "stopped session") {
		t.Fatalf("stopped recipient: %s", text)
	}
	w.read(func(tx *store.Tx) error {
		for _, id := range []string{parent.ID, peer.ID, child.ID, foreign.ID} {
			messages, err := tx.Queued(id)
			if err != nil {
				return err
			}
			for _, message := range messages {
				if message.Text == "forbidden" || message.Text == "stopped" {
					t.Errorf("refused message queued: %+v", message)
				}
			}
		}
		return nil
	})
}

func TestRoutesRefuseBadRequests(t *testing.T) {
	w := newWorld(t)
	app, c := w.runApp()
	ws, err := app.rt.AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	code, body := c.post("/session", map[string]string{"action": "new", "workspace": ws.ID, "profile": "missing"}, `{"text":"start"}`)
	if code != http.StatusConflict || !strings.Contains(body, `no profile named "missing"`) {
		t.Fatalf("unknown profile: %d %s", code, body)
	}
	if code, _ := c.post("/session", map[string]string{"action": "bogus", "id": "x"}, ""); code != http.StatusBadRequest {
		t.Fatalf("unknown action: %d", code)
	}
}

func TestDeletingASessionLeavesOthers(t *testing.T) {
	w := newWorld(t)
	app, c := w.runApp()
	ws, err := app.rt.AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	doomed := w.newSession(c, ws.ID, "first")
	kept := w.newSession(c, ws.ID, "second")
	doomedChild := w.createChild(c, doomed, "worker", "")
	keptChild := w.createChild(c, kept, "worker", "")
	pids := []int{w.lastPid(doomed.ID), w.lastPid(doomedChild.ID)}
	dirty := filepath.Join(doomedChild.Dir, "unfinished")
	if err := os.WriteFile(dirty, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, body := c.post("/session", map[string]string{"action": "delete", "id": doomed.ID}, ""); code != http.StatusConflict || !strings.Contains(body, "uncommitted changes") {
		t.Fatalf("delete with dirty work: %d %s", code, body)
	}
	if len(w.family(doomed.ID)) != 2 || !w.session(doomed.ID).Running {
		t.Fatal("refused deletion changed the session")
	}
	for i, a := range []store.Session{doomed, doomedChild} {
		if gone(pids[i]) || w.session(a.ID).Activity == store.Exited {
			t.Fatalf("refused deletion stopped %s", a.ID)
		}
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	c.sessionAction("delete", doomed.ID)
	if n := len(w.family(doomed.ID)); n != 0 || !gone(pids[0]) || !gone(pids[1]) {
		t.Fatal("the deleted session left sessions or processes")
	}
	w.noPath(doomedChild.Dir, "the deleted session's worktree")
	for _, a := range []store.Session{kept, keptChild} {
		if w.starts(a.ID) != 1 || !w.session(kept.ID).Running {
			t.Fatalf("deleting a session affected %s", a.ID)
		}
	}
	c.sessionAction("delete", kept.ID)
	if list := c.snapshot(); len(list) != 1 || len(list[0].Sessions) != 0 {
		t.Fatalf("deleting the last session removed its workspace: %+v", list)
	}
}

func TestDeletingAWorkspaceEndsItsSessions(t *testing.T) {
	w := newWorld(t)
	app, c := w.runApp()
	ws, err := app.rt.AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	parent := w.newSession(c, ws.ID, "first")
	child := w.createChild(c, parent, "worker", "")
	pids := []int{w.lastPid(parent.ID), w.lastPid(child.ID)}
	if code, body := c.post("/workspace", map[string]string{"action": "delete", "id": ws.ID}, ""); code != http.StatusOK {
		t.Fatalf("deleting the workspace: %d %s", code, body)
	}
	w.until("no workspaces", func() bool { return len(c.snapshot()) == 0 })
	if !gone(pids[0]) || !gone(pids[1]) {
		t.Fatal("the deleted workspace's agents run")
	}
	w.noPath(child.Dir, "the deleted workspace's worktree")
	w.noPath(filepath.Join(w.root, ".hypercube"), "the session worktree directory")
}

// A restart brings back the sessions the user left running, waiting for the
// user's prompt, and leaves a stopped session stopped.
func TestStartAppRestoresRunningSessions(t *testing.T) {
	w := newWorld(t)
	app, c := w.runApp()
	ws, err := app.rt.AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	running := w.newSession(c, ws.ID, "first")
	stopped := w.newSession(c, ws.ID, "second")
	stoppedPid := w.lastPid(stopped.ID)
	c.sessionAction("stop", stopped.ID)
	if w.session(stopped.ID).Running || !gone(stoppedPid) {
		t.Fatal("stop left the session running")
	}
	app.shutdown()
	if !w.session(running.ID).Running || w.session(running.ID).Activity != store.Exited {
		t.Fatal("shutdown did not end the agents and keep the running mark")
	}
	w.runApp()
	w.until("the restored session", func() bool { return w.starts(running.ID) == 2 && w.ready(running.ID) })
	if !w.session(running.ID).AwaitingUser {
		t.Fatal("the restored session does not await the user")
	}
	// startApp restores every running session before it returns.
	if w.session(stopped.ID).Running || w.starts(stopped.ID) != 1 {
		t.Fatal("the restart started a stopped session")
	}
}

// TestAppProcess exercises real app exits against isolated state and plugins.
func TestAppProcess(t *testing.T) {
	w := newWorld(t)
	binary := filepath.Join(w.plugins, "hypercube")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	if out, err := exec.Command(binary, "unknown").CombinedOutput(); err == nil || !strings.Contains(string(out), "usage: hypercube") {
		t.Fatalf("unknown commands should print the usage: %v %s", err, out)
	}
	version, err := exec.Command(binary, "version").CombinedOutput()
	if err != nil || !strings.HasPrefix(string(version), "Hypercube v") && strings.TrimSpace(string(version)) != "Hypercube devel" {
		t.Fatalf("version: %v %s", err, version)
	}
	// Finder supplies no arguments. A stand-in login shell captures the launch
	// without touching the user's shell configuration or starting a native app.
	shell := filepath.Join(w.dir, "login shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" \"$HYPERCUBE_APP_EXECUTABLE\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	launch := exec.Command(binary)
	launch.Env = append(os.Environ(), "SHELL="+shell)
	wantLaunch := "-i\n-c\nexec \"$HYPERCUBE_APP_EXECUTABLE\" ui --menu\n" + binary + "\n"
	if out, err := launch.CombinedOutput(); err != nil || string(out) != wantLaunch {
		t.Fatalf("Finder launch = %q, %v; want %q", out, err, wantLaunch)
	}
	// Exercise actual login shells with disposable homes. Their login files
	// redirect the native launch to a stand-in, so no window or agent starts.
	for _, shell := range []struct{ path, profile, setup string }{
		{"/bin/zsh", ".zprofile", `export HYPERCUBE_APP_EXECUTABLE="$HYPERCUBE_TEST_APP"`},
		{"/bin/bash", ".bash_profile", `export HYPERCUBE_APP_EXECUTABLE="$HYPERCUBE_TEST_APP"`},
		{"/bin/tcsh", ".login", `setenv HYPERCUBE_APP_EXECUTABLE "$HYPERCUBE_TEST_APP"`},
	} {
		t.Run(filepath.Base(shell.path), func(t *testing.T) {
			home := t.TempDir()
			target := filepath.Join(home, "native app stand-in")
			if err := os.WriteFile(target, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, shell.profile), []byte(shell.setup+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary)
			cmd.Env = append(os.Environ(), "SHELL="+shell.path, "HOME="+home, "ZDOTDIR="+home, "HYPERCUBE_TEST_APP="+target)
			out, err := cmd.Output()
			if err != nil || string(out) != "ui\n--menu\n" {
				t.Fatalf("login shell launch = %q, %v", out, err)
			}
		})
	}
	ws, err := session.New(w.db, "").AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	var app *exec.Cmd
	var c client
	start := func() {
		t.Helper()
		l := listen(t)
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		app = exec.Command(binary, "ui", fmt.Sprint(port))
		app.Stderr = os.Stderr
		if err := app.Start(); err != nil {
			t.Fatal(err)
		}
		c = client{t, fmt.Sprintf("http://127.0.0.1:%d", port)}
		w.until("the app serving", func() bool {
			res, err := http.Get(c.base + "/profiles")
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
	parent := w.session(c.newSession(ws.ID, "first"))
	w.until("the parent", func() bool { return w.starts(parent.ID) == 1 && w.session(parent.ID).Activity == store.Idle })
	// The agent's harness runs `hypercube mcp`, which reaches the app.
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "create_session", "arguments": map[string]string{"name": "w1", "title": "Title", "profile": "worker", "message": "brief"}}})
	bridge := exec.Command(binary, "mcp")
	bridge.Env = append(os.Environ(), "HYPERCUBE_SESSION_ID="+parent.ID, "HYPERCUBE_URL="+c.base)
	bridge.Stdin = bytes.NewReader(append(body, '\n'))
	out, err := bridge.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Created w1") {
		t.Fatalf("mcp bridge: %v %s", err, out)
	}
	child := w.family(parent.ID)[1]
	w.until("the child's brief", func() bool {
		return w.got(child.ID, "brief") && w.session(child.ID).Activity == store.Idle
	})
	pids := []int{w.lastPid(parent.ID), w.lastPid(child.ID)}

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
		if w.starts(parent.ID) != 1 || w.starts(child.ID) != 1 || w.session(parent.ID).AwaitingUser {
			t.Fatal("restart replaced agents or awaits user")
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
	start()
	w.until("agents after Quit", func() bool {
		return w.starts(parent.ID) == 2 && w.starts(child.ID) == 2 && w.ready(parent.ID) && w.ready(child.ID)
	})
	if !w.session(parent.ID).AwaitingUser {
		t.Fatal("launch after Quit did not await user")
	}
}
