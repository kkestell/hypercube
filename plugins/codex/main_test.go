package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLaunch(t *testing.T) {
	dir := t.TempDir()
	var env map[string]string
	run := func(input string) []string {
		t.Helper()
		var out bytes.Buffer
		if err := launch(strings.NewReader(input), &out); err != nil {
			t.Fatal(err)
		}
		var o struct {
			Argv []string
			Env  map[string]string
		}
		if err := json.Unmarshal(out.Bytes(), &o); err != nil {
			t.Fatal(err)
		}
		env = o.Env
		return o.Argv
	}
	config := func(argv []string, prefix string) string {
		for i, arg := range argv {
			if arg == "-c" && strings.HasPrefix(argv[i+1], prefix) {
				return argv[i+1]
			}
		}
		t.Fatalf("no -c %s in %q", prefix, argv)
		return ""
	}

	argv := run(`{"hypercube":"/bin/hc","env":{"HYPERCUBE_AGENT_ID":"a"},"agent":"a","model":"m","effort":"low","prompt":"go","state_dir":"` + dir + `"}`)
	if !slices.Equal(argv[1:6], []string{"run", "/bin/hc", "codex", "--model", "m"}) || !slices.Equal(argv[len(argv)-2:], []string{"--", "go"}) {
		t.Fatal(argv)
	}
	if b, err := os.ReadFile(env[sessionVar]); err != nil || len(b) != 0 {
		t.Fatal(env, err, string(b))
	}
	if got := config(argv, "model_reasoning_effort="); got != `model_reasoning_effort="low"` {
		t.Error(got)
	}
	if mcp := config(argv, "mcp_servers.hypercube="); !strings.Contains(mcp, `command="/bin/hc",args=["mcp"],env={"HYPERCUBE_AGENT_ID"="a"}`) {
		t.Error(mcp)
	}
	hooks := config(argv, "hooks=")
	for _, want := range []string{`'hook' '/bin/hc' 'SessionStart'"`, `"/<session-flags>/config.toml:stop:0:0"={trusted_hash="sha256:`} {
		if !strings.Contains(hooks, want) {
			t.Errorf("hooks lack %s: %s", want, hooks)
		}
	}

	argv = run(`{"hypercube":"/bin/hc","agent":"a","model":"m","effort":"low","session":"s1","state_dir":"` + dir + `"}`)
	if b, _ := os.ReadFile(env[sessionVar]); argv[4] != "resume" || string(b) != "s1" || !slices.Equal(argv[len(argv)-2:], []string{"--", "s1"}) {
		t.Fatal(argv)
	}
}

// The hash Codex 0.166.0 reports in hooks/list for this hook.
func TestHookHash(t *testing.T) {
	got := hookHash("permission_request", "/tmp/hcx/hook.sh PermissionRequest")
	if want := "sha256:9b0c153c8195b7d9e5339cf6d8ab8c59636529f99aed55a8f0a279738e58ab49"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestHook(t *testing.T) {
	dir := t.TempDir()
	hypercube := filepath.Join(dir, "hypercube")
	script := "#!/bin/sh\ncat >\"" + dir + "/$2\"\necho '{\"context\":\"queued\"}'\n"
	if err := os.WriteFile(hypercube, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessionVar, filepath.Join(dir, "session"))
	run := func(event, input string) string {
		t.Helper()
		var out bytes.Buffer
		if err := hook(hypercube, event, strings.NewReader(input), &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	if got := run("Stop", `{}`); !strings.Contains(got, `"decision":"block","reason":"queued"`) {
		t.Error(got)
	}
	if got := run("UserPromptSubmit", `{"session_id":"s1"}`); !strings.Contains(got, `"additionalContext":"queued"`) {
		t.Error(got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "prompt")); string(b) != `{"session":"s1"}` {
		t.Errorf("prompt report = %s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "session")); string(b) != "s1" {
		t.Errorf("session = %s", b)
	}
	if got := run("PostToolUse", `{"agent_id":"native"}`); got != "" {
		t.Error("a native subagent's event was reported", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "tool")); err == nil {
		t.Error("a native subagent's tool use was reported")
	}
}

func TestWaitLoop(t *testing.T) {
	dir := t.TempDir()
	// The stand-in prints messages once, then ends as a replaced waiter does.
	hypercube := filepath.Join(dir, "hypercube")
	script := "#!/bin/sh\nif [ -e \"" + dir + "/woken\" ]; then exit 0; fi\ntouch \"" + dir + "/woken\"\necho hello\nexit 2\n"
	if err := os.WriteFile(hypercube, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	codex := "#!/bin/sh\necho \"$@\" >>\"" + dir + "/codex.log\"\n"
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(codex), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	session := filepath.Join(dir, "session")
	t.Setenv(sessionVar, session)
	// A new session has no ID until its first prompt.
	if err := os.WriteFile(session, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(100*time.Millisecond, func() { os.WriteFile(session, []byte("s1"), 0o644) })
	if err := waitLoop(hypercube); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "codex.log")); string(b) != "queue --thread s1 --message hello\n" {
		t.Errorf("codex = %s", b)
	}
}
