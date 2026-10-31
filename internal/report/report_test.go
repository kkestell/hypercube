package report

import (
	"bytes"
	"encoding/json"
	"errors"
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
		s.Agents = []store.Agent{{ID: "a", Name: "lead", Role: store.Lead}, {ID: "b", Name: "w1", Role: store.Worker}}
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

	if r := run(t, "start", `{}`); !strings.Contains(r.Context, "You are default/w1") {
		t.Errorf("start context = %q", r.Context)
	}

	queue("hello")
	if r := run(t, "prompt", `{"session":"s1"}`); !strings.HasPrefix(r.Context, "Message from default/lead:\nhello\n") {
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
		s.Agents = []store.Agent{{ID: "a", Name: "lead", Role: store.Lead}, {ID: "b", Name: "w1", Role: store.Worker}}
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
			{ID: "o", Name: "lead", Role: store.Lead},
			{ID: "w", Name: "worker", Role: store.Worker},
			{ID: "other-lead", Name: "lead", Role: store.Lead, Team: "other"},
			{ID: "other-worker", Name: "worker", Role: store.Worker, Team: "other", Activity: store.Busy},
		}
		s.Messages = []store.Message{{From: "o", To: "other-worker", Text: "kept"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got := run(t, "start", `{}`).Context
	if strings.Contains(got, "default/") || !strings.Contains(got, "other/lead") {
		t.Fatal("session context exposed the wrong team", got)
	}
	run(t, "failed", `{"error":" rate limited "}`)
	s, err := store.Load(path)
	if err != nil || len(s.Messages) != 2 || s.Messages[0].Text != "kept" || s.Messages[1].To != "other-lead" || s.Messages[1].Text != "My turn failed: rate limited" || s.Agents[3].Activity != store.Idle {
		t.Fatal(s, err)
	}
}
