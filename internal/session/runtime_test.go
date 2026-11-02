package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
	"github.com/kkestell/hypercube/internal/store"
)

func TestStandIn(t *testing.T) { harnesstest.Run() }

// fixture is a runtime whose agents are the stand-in plugin, in a Git
// workspace.
type fixture struct {
	t         *testing.T
	db        *store.DB
	rt        *Runtime
	logs      string
	plugins   string
	root      string
	workspace store.Workspace
	// created holds the leads made by session, whose creation prompt and reply
	// inputs and transcript leave out.
	created map[string]bool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{t: t, logs: filepath.Join(dir, "logs"), plugins: filepath.Join(dir, "plugins"), root: filepath.Join(dir, "repo"), created: map[string]bool{}}
	for _, d := range []string{f.logs, f.plugins, f.root, filepath.Join(dir, "config/hypercube")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("HYPERCUBE_TEST_LOGS", f.logs)
	harnesstest.Install(t, f.plugins, "test")
	config := `{"profiles":[
		{"name":"lead","role":"lead","description":"Lead","harness":"test","model":"m","effort":"low"},
		{"name":"worker","role":"work","description":"Worker","harness":"test","model":"m","effort":"low"}]}`
	if err := os.WriteFile(filepath.Join(dir, "config/hypercube/config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", append([]string{"-C", f.root, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}
	var err error
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
	f.t.Cleanup(rt.Shutdown)
	return rt
}

// session creates a session from the prompt "start", and returns it with its
// lead once the lead has answered.
func (f *fixture) session() (store.Session, store.Agent) {
	f.t.Helper()
	s, lead, err := f.rt.Create(f.workspace.ID, "lead", "start")
	if err != nil {
		f.t.Fatal(err)
	}
	f.created[lead.ID] = true
	f.until("the lead idle after the first prompt", func() bool {
		items, err := f.rt.Transcript(lead.ID)
		return err == nil && len(items) == 2 && f.agent(lead.ID).Activity == store.Idle
	})
	return s, lead
}

func (f *fixture) read(fn func(*store.Tx) error) {
	f.t.Helper()
	if err := f.db.Read(fn); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) agents(session string) (list []store.Agent) {
	f.t.Helper()
	f.read(func(tx *store.Tx) error { var err error; list, err = tx.Agents(session); return err })
	return list
}

func (f *fixture) agent(id string) (a store.Agent) {
	f.t.Helper()
	f.read(func(tx *store.Tx) error { var err error; a, err = tx.Agent(id); return err })
	return a
}

func (f *fixture) session1(id string) (s store.Session) {
	f.t.Helper()
	f.read(func(tx *store.Tx) error { var err error; s, err = tx.Session(id); return err })
	return s
}

func (f *fixture) queued(id string) (m []store.Message) {
	f.t.Helper()
	f.read(func(tx *store.Tx) error { var err error; m, err = tx.Queued(id); return err })
	return m
}

func (f *fixture) queue(from, to, text string) {
	f.t.Helper()
	if err := f.db.Tx(func(tx *store.Tx) error { return tx.Queue(store.Message{From: from, To: to, Text: text}) }); err != nil {
		f.t.Fatal(err)
	}
	f.rt.Deliver(to)
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

// inputs lists the texts sent to the agent's plugin. For a lead made by
// session, the creation prompt is left out.
func (f *fixture) inputs(agent string) (texts []string) {
	for _, e := range f.log(agent, "input") {
		texts = append(texts, e.Text)
	}
	if f.created[agent] && len(texts) > 0 {
		texts = texts[1:]
	}
	return texts
}

func (f *fixture) transcript(agent string) []harness.Item {
	f.t.Helper()
	items, err := f.rt.Transcript(agent)
	if err != nil {
		f.t.Fatal(err)
	}
	if f.created[agent] {
		items = items[min(2, len(items)):]
	}
	plain := make([]harness.Item, len(items))
	for i, item := range items {
		plain[i] = item.Item
	}
	return plain
}

// texts summarizes a transcript as kind:text, with a tool's output for tools.
func summary(items []harness.Item) (parts []string) {
	for _, it := range items {
		text := it.Text
		if it.Kind == harness.KindTool {
			text = it.Tool + "=" + it.Output
		}
		parts = append(parts, it.Kind+":"+text)
	}
	return parts
}

func (f *fixture) input(agent, text string) {
	f.t.Helper()
	if err := f.rt.Input(agent, text); err != nil {
		f.t.Fatal(err)
	}
}

func TestSessionStartsAndDeliversInOrder(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	if f.session1(s.ID).AwaitingUser || !f.session1(s.ID).Running {
		t.Fatalf("new session: %+v", f.session1(s.ID))
	}
	// The plugin starts in the lead's directory with the team's instructions.
	start := f.log(lead.ID, "start")
	if len(start) != 1 || start[0].Dir != f.root || start[0].Role != "lead" ||
		!strings.Contains(start[0].Instructions, "You are "+lead.Address()) ||
		start[0].Env["HYPERCUBE_AGENT_ID"] != lead.ID {
		t.Fatalf("start = %v", start)
	}
	if got := f.agent(lead.ID).Conversation; got != lead.ID+"-conversation" {
		t.Fatalf("conversation = %q", got)
	}
	// Each queued message goes out once the plugin accepts the one before it,
	// and leaves the queue when it does.
	f.queue("user", lead.ID, "first")
	f.queue("user", lead.ID, "second")
	f.queue("user", lead.ID, "third")
	f.until("three deliveries", func() bool { return len(f.inputs(lead.ID)) == 3 && len(f.queued(lead.ID)) == 0 })
	if got := f.inputs(lead.ID); got[0] != "Message from user:\nfirst" || got[2] != "Message from user:\nthird" {
		t.Fatalf("inputs = %q", got)
	}
	f.until("the lead idle", func() bool { return f.agent(lead.ID).Activity == store.Idle && len(f.transcript(lead.ID)) == 6 })
	// The plugin's replies and the next delivery race, so a reply can land
	// after the next message's item. The messages keep their order.
	var users, acks []string
	for _, part := range summary(f.transcript(lead.ID)) {
		if strings.HasPrefix(part, "user:") {
			users = append(users, strings.TrimPrefix(part, "user:Message from user:\n"))
		} else {
			acks = append(acks, part)
		}
	}
	if strings.Join(users, ",") != "first,second,third" || len(acks) != 3 {
		t.Fatalf("transcript users %q, replies %q", users, acks)
	}
	// Starting a running session changes nothing.
	if err := f.rt.Start(s.ID); err != nil || len(f.log(lead.ID, "start")) != 1 {
		t.Fatalf("start of a running session: %v", err)
	}
}

func TestRefusedMessageStaysQueued(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	if err := f.rt.save(func(w *write) error {
		if err := w.Queue(store.Message{From: "user", To: lead.ID, Text: "!reject this"}); err != nil {
			return err
		}
		return w.Queue(store.Message{From: "user", To: lead.ID, Text: "after"})
	}); err != nil {
		t.Fatal(err)
	}
	f.rt.Deliver(lead.ID)
	f.until("a refusal", func() bool {
		for _, it := range f.transcript(lead.ID) {
			if it.Kind == harness.KindError && strings.Contains(it.Text, "rejected") {
				return true
			}
		}
		return false
	})
	time.Sleep(100 * time.Millisecond)
	if q := f.queued(lead.ID); len(q) != 2 || len(f.inputs(lead.ID)) != 1 {
		t.Fatalf("queue after a refusal: %+v, inputs %q", q, f.inputs(lead.ID))
	}
	// Dropping the bad message lets the next delivery go through.
	if err := f.db.Tx(func(tx *store.Tx) error { return tx.Dequeue(f.queued(lead.ID)[0].ID) }); err != nil {
		t.Fatal(err)
	}
	f.rt.Deliver(lead.ID)
	f.until("the next message", func() bool { return len(f.queued(lead.ID)) == 0 })
}

func TestRestoredSessionHoldsMessagesUntilThePrompt(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	if err := f.rt.Stop(s.ID); err != nil {
		t.Fatal(err)
	}
	if a := f.agent(lead.ID); a.Activity != store.Exited || f.session1(s.ID).Running || len(f.log(lead.ID, "shutdown")) != 1 {
		t.Fatalf("after stop: %+v", a)
	}
	if err := f.rt.Input(lead.ID, "to a stopped agent"); err == nil {
		t.Fatal("input reached a stopped agent")
	}
	f.queue("user", lead.ID, "queued while stopped")
	if err := f.rt.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	f.until("the lead idle", func() bool { return f.agent(lead.ID).Activity == store.Idle })
	start := f.log(lead.ID, "start")
	if len(start) != 2 || start[1].Conversation != lead.ID+"-conversation" || !f.session1(s.ID).AwaitingUser {
		t.Fatalf("restart: %v awaiting=%v", start, f.session1(s.ID).AwaitingUser)
	}
	time.Sleep(200 * time.Millisecond)
	if len(f.inputs(lead.ID)) != 0 || len(f.queued(lead.ID)) != 1 {
		t.Fatalf("a restarted session delivered before the user's prompt: %q", f.inputs(lead.ID))
	}
	// The user's prompt goes first; then the queue.
	f.input(lead.ID, "continue")
	f.until("the held message", func() bool { return len(f.inputs(lead.ID)) == 2 && len(f.queued(lead.ID)) == 0 })
	if got := f.inputs(lead.ID); got[0] != "continue" || !strings.Contains(got[1], "queued while stopped") {
		t.Fatalf("inputs = %q", got)
	}
	if got := f.session1(s.ID); got.AwaitingUser || got.Title != "start" {
		t.Fatalf("session = %+v", got)
	}
}

func TestShutdownKeepsTheRunningMarkAndRestoreResumes(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	f.input(lead.ID, "!work")
	f.until("a working lead", func() bool { return f.agent(lead.ID).Activity == store.Busy })
	pid := f.log(lead.ID, "start")[0].Pid
	f.rt.Shutdown()
	// Shutdown saves the unfinished item, ends the process, and leaves the
	// session marked running.
	if a := f.agent(lead.ID); a.Activity != store.Exited || !f.session1(s.ID).Running {
		t.Fatalf("after shutdown: %+v", a)
	}
	if got := summary(f.transcript(lead.ID)); strings.Join(got, "|") != "user:!work|text:working" {
		t.Fatalf("transcript = %q", got)
	}
	if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
		t.Fatalf("the plugin process remains: %v", err)
	}
	if err := f.rt.Start(s.ID); err == nil {
		t.Fatal("a finished runtime started a session")
	}
	rt := f.newRuntime()
	f.rt = rt
	rt.Restore()
	f.until("the restored lead", func() bool { return f.agent(lead.ID).Activity == store.Idle })
	if start := f.log(lead.ID, "start"); len(start) != 2 || start[1].Conversation != lead.ID+"-conversation" || !f.session1(s.ID).AwaitingUser {
		t.Fatalf("restore: %v", start)
	}
}

func TestMessagesGoOutWhileTheAgentWorks(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "!work")
	f.until("a working lead", func() bool { return f.agent(lead.ID).Activity == store.Busy })
	f.queue("user", lead.ID, "change course")
	f.until("steering", func() bool { return len(f.inputs(lead.ID)) == 2 && len(f.queued(lead.ID)) == 0 })
	f.input(lead.ID, "!finish")
	f.until("an idle lead", func() bool { return f.agent(lead.ID).Activity == store.Idle })
	// A lead's finished turn is a reply for the user to see.
	if !f.session1(f.agent(lead.ID).Session).Unseen {
		t.Fatal("the lead's reply is not marked unseen")
	}
}

func TestTranscriptKeepsOrderAndReplacesItems(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "!work")
	f.until("an unfinished item", func() bool {
		items := f.transcript(lead.ID)
		return len(items) == 2 && items[1].Text == "working" && !items[1].Done
	})
	f.input(lead.ID, "!finish")
	f.until("the replaced item", func() bool { return f.agent(lead.ID).Activity == store.Idle })
	// The item's later version replaces it in place instead of appearing again.
	if got := summary(f.transcript(lead.ID)); strings.Join(got, "|") != "user:!work|text:finished|user:!finish" {
		// The finish prompt is recorded after the plugin's reply arrives, so
		// either order of the last two is the same item set.
		if strings.Join(got, "|") != "user:!work|user:!finish|text:finished" && strings.Join(got, "|") != "user:!work|text:finished|user:!finish" {
			t.Fatalf("transcript = %q", got)
		}
	}
	f.input(lead.ID, "!stream")
	f.until("the streamed turn", func() bool { return f.agent(lead.ID).Activity == store.Idle && len(f.transcript(lead.ID)) == 6 })
	items := f.transcript(lead.ID)
	text, tool := items[4], items[5]
	if items[3].Text != "!stream" || text.Text != "Hello!" || !text.Done || tool.Kind != harness.KindTool || tool.Output != "ok" || tool.Input != nil {
		t.Fatalf("streamed items = %+v", items[3:])
	}
	if tool.Parent != text.ID || !strings.HasSuffix(text.ID, ":s") {
		t.Fatalf("the tool's parent is %q, the text's ID %q", tool.Parent, text.ID)
	}
}

func TestAttentionRequests(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "!ask")
	f.until("attention", func() bool { return f.agent(lead.ID).Attention })
	if requests := f.rt.Requests(); len(requests) != 1 || requests[0].Agent != lead.ID || requests[0].Item.Kind != harness.KindAttention {
		t.Fatalf("requests = %+v", requests)
	}
	var request string
	for _, it := range f.transcript(lead.ID) {
		if it.Kind == harness.KindAttention {
			request = it.ID
			if it.Text != "Proceed?" || len(it.Choices) != 2 || it.Done {
				t.Fatalf("request = %+v", it)
			}
		}
	}
	if err := f.rt.Answer(lead.ID, "nothing", "yes", ""); err == nil || !strings.Contains(err.Error(), "finished") {
		t.Fatalf("answer to an unknown request: %v", err)
	}
	if err := f.rt.Answer(lead.ID, request, "yes", ""); err != nil {
		t.Fatal(err)
	}
	f.until("the answered request", func() bool { return !f.agent(lead.ID).Attention })
	if requests := f.rt.Requests(); len(requests) != 0 {
		t.Fatalf("answered requests = %+v", requests)
	}
	if got := f.log(lead.ID, "answer"); len(got) != 1 || got[0].Request != "q" || got[0].Choice != "yes" {
		t.Fatalf("the plugin was answered with %v", got)
	}
	var answered harness.Item
	for _, it := range f.transcript(lead.ID) {
		if it.ID == request {
			answered = it
		}
	}
	if !answered.Done || answered.Answer != "yes" {
		t.Fatalf("answered item = %+v", answered)
	}
	if err := f.rt.Answer(lead.ID, request, "no", ""); err == nil || !strings.Contains(err.Error(), "finished") {
		t.Fatalf("a second answer: %v", err)
	}
}

func TestRequestsNewestFirst(t *testing.T) {
	r := &Runtime{live: map[string]*live{
		"a": {id: "a", pending: map[string]*pendingItem{
			"older": {item: harness.Item{ID: "older", At: 1, Kind: harness.KindAttention}},
			"z":     {item: harness.Item{ID: "z", At: 2, Kind: harness.KindAttention}},
			"text":  {item: harness.Item{ID: "text", At: 3, Kind: harness.KindText}},
		}},
		"b": {id: "b", pending: map[string]*pendingItem{
			"a": {item: harness.Item{ID: "a", At: 2, Kind: harness.KindAttention}},
		}},
	}}
	requests := r.Requests()
	if len(requests) != 3 || requests[0].Item.ID != "z" || requests[1].Item.ID != "a" || requests[2].Item.ID != "older" || requests[1].Agent != "b" {
		t.Fatalf("requests = %+v", requests)
	}
}

func TestInterruptAndUsage(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "!work")
	f.until("a working lead", func() bool { return f.agent(lead.ID).Activity == store.Busy })
	if err := f.rt.Interrupt(lead.ID); err != nil {
		t.Fatal(err)
	}
	f.until("an idle lead", func() bool { return f.agent(lead.ID).Activity == store.Idle })
	// The interrupted turn stays, and nothing sends it again.
	if got := summary(f.transcript(lead.ID)); strings.Join(got, "|") != "user:!work|text:working" || len(f.inputs(lead.ID)) != 1 {
		t.Fatalf("transcript = %q", got)
	}
	f.input(lead.ID, "!usage")
	f.until("usage", func() bool {
		return f.agent(lead.ID).Usage == harness.Usage{Context: 45200, Window: 200000, Cost: 0.31}
	})
}

func TestWorkerFailureReachesItsLead(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	other, otherLead := f.session()
	worker := f.addWorker(lead, "w1", "")
	f.input(worker.ID, "!fail")
	f.until("the failure message", func() bool {
		for _, in := range f.inputs(lead.ID) {
			if strings.Contains(in, "My turn failed: test provider failure") && strings.Contains(in, s.ID+"/w1") {
				return true
			}
		}
		return false
	})
	if len(f.inputs(otherLead.ID)) != 0 || f.agent(worker.ID).Activity != store.Idle {
		t.Fatalf("the failure reached %s's lead, or the worker is %s", other.ID, f.agent(worker.ID).Activity)
	}
}

func (f *fixture) addWorker(lead store.Agent, name, message string) store.Agent {
	f.t.Helper()
	cfg, err := harness.LoadConfig()
	if err != nil {
		f.t.Fatal(err)
	}
	profile, err := cfg.Worker("worker")
	if err != nil {
		f.t.Fatal(err)
	}
	w, err := f.rt.AddWorker(lead.ID, name, profile, "", message)
	if err != nil {
		f.t.Fatal(err)
	}
	return w
}

func TestWorkerLifecycle(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	worker := f.addWorker(lead, "w1", "the brief")
	if _, err := os.Stat(filepath.Join(worker.Dir, ".git")); err != nil {
		t.Fatal("no worktree:", err)
	}
	// The brief is the worker's first input.
	f.until("the brief", func() bool { return len(f.queued(worker.ID)) == 0 && len(f.inputs(worker.ID)) == 1 })
	if got := f.inputs(worker.ID)[0]; !strings.Contains(got, "the brief") || !strings.Contains(got, lead.Address()) {
		t.Fatalf("brief = %q", got)
	}
	// The transcript shows the brief as a message from the lead.
	if items := f.transcript(worker.ID); len(items) == 0 || items[0].From != lead.Address() || items[0].Text != "the brief" {
		t.Fatalf("brief item = %+v", items)
	}
	if start := f.log(worker.ID, "start"); start[0].Role != "worker" || start[0].Dir != worker.Dir {
		t.Fatalf("worker start = %v", start)
	}
	if _, err := f.rt.AddWorker(lead.ID, "w1", harness.Profile{Spec: harness.Spec{Harness: "test"}}, "", ""); err == nil {
		t.Fatal("a second worker took the name")
	}
	// A dirty worktree keeps its worker running.
	dirty := filepath.Join(worker.Dir, "unfinished")
	if err := os.WriteFile(dirty, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.RemoveWorker(lead.ID, "w1"); err == nil || len(f.agents(lead.Session)) != 2 || len(f.log(worker.ID, "shutdown")) != 0 {
		t.Fatalf("removing a dirty worker: %v", err)
	}
	os.Remove(dirty)
	pid := f.log(worker.ID, "start")[0].Pid
	if _, err := f.rt.RemoveWorker(lead.ID, "w1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(worker.Dir); !os.IsNotExist(err) || len(f.agents(lead.Session)) != 1 || syscall.Kill(pid, 0) != syscall.ESRCH {
		t.Fatal("the worker, its worktree, or its process remains")
	}
	// A worker that cannot start leaves nothing behind.
	if _, err := f.rt.AddWorker(lead.ID, "w2", harness.Profile{Spec: harness.Spec{Harness: "missing"}}, "", ""); err == nil || !strings.Contains(err.Error(), "hypercube-harness-missing") {
		t.Fatalf("worker with no plugin: %v", err)
	}
	if len(f.agents(lead.Session)) != 1 {
		t.Fatal("the failed worker remains")
	}
	if _, err := os.Stat(filepath.Join(f.root, ".hypercube", lead.Session, "w2")); !os.IsNotExist(err) {
		t.Fatal("the failed worker's worktree remains")
	}
	if err := f.rt.Stop(lead.Session); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rt.AddWorker(lead.ID, "w3", harness.Profile{Spec: harness.Spec{Harness: "test"}}, ".", ""); err == nil {
		t.Fatal("a stopped session took a worker")
	}
}

// A worker can be created while another agent's request to its plugin is
// waiting, as when a native tool call reaches the MCP tools.
func TestWorkerCreatedWhileARequestWaits(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	held := make(chan error, 1)
	go func() { held <- f.rt.Input(lead.ID, "!hold") }()
	f.until("the held input", func() bool { return len(f.inputs(lead.ID)) == 1 })
	f.addWorker(lead, "w1", "brief")
	f.queue("user", lead.ID, "queued behind the held input")
	select {
	case err := <-held:
		t.Fatalf("the held input returned early: %v", err)
	default:
	}
	if err := f.rt.Interrupt(lead.ID); err != nil {
		t.Fatal(err)
	}
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	f.until("the queued message", func() bool { return len(f.queued(lead.ID)) == 0 })
}

func TestRelaunchAfterTheProcessEnds(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "hello")
	f.until("the first turn", func() bool { return len(f.transcript(lead.ID)) == 2 })
	old := f.rt.live[lead.ID]
	f.input(lead.ID, "!crash")
	f.until("a relaunch", func() bool { return len(f.log(lead.ID, "start")) == 2 && f.agent(lead.ID).Activity == store.Idle })
	if start := f.log(lead.ID, "start"); start[1].Conversation != lead.ID+"-conversation" {
		t.Fatalf("relaunch did not resume: %v", start[1])
	}
	// The ended launch left a note, and the new launch's items follow the old
	// ones even though it numbers its own from the start.
	f.input(lead.ID, "again")
	f.until("a turn after the relaunch", func() bool { return len(f.transcript(lead.ID)) == 6 })
	got := summary(f.transcript(lead.ID))
	if !strings.HasPrefix(got[3], "error:The agent ended: ") || got[0] != "user:hello" || got[1] != "text:ack" || got[2] != "user:!crash" || got[4] != "user:again" || got[5] != "text:ack" {
		t.Fatalf("transcript = %q", got)
	}
	// Events from the ended launch change nothing.
	before := f.agent(lead.ID)
	f.rt.event(old, harness.Event{State: harness.Working, Items: []harness.Item{{ID: "late", Kind: harness.KindText, Text: "stale", Done: true}}})
	if f.agent(lead.ID).Activity != before.Activity || len(f.transcript(lead.ID)) != 6 {
		t.Fatal("an old launch's event was applied")
	}
}

func TestCreateSendsTheFirstPrompt(t *testing.T) {
	f := newFixture(t)
	s, lead, err := f.rt.Create(f.workspace.ID, "lead", "\n  Fix the  parser \nand the lexer")
	if err != nil {
		t.Fatal(err)
	}
	if got := f.session1(s.ID).Title; got != "Fix the parser" {
		t.Errorf("title = %q", got)
	}
	if got := f.inputs(lead.ID); len(got) != 1 || got[0] != "\n  Fix the  parser \nand the lexer" {
		t.Errorf("plugin inputs = %q", got)
	}
	f.until("the prompt in the transcript", func() bool {
		items := f.transcript(lead.ID)
		return len(items) > 0 && items[0].Kind == harness.KindUser && items[0].Text == "\n  Fix the  parser \nand the lexer"
	})
	png := harness.Image{MIMEType: "image/png", Data: "iVBORw0KGgo="}
	s, lead, err = f.rt.Create(f.workspace.ID, "lead", "", png)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.session1(s.ID).Title; got != "Image" {
		t.Errorf("image-only title = %q", got)
	}
}

func TestCreateRefusesABadPromptBeforeMakingAnything(t *testing.T) {
	f := newFixture(t)
	for _, c := range []struct {
		text   string
		images []harness.Image
	}{
		{"", nil},
		{" \n", nil},
		{"", []harness.Image{{MIMEType: "image/png", Data: "bm90IGEgcG5n"}}},
	} {
		if _, _, err := f.rt.Create(f.workspace.ID, "lead", c.text, c.images...); err == nil {
			t.Errorf("Create accepted %q and %d images", c.text, len(c.images))
		}
	}
	f.read(func(tx *store.Tx) error {
		sessions, err := tx.Sessions(f.workspace.ID)
		if err != nil || len(sessions) != 0 {
			t.Errorf("sessions = %v, %v", sessions, err)
		}
		return nil
	})
	if entries, _ := os.ReadDir(f.logs); len(entries) != 0 {
		t.Errorf("a plugin started: %v", entries)
	}
}

func TestAgentThatKeepsEndingStaysEnded(t *testing.T) {
	f := newFixture(t)
	t.Setenv("HYPERCUBE_TEST_CRASH", "1")
	// The plugin can exit before the first prompt reaches it, which Create
	// reports; the session and its lead remain.
	s, lead, _ := f.rt.Create(f.workspace.ID, "lead", "start")
	f.until("the agent to stay ended", func() bool {
		return len(f.log(lead.ID, "start")) == maxQuickExits && f.agent(lead.ID).Activity == store.Exited && f.rt.live[lead.ID] == nil
	})
	time.Sleep(300 * time.Millisecond)
	if n := len(f.log(lead.ID, "start")); n != maxQuickExits {
		t.Fatalf("%d launches", n)
	}
	var ended int
	for _, it := range f.transcript(lead.ID) {
		if strings.HasPrefix(it.Text, "The agent ended: ") {
			ended++
		}
	}
	if ended != maxQuickExits || !f.session1(s.ID).Running {
		t.Fatalf("%d notes, session %+v", ended, f.session1(s.ID))
	}
}

func TestFailedResumeIsAnError(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	if err := f.rt.Stop(s.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tx(func(tx *store.Tx) error {
		a, err := tx.Agent(lead.ID)
		a.Conversation = "unresumable"
		if err != nil {
			return err
		}
		return tx.SaveAgent(a)
	}); err != nil {
		t.Fatal(err)
	}
	err := f.rt.Start(s.ID)
	if err == nil || !strings.Contains(err.Error(), "cannot resume conversation unresumable") {
		t.Fatalf("start: %v", err)
	}
	// The agent stays ended with the error in its transcript, and no fresh
	// conversation starts in its place.
	var shown bool
	for _, it := range f.transcript(lead.ID) {
		shown = shown || it.Kind == harness.KindError && strings.Contains(it.Text, "cannot resume")
	}
	starts := f.log(lead.ID, "start")
	if !shown || f.agent(lead.ID).Activity != store.Exited || f.agent(lead.ID).Conversation != "unresumable" || len(starts) != 2 || starts[1].Conversation != "unresumable" {
		t.Fatalf("after a failed resume: shown=%v agent=%+v starts=%v", shown, f.agent(lead.ID), starts)
	}
	if f.rt.live[lead.ID] != nil {
		t.Fatal("the failed agent is registered as running")
	}
}

func TestUnsupportedHarnessFailsTheStart(t *testing.T) {
	f := newFixture(t)
	script := "#!/bin/sh\necho 'unsupported harness: not ported' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(f.plugins, "hypercube-harness-old"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Tx(func(tx *store.Tx) error {
		if err := tx.AddSession(store.Session{ID: "s", Workspace: f.workspace.ID}); err != nil {
			return err
		}
		return tx.AddAgent(store.Agent{ID: "a", Session: "s", Name: "lead", Role: store.Lead, Harness: "old", Dir: f.root, Activity: store.Exited})
	}); err != nil {
		t.Fatal(err)
	}
	err := f.rt.Start("s")
	if err == nil || !strings.Contains(err.Error(), "unsupported harness: not ported") {
		t.Fatalf("start: %v", err)
	}
	if got := summary(f.transcript("a")); len(got) != 1 || !strings.Contains(got[0], "unsupported harness") || f.agent("a").Activity != store.Exited {
		t.Fatalf("transcript = %q", got)
	}
}

func TestDeletingASessionEndsItsAgents(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	worker := f.addWorker(lead, "w1", "")
	f.input(worker.ID, "!child")
	var child int
	f.until("the worker's child", func() bool {
		for _, e := range f.log(worker.ID, "child") {
			child = e.Pid
		}
		return child != 0
	})
	if err := f.rt.Delete(s.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.agents(s.ID)) != 0 || len(f.log(lead.ID, "shutdown")) != 1 || len(f.log(worker.ID, "shutdown")) != 1 {
		t.Fatal("the session or its agents remain")
	}
	f.until("the child to end", func() bool { return syscall.Kill(child, 0) == syscall.ESRCH })
	if _, err := os.Stat(worker.Dir); !os.IsNotExist(err) {
		t.Fatal("the worktree remains")
	}
}

func TestShellsEndWithTheirSession(t *testing.T) {
	f := newFixture(t)
	f.rt.Terminals.Command = []string{"/bin/sleep", "300"}
	s, _ := f.session()
	open := func() interface{ Done() <-chan struct{} } {
		sh, err := f.rt.Shell(s.ID, 80, 24)
		if err != nil {
			t.Fatal(err)
		}
		return sh
	}
	ended := func(sh interface{ Done() <-chan struct{} }) bool {
		select {
		case <-sh.Done():
			return true
		default:
			return false
		}
	}
	sh := open()
	if err := f.rt.Stop(s.ID); err != nil || !ended(sh) {
		t.Fatalf("Stop left the shell running: %v", err)
	}
	if _, err := f.rt.Shell(s.ID, 80, 24); err == nil {
		t.Fatal("a stopped session opened a shell")
	}
	if err := f.rt.Start(s.ID); err != nil {
		t.Fatal(err)
	}
	sh = open()
	if err := f.rt.Delete(s.ID); err != nil || !ended(sh) {
		t.Fatalf("Delete left the shell running: %v", err)
	}
	s, _ = f.session()
	sh = open()
	f.rt.Shutdown()
	if !ended(sh) {
		t.Fatal("Shutdown left the shell running")
	}
}

func TestImagePrompts(t *testing.T) {
	f := newFixture(t)
	s, lead := f.session()
	png := harness.Image{MIMEType: "image/png", Data: "iVBORw0KGgo="}
	if err := f.rt.Input(lead.ID, "", harness.Image{MIMEType: "image/png", Data: "bm90IGEgcG5n"}); err == nil {
		t.Fatal("input accepted bytes that are not a PNG")
	}
	if err := f.rt.Input(lead.ID, "  "); err == nil {
		t.Fatal("input accepted neither text nor an image")
	}
	// An image alone is a prompt: the plugin gets it, and the user's item holds
	// it. The session keeps its title.
	if err := f.rt.Input(lead.ID, "", png); err != nil {
		t.Fatal(err)
	}
	if got := f.session1(s.ID).Title; got != "start" {
		t.Errorf("title = %q", got)
	}
	if got := f.log(lead.ID, "input"); len(got) != 2 || got[1].Images != 1 {
		t.Errorf("plugin inputs = %v", got)
	}
	f.until("the image prompt in the transcript", func() bool {
		items := f.transcript(lead.ID)
		return len(items) > 0 && items[0].Kind == harness.KindUser && len(items[0].Images) == 1 && items[0].Images[0] == png
	})
	// An image item from the plugin reaches the transcript whole.
	f.input(lead.ID, "!image")
	f.until("the assistant's image", func() bool {
		for _, it := range f.transcript(lead.ID) {
			if it.Kind == harness.KindText && it.Text == "" && len(it.Images) == 1 && it.Images[0] == png {
				return true
			}
		}
		return false
	})
}

func TestTranscriptTimestampsSurviveUpdatesAndReload(t *testing.T) {
	f := newFixture(t)
	_, lead := f.session()
	f.input(lead.ID, "!work")
	f.until("the unfinished item", func() bool { return len(f.transcript(lead.ID)) == 2 })
	first := f.transcript(lead.ID)[1]
	if first.At == 0 {
		t.Fatal("streamed item has no arrival timestamp")
	}
	// Make replacing the timestamp on completion observable.
	time.Sleep(2 * time.Millisecond)
	f.input(lead.ID, "!finish")
	f.until("the finished item", func() bool { return f.agent(lead.ID).Activity == store.Idle })
	f.rt.Shutdown()
	reloaded := f.newRuntime()
	items, err := reloaded.Transcript(lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range items {
		if it.At == 0 {
			t.Fatalf("saved item %q has no arrival timestamp", it.ID)
		}
		if it.ID == first.ID {
			found = true
			if it.At != first.At || !it.Done || it.Text != "finished" {
				t.Fatalf("arrival timestamp changed or item was not completed: %+v", it)
			}
		}
	}
	if !found {
		t.Fatal("reloaded transcript lost the completed item")
	}
}

func TestApplyEvent(t *testing.T) {
	worker := store.Agent{Role: store.Worker, Activity: store.Busy}
	lead := store.Agent{Role: store.Lead, Activity: store.Idle}
	var s store.Session
	applyEvent(&lead, &s, harness.Event{State: harness.Working, Conversation: "c1"})
	if lead.Activity != store.Busy || lead.Conversation != "c1" || s.Unseen {
		t.Fatalf("working: %+v %+v", lead, s)
	}
	applyEvent(&lead, &s, harness.Event{Items: []harness.Item{{ID: "x", Kind: harness.KindText}}})
	if lead.Activity != store.Busy || lead.Conversation != "c1" {
		t.Fatalf("an event without state or conversation changed the agent: %+v", lead)
	}
	applyEvent(&lead, &s, harness.Event{State: harness.Idle, Usage: &harness.Usage{Context: 45200, Window: 200000, Cost: 0.31}})
	if lead.Activity != store.Idle || !s.Unseen || lead.Usage != (harness.Usage{Context: 45200, Window: 200000, Cost: 0.31}) {
		t.Fatalf("lead idle: %+v %+v", lead, s)
	}
	s.Unseen = false
	applyEvent(&lead, &s, harness.Event{State: harness.Idle})
	if s.Unseen {
		t.Fatal("a lead that was already idle marked its session unseen")
	}
	applyEvent(&worker, &s, harness.Event{State: harness.Idle})
	if worker.Activity != store.Idle || s.Unseen {
		t.Fatalf("worker idle: %+v %+v", worker, s)
	}
}
