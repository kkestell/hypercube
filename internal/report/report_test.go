package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// open returns a database holding workspace w with the given sessions and
// agents, each agent in the session named by its Session field.
func open(t *testing.T, sessions []store.Session, agents ...store.Agent) *store.DB {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
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

func queued(t *testing.T, db *store.DB, id string) (m []store.Message) {
	t.Helper()
	change(t, db, func(tx *store.Tx) error { var err error; m, err = tx.Queued(id); return err })
	return m
}

func writeConfig(t *testing.T, config string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "hypercube"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hypercube/config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
}

// instructions returns what the agent is told at start.
func instructions(t *testing.T, db *store.DB, id string) string {
	t.Helper()
	cfg, err := harness.LoadConfig()
	var profiles []harness.Profile
	if err == nil {
		profiles = cfg.Workers()
	}
	var text string
	if err := db.Read(func(tx *store.Tx) error {
		a, err := tx.Agent(id)
		if err != nil {
			return err
		}
		if a.Role != store.Lead {
			profiles = nil
		}
		text, err = Instructions(tx, a, profiles)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return text
}

func TestInstructionsListWorkerRoles(t *testing.T) {
	writeConfig(t, `{"profiles":[
		{"name":"opus","role":"lead","description":"leading","harness":"claude","model":"opus","effort":"high"},
		{"name":"planner","role":"plan","description":"designing","harness":"claude","model":"opus","effort":"high"},
		{"name":"builder","role":"work","description":"building","harness":"codex","model":"sol","effort":"high"},
		{"name":"reviewer","role":"review","description":"reviewing","harness":"claude","model":"sonnet","effort":"high"}
	]}`)
	db := open(t, []store.Session{{ID: "session-1"}}, store.Agent{ID: "a", Session: "session-1", Name: "lead", Role: store.Lead})
	text := instructions(t, db, "a")
	for _, want := range []string{
		"role says which work each profile suits: plan, work, or review",
		"- planner (plan; claude, opus, high): designing",
		"- builder (work; codex, sol, high): building",
		"- reviewer (review; claude, sonnet, high): reviewing",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("instructions missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "- opus (") {
		t.Fatalf("lead profile listed as worker: %s", text)
	}
}

func TestInstructionsScopeToTheSession(t *testing.T) {
	db := open(t, []store.Session{{ID: "session-1"}, {ID: "other", Title: "Fix the parser"}},
		store.Agent{ID: "o", Session: "session-1", Name: "lead", Role: store.Lead},
		store.Agent{ID: "w", Session: "session-1", Name: "worker", Role: store.Worker},
		store.Agent{ID: "other-lead", Session: "other", Name: "lead", Role: store.Lead},
		store.Agent{ID: "other-worker", Session: "other", Name: "worker", Role: store.Worker})
	got := instructions(t, db, "other-worker")
	if !strings.Contains(got, "You are other/worker, the worker") || strings.Contains(got, "session-1/") || !strings.Contains(got, "other/lead") || !strings.Contains(got, `"other", titled "Fix the parser"`) {
		t.Fatal("worker instructions exposed the wrong team", got)
	}
	writeConfig(t, `{"profiles":[{"name":"l","role":"lead","description":"d","harness":"h","model":"m","effort":"e"},{"name":"w","role":"work","description":"d","harness":"h","model":"m","effort":"e"}]}`)
	if got := instructions(t, db, "other-lead"); !strings.Contains(got, "Other leads: session-1/lead (New Session)") || strings.Contains(got, "session-1/worker") {
		t.Fatal("lead instructions missed the other lead", got)
	}
}

func TestFormatAttributesMessages(t *testing.T) {
	db := open(t, []store.Session{{ID: "session-1"}},
		store.Agent{ID: "a", Session: "session-1", Name: "lead", Role: store.Lead})
	var got string
	if err := db.Read(func(tx *store.Tx) error {
		got = Format(tx, []store.Message{{From: "a", Text: "hello"}, {From: "user", Text: "hi"}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := "Message from session-1/lead:\nhello\n\nsession-1/lead cannot see your chat. To answer, use the hypercube send_message tool to session-1/lead.\n\nMessage from user:\nhi"
	if got != want {
		t.Fatalf("Format = %q", got)
	}
}

func TestApply(t *testing.T) {
	worker := store.Agent{Role: store.Worker, Activity: store.Busy}
	lead := store.Agent{Role: store.Lead, Activity: store.Idle}
	var s store.Session
	Apply(&lead, &s, harness.Event{State: harness.Working, Conversation: "c1"})
	if lead.Activity != store.Busy || lead.Conversation != "c1" || s.Unseen {
		t.Fatalf("working: %+v %+v", lead, s)
	}
	Apply(&lead, &s, harness.Event{Items: []harness.Item{{ID: "x", Kind: harness.KindText}}})
	if lead.Activity != store.Busy || lead.Conversation != "c1" {
		t.Fatalf("an event without state or conversation changed the agent: %+v", lead)
	}
	Apply(&lead, &s, harness.Event{State: harness.Idle, Usage: &harness.Usage{Context: 45200, Window: 200000, Cost: 0.31}})
	if lead.Activity != store.Idle || !s.Unseen || lead.Usage != (store.Usage{Context: 45200, Window: 200000, Cost: 0.31}) {
		t.Fatalf("lead idle: %+v %+v", lead, s)
	}
	s.Unseen = false
	Apply(&lead, &s, harness.Event{State: harness.Idle})
	if s.Unseen {
		t.Fatal("a lead that was already idle marked its session unseen")
	}
	Apply(&worker, &s, harness.Event{State: harness.Idle})
	if worker.Activity != store.Idle || s.Unseen {
		t.Fatalf("worker idle: %+v %+v", worker, s)
	}
}

func TestFailureGoesToTheLeadOfTheWorkersSession(t *testing.T) {
	db := open(t, []store.Session{{ID: "session-1"}, {ID: "other"}},
		store.Agent{ID: "l", Session: "session-1", Name: "lead", Role: store.Lead},
		store.Agent{ID: "w", Session: "session-1", Name: "worker", Role: store.Worker},
		store.Agent{ID: "other-lead", Session: "other", Name: "lead", Role: store.Lead})
	change(t, db, func(tx *store.Tx) error {
		w, _ := tx.Agent("w")
		l, _ := tx.Agent("l")
		if err := Failure(tx, w, " rate limited "); err != nil {
			return err
		}
		// A lead's own failure is for the user, who sees it in the transcript.
		return Failure(tx, l, "ignored")
	})
	m := queued(t, db, "l")
	if len(m) != 1 || m[0].From != "w" || m[0].Text != "My turn failed: rate limited" || len(queued(t, db, "other-lead")) != 0 {
		t.Fatalf("lead's messages = %+v", m)
	}
	if failure(" ") != "My turn failed." || !strings.HasSuffix(failure(strings.Repeat("é", 2500)), "…") {
		t.Error("failure text")
	}
}

func TestPromptReleasesAndTitles(t *testing.T) {
	titled := func(a store.Agent, s store.Session, text string) (store.Session, string) {
		a.ID = "x"
		Prompt(a, &s, text)
		return s, s.Title
	}
	idleLead := store.Agent{Role: store.Lead, Activity: store.Idle}
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
		if _, got := titled(idleLead, store.Session{AwaitingUser: true}, c.text); got != c.want {
			t.Errorf("%s: title = %q, want %q", c.name, got, c.want)
		}
	}
	s, _ := titled(idleLead, store.Session{AwaitingUser: true}, "first")
	if s.AwaitingUser || s.Title != "first" {
		t.Errorf("first prompt: %+v", s)
	}
	if got := func() string { s, _ := titled(idleLead, s, "second"); return s.Title }(); got != "first" {
		t.Errorf("a later prompt changed the title to %q", got)
	}
	if _, got := titled(store.Agent{Role: store.Lead, Activity: store.Busy}, store.Session{}, "busy"); got != "" {
		t.Errorf("a busy lead's prompt set title %q", got)
	}
	if _, got := titled(idleLead, store.Session{Title: "Mine"}, "other"); got != "Mine" {
		t.Errorf("a titled session's title became %q", got)
	}
	if s, got := titled(store.Agent{Role: store.Worker, Activity: store.Idle}, store.Session{AwaitingUser: true}, "worker prompt"); got != "" || s.AwaitingUser {
		t.Errorf("a worker's prompt: %+v", s)
	}
}
