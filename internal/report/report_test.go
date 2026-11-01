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
	path := filepath.Join(t.TempDir(), "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "a")
	if err := store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "a", Name: "lead", Role: store.Lead, Team: "session-1"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
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
	path := filepath.Join(t.TempDir(), "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "b")
	store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "a", Name: "lead", Role: store.Lead, Team: "session-1"}, {ID: "b", Name: "w1", Role: store.Worker, Team: "session-1"}}
		return nil
	})
	queue := func(text string) {
		store.Update(path, func(s *store.State) error {
			s.Messages = append(s.Messages, store.Message{From: "a", To: "b", Text: text})
			return nil
		})
	}
	load := func() store.State {
		s, _ := store.Load(path)
		return s
	}

	if r := run(t, "start", `{}`); !strings.Contains(r.Context, "You are session-1/w1") {
		t.Errorf("start context = %q", r.Context)
	}

	queue("hello")
	if r := run(t, "prompt", `{"session":"s1"}`); !strings.HasPrefix(r.Context, "Message from session-1/lead:\nhello\n") {
		t.Errorf("prompt context = %q", r.Context)
	}
	if s := load(); s.Agents[1].Activity != store.Busy || len(s.Messages) != 0 || s.Agents[1].SessionID != "s1" {
		t.Errorf("after prompt: %+v", s)
	}

	queue("mid-turn")
	if r := run(t, "tool", `{}`); !strings.Contains(r.Context, "mid-turn") {
		t.Errorf("tool context = %q", r.Context)
	}
	if r := run(t, "tool", `{}`); r.Context != "" {
		t.Errorf("empty queue produced %+v", r)
	}

	queue("more")
	if r := run(t, "stop", `{}`); !strings.Contains(r.Context, "more") {
		t.Errorf("stop with a message = %+v", r)
	}
	if s := load(); s.Agents[1].Activity != store.Busy {
		t.Errorf("continued stop marked agent %s", s.Agents[1].Activity)
	}
	if r := run(t, "stop", `{}`); r.Context != "" {
		t.Errorf("stop without messages = %+v", r)
	}
	if s := load(); s.Agents[1].Activity != store.Idle {
		t.Errorf("stop left agent %s", s.Agents[1].Activity)
	}

	run(t, "approval", `{}`)
	if s := load(); !s.Agents[1].Attention {
		t.Error("approval did not flag attention")
	}
	run(t, "answered", `{}`)
	if s := load(); s.Agents[1].Attention {
		t.Error("answered approval is still flagged")
	}

	run(t, "usage", `{"context":45200,"window":200000,"cost":0.31}`)
	if u := load().Agents[1].Usage; u != (store.Usage{Context: 45200, Window: 200000, Cost: 0.31}) {
		t.Errorf("usage = %+v", u)
	}
}

func TestWorkerPromptReleasesOnlyItsSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "w")
	if err := store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{
			{ID: "l", Role: store.Lead, Team: "t", AwaitingUser: true},
			{ID: "w", Role: store.Worker, Team: "t"},
			{ID: "other", Role: store.Lead, Team: "other", AwaitingUser: true},
		}
		s.Messages = []store.Message{{From: "user", To: "w", Text: "queued"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := run(t, "prompt", `{"text":"continue"}`).Context; !strings.Contains(got, "queued") {
		t.Fatalf("prompt context = %q", got)
	}
	s, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.AwaitingUser("t") || !s.AwaitingUser("other") || len(s.Messages) != 0 {
		t.Fatalf("state after worker prompt: %+v", s)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestFailedResponseKeepsMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "b")
	store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "a", Name: "lead", Role: store.Lead, Team: "session-1"}, {ID: "b", Name: "w1", Role: store.Worker, Team: "session-1"}}
		s.Messages = []store.Message{{From: "a", To: "b", Text: "hello"}}
		return nil
	})
	if err := Run("tool", strings.NewReader(`{}`), failingWriter{}); err == nil {
		t.Fatal("Run succeeded with a failing writer")
	}
	if s, _ := store.Load(path); len(s.Messages) != 1 {
		t.Errorf("messages = %+v", s.Messages)
	}
}

func TestSessionContextAndFailureRouting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "other-worker")
	if err := store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{
			{ID: "o", Name: "lead", Role: store.Lead, Team: "session-1"},
			{ID: "w", Name: "worker", Role: store.Worker, Team: "session-1"},
			{ID: "other-lead", Name: "lead", Role: store.Lead, Team: "other", Title: "Fix the parser"},
			{ID: "other-worker", Name: "worker", Role: store.Worker, Team: "other", Activity: store.Busy},
		}
		s.Messages = []store.Message{{From: "o", To: "other-worker", Text: "kept"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got := run(t, "start", `{}`).Context
	if strings.Contains(got, "session-1/") || !strings.Contains(got, "other/lead") || !strings.Contains(got, `"other", titled "Fix the parser"`) {
		t.Fatal("session context exposed the wrong team", got)
	}
	run(t, "failed", `{"error":" rate limited "}`)
	s, err := store.Load(path)
	if err != nil || len(s.Messages) != 2 || s.Messages[0].Text != "kept" || s.Messages[1].To != "other-lead" || s.Messages[1].Text != "My turn failed: rate limited" || s.Agents[3].Activity != store.Idle {
		t.Fatal(s, err)
	}
}

func TestToolLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "a")
	store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "a", Name: "lead", Role: store.Lead, Team: "session-1", Harness: "claude", Model: "opus"}}
		return nil
	})
	run(t, "toolfailed", `{"tool":"mcp__hypercube__send_message","input":{"to":"nobody"},"error":"no agent"}`)
	run(t, "denied", `{"tool":"Bash","input":{"command":"rm -rf /"},"reason":"destructive"}`)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(path), "tools.log"))
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
	if failed["msg"] != "tool failed" || failed["error"] != "no agent" || failed["session"] != "session-1" || failed["agent"] != "lead" || failed["model"] != "opus" {
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
	path := filepath.Join(t.TempDir(), "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	agents := func(lead store.Agent) {
		t.Helper()
		lead.ID, lead.Name, lead.Role, lead.Team = "l", "lead", store.Lead, "t"
		err := store.Update(path, func(s *store.State) error {
			s.Agents = []store.Agent{lead, {ID: "w", Name: "worker", Role: store.Worker, Team: "t"}}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	prompt := func(id, text string) string {
		t.Helper()
		t.Setenv("HYPERCUBE_AGENT_ID", id)
		b, _ := json.Marshal(map[string]string{"text": text})
		run(t, "prompt", string(b))
		s, err := store.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return s.Agents[0].Title
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
		agents(store.Agent{})
		if got := prompt("l", c.text); got != c.want {
			t.Errorf("%s: title = %q, want %q", c.name, got, c.want)
		}
	}

	agents(store.Agent{})
	prompt("l", "first")
	if got := prompt("l", "second"); got != "first" {
		t.Errorf("a later prompt changed the title to %q", got)
	}
	agents(store.Agent{Activity: store.Busy})
	if got := prompt("l", "busy"); got != "" {
		t.Errorf("a busy lead's prompt set title %q", got)
	}
	agents(store.Agent{Title: "Mine"})
	if got := prompt("l", "other"); got != "Mine" {
		t.Errorf("a titled lead's title became %q", got)
	}
	agents(store.Agent{})
	prompt("w", "worker prompt")
	if s, _ := store.Load(path); s.Agents[0].Title != "" || s.Agents[1].Title != "" {
		t.Errorf("a worker's prompt set a title: %+v", s.Agents)
	}
}
