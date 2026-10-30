package hook

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func run(t *testing.T, event string) response {
	t.Helper()
	var out bytes.Buffer
	if err := Run(event, strings.NewReader(`{"session_id":"s1"}`), &out); err != nil {
		t.Fatal(err)
	}
	var r response
	if out.Len() > 0 {
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func TestDelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "b")
	store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "a", Name: "orchestrator", Role: store.Orchestrator}, {ID: "b", Name: "w1", Role: store.Worker}}
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

	if r := run(t, "SessionStart"); !strings.Contains(r.HookSpecificOutput.AdditionalContext, "You are w1 (b)") {
		t.Errorf("SessionStart context = %q", r.HookSpecificOutput.AdditionalContext)
	}
	if s := load(); s.Agents[1].SessionID != "" {
		t.Errorf("SessionStart recorded session ID %q", s.Agents[1].SessionID)
	}

	queue("hello")
	r := run(t, "UserPromptSubmit")
	if got := r.HookSpecificOutput.AdditionalContext; got != "Message from orchestrator (a):\nhello" {
		t.Errorf("UserPromptSubmit context = %q", got)
	}
	if s := load(); s.Agents[1].Activity != store.Busy || len(s.Messages) != 0 || s.Agents[1].SessionID != "s1" {
		t.Errorf("after UserPromptSubmit: %+v", s)
	}

	queue("more")
	if r := run(t, "Stop"); r.Decision != "block" || !strings.Contains(r.Reason, "more") {
		t.Errorf("Stop with a message = %+v", r)
	}
	if s := load(); s.Agents[1].Activity != store.Busy {
		t.Errorf("blocked Stop marked agent %s", s.Agents[1].Activity)
	}
	if r := run(t, "Stop"); r.Decision != "" {
		t.Errorf("Stop without messages = %+v", r)
	}
	if s := load(); s.Agents[1].Activity != store.Idle {
		t.Errorf("Stop left agent %s", s.Agents[1].Activity)
	}
}

func TestPostToolUseDelivers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "b")
	store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "a", Name: "orchestrator", Role: store.Orchestrator}, {ID: "b", Name: "w1", Role: store.Worker, Activity: store.Busy}}
		s.Messages = []store.Message{{From: "a", To: "b", Text: "mid-turn"}, {From: "a", To: "a", Text: "other"}}
		return nil
	})
	r := run(t, "PostToolUse")
	if got := r.HookSpecificOutput.AdditionalContext; got != "Message from orchestrator (a):\nmid-turn" {
		t.Errorf("PostToolUse context = %q", got)
	}
	if s, _ := store.Load(path); len(s.Messages) != 1 || s.Messages[0].To != "a" {
		t.Errorf("messages left: %+v", s.Messages)
	}
	if r := run(t, "PostToolUse"); r.HookSpecificOutput != nil {
		t.Errorf("empty queue produced %+v", r)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestFailedResponseKeepsMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "b")
	store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "a", Name: "orchestrator", Role: store.Orchestrator}, {ID: "b", Name: "w1", Role: store.Worker}}
		s.Messages = []store.Message{{From: "a", To: "b", Text: "hello"}}
		return nil
	})
	if err := Run("PostToolUse", strings.NewReader(`{}`), failingWriter{}); err == nil {
		t.Fatal("Run succeeded with a failing writer")
	}
	if s, _ := store.Load(path); len(s.Messages) != 1 {
		t.Errorf("messages = %+v", s.Messages)
	}
}

func TestNativeSubagentsAndTurnFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "w")
	if err := store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "o", Name: "orchestrator", Role: store.Orchestrator}, {ID: "w", Name: "worker", Role: store.Worker, Activity: store.Busy, SessionID: "main"}}
		s.Messages = []store.Message{{From: "o", To: "w", Text: "main only"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "Stop", "StopFailure"} {
		var out bytes.Buffer
		if err := Run(event, strings.NewReader(`{"agent_id":"native","session_id":"sub","error":"sub failure"}`), &out); err != nil {
			t.Fatal(err)
		}
		s, _ := store.Load(path)
		if out.Len() != 0 || len(s.Messages) != 1 || s.Agents[1].SessionID != "main" || s.Agents[1].Activity != store.Busy {
			t.Fatalf("%s changed the main conversation: %+v", event, s)
		}
	}
	if err := Run("PermissionRequest", strings.NewReader(`{"agent_id":"native"}`), io.Discard); err != nil {
		t.Fatal(err)
	}
	s, _ := store.Load(path)
	if !s.Agents[1].Attention {
		t.Fatal("native subagent approval did not flag attention")
	}
	if err := Run("StopFailure", strings.NewReader(`{"error":"provider.api","error_details":" rate limited "}`), io.Discard); err != nil {
		t.Fatal(err)
	}
	s, _ = store.Load(path)
	if len(s.Messages) != 2 || s.Messages[0].Text != "main only" || s.Messages[1].To != "o" || s.Messages[1].Text != "My turn failed: rate limited" || s.Agents[1].Activity != store.Idle {
		t.Fatal(s)
	}
	r := run(t, "PostToolUse")
	if r.HookSpecificOutput == nil || !strings.Contains(r.HookSpecificOutput.AdditionalContext, "main only") {
		t.Fatal(r)
	}
}

func TestSessionContextAndFailureRouting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "other-worker")
	if err := store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{
			{ID: "o", Name: "orchestrator", Role: store.Orchestrator},
			{ID: "w", Name: "worker", Role: store.Worker},
			{ID: "other-orch", Name: "orchestrator", Role: store.Orchestrator, Team: "other"},
			{ID: "other-worker", Name: "worker", Role: store.Worker, Team: "other"},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got := run(t, "SessionStart").HookSpecificOutput.AdditionalContext
	if strings.Contains(got, "orchestrator (o)") || strings.Contains(got, "worker (w)") || !strings.Contains(got, "orchestrator (other-orch)") {
		t.Fatal("session context exposed the wrong team", got)
	}
	if err := Run("StopFailure", strings.NewReader(`{"error":"failure in other session"}`), io.Discard); err != nil {
		t.Fatal(err)
	}
	s, err := store.Load(path)
	if err != nil || len(s.Messages) != 1 || s.Messages[0].To != "other-orch" {
		t.Fatal(s, err)
	}
}
