package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// This process stands in for either native harness. Messages arrive through
// real hypercube hooks and waiters, while terminal input remains in its editor.
func TestNativeHarness(t *testing.T) {
	if os.Getenv("HYPERCUBE_TEST_NATIVE") != "1" {
		return
	}
	binary := os.Getenv("HYPERCUBE_TEST_EXE")
	id := os.Getenv("HYPERCUBE_AGENT_ID")
	log := func(kind, text string) {
		entry := map[string]any{"kind": kind, "text": text}
		if kind == "launch" {
			entry["argv"] = os.Args
		}
		b, _ := json.Marshal(entry)
		f, err := os.OpenFile(filepath.Join(os.Getenv("HYPERCUBE_TEST_LOGS"), id+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			os.Exit(90)
		}
		_, _ = f.Write(append(b, '\n'))
		_ = f.Close()
	}
	hook := func(event string, extra map[string]string) {
		input := map[string]string{"session_id": id + "-session"}
		for k, v := range extra {
			input[k] = v
		}
		b, _ := json.Marshal(input)
		cmd := exec.Command(binary, "hook", event)
		cmd.Stdin = bytes.NewReader(b)
		out, err := cmd.CombinedOutput()
		if err != nil {
			log("error", string(out))
			os.Exit(91)
		}
		log(event, string(out))
	}
	wake := make(chan string)
	startWaiter := func() {
		cmd := exec.Command(binary, "wait")
		var out bytes.Buffer
		cmd.Stderr = &out
		if err := cmd.Start(); err != nil {
			os.Exit(92)
		}
		go func() {
			err := cmd.Wait()
			if e, ok := err.(*exec.ExitError); ok && e.ExitCode() == 2 {
				wake <- out.String()
			}
		}()
	}
	log("launch", "")
	hook("SessionStart", nil)
	// A worker's initial turn consumes the queued brief; the orchestrator waits.
	if slices.Contains(os.Args, "Check your Hypercube messages.") {
		hook("UserPromptSubmit", nil)
		hook("Stop", nil)
	}
	startWaiter()
	input := make(chan string)
	go func() {
		scan := bufio.NewScanner(os.Stdin)
		for scan.Scan() {
			input <- scan.Text()
		}
	}()
	for {
		select {
		case text := <-wake:
			log("wake", text)
			hook("UserPromptSubmit", nil)
			hook("Stop", nil)
			startWaiter()
		case text := <-input:
			log("input", text)
			switch text {
			case "exit":
				os.Exit(0)
			case "work":
				hook("UserPromptSubmit", nil)
				log("working", "")
			case "tool":
				hook("PostToolUse", nil)
			case "finish":
				hook("Stop", nil)
				startWaiter()
			case "fail":
				hook("StopFailure", map[string]string{"error_details": "test provider failure"})
				startWaiter()
			default:
				hook("UserPromptSubmit", nil)
				hook("Stop", nil)
				startWaiter()
			}
			log("handled", text)
		}
	}
}

func TestTmuxNativeLifecycle(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Fatal("tmux is required")
	}
	socket, err := os.MkdirTemp("/tmp", "hc-tmux-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-server").Run() })
	dir := t.TempDir()
	binary := filepath.Join(dir, "hypercube")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	testBinary, _ := os.Executable()
	fake := filepath.Join(dir, "bin")
	logs := filepath.Join(dir, "logs")
	for _, d := range []string{fake, logs, filepath.Join(dir, "config/hypercube")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"claude", "opencode"} {
		script := "#!/bin/sh\nexport HYPERCUBE_TEST_NATIVE=1\nexec \"$HYPERCUBE_TEST_BINARY\" -test.run '^TestNativeHarness$' -- \"$@\"\n"
		if err := os.WriteFile(filepath.Join(fake, kind), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", fake+":"+os.Getenv("PATH"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("HYPERCUBE_TEST_EXE", binary)
	t.Setenv("HYPERCUBE_TEST_BINARY", testBinary)
	t.Setenv("HYPERCUBE_TEST_LOGS", logs)
	config := `{"orchestrator":{"harness":"claude","model":"haiku","effort":"low"},"profiles":[{"name":"worker","description":"test","harness":"opencode","model":"p/m","effort":"high"}]}`
	if err := os.WriteFile(filepath.Join(dir, "config/hypercube/config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "repo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "init")
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	path := store.Path(workspace.Name(root))
	cli := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("hypercube %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	until := func(what string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		entries, _ := os.ReadDir(logs)
		for _, entry := range entries {
			b, _ := os.ReadFile(filepath.Join(logs, entry.Name()))
			t.Logf("%s:\n%s", entry.Name(), b)
		}
		for _, args := range [][]string{{"list-panes", "-a", "-F", "#{pane_id} #{pane_dead} #{pane_dead_status} #{@hypercube_id}"}, {"show-hooks", "-t", "=" + workspace.Name(root) + ":"}} {
			out, _ := exec.Command("tmux", args...).CombinedOutput()
			t.Logf("tmux %v: %s", args, out)
		}
		s := stateForDiagnostics(path)
		t.Logf("state: %s", s)
		t.Fatalf("timed out waiting for %s", what)
	}
	state := func() store.State {
		s, err := store.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	logOf := func(id string) string { b, _ := os.ReadFile(filepath.Join(logs, id+".log")); return string(b) }
	call := func(id, name string, args map[string]string) string {
		t.Helper()
		request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}}
		b, _ := json.Marshal(request)
		cmd := exec.Command(binary, "mcp")
		cmd.Env = append(os.Environ(), "HYPERCUBE_AGENT_ID="+id, "HYPERCUBE_STATE="+path)
		cmd.Stdin = bytes.NewReader(append(b, '\n'))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("MCP: %v\n%s", err, out)
		}
		var response struct {
			Result struct {
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &response); err != nil || response.Result.IsError {
			t.Fatalf("MCP %s: %s (%v)", name, out, err)
		}
		return string(out)
	}
	cli("start", root)
	until("orchestrator waiter", func() bool {
		s := state()
		return len(s.Agents) == 1 && s.Agents[0].Waiter > 0 && s.Agents[0].Target != ""
	})
	orch := state().Agents[0]
	call(orch.ID, "create_agent", map[string]string{"name": "worker", "profile": "worker", "message": "first brief"})
	until("worker's first brief", func() bool {
		s := state()
		return len(s.Agents) == 2 && strings.Contains(logOf(s.Agents[1].ID), "first brief") && s.Agents[1].Waiter > 0
	})
	worker := state().Agents[1]
	if _, err := os.Stat(filepath.Join(worker.Dir, ".git")); err != nil {
		t.Fatal("worker did not get a worktree", err)
	}
	tmux("send-keys", "-t", worker.Target, "-l", "draft stays here")
	call(orch.ID, "send_message", map[string]string{"to": worker.ID, "text": "wake while drafting"})
	until("idle wake-up", func() bool { return strings.Contains(logOf(worker.ID), "wake while drafting") })
	tmux("send-keys", "-t", worker.Target, "Enter")
	until("draft preserved", func() bool { return strings.Contains(logOf(worker.ID), `"kind":"handled","text":"draft stays here"`) })
	tmux("send-keys", "-t", worker.Target, "work", "Enter")
	until("busy worker", func() bool {
		return state().Agents[1].Activity == store.Busy && strings.Contains(logOf(worker.ID), `"kind":"working"`)
	})
	call(orch.ID, "send_message", map[string]string{"to": worker.ID, "text": "change course during turn"})
	tmux("send-keys", "-t", worker.Target, "tool", "Enter")
	until("mid-turn steering", func() bool { return strings.Contains(logOf(worker.ID), "change course during turn") })
	tmux("send-keys", "-t", worker.Target, "fail", "Enter")
	until("failure reaches orchestrator", func() bool { return strings.Contains(logOf(orch.ID), "test provider failure") })
	tmux("send-keys", "-t", worker.Target, "exit", "Enter")
	until("resume after exit", func() bool { return strings.Count(logOf(worker.ID), `"kind":"launch"`) == 2 })
	if state().Agents[1].Target != worker.Target || !strings.Contains(logOf(worker.ID), worker.ID+"-session") {
		t.Fatal("relaunch lost the pane or session")
	}
	// A stale target must never kill a different pane.
	stranger := strings.TrimSpace(tmux("split-window", "-d", "-P", "-F", "#{pane_id}", "-t", orch.Target, "/bin/sleep", "30"))
	if err := store.Update(path, func(s *store.State) error { s.Agents[1].Target = stranger; return nil }); err != nil {
		t.Fatal(err)
	}
	call(orch.ID, "remove_agent", map[string]string{"id": worker.ID})
	if got := strings.TrimSpace(tmux("display-message", "-p", "-t", stranger, "#{pane_id}")); got != stranger {
		t.Fatalf("unrelated pane was closed: %q", got)
	}
	if out, err := exec.Command("tmux", "display-message", "-p", "-t", worker.Target, "#{pane_id}").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) != "" {
		t.Fatalf("removed agent's real pane remains: %s", out)
	}
	if _, err := os.Stat(worker.Dir); !os.IsNotExist(err) {
		t.Fatal("worker worktree remains")
	}
	if len(state().Agents) != 1 {
		t.Fatal("removed worker remains")
	}
	// Stop and restart retain the main conversation and its queued messages.
	cli("stop", root)
	if err := store.Update(path, func(s *store.State) error {
		s.Messages = append(s.Messages, store.Message{From: "user", To: orch.ID, Text: "queued while stopped"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cli("start", root)
	until("stopped queue delivered after resume", func() bool { return strings.Contains(logOf(orch.ID), "queued while stopped") })
	cli("stop", root)
	cli("delete", root)
	if got := cli("status"); strings.Contains(got, root) {
		t.Fatalf("deleted workspace listed: %s", got)
	}
}

func stateForDiagnostics(path string) string { b, _ := os.ReadFile(path); return string(b) }
