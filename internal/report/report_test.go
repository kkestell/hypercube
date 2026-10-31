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
			{ID: "other-lead", Name: "lead", Role: store.Lead, Team: "other"},
			{ID: "other-worker", Name: "worker", Role: store.Worker, Team: "other", Activity: store.Busy},
		}
		s.Messages = []store.Message{{From: "o", To: "other-worker", Text: "kept"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got := run(t, "start", `{}`).Context
	if strings.Contains(got, "session-1/") || !strings.Contains(got, "other/lead") {
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
