package hook

import (
	"bytes"
	"encoding/json"
	"os"
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

func TestOrchestratorModels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("HYPERCUBE_AGENT_ID", "a")
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "hypercube"), 0o755)
	os.WriteFile(filepath.Join(dir, "hypercube", "config.json"), []byte(`{
  "orchestrator": {"harness": "claude", "model": "opus", "effort": "high"},
  "worker": {"harness": "codex", "model": "gpt-6.1-sol", "effort": "medium"},
  "models": "- harness codex, model gpt-6.1-sol, effort medium: coding.\n"
}`), 0o644)
	store.Update(path, func(s *store.State) error {
		s.Agents = []store.Agent{{ID: "a", Name: "orchestrator", Role: store.Orchestrator}}
		return nil
	})

	got := run(t, "SessionStart").HookSpecificOutput.AdditionalContext
	if !strings.Contains(got, "\n- harness codex, model gpt-6.1-sol, effort medium: coding.\n") {
		t.Errorf("SessionStart context = %q", got)
	}
}

func TestNoGitFacts(t *testing.T) {
	dir := t.TempDir()
	orchDir, workerDir := t.TempDir(), t.TempDir()
	path := filepath.Join(dir, "w.json")
	t.Setenv("HYPERCUBE_STATE", path)
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "hypercube"), 0o755)
	os.WriteFile(filepath.Join(dir, "hypercube", "config.json"), []byte(`{
  "orchestrator": {"harness": "claude", "model": "opus", "effort": "high"},
  "worker": {"harness": "codex", "model": "gpt-6.1-sol", "effort": "medium"},
  "models": "- harness codex, model gpt-6.1-sol, effort medium: coding.\n"
}`), 0o644)
	store.Update(path, func(s *store.State) error {
		s.NoGit = true
		s.Agents = []store.Agent{
			{ID: "a", Name: "orchestrator", Role: store.Orchestrator, Dir: orchDir},
			{ID: "b", Name: "w1", Role: store.Worker, Dir: workerDir},
		}
		return nil
	})

	t.Setenv("HYPERCUBE_AGENT_ID", "a")
	got := run(t, "SessionStart").HookSpecificOutput.AdditionalContext
	for _, want := range []string{"create_agent(name, dir,", "Tell each worker how to handle branches and commits in its directory."} {
		if !strings.Contains(got, want) {
			t.Errorf("orchestrator context lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"on branch", "Merge a worker's branch"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("orchestrator context contains %q:\n%s", unwanted, got)
		}
	}

	t.Setenv("HYPERCUBE_AGENT_ID", "b")
	got = run(t, "SessionStart").HookSpecificOutput.AdditionalContext
	for _, want := range []string{"Your working directory is " + workerDir + ".", "Handle branches and commits as the orchestrator tells you"} {
		if !strings.Contains(got, want) {
			t.Errorf("worker context lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "on branch") {
		t.Errorf("worker context contains \"on branch\":\n%s", got)
	}
}
