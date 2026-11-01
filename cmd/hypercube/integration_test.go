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

func (w *world) queued(id string) (m []store.Message) {
	w.t.Helper()
	w.read(func(tx *store.Tx) error { var err error; m, err = tx.Queued(id); return err })
	return m
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
	list, _ := store.Snapshot(w.db)
	b, _ := json.MarshalIndent(list, "", "  ")
	w.t.Logf("database: %s", b)
	w.t.Fatalf("timed out waiting for %s", what)
}

func (w *world) logOf(id string) string {
	b, _ := os.ReadFile(filepath.Join(w.logs, id+".log"))
	return string(b)
}

// starts counts how many times the agent's plugin started.
func (w *world) starts(id string) int { return strings.Count(w.logOf(id), `"kind":"start"`) }

// lastPid is the process ID of the agent's most recent plugin.
func (w *world) lastPid(id string) int {
	w.t.Helper()
	var pid int
	for _, line := range strings.Split(w.logOf(id), "\n") {
		var entry struct {
			Kind string
			Pid  int
		}
		if json.Unmarshal([]byte(line), &entry) == nil && entry.Kind == "start" {
			pid = entry.Pid
		}
	}
	if pid == 0 {
		w.t.Fatalf("agent %s has no plugin process", id)
	}
	return pid
}

func gone(pid int) bool { return syscall.Kill(pid, 0) == syscall.ESRCH }

// client drives an app over its HTTP address.
type client struct {
	t    *testing.T
	base string
}

func (c client) post(path string, values map[string]string) (int, string) {
	c.t.Helper()
	q := url.Values{}
	for k, v := range values {
		q.Set(k, v)
	}
	req, _ := http.NewRequest("POST", c.base+path+"?"+q.Encode(), nil)
	req.Header.Set("Origin", c.base)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, strings.TrimSpace(string(b))
}

func (c client) act(values map[string]string) string {
	c.t.Helper()
	code, body := c.post("/session", values)
	if code != http.StatusOK {
		c.t.Fatalf("session %v: %d %s", values, code, body)
	}
	return body
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

func (c client) snapshot() []store.ShownWorkspace {
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
	var msg struct{ Workspaces []store.ShownWorkspace }
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

// TestTeams runs the app in this process with the stand-in plugin: several
// sessions with workers, messages, failures, restarts, and deletions.
func TestTeams(t *testing.T) {
	w := newWorld(t)
	var app *app
	var c client
	start := func() {
		t.Helper()
		listener := listen(t)
		var err error
		if app, err = startApp(listener, w.plugins); err != nil {
			t.Fatal(err)
		}
		c = client{t, "http://" + listener.Addr().String()}
	}
	start()
	t.Cleanup(func() { app.shutdown() })
	input := func(agent, text string) {
		t.Helper()
		if err := app.rt.Input(agent, text); err != nil {
			t.Fatal(err)
		}
	}
	ready := func(id string) func() bool {
		return func() bool { return w.starts(id) > 0 && w.agent(id).Activity == store.Idle }
	}

	ws, err := session.AddWorkspace(w.db, w.root)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := session.AddWorkspace(w.db, filepath.Join(w.root, ".")); err != nil || again.ID != ws.ID {
		t.Fatalf("adding the folder again: %+v, %v", again, err)
	}
	if code, body := c.post("/session", map[string]string{"action": "new", "workspace": ws.ID, "lead": "missing"}); code != http.StatusConflict || !strings.Contains(body, `no lead profile named "missing"`) {
		t.Fatalf("unknown lead: %d %s", code, body)
	}
	first := c.act(map[string]string{"action": "new", "workspace": ws.ID, "lead": "first"})
	lead := w.agents(first)[0]
	w.until("the first lead", ready(lead.ID))
	if lead = w.agent(lead.ID); lead.Model != "haiku" || w.session(first).AwaitingUser {
		t.Fatalf("first session: %+v %+v", lead, w.session(first))
	}
	c.tool(lead.ID, "create_agent", map[string]string{"name": "worker", "profile": "worker", "message": "first brief"})
	w.until("the worker's brief", func() bool {
		list := w.agents(first)
		return len(list) == 2 && strings.Contains(w.logOf(list[1].ID), "first brief") && len(w.queued(list[1].ID)) == 0
	})
	worker := w.agents(first)[1]
	if _, err := os.Stat(filepath.Join(worker.Dir, ".git")); err != nil {
		t.Fatal("worker did not get a worktree", err)
	}
	// Sessions are separate: their workers, names, and worktrees.
	second := c.act(map[string]string{"action": "new", "workspace": ws.ID, "lead": "second"})
	otherLead := w.agents(second)[0]
	w.until("the second lead", ready(otherLead.ID))
	if w.agent(otherLead.ID).Model != "sol" {
		t.Fatalf("chosen lead model = %q", w.agent(otherLead.ID).Model)
	}
	c.tool(otherLead.ID, "create_agent", map[string]string{"name": "worker", "profile": "worker", "message": "second session brief"})
	w.until("the second session's worker", func() bool {
		list := w.agents(second)
		return len(list) == 2 && strings.Contains(w.logOf(list[1].ID), "second session brief")
	})
	otherWorker := w.agents(second)[1]
	if otherWorker.Dir == worker.Dir || otherWorker.Branch == worker.Branch {
		t.Fatal("sessions share a worker worktree or branch")
	}
	for _, pair := range [][2]store.Agent{{lead, otherWorker}, {otherLead, worker}} {
		if listed := c.tool(pair[0].ID, "list_agents", nil); strings.Contains(listed, pair[1].Address()) {
			t.Fatal("list_agents exposed another session", listed)
		}
	}
	// Deleting a session protects a worker's dirty worktree.
	third := c.act(map[string]string{"action": "new", "workspace": ws.ID, "lead": "first"})
	thirdLead := w.agents(third)[0]
	w.until("the third lead", ready(thirdLead.ID))
	c.tool(thirdLead.ID, "create_agent", map[string]string{"name": "worker", "profile": "worker"})
	w.until("the third session's worker", func() bool { return len(w.agents(third)) == 2 && w.starts(w.agents(third)[1].ID) == 1 })
	thirdWorker := w.agents(third)[1]
	dirty := filepath.Join(thirdWorker.Dir, "unfinished")
	if err := os.WriteFile(dirty, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, body := c.post("/session", map[string]string{"action": "delete", "id": third}); code != http.StatusConflict || !strings.Contains(body, "uncommitted changes") {
		t.Fatalf("session deletion failed to protect dirty work: %d %s", code, body)
	}
	if len(w.agents(third)) != 2 || strings.Contains(w.logOf(thirdWorker.ID), `"kind":"shutdown"`) {
		t.Fatal("refused deletion changed the session")
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	thirdPid := w.lastPid(thirdWorker.ID)
	c.act(map[string]string{"action": "delete", "id": third})
	if len(w.agents(third)) != 0 || !gone(thirdPid) {
		t.Fatal("session deletion left its agents or their processes")
	}
	if _, err := os.Stat(thirdWorker.Dir); !os.IsNotExist(err) {
		t.Fatal("deleted session's worktree remains", err)
	}
	if w.starts(worker.ID) != 1 || strings.Contains(w.logOf(worker.ID), `"kind":"shutdown"`) {
		t.Fatal("session deletion affected another session")
	}
	// Starting a running session changes nothing.
	c.act(map[string]string{"action": "start", "id": first})
	if w.starts(lead.ID) != 1 || w.starts(worker.ID) != 1 {
		t.Fatal("start relaunched a running session")
	}
	// Messages go to the agent named in the sender's session.
	c.tool(otherLead.ID, "send_message", map[string]string{"to": "worker", "text": "only second worker"})
	w.until("session-local name delivery", func() bool { return strings.Contains(w.logOf(otherWorker.ID), "only second worker") })
	c.tool(otherLead.ID, "send_message", map[string]string{"to": lead.Address(), "text": "from the other lead"})
	w.until("lead-to-lead delivery", func() bool { return strings.Contains(w.logOf(lead.ID), "from the other lead") })
	if strings.Contains(w.logOf(worker.ID), "only second worker") {
		t.Fatal("message crossed sessions")
	}
	// A turn that delivered messages leaves a session untitled. The first
	// prompt typed while idle titles it, and later prompts leave the title.
	w.until("the lead idle after the message", func() bool { return w.agent(lead.ID).Activity == store.Idle })
	if w.session(first).Title != "" {
		t.Fatal("a delivered message titled the session")
	}
	input(lead.ID, "Fix the parser")
	if w.session(first).Title != "Fix the parser" {
		t.Fatalf("title = %q", w.session(first).Title)
	}
	input(lead.ID, "Something else")
	if w.session(first).Title != "Fix the parser" {
		t.Fatal("a later prompt changed the title")
	}
	w.until("the lead's reply unseen", func() bool { return w.session(first).Unseen })
	c.act(map[string]string{"action": "seen", "id": first})
	if w.session(first).Unseen {
		t.Fatal("seen left the reply unseen")
	}
	// A worker that fails tells its own lead. Messages reach it mid-turn.
	input(worker.ID, "!work")
	w.until("a busy worker", func() bool { return w.agent(worker.ID).Activity == store.Busy })
	c.tool(lead.ID, "send_message", map[string]string{"to": worker.Name, "text": "change course during turn"})
	w.until("mid-turn steering", func() bool { return strings.Contains(w.logOf(worker.ID), "change course during turn") })
	input(worker.ID, "!fail")
	w.until("the failure reaches the lead", func() bool { return strings.Contains(w.logOf(lead.ID), "test provider failure") })
	if strings.Contains(w.logOf(otherLead.ID), "test provider failure") {
		t.Fatal("failure reached another session's lead")
	}
	// A worker whose process ends comes back with its conversation.
	input(worker.ID, "!crash")
	w.until("the resumed worker", func() bool { return w.starts(worker.ID) == 2 && w.agent(worker.ID).Activity == store.Idle })
	if !strings.Contains(w.logOf(worker.ID), `"conversation":"`+worker.ID+`-conversation"`) {
		t.Fatal("relaunch lost the conversation")
	}
	workerPid := w.lastPid(worker.ID)
	c.tool(lead.ID, "remove_agent", map[string]string{"name": worker.Name})
	if _, err := os.Stat(worker.Dir); !os.IsNotExist(err) || len(w.agents(first)) != 1 || !gone(workerPid) {
		t.Fatal("the removed worker, its worktree, or its process remains")
	}

	// A stopped session stays stopped when the app restarts, and keeps its
	// conversations and queued messages.
	otherPid := w.lastPid(otherLead.ID)
	c.act(map[string]string{"action": "stop", "id": second})
	if w.session(second).Running || !gone(otherPid) {
		t.Fatal("stop left the session running")
	}
	w.until("the view lists the session stopped", func() bool {
		for _, s := range c.snapshot()[0].Sessions {
			if s.Running != (s.ID == first) {
				return false
			}
		}
		return true
	})
	if code, _ := c.post("/session", map[string]string{"action": "bogus", "id": second}); code != http.StatusBadRequest {
		t.Fatal("an unknown action was accepted")
	}
	queue := func(to, text string) {
		t.Helper()
		if err := w.db.Tx(func(tx *store.Tx) error { return tx.Queue(store.Message{From: "user", To: to, Text: text}) }); err != nil {
			t.Fatal(err)
		}
	}
	queue(otherLead.ID, "queued while stopped")
	queue(otherWorker.ID, "other queued while stopped")
	app.shutdown()
	start()
	time.Sleep(300 * time.Millisecond)
	if w.starts(otherLead.ID) != 1 || w.session(second).Running {
		t.Fatal("restarting the app started a stopped session")
	}
	// Start resumes the conversations and waits for the user's prompt.
	c.act(map[string]string{"action": "start", "id": second})
	w.until("the started session", func() bool { return ready(otherLead.ID)() && ready(otherWorker.ID)() && w.starts(otherWorker.ID) == 2 })
	if !w.session(second).AwaitingUser || !strings.Contains(w.logOf(otherWorker.ID), `"conversation":"`+otherWorker.ID+`-conversation"`) {
		t.Fatal("started session did not resume and await the user")
	}
	time.Sleep(300 * time.Millisecond)
	if strings.Contains(w.logOf(otherLead.ID), "queued while stopped") || len(w.queued(otherLead.ID)) != 1 || len(w.queued(otherWorker.ID)) != 1 {
		t.Fatal("a started session delivered messages before a user prompt")
	}
	input(otherLead.ID, "continue")
	w.until("the started session released", func() bool {
		return strings.Contains(w.logOf(otherLead.ID), "queued while stopped") && strings.Contains(w.logOf(otherWorker.ID), "other queued while stopped")
	})

	// When the app ends, sessions the user left running start again at the next
	// launch, waiting for the user.
	c.act(map[string]string{"action": "stop", "id": second})
	queue(lead.ID, "queued at shutdown")
	leadStarts, leadPid := w.starts(lead.ID), w.lastPid(lead.ID)
	app.shutdown()
	if !gone(leadPid) || !w.session(first).Running || w.agent(lead.ID).Activity != store.Exited {
		t.Fatal("shutdown did not end the agents and keep the running mark")
	}
	start()
	w.until("the restored session", func() bool { return w.starts(lead.ID) == leadStarts+1 && ready(lead.ID)() })
	if w.session(second).Running || !w.session(first).AwaitingUser {
		t.Fatal("restore started the wrong sessions")
	}
	time.Sleep(300 * time.Millisecond)
	if strings.Contains(w.logOf(lead.ID), "queued at shutdown") {
		t.Fatal("a restored session delivered messages before a user prompt")
	}
	input(lead.ID, "continue")
	w.until("the restored session released", func() bool { return strings.Contains(w.logOf(lead.ID), "queued at shutdown") })

	// A new title shows to agents; the address stays, and prompts leave it.
	c.act(map[string]string{"action": "rename", "id": first, "title": "#{session_name} -triage;"})
	c.act(map[string]string{"action": "start", "id": second})
	w.until("the second session running", func() bool { return ready(otherLead.ID)() && w.starts(otherLead.ID) == 3 })
	if listed := c.tool(otherLead.ID, "list_agents", nil); !strings.Contains(listed, lead.Address()) || !strings.Contains(listed, "#{session_name} -triage;") {
		t.Fatal("list_agents missed the new title", listed)
	}
	input(lead.ID, "Another line")
	if got := w.session(first).Title; got != "#{session_name} -triage;" {
		t.Fatalf("a prompt replaced the user's title: %q", got)
	}
	// Deleting the last session leaves the workspace; deleting the workspace
	// removes it and its worktrees.
	c.act(map[string]string{"action": "delete", "id": second})
	c.tool(lead.ID, "create_agent", map[string]string{"name": "last", "profile": "worker"})
	w.until("the last worker", func() bool { return len(w.agents(first)) == 2 && w.starts(w.agents(first)[1].ID) == 1 })
	lastWorker := w.agents(first)[1]
	c.act(map[string]string{"action": "delete", "id": first})
	w.until("a workspace without sessions", func() bool { list := c.snapshot(); return len(list) == 1 && len(list[0].Sessions) == 0 })
	if _, err := os.Stat(lastWorker.Dir); !os.IsNotExist(err) {
		t.Fatal("deleted session's worktree remains", err)
	}
	last := c.act(map[string]string{"action": "new", "workspace": ws.ID, "lead": "first"})
	lastLead := w.agents(last)[0]
	w.until("the new session's lead", ready(lastLead.ID))
	c.tool(lastLead.ID, "create_agent", map[string]string{"name": "w", "profile": "worker"})
	w.until("the new session's worker", func() bool { return len(w.agents(last)) == 2 && w.starts(w.agents(last)[1].ID) == 1 })
	finalWorker := w.agents(last)[1]
	if code, body := c.post("/workspace", map[string]string{"action": "delete", "id": ws.ID}); code != http.StatusOK {
		t.Fatalf("deleting the workspace: %d %s", code, body)
	}
	w.until("no workspaces", func() bool { return len(c.snapshot()) == 0 })
	if !gone(w.lastPid(lastLead.ID)) || !gone(w.lastPid(finalWorker.ID)) {
		t.Fatal("the deleted workspace's agents run")
	}
	if _, err := os.Stat(finalWorker.Dir); !os.IsNotExist(err) {
		t.Fatal("deleted workspace's worktree remains", err)
	}
	if _, err := os.Stat(filepath.Join(w.root, ".hypercube")); !os.IsNotExist(err) {
		t.Fatal("session worktree directories remain", err)
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
	ws, err := session.AddWorkspace(w.db, w.root)
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
	first := c.act(map[string]string{"action": "new", "workspace": ws.ID, "lead": "first"})
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
		return strings.Contains(w.logOf(worker.ID), "brief") && w.agent(worker.ID).Activity == store.Idle
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
	if !w.session(first).Running || w.agent(lead.ID).Activity != store.Exited || !strings.Contains(w.logOf(lead.ID), `"kind":"shutdown"`) {
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
