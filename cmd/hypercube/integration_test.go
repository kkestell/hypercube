package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
	dir := t.TempDir()
	w := &world{t: t, dir: dir, plugins: filepath.Join(dir, "plugins"), logs: filepath.Join(dir, "logs"), root: filepath.Join(dir, "repo")}
	for _, d := range []string{w.plugins, w.logs, w.root, filepath.Join(dir, "config/hypercube")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	harnesstest.Install(t, w.plugins, "test")
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state with spaces"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("HYPERCUBE_TEST_LOGS", w.logs)
	config := `{"profiles":[
		{"name":"first","role":"lead","description":"First lead","harness":"test","model":"haiku","effort":"low"},
		{"name":"second","role":"lead","description":"Second lead","harness":"test","model":"sol","effort":"high"},
		{"name":"worker","role":"work","description":"test","harness":"test","model":"p/m","effort":"high"}
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
	var err error
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

func (w *world) agents(session string) (list []store.Agent) {
	w.t.Helper()
	w.read(func(tx *store.Tx) error { var err error; list, err = tx.Agents(session); return err })
	return list
}

func (w *world) agent(id string) (a store.Agent) {
	w.t.Helper()
	w.read(func(tx *store.Tx) error { var err error; a, err = tx.Agent(id); return err })
	return a
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

// newSession starts a session with the named lead and returns its ID.
func (c client) newSession(workspace, lead string) string {
	c.t.Helper()
	code, body := c.post("/session", map[string]string{"action": "new", "workspace": workspace, "lead": lead}, `{"text":"start"}`)
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

// asAgent adds the agent's identity and the app's origin to every request.
type asAgent struct{ agent, origin string }

func (a asAgent) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set(mcp.AgentHeader, a.agent)
	r.Header.Set("Origin", a.origin)
	return http.DefaultTransport.RoundTrip(r)
}

// tool calls an MCP tool as the agent through the app's route and returns its text.
func (c client) tool(agent, name string, args map[string]string) string {
	c.t.Helper()
	session, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(c.t.Context(), &sdk.StreamableClientTransport{
		Endpoint:             c.base + "/mcp",
		HTTPClient:           &http.Client{Transport: asAgent{agent, c.base}},
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
	text := res.Content[0].(*sdk.TextContent).Text
	if res.IsError {
		c.t.Fatalf("MCP %s: %s", name, text)
	}
	return text
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
	a, err := startApp(listener, w.plugins)
	if err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(a.shutdown)
	return a, client{w.t, "http://" + listener.Addr().String()}
}

// ready reports whether the agent's plugin has started and the agent is idle.
func (w *world) ready(id string) bool { return w.starts(id) > 0 && w.agent(id).Activity == store.Idle }

// newLead starts a session with the named lead and waits for the lead.
func (w *world) newLead(c client, workspace, profile string) (store.Session, store.Agent) {
	w.t.Helper()
	id := c.newSession(workspace, profile)
	lead := w.agents(id)[0]
	w.until("the lead of "+id, func() bool { return w.ready(lead.ID) })
	return w.session(id), w.agent(lead.ID)
}

// newWorker has the lead create a worker through MCP and waits for the
// worker's plugin to start and receive the brief, if any.
func (w *world) newWorker(c client, lead store.Agent, name, brief string) store.Agent {
	w.t.Helper()
	args := map[string]string{"name": name, "profile": "worker"}
	if brief != "" {
		args["message"] = brief
	}
	c.tool(lead.ID, "create_agent", args)
	var worker store.Agent
	w.until("worker "+name, func() bool {
		for _, a := range w.agents(lead.Session) {
			if a.Name == name {
				worker = a
			}
		}
		return worker.ID != "" && w.starts(worker.ID) > 0 && (brief == "" || w.got(worker.ID, brief))
	})
	return worker
}

func (w *world) noPath(path, what string) {
	w.t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		w.t.Fatalf("%s remains: %v", what, err)
	}
}

func TestMCPToolsManageWorkers(t *testing.T) {
	w := newWorld(t)
	app, c := w.runApp()
	ws, err := app.rt.AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	_, lead := w.newLead(c, ws.ID, "first")
	if lead.Model != "haiku" {
		t.Fatalf("the chosen lead's model is %q", lead.Model)
	}
	worker := w.newWorker(c, lead, "worker", "the brief")
	if _, err := os.Stat(filepath.Join(worker.Dir, ".git")); err != nil {
		t.Fatalf("the worker has no worktree: %v", err)
	}
	c.tool(lead.ID, "send_message", map[string]string{"to": "worker", "text": "a later message"})
	w.until("the message", func() bool { return w.got(worker.ID, "a later message") })
	if listed := c.tool(lead.ID, "list_agents", nil); !strings.Contains(listed, worker.Address()) {
		t.Fatalf("list_agents missed the worker: %s", listed)
	}
	pid := w.lastPid(worker.ID)
	c.tool(lead.ID, "remove_agent", map[string]string{"name": "worker"})
	w.noPath(worker.Dir, "the removed worker's worktree")
	if n := len(w.agents(lead.Session)); n != 1 || !gone(pid) {
		t.Fatalf("the removed worker remains: %d agents, process gone = %v", n, gone(pid))
	}
}

func TestSessionsAreIsolated(t *testing.T) {
	w := newWorld(t)
	app, c := w.runApp()
	ws, err := app.rt.AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	_, lead := w.newLead(c, ws.ID, "first")
	_, otherLead := w.newLead(c, ws.ID, "second")
	worker := w.newWorker(c, lead, "worker", "")
	otherWorker := w.newWorker(c, otherLead, "worker", "")
	if worker.Dir == otherWorker.Dir || worker.Branch == otherWorker.Branch {
		t.Fatal("the sessions share a worktree or branch")
	}
	for _, pair := range [][2]store.Agent{{lead, otherWorker}, {otherLead, worker}} {
		if listed := c.tool(pair[0].ID, "list_agents", nil); strings.Contains(listed, pair[1].Address()) {
			t.Fatalf("list_agents exposed another session's worker: %s", listed)
		}
	}
	// A name reaches the sender's own session; another session's lead is
	// reached by address. The delivery to the second lead shows the first
	// message had its chance to cross.
	c.tool(otherLead.ID, "send_message", map[string]string{"to": "worker", "text": "to my worker"})
	c.tool(otherLead.ID, "send_message", map[string]string{"to": lead.Address(), "text": "to the other lead"})
	w.until("the deliveries", func() bool { return w.got(otherWorker.ID, "to my worker") && w.got(lead.ID, "to the other lead") })
	if w.got(worker.ID, "to my worker") {
		t.Fatal("a message crossed to another session's worker")
	}
}

func TestRoutesRefuseBadRequests(t *testing.T) {
	w := newWorld(t)
	app, c := w.runApp()
	ws, err := app.rt.AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	code, body := c.post("/session", map[string]string{"action": "new", "workspace": ws.ID, "lead": "missing"}, `{"text":"start"}`)
	if code != http.StatusConflict || !strings.Contains(body, `no lead profile named "missing"`) {
		t.Fatalf("unknown lead: %d %s", code, body)
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
	doomed, doomedLead := w.newLead(c, ws.ID, "first")
	kept, keptLead := w.newLead(c, ws.ID, "second")
	doomedWorker := w.newWorker(c, doomedLead, "worker", "")
	keptWorker := w.newWorker(c, keptLead, "worker", "")
	pids := []int{w.lastPid(doomedLead.ID), w.lastPid(doomedWorker.ID)}
	c.sessionAction("delete", doomed.ID)
	if n := len(w.agents(doomed.ID)); n != 0 || !gone(pids[0]) || !gone(pids[1]) {
		t.Fatal("the deleted session left agents or processes")
	}
	w.noPath(doomedWorker.Dir, "the deleted session's worktree")
	for _, a := range []store.Agent{keptLead, keptWorker} {
		if w.starts(a.ID) != 1 || len(w.log(a.ID, "shutdown")) != 0 || !w.session(kept.ID).Running {
			t.Fatalf("deleting a session affected %s", a.Address())
		}
	}
}

func TestDeletingAWorkspaceEndsItsSessions(t *testing.T) {
	w := newWorld(t)
	app, c := w.runApp()
	ws, err := app.rt.AddWorkspace(w.root)
	if err != nil {
		t.Fatal(err)
	}
	_, lead := w.newLead(c, ws.ID, "first")
	worker := w.newWorker(c, lead, "worker", "")
	pids := []int{w.lastPid(lead.ID), w.lastPid(worker.ID)}
	if code, body := c.post("/workspace", map[string]string{"action": "delete", "id": ws.ID}, ""); code != http.StatusOK {
		t.Fatalf("deleting the workspace: %d %s", code, body)
	}
	w.until("no workspaces", func() bool { return len(c.snapshot()) == 0 })
	if !gone(pids[0]) || !gone(pids[1]) {
		t.Fatal("the deleted workspace's agents run")
	}
	w.noPath(worker.Dir, "the deleted workspace's worktree")
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
	running, runningLead := w.newLead(c, ws.ID, "first")
	stopped, stoppedLead := w.newLead(c, ws.ID, "second")
	stoppedPid := w.lastPid(stoppedLead.ID)
	c.sessionAction("stop", stopped.ID)
	if w.session(stopped.ID).Running || !gone(stoppedPid) {
		t.Fatal("stop left the session running")
	}
	app.shutdown()
	if !w.session(running.ID).Running || w.agent(runningLead.ID).Activity != store.Exited {
		t.Fatal("shutdown did not end the agents and keep the running mark")
	}
	w.runApp()
	w.until("the restored session", func() bool { return w.starts(runningLead.ID) == 2 && w.ready(runningLead.ID) })
	if !w.session(running.ID).AwaitingUser {
		t.Fatal("the restored session does not await the user")
	}
	// startApp restores every running session before it returns.
	if w.session(stopped.ID).Running || w.starts(stoppedLead.ID) != 1 {
		t.Fatal("the restart started a stopped session")
	}
}

// TestAppProcess runs the built command: the app serves before it restores
// agents, tools reach it through `hypercube mcp`, and SIGTERM ends the agents
// while the sessions stay marked running. Agents also end when the app is
// killed outright.
func TestAppProcess(t *testing.T) {
	w := newWorld(t)
	binary := filepath.Join(w.plugins, "hypercube")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	if out, err := exec.Command(binary).CombinedOutput(); err == nil || !strings.Contains(string(out), "usage: hypercube") {
		t.Fatalf("no arguments should print the usage: %v %s", err, out)
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
			res, err := http.Get(c.base + "/leads")
			if err == nil {
				res.Body.Close()
			}
			return err == nil
		})
	}
	t.Cleanup(func() {
		if app != nil && app.ProcessState == nil {
			_ = app.Process.Kill()
			_ = app.Wait()
		}
	})
	start()
	first := c.newSession(ws.ID, "first")
	lead := w.agents(first)[0]
	w.until("the lead", func() bool { return w.starts(lead.ID) == 1 && w.agent(lead.ID).Activity == store.Idle })
	// The agent's harness runs `hypercube mcp`, which reaches the app.
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "create_agent", "arguments": map[string]string{"name": "w1", "profile": "worker", "message": "brief"}}})
	bridge := exec.Command(binary, "mcp")
	bridge.Env = append(os.Environ(), "HYPERCUBE_AGENT_ID="+lead.ID, "HYPERCUBE_URL="+c.base)
	bridge.Stdin = bytes.NewReader(append(body, '\n'))
	out, err := bridge.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Created "+first+"/w1") {
		t.Fatalf("mcp bridge: %v %s", err, out)
	}
	worker := w.agents(first)[1]
	w.until("the worker's brief", func() bool {
		return w.got(worker.ID, "brief") && w.agent(worker.ID).Activity == store.Idle
	})
	pids := []int{w.lastPid(lead.ID), w.lastPid(worker.ID)}

	// SIGTERM ends the agents and keeps the running mark.
	if err := app.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := app.Wait(); err != nil {
		t.Fatalf("the app's exit: %v", err)
	}
	for _, pid := range pids {
		if !gone(pid) {
			t.Fatalf("agent process %d remains", pid)
		}
	}
	if !w.session(first).Running || w.agent(lead.ID).Activity != store.Exited || len(w.log(lead.ID, "shutdown")) == 0 {
		t.Fatal("shutdown did not end the agents and keep the running mark")
	}
	// The next launch restores the session, waiting for the user.
	start()
	w.until("the restored agents", func() bool {
		return w.starts(lead.ID) == 2 && w.starts(worker.ID) == 2 && w.agent(lead.ID).Activity == store.Idle && w.agent(worker.ID).Activity == store.Idle
	})
	if !w.session(first).AwaitingUser {
		t.Fatal("the restored session does not await the user")
	}
	// A killed app leaves no agents: a plugin ends when its connection does.
	pids = []int{w.lastPid(lead.ID), w.lastPid(worker.ID)}
	_ = app.Process.Kill()
	_ = app.Wait()
	w.until("the agents to end with the app", func() bool { return gone(pids[0]) && gone(pids[1]) })
}
