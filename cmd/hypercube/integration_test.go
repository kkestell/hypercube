package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
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
	// A worker's initial turn consumes the queued brief; the lead waits.
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
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state with spaces"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("HYPERCUBE_TEST_EXE", binary)
	t.Setenv("HYPERCUBE_TEST_BINARY", testBinary)
	t.Setenv("HYPERCUBE_TEST_LOGS", logs)
	config := `{"lead":{"harness":"claude","model":"haiku","effort":"low"},"profiles":[{"name":"worker","description":"test","harness":"opencode","model":"p/m","effort":"high"}]}`
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
		cmd.Dir = root
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
	until("lead waiter", func() bool {
		s := state()
		return len(s.Agents) == 1 && s.Agents[0].Waiter > 0 && s.Agents[0].Target != ""
	})
	lead := state().Agents[0]
	call(lead.ID, "create_agent", map[string]string{"name": "worker", "profile": "worker", "message": "first brief"})
	until("worker's first brief", func() bool {
		s := state()
		return len(s.Agents) == 2 && strings.Contains(logOf(s.Agents[1].ID), "first brief") && s.Agents[1].Waiter > 0
	})
	worker := state().Agents[1]
	if _, err := os.Stat(filepath.Join(worker.Dir, ".git")); err != nil {
		t.Fatal("worker did not get a worktree", err)
	}
	// Independent teams share a tmux session, with names scoped to windows.
	cli("new", "-d", "review")
	until("second lead waiter", func() bool {
		s := state()
		return len(s.Agents) == 3 && s.Agents[2].Waiter > 0
	})
	otherLead := state().Agents[2]
	call(otherLead.ID, "create_agent", map[string]string{"name": "worker", "profile": "worker", "message": "second session brief"})
	until("second session worker", func() bool {
		s := state()
		return len(s.Agents) == 4 && s.Agents[3].Waiter > 0 && strings.Contains(logOf(s.Agents[3].ID), "second session brief")
	})
	otherWorker := state().Agents[3]
	if otherWorker.Dir == worker.Dir || otherWorker.Branch == worker.Branch {
		t.Fatal("sessions share a worker worktree or branch")
	}
	windowOf := func(pane string) string {
		return strings.TrimSpace(tmux("display-message", "-p", "-t", pane, "#{window_id}"))
	}
	firstWindow, otherWindow := windowOf(lead.Target), windowOf(otherLead.Target)
	if firstWindow == otherWindow || windowOf(worker.Target) != firstWindow || windowOf(otherWorker.Target) != otherWindow {
		t.Fatal("team panes are not in separate windows")
	}
	if got := strings.Fields(tmux("list-sessions", "-F", "#{session_name}")); len(got) != 1 {
		t.Fatalf("expected one workspace tmux session, got %v", got)
	}
	if got := strings.Fields(tmux("list-windows", "-t", "="+workspace.Name(root), "-F", "#{window_name}")); !slices.Equal(got, []string{"default", "review"}) {
		t.Fatalf("window switcher names: %v", got)
	}
	for _, pair := range [][2]store.Agent{{lead, otherWorker}, {otherLead, worker}} {
		if listed := call(pair[0].ID, "list_agents", nil); strings.Contains(listed, pair[1].Address()) {
			t.Fatal("list_agents exposed another session", listed)
		}
	}
	// Send real prefix keys through an attached tmux client. No shell or text
	// is sent into a harness pane.
	attachClient := func() string {
		client := exec.Command("tmux", "-C", "attach-session", "-t", "="+workspace.Name(root))
		input, err := client.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		client.Stdout, client.Stderr = io.Discard, io.Discard
		if err := client.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = input.Close(); _ = client.Process.Kill(); _ = client.Wait() })
		name := ""
		until("tmux client", func() bool {
			name = strings.TrimSpace(tmux("list-clients", "-F", "#{client_name}"))
			return name != ""
		})
		return name
	}
	clientName := attachClient()
	tmux("select-window", "-t", otherWindow)
	tmux("select-pane", "-t", otherWorker.Target)
	tmux("send-keys", "-K", "-c", clientName, "C-b", "c")
	until("new-session shortcut", func() bool { return len(state().Agents) == 5 && state().Agents[4].Waiter > 0 })
	shortcutLead := state().Agents[4]
	shortcutWindow := windowOf(shortcutLead.Target)
	if shortcutLead.Dir != root || shortcutWindow == otherWindow {
		t.Fatal("shortcut did not create a new session in the active workspace")
	}
	tmux("send-keys", "-K", "-c", clientName, "C-b", "X")
	tmux("send-keys", "-K", "-c", clientName, "n")
	if len(state().Agents) != 5 {
		t.Fatal("cancelled deletion changed the team")
	}
	call(shortcutLead.ID, "create_agent", map[string]string{"name": "worker", "profile": "worker"})
	until("shortcut session worker", func() bool { return len(state().Agents) == 6 && state().Agents[5].Waiter > 0 })
	shortcutWorker := state().Agents[5]
	dirty := filepath.Join(shortcutWorker.Dir, "unfinished")
	if err := os.WriteFile(dirty, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	refuse := exec.Command(binary, "delete-session", shortcutLead.Team)
	refuse.Dir = root
	if out, err := refuse.CombinedOutput(); err == nil || !strings.Contains(string(out), "uncommitted changes") {
		t.Fatalf("session deletion failed to protect dirty work: %v %s", err, out)
	}
	if windowOf(shortcutLead.Target) != shortcutWindow || len(state().Agents) != 6 {
		t.Fatal("refused deletion changed the team")
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	tmux("send-keys", "-K", "-c", clientName, "C-b", "X")
	tmux("send-keys", "-K", "-c", clientName, "y")
	until("delete-session shortcut", func() bool { return len(state().Agents) == 4 })
	if _, err := os.Stat(shortcutWorker.Dir); !os.IsNotExist(err) {
		t.Fatal("deleted session's worktree remains", err)
	}
	if windowOf(worker.Target) != firstWindow || windowOf(otherWorker.Target) != otherWindow {
		t.Fatal("delete shortcut affected another session")
	}
	cli("start", root)
	if len(state().Agents) != 4 {
		t.Fatal("deleted session returned after start")
	}
	call(otherLead.ID, "send_message", map[string]string{"to": "worker", "text": "only second worker"})
	until("session-local name delivery", func() bool { return strings.Contains(logOf(otherWorker.ID), "only second worker") })
	if strings.Contains(logOf(worker.ID), "only second worker") {
		t.Fatal("message crossed sessions")
	}
	call(otherLead.ID, "send_message", map[string]string{"to": lead.Address(), "text": "from the other lead"})
	until("lead-to-lead delivery", func() bool { return strings.Contains(logOf(lead.ID), "from the other lead") })
	// Selection, user renaming, and window indexes must not affect routing.
	tmux("select-window", "-t", otherWindow)
	tmux("rename-window", "-t", firstWindow, "renamed")
	call(lead.ID, "create_agent", map[string]string{"name": "extra", "profile": "worker", "dir": "."})
	until("extra worker", func() bool { return len(state().Agents) == 5 && state().Agents[4].Waiter > 0 })
	extra := state().Agents[4]
	if windowOf(extra.Target) != firstWindow {
		t.Fatal("worker creation used the selected window")
	}
	call(lead.ID, "remove_agent", map[string]string{"name": extra.Name})
	cli("start", root)
	if strings.Count(logOf(lead.ID), `"kind":"launch"`) != 1 || strings.Count(logOf(otherLead.ID), `"kind":"launch"`) != 1 {
		t.Fatal("reattach relaunched an existing session")
	}
	if got := strings.TrimSpace(tmux("display-message", "-p", "-t", "="+workspace.Name(root)+":", "#{window_id}")); got != otherWindow {
		t.Fatal("reattach changed the selected window", got)
	}
	tmux("send-keys", "-t", worker.Target, "-l", "draft stays here")
	call(lead.ID, "send_message", map[string]string{"to": worker.Name, "text": "wake while drafting"})
	until("idle wake-up", func() bool { return strings.Contains(logOf(worker.ID), "wake while drafting") })
	tmux("send-keys", "-t", worker.Target, "Enter")
	until("draft preserved", func() bool { return strings.Contains(logOf(worker.ID), `"kind":"handled","text":"draft stays here"`) })
	tmux("send-keys", "-t", worker.Target, "work", "Enter")
	until("busy worker", func() bool {
		return state().Agents[1].Activity == store.Busy && strings.Contains(logOf(worker.ID), `"kind":"working"`)
	})
	call(lead.ID, "send_message", map[string]string{"to": worker.Name, "text": "change course during turn"})
	tmux("send-keys", "-t", worker.Target, "tool", "Enter")
	until("mid-turn steering", func() bool { return strings.Contains(logOf(worker.ID), "change course during turn") })
	tmux("send-keys", "-t", worker.Target, "fail", "Enter")
	until("failure reaches lead", func() bool { return strings.Contains(logOf(lead.ID), "test provider failure") })
	if strings.Contains(logOf(otherLead.ID), "test provider failure") {
		t.Fatal("failure reached another session's lead")
	}
	tmux("send-keys", "-t", worker.Target, "exit", "Enter")
	until("resume after exit", func() bool { return strings.Count(logOf(worker.ID), `"kind":"launch"`) == 2 })
	if state().Agents[1].Target != worker.Target || !strings.Contains(logOf(worker.ID), worker.ID+"-session") {
		t.Fatal("relaunch lost the pane or session")
	}
	// A stale target in another team's window must not close its worker.
	stranger := otherWorker.Target
	if err := store.Update(path, func(s *store.State) error { s.Agents[1].Target = stranger; return nil }); err != nil {
		t.Fatal(err)
	}
	call(lead.ID, "remove_agent", map[string]string{"name": worker.Name})
	if got := strings.TrimSpace(tmux("display-message", "-p", "-t", stranger, "#{pane_id}")); got != stranger {
		t.Fatalf("unrelated pane was closed: %q", got)
	}
	if out, err := exec.Command("tmux", "display-message", "-p", "-t", worker.Target, "#{pane_id}").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) != "" {
		t.Fatalf("removed agent's real pane remains: %s", out)
	}
	if _, err := os.Stat(worker.Dir); !os.IsNotExist(err) {
		t.Fatal("worker worktree remains")
	}
	if len(state().Agents) != 3 {
		t.Fatal("removed worker remains")
	}
	// Closing one window and starting again resumes only that team's agents.
	tmux("kill-window", "-t", otherWindow)
	cli("start", root)
	until("closed window resumed", func() bool {
		return strings.Count(logOf(otherWorker.ID), `"kind":"launch"`) == 2
	})
	if strings.Count(logOf(lead.ID), `"kind":"launch"`) != 1 || !strings.Contains(logOf(otherWorker.ID), otherWorker.ID+"-session") {
		t.Fatal("window resume restarted the wrong team or lost its session")
	}
	// Stop and restart retain the main conversation and its queued messages.
	cli("stop", root)
	if err := store.Update(path, func(s *store.State) error {
		s.Messages = append(s.Messages, store.Message{From: "user", To: lead.ID, Text: "queued while stopped"})
		s.Messages = append(s.Messages, store.Message{From: "user", To: otherWorker.ID, Text: "other queued while stopped"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cli("start", root)
	until("stopped queue delivered after resume", func() bool { return strings.Contains(logOf(lead.ID), "queued while stopped") })
	until("second stopped queue delivered after resume", func() bool { return strings.Contains(logOf(otherWorker.ID), "other queued while stopped") })
	if strings.Contains(logOf(lead.ID), "other queued while stopped") || strings.Contains(logOf(otherWorker.ID), `Message from user:\nqueued while stopped`) {
		t.Fatal("resumed queues crossed sessions")
	}
	// Deleting the final window must finish cleanup even as tmux exits.
	cli("delete-session", otherLead.Team)
	call(lead.ID, "create_agent", map[string]string{"name": "last", "profile": "worker"})
	until("last worker", func() bool { return len(state().Agents) == 2 && state().Agents[1].Waiter > 0 })
	lastWorker := state().Agents[1]
	clientName = attachClient()
	tmux("send-keys", "-K", "-c", clientName, "C-b", "X")
	tmux("send-keys", "-K", "-c", clientName, "y")
	until("last session deleted", func() bool {
		_, stateErr := os.Stat(path)
		_, worktreeErr := os.Stat(lastWorker.Dir)
		return os.IsNotExist(stateErr) && os.IsNotExist(worktreeErr)
	})
	if got := cli("status"); strings.Contains(got, root) {
		t.Fatalf("deleted workspace listed: %s", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".hypercube")); !os.IsNotExist(err) {
		t.Fatal("session worktree directories remain", err)
	}
}

func stateForDiagnostics(path string) string { b, _ := os.ReadFile(path); return string(b) }
