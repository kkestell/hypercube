package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func run(t *testing.T, event, input string) response {
	t.Helper()
	var out bytes.Buffer
	if err := Run(event, strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	var r response
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// open returns a database holding workspace w with the given sessions and
// agents, each agent in the session named by its Session field. The reports
// come from agent self.
func open(t *testing.T, self string, sessions []store.Session, agents ...store.Agent) *store.DB {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	t.Setenv("HYPERCUBE_AGENT_ID", self)
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: "/repo"}); err != nil {
			return err
		}
		for _, s := range sessions {
			s.Workspace = "w"
			if err := tx.AddSession(s); err != nil {
				return err
			}
		}
		for _, a := range agents {
			if a.Activity == "" {
				a.Activity = store.Idle
			}
			if err := tx.AddAgent(a); err != nil {
				return err
			}
		}
		return nil
	})
	return db
}

func change(t *testing.T, db *store.DB, fn func(*store.Tx) error) {
	t.Helper()
	if err := db.Tx(fn); err != nil {
		t.Fatal(err)
	}
}

func agent(t *testing.T, db *store.DB, id string) (a store.Agent) {
	t.Helper()
	change(t, db, func(tx *store.Tx) error { var err error; a, err = tx.Agent(id); return err })
	return a
}

func session(t *testing.T, db *store.DB, id string) (s store.Session) {
	t.Helper()
	change(t, db, func(tx *store.Tx) error { var err error; s, err = tx.Session(id); return err })
	return s
}

func queued(t *testing.T, db *store.DB, id string) (m []store.Message) {
	t.Helper()
	change(t, db, func(tx *store.Tx) error { var err error; m, err = tx.Queued(id); return err })
	return m
}

func queue(t *testing.T, db *store.DB, from, to, text string) {
	t.Helper()
	change(t, db, func(tx *store.Tx) error { return tx.Queue(store.Message{From: from, To: to, Text: text}) })
}

func TestStartListsWorkerRoles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "hypercube"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{"profiles":[
		{"name":"opus","role":"lead","description":"leading","harness":"claude","model":"opus","effort":"high"},
		{"name":"planner","role":"plan","description":"designing","harness":"claude","model":"opus","effort":"high"},
		{"name":"builder","role":"work","description":"building","harness":"codex","model":"sol","effort":"high"},
		{"name":"reviewer","role":"review","description":"reviewing","harness":"claude","model":"sonnet","effort":"high"}
	]}`
	if err := os.WriteFile(filepath.Join(dir, "hypercube/config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	open(t, "a", []store.Session{{ID: "session-1"}}, store.Agent{ID: "a", Session: "session-1", Name: "lead", Role: store.Lead})
	context := run(t, "start", `{}`).Context
	for _, want := range []string{
		"role says which work each profile suits: plan, work, or review",
		"- planner (plan; claude, opus, high): designing",
		"- builder (work; codex, sol, high): building",
		"- reviewer (review; claude, sonnet, high): reviewing",
	} {
		if !strings.Contains(context, want) {
			t.Errorf("context missing %q: %s", want, context)
		}
	}
	if strings.Contains(context, "- opus (") {
		t.Fatalf("lead profile listed as worker: %s", context)
	}
}

func TestDelivery(t *testing.T) {
	db := open(t, "b", []store.Session{{ID: "session-1"}},
		store.Agent{ID: "a", Session: "session-1", Name: "lead", Role: store.Lead},
		store.Agent{ID: "b", Session: "session-1", Name: "w1", Role: store.Worker})

	if r := run(t, "start", `{}`); !strings.Contains(r.Context, "You are session-1/w1") {
		t.Errorf("start context = %q", r.Context)
	}

	queue(t, db, "a", "b", "hello")
	if r := run(t, "prompt", `{"conversation":"s1"}`); !strings.HasPrefix(r.Context, "Message from session-1/lead:\nhello\n") {
		t.Errorf("prompt context = %q", r.Context)
	}
	if a := agent(t, db, "b"); a.Activity != store.Busy || len(queued(t, db, "b")) != 0 || a.Conversation != "s1" {
		t.Errorf("after prompt: %+v", a)
	}

	queue(t, db, "a", "b", "mid-turn")
	if r := run(t, "tool", `{}`); !strings.Contains(r.Context, "mid-turn") {
		t.Errorf("tool context = %q", r.Context)
	}
	if r := run(t, "tool", `{}`); r.Context != "" {
		t.Errorf("empty queue produced %+v", r)
	}

	queue(t, db, "a", "b", "more")
	if r := run(t, "stop", `{}`); !strings.Contains(r.Context, "more") {
		t.Errorf("stop with a message = %+v", r)
	}
	if a := agent(t, db, "b"); a.Activity != store.Busy {
		t.Errorf("continued stop marked agent %s", a.Activity)
	}
	if r := run(t, "stop", `{}`); r.Context != "" {
		t.Errorf("stop without messages = %+v", r)
	}
	if a := agent(t, db, "b"); a.Activity != store.Idle {
		t.Errorf("stop left agent %s", a.Activity)
	}
	if session(t, db, "session-1").Unseen {
		t.Error("a worker's stop marked the session unseen")
	}

	run(t, "approval", `{}`)
	if !agent(t, db, "b").Attention {
		t.Error("approval did not flag attention")
	}
	run(t, "answered", `{}`)
	if agent(t, db, "b").Attention {
		t.Error("answered approval is still flagged")
	}

	run(t, "usage", `{"context":45200,"window":200000,"cost":0.31}`)
	if u := agent(t, db, "b").Usage; u != (store.Usage{Context: 45200, Window: 200000, Cost: 0.31}) {
		t.Errorf("usage = %+v", u)
	}

	t.Setenv("HYPERCUBE_AGENT_ID", "a")
	run(t, "stop", `{}`)
	if !session(t, db, "session-1").Unseen {
		t.Error("the lead's reply did not mark the session unseen")
	}
}

func TestWorkerPromptReleasesOnlyItsSession(t *testing.T) {
	db := open(t, "w", []store.Session{{ID: "t", AwaitingUser: true}, {ID: "other", AwaitingUser: true}},
		store.Agent{ID: "l", Session: "t", Name: "lead", Role: store.Lead},
		store.Agent{ID: "w", Session: "t", Name: "worker", Role: store.Worker},
		store.Agent{ID: "other", Session: "other", Name: "lead", Role: store.Lead})
	queue(t, db, "user", "w", "queued")
	if got := run(t, "prompt", `{"text":"continue"}`).Context; !strings.Contains(got, "queued") {
		t.Fatalf("prompt context = %q", got)
	}
	if session(t, db, "t").AwaitingUser || !session(t, db, "other").AwaitingUser || len(queued(t, db, "w")) != 0 {
		t.Fatal("a worker's prompt released the wrong session")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestFailedResponseKeepsMessages(t *testing.T) {
	db := open(t, "b", []store.Session{{ID: "session-1"}},
		store.Agent{ID: "a", Session: "session-1", Name: "lead", Role: store.Lead},
		store.Agent{ID: "b", Session: "session-1", Name: "w1", Role: store.Worker})
	queue(t, db, "a", "b", "hello")
	if err := Run("tool", strings.NewReader(`{}`), failingWriter{}); err == nil {
		t.Fatal("Run succeeded with a failing writer")
	}
	if m := queued(t, db, "b"); len(m) != 1 {
		t.Errorf("messages = %+v", m)
	}
}

func TestSessionContextAndFailureRouting(t *testing.T) {
	db := open(t, "other-worker", []store.Session{{ID: "session-1"}, {ID: "other", Title: "Fix the parser"}},
		store.Agent{ID: "o", Session: "session-1", Name: "lead", Role: store.Lead},
		store.Agent{ID: "w", Session: "session-1", Name: "worker", Role: store.Worker},
		store.Agent{ID: "other-lead", Session: "other", Name: "lead", Role: store.Lead},
		store.Agent{ID: "other-worker", Session: "other", Name: "worker", Role: store.Worker, Activity: store.Busy})
	queue(t, db, "o", "other-worker", "kept")
	got := run(t, "start", `{}`).Context
	if strings.Contains(got, "session-1/") || !strings.Contains(got, "other/lead") || !strings.Contains(got, `"other", titled "Fix the parser"`) {
		t.Fatal("session context exposed the wrong team", got)
	}
	run(t, "failed", `{"error":" rate limited "}`)
	m := queued(t, db, "other-lead")
	if len(m) != 1 || m[0].Text != "My turn failed: rate limited" || len(queued(t, db, "other-worker")) != 1 || agent(t, db, "other-worker").Activity != store.Idle {
		t.Fatal(m)
	}
	config := filepath.Join(t.TempDir(), "hypercube", "config.json")
	os.MkdirAll(filepath.Dir(config), 0o755)
	os.WriteFile(config, []byte(`{"profiles":[{"name":"l","role":"lead","description":"d","harness":"h","model":"m","effort":"e"},{"name":"w","role":"work","description":"d","harness":"h","model":"m","effort":"e"}]}`), 0o644)
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(filepath.Dir(config)))
	t.Setenv("HYPERCUBE_AGENT_ID", "other-lead")
	if got := run(t, "start", `{}`).Context; !strings.Contains(got, "Other leads: session-1/lead (New Session)") {
		t.Fatal("lead context missed the other lead", got)
	}
}

func TestToolLog(t *testing.T) {
	open(t, "a", []store.Session{{ID: "session-1"}}, store.Agent{ID: "a", Session: "session-1", Name: "lead", Role: store.Lead, Harness: "claude", Model: "opus"})
	run(t, "toolfailed", `{"tool":"mcp__hypercube__send_message","input":{"to":"nobody"},"error":"no agent"}`)
	run(t, "denied", `{"tool":"Bash","input":{"command":"rm -rf /"},"reason":"destructive"}`)
	b, err := os.ReadFile(filepath.Join(store.Dir(), "tools.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("log = %s", b)
	}
	var failed, denied map[string]any
	if json.Unmarshal([]byte(lines[0]), &failed) != nil || json.Unmarshal([]byte(lines[1]), &denied) != nil {
		t.Fatalf("log = %s", b)
	}
	if failed["msg"] != "tool failed" || failed["error"] != "no agent" || failed["session"] != "session-1" || failed["workspace"] != "/repo" || failed["agent"] != "lead" || failed["model"] != "opus" {
		t.Errorf("failed = %v", failed)
	}
	if input, _ := failed["input"].(map[string]any); input["to"] != "nobody" {
		t.Errorf("failed input = %v", failed["input"])
	}
	if denied["msg"] != "tool denied" || denied["tool"] != "Bash" || denied["reason"] != "destructive" {
		t.Errorf("denied = %v", denied)
	}
}

func TestPromptTitlesIdleUntitledLead(t *testing.T) {
	var db *store.DB
	setup := func(s store.Session, lead store.Agent) {
		t.Helper()
		s.ID = "t"
		lead.ID, lead.Session, lead.Name, lead.Role = "l", "t", "lead", store.Lead
		db = open(t, "l", []store.Session{s}, lead, store.Agent{ID: "w", Session: "t", Name: "worker", Role: store.Worker})
	}
	prompt := func(id, text string) string {
		t.Helper()
		t.Setenv("HYPERCUBE_AGENT_ID", id)
		b, _ := json.Marshal(map[string]string{"text": text})
		run(t, "prompt", string(b))
		return session(t, db, "t").Title
	}
	long := strings.Repeat("a", 250)
	for _, c := range []struct{ name, text, want string }{
		{"first line", "Fix the parser\nand the lexer", "Fix the parser"},
		{"blank lines and spacing", "\n  \n  Fix \t the   parser  \nmore", "Fix the parser"},
		{"long line", long, strings.Repeat("a", 199) + "…"},
		{"exactly 200", long[:200], long[:200]},
		{"non-ASCII", strings.Repeat("é", 250), strings.Repeat("é", 199) + "…"},
		{"only whitespace", " \n\t ", ""},
		{"no text", "", ""},
	} {
		setup(store.Session{}, store.Agent{})
		if got := prompt("l", c.text); got != c.want {
			t.Errorf("%s: title = %q, want %q", c.name, got, c.want)
		}
	}

	setup(store.Session{}, store.Agent{})
	prompt("l", "first")
	if got := prompt("l", "second"); got != "first" {
		t.Errorf("a later prompt changed the title to %q", got)
	}
	setup(store.Session{}, store.Agent{Activity: store.Busy})
	if got := prompt("l", "busy"); got != "" {
		t.Errorf("a busy lead's prompt set title %q", got)
	}
	setup(store.Session{Title: "Mine"}, store.Agent{})
	if got := prompt("l", "other"); got != "Mine" {
		t.Errorf("a titled session's title became %q", got)
	}
	setup(store.Session{}, store.Agent{})
	if got := prompt("w", "worker prompt"); got != "" {
		t.Errorf("a worker's prompt set title %q", got)
	}
}
