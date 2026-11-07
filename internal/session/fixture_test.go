package session

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/terminal"
)

func TestStandIn(t *testing.T) { harnesstest.Run() }

// lead is the settings of the fixture's top-level sessions.
var lead = harness.Settings{Harness: "test", Model: "m", Effort: "low"}

// fixture is a runtime whose agents are the stand-in plugin, in a Git
// workspace.
type fixture struct {
	t         *testing.T
	dir       string // removed after the runtimes quit
	db        *store.DB
	rt        *Runtime
	logs      string
	plugins   string
	root      string
	workspace store.Workspace
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir, err := os.MkdirTemp("", "hc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	f := &fixture{t: t, dir: dir, logs: filepath.Join(dir, "logs"), plugins: filepath.Join(dir, "plugins"), root: filepath.Join(dir, "repo")}
	for _, d := range []string{f.logs, f.plugins, f.root, filepath.Join(dir, "config/hypercube")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("HYPERCUBE_TEST_LOGS", f.logs)
	harnesstest.Install(t, f.plugins, "test")
	f.git(f.root, "init", "-q", "-b", "main")
	f.git(f.root, "commit", "-q", "--allow-empty", "-m", "init")
	if f.root, err = filepath.EvalSymlinks(f.root); err != nil {
		t.Fatal(err)
	}
	if f.db, err = store.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.db.Close() })
	f.rt = f.newRuntime()
	if f.workspace, err = f.rt.AddWorkspace(f.root); err != nil {
		t.Fatal(err)
	}
	return f
}

// newRuntime returns a runtime over the fixture's database, ended when the test
// ends. The test can end it sooner.
func (f *fixture) newRuntime() *Runtime {
	rt := New(f.db, "http://127.0.0.1:1")
	rt.PluginDir = f.plugins
	f.t.Cleanup(rt.Quit)
	return rt
}

func (f *fixture) git(dir string, args ...string) {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// session creates a session from "start" and waits for its reply. Its transcript reads
// "user:start|text:ack".
func (f *fixture) session() store.Session {
	f.t.Helper()
	s, err := f.rt.Create(f.workspace.ID, lead, false, "start")
	if err != nil {
		f.t.Fatal(err)
	}
	f.until("the session idle after the first prompt", func() bool {
		return f.summary(s.ID) == "user:start|text:ack" && f.storedSession(s.ID).Activity == store.Idle
	})
	return s
}

// seed adds the stopped session "s" and its children, without launching them.
func (f *fixture) seed(children ...store.Session) {
	f.t.Helper()
	err := f.db.Tx(func(tx *store.Tx) error {
		if err := tx.AddSession(store.Session{ID: "s", Workspace: f.workspace.ID, Dir: f.root, Activity: store.Exited}); err != nil {
			return err
		}
		for _, a := range children {
			a.Parent, a.Workspace, a.Activity = "s", f.workspace.ID, store.Exited
			if err := tx.AddSession(a); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) child(parent store.Session, name, message string) store.Session {
	f.t.Helper()
	child, err := f.rt.CreateChild(parent.ID, ChildOptions{Name: name, Title: "Title", Dir: "", Message: message})
	if err != nil {
		f.t.Fatal(err)
	}
	return child
}

func (f *fixture) read(fn func(*store.Tx) error) {
	f.t.Helper()
	if err := f.db.Read(fn); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) family(session string) (list []store.Session) {
	f.t.Helper()
	f.read(func(tx *store.Tx) error {
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

func (f *fixture) storedSession(id string) (s store.Session) {
	f.t.Helper()
	f.read(func(tx *store.Tx) error { var err error; s, err = tx.Session(id); return err })
	return s
}

// idle reports whether the session's agent is idle and the delivery its idle
// event started has finished, so a message queued now stays queued. The
// database is read first: the event saves the idle state and starts the
// delivery while holding mu, so the check under mu then sees the delivery.
func (f *fixture) idle(id string) bool {
	if f.storedSession(id).Activity != store.Idle {
		return false
	}
	f.rt.mu.Lock()
	defer f.rt.mu.Unlock()
	l := f.rt.live[id]
	return l != nil && l.ready != 0 && !l.delivering()
}

func (f *fixture) queued(id string) (m []store.Message) {
	f.t.Helper()
	f.read(func(tx *store.Tx) error { var err error; m, err = tx.Queued(id); return err })
	return m
}

// queue queues the messages for the agent in one transaction, then delivers.
func (f *fixture) queue(from, to string, texts ...string) {
	f.t.Helper()
	if err := f.db.Tx(func(tx *store.Tx) error {
		for _, text := range texts {
			if err := tx.Queue(store.Message{From: from, To: to, Text: text}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		f.t.Fatal(err)
	}
	f.rt.Deliver(to)
}

// shellEnded reports whether the session's shell has ended and its holder is
// gone.
func (f *fixture) shellEnded(session string) bool {
	_, err := os.Stat(filepath.Join(store.Dir(), "run", terminal.Name(session)))
	return errors.Is(err, os.ErrNotExist)
}

func (f *fixture) until(what string, check func() bool) {
	f.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	entries, _ := os.ReadDir(f.logs)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(f.logs, e.Name()))
		f.t.Logf("%s:\n%s", e.Name(), b)
	}
	f.t.Fatalf("timed out waiting for %s", what)
}

// log returns the stand-in's log entries for the agent with the given kind.
func (f *fixture) log(agent, kind string) []harnesstest.Entry {
	f.t.Helper()
	return harnesstest.Log(f.t, f.logs, agent, kind)
}

// inputs lists the texts sent to the agent's plugin.
func (f *fixture) inputs(agent string) (texts []string) {
	f.t.Helper()
	for _, e := range f.log(agent, "input") {
		texts = append(texts, e.Text)
	}
	return texts
}

func (f *fixture) input(agent, text string) {
	f.t.Helper()
	if err := f.rt.Input(agent, text); err != nil {
		f.t.Fatal(err)
	}
}

// saved returns the agent's transcript items with their positions, without its
// state markers.
func (f *fixture) saved(agent string) []TranscriptItem {
	f.t.Helper()
	items, err := f.rt.Transcript(agent)
	if err != nil {
		f.t.Fatal(err)
	}
	var shown []TranscriptItem
	for _, item := range items {
		if item.Kind != harness.KindState {
			shown = append(shown, item)
		}
	}
	return shown
}

func (f *fixture) transcript(agent string) []harness.Item {
	f.t.Helper()
	var plain []harness.Item
	for _, item := range f.saved(agent) {
		plain = append(plain, item.Item)
	}
	return plain
}

// summary writes the agent's transcript as kind:text items joined by "|",
// with a tool's output for its text.
func (f *fixture) summary(agent string) string {
	f.t.Helper()
	var parts []string
	for _, it := range f.transcript(agent) {
		text := it.Text
		if it.Kind == harness.KindTool {
			text = it.Tool + "=" + it.Output
		}
		parts = append(parts, it.Kind+":"+text)
	}
	return strings.Join(parts, "|")
}

// park ends the session's agent as its idle timeout does.
func (f *fixture) park(id string) {
	f.t.Helper()
	f.rt.mu.Lock()
	lives := f.rt.selectLaunches(func(l *live) bool { return l.id == id })
	f.rt.mu.Unlock()
	if len(lives) == 0 {
		f.t.Fatalf("%s has no agent to park", id)
	}
	f.rt.stopLaunches(lives)
}

// wake starts the session's agent and waits for it to start.
func (f *fixture) wake(id string) error {
	l, err := f.rt.wake(id)
	if err != nil {
		return err
	}
	return f.rt.awaitStart(l)
}

// awake reports whether the session has an agent.
func (f *fixture) awake(id string) bool {
	f.rt.mu.Lock()
	defer f.rt.mu.Unlock()
	return f.rt.live[id] != nil
}

func (f *fixture) requests() []Request {
	f.t.Helper()
	requests, err := f.rt.Requests()
	if err != nil {
		f.t.Fatal(err)
	}
	return requests
}

func (f *fixture) restore() {
	f.t.Helper()
	if err := f.rt.Restore(nil); err != nil {
		f.t.Fatal(err)
	}
}
