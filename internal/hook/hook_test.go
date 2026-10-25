package hook

import (
	"bytes"
	"encoding/json"
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
			s.Messages = append(s.Messages, store.Message{ID: store.NewID(), From: "a", To: "b", Text: text})
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
	if s := load(); s.Agents[1].SessionID != "s1" {
		t.Errorf("session ID = %q", s.Agents[1].SessionID)
	}

	queue("hello")
	r := run(t, "UserPromptSubmit")
	if got := r.HookSpecificOutput.AdditionalContext; got != "Message from orchestrator (a):\nhello" {
		t.Errorf("UserPromptSubmit context = %q", got)
	}
	if s := load(); s.Agents[1].Activity != store.Busy || len(s.Messages) != 0 {
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
