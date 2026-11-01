package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

// This process stands in for a harness plugin's launch. It starts the
// stand-in harness below, resuming the saved session.
func TestHarnessPlugin(t *testing.T) {
	if os.Getenv("HYPERCUBE_TEST_PLUGIN") != "1" {
		return
	}
	var in harness.LaunchInput
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		os.Exit(93)
	}
	argv := []string{os.Getenv("HYPERCUBE_TEST_BINARY"), "-test.run", "^TestNativeHarness$", "--"}
	if in.Conversation != "" {
		argv = append(argv, "--resume", in.Conversation)
	}
	if in.Prompt != "" {
		argv = append(argv, in.Prompt)
	}
	_ = json.NewEncoder(os.Stdout).Encode(harness.LaunchOutput{Argv: argv, Env: map[string]string{"HYPERCUBE_TEST_NATIVE": "1"}})
	os.Exit(0)
}

// This process stands in for a native harness and its plugin. Messages arrive
// through real hypercube reports and waiters, while terminal input remains in
// its editor.
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
	report := func(event string, input map[string]string) {
		b, _ := json.Marshal(input)
		cmd := exec.Command(binary, "report", event)
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
		cmd.Stdout = &out
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
	// Like the real harnesses, every prompt report carries the prompt's text,
	// including a wake-up's delivered messages.
	prompt := map[string]string{"conversation": id + "-conversation"}
	withText := func(text string) map[string]string {
		return map[string]string{"conversation": id + "-conversation", "text": text}
	}
	log("launch", "")
	report("start", nil)
	// A worker's initial turn consumes the queued brief; the lead waits.
	if slices.Contains(os.Args, session.Nudge) {
		report("prompt", prompt)
		report("stop", nil)
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
			report("prompt", withText(text))
			report("stop", nil)
			startWaiter()
		case text := <-input:
			log("input", text)
			switch text {
			case "exit":
				os.Exit(0)
			case "work":
				report("prompt", withText(text))
				log("working", "")
			case "tool":
				report("tool", nil)
			case "finish":
				report("stop", nil)
				startWaiter()
			case "fail":
				report("failed", map[string]string{"error": "test provider failure"})
				startWaiter()
			default:
				report("prompt", withText(text))
				report("stop", nil)
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
	logs := filepath.Join(dir, "logs")
	for _, d := range []string{logs, filepath.Join(dir, "config/hypercube")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\nexport HYPERCUBE_TEST_PLUGIN=1\nexec \"$HYPERCUBE_TEST_BINARY\" -test.run '^TestHarnessPlugin$' -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "hypercube-harness-test"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state with spaces"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("HYPERCUBE_TEST_EXE", binary)
	t.Setenv("HYPERCUBE_TEST_BINARY", testBinary)
	t.Setenv("HYPERCUBE_TEST_LOGS", logs)
	config := `{"profiles":[
		{"name":"first","role":"lead","description":"First lead","harness":"test","model":"haiku","effort":"low"},
		{"name":"second","role":"lead","description":"Second lead","harness":"test","model":"sol","effort":"high"},
		{"name":"worker","role":"work","description":"test","harness":"test","model":"p/m","effort":"high"}
	]}`
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
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	read := func(fn func(*store.Tx) error) {
		t.Helper()
		if err := db.Read(fn); err != nil {
			t.Fatal(err)
		}
	}
	agents := func(session string) (list []store.Agent) {
		t.Helper()
		read(func(tx *store.Tx) error { var err error; list, err = tx.Agents(session); return err })
		return list
	}
	agent := func(id string) (a store.Agent) {
		t.Helper()
		read(func(tx *store.Tx) error { var err error; a, err = tx.Agent(id); return err })
		return a
	}
	sessionOf := func(id string) (s store.Session) {
		t.Helper()
		read(func(tx *store.Tx) error { var err error; s, err = tx.Session(id); return err })
		return s
	}
	queued := func(id string) (m []store.Message) {
		t.Helper()
		read(func(tx *store.Tx) error { var err error; m, err = tx.Queued(id); return err })
		return m
	}
	queue := func(to, text string) {
		t.Helper()
		if err := db.Tx(func(tx *store.Tx) error { return tx.Queue(store.Message{From: "user", To: to, Text: text}) }); err != nil {
			t.Fatal(err)
		}
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
		out, _ := exec.Command("tmux", "list-panes", "-a", "-F", "#{session_name} #{pane_id} #{pane_dead} #{pane_dead_status} #{@hypercube_agent}").CombinedOutput()
		t.Logf("tmux panes:\n%s", out)
		list, _ := store.Snapshot(db)
		b, _ := json.MarshalIndent(list, "", "  ")
		t.Logf("database: %s", b)
		t.Fatalf("timed out waiting for %s", what)
	}
	logOf := func(id string) string { b, _ := os.ReadFile(filepath.Join(logs, id+".log")); return string(b) }
	launches := func(id string) int { return strings.Count(logOf(id), `"kind":"launch"`) }
	call := func(id, name string, args map[string]string) string {
		t.Helper()
		request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}}
		b, _ := json.Marshal(request)
		cmd := exec.Command(binary, "mcp")
		cmd.Env = append(os.Environ(), "HYPERCUBE_AGENT_ID="+id)
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

	// The view is the app process; the test drives it through its endpoints.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("http://localhost:%d", listener.Addr().(*net.TCPAddr).Port)
	port := fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()
	var view *exec.Cmd
	startView := func() {
		t.Helper()
		view = exec.Command(binary, "ui", port)
		if err := view.Start(); err != nil {
			t.Fatal(err)
		}
		until("view serving", func() bool {
			res, err := http.Get(base + "/leads")
			if err == nil {
				res.Body.Close()
			}
			return err == nil
		})
	}
	stopView := func() { _ = view.Process.Kill(); _ = view.Wait() }
	defer func() { stopView() }()
	post := func(path string, values map[string]string) (int, string) {
		t.Helper()
		q := url.Values{}
		for k, v := range values {
			q.Set(k, v)
		}
		req, _ := http.NewRequest("POST", base+path+"?"+q.Encode(), nil)
		req.Header.Set("Origin", base)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, strings.TrimSpace(string(b))
	}
	act := func(values map[string]string) string {
		t.Helper()
		code, body := post("/session", values)
		if code != http.StatusOK {
			t.Fatalf("session %v: %d %s", values, code, body)
		}
		return body
	}
	snapshot := func() []store.ShownWorkspace {
		t.Helper()
		conn, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(base, "http")+"/state", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.CloseNow()
		_, data, err := conn.Read(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var list []store.ShownWorkspace
		if err := json.Unmarshal(data, &list); err != nil {
			t.Fatal(err)
		}
		return list
	}
	paneSession := func(pane string) string { return tmux("display-message", "-p", "-t", pane, "#{session_name}") }
	ready := func(id string) func() bool {
		return func() bool { a := agent(id); return a.Waiter > 0 && a.Pane != "" }
	}

	if out, err := exec.Command(binary).CombinedOutput(); err == nil || !strings.Contains(string(out), "usage: hypercube") {
		t.Fatalf("no arguments should print the usage: %v %s", err, out)
	}
	startView()
	w, err := session.AddWorkspace(db, root)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := session.AddWorkspace(db, filepath.Join(root, ".")); err != nil || again.ID != w.ID {
		t.Fatalf("adding the folder again: %+v, %v", again, err)
	}
	if code, body := post("/session", map[string]string{"action": "new", "workspace": w.ID, "lead": "missing"}); code != http.StatusConflict || !strings.Contains(body, `no lead profile named "missing"`) {
		t.Fatalf("unknown lead: %d %s", code, body)
	}
	first := act(map[string]string{"action": "new", "workspace": w.ID, "lead": "first"})
	lead := agents(first)[0]
	until("lead waiter", ready(lead.ID))
	lead = agent(lead.ID)
	if lead.Model != "haiku" || sessionOf(first).AwaitingUser || paneSession(lead.Pane) != "hypercube-"+first {
		t.Fatalf("first session: %+v %+v", lead, sessionOf(first))
	}
	call(lead.ID, "create_agent", map[string]string{"name": "worker", "profile": "worker", "message": "first brief"})
	until("worker's first brief", func() bool {
		list := agents(first)
		return len(list) == 2 && strings.Contains(logOf(list[1].ID), "first brief") && list[1].Waiter > 0
	})
	worker := agents(first)[1]
	if _, err := os.Stat(filepath.Join(worker.Dir, ".git")); err != nil {
		t.Fatal("worker did not get a worktree", err)
	}
	// Each session has its own tmux session; names are scoped to sessions.
	second := act(map[string]string{"action": "new", "workspace": w.ID, "lead": "second"})
	otherLead := agents(second)[0]
	until("second lead waiter", ready(otherLead.ID))
	if otherLead.Model != "sol" {
		t.Fatalf("chosen lead model = %q", otherLead.Model)
	}
	call(otherLead.ID, "create_agent", map[string]string{"name": "worker", "profile": "worker", "message": "second session brief"})
	until("second session worker", func() bool {
		list := agents(second)
		return len(list) == 2 && list[1].Waiter > 0 && strings.Contains(logOf(list[1].ID), "second session brief")
	})
	otherLead, otherWorker := agent(otherLead.ID), agents(second)[1]
	if otherWorker.Dir == worker.Dir || otherWorker.Branch == worker.Branch {
		t.Fatal("sessions share a worker worktree or branch")
	}
	if paneSession(worker.Pane) != "hypercube-"+first || paneSession(otherWorker.Pane) != "hypercube-"+second {
		t.Fatal("team panes are not in their sessions' tmux sessions")
	}
	for _, pair := range [][2]store.Agent{{lead, otherWorker}, {otherLead, worker}} {
		if listed := call(pair[0].ID, "list_agents", nil); strings.Contains(listed, pair[1].Address()) {
			t.Fatal("list_agents exposed another session", listed)
		}
	}
	// The terminal client's status lines, pane titles, and key bindings are not
	// configured.
	for _, args := range [][]string{{"show-options", "-t", "=hypercube-" + first + ":"}, {"show-options", "-w", "-t", lead.Pane}} {
		got := tmux(args...)
		for _, option := range []string{"status", "mouse", "status-format", "pane-border-status", "pane-border-format", "window-status-format"} {
			if strings.Contains(got, option+" ") {
				t.Fatalf("%v sets %s:\n%s", args, option, got)
			}
		}
	}
	// Deleting a session protects a worker's dirty worktree.
	third := act(map[string]string{"action": "new", "workspace": w.ID, "lead": "first"})
	thirdLead := agents(third)[0]
	until("third lead", ready(thirdLead.ID))
	call(thirdLead.ID, "create_agent", map[string]string{"name": "worker", "profile": "worker"})
	until("third session worker", func() bool { list := agents(third); return len(list) == 2 && list[1].Waiter > 0 })
	thirdWorker := agents(third)[1]
	dirty := filepath.Join(thirdWorker.Dir, "unfinished")
	if err := os.WriteFile(dirty, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, body := post("/session", map[string]string{"action": "delete", "id": third}); code != http.StatusConflict || !strings.Contains(body, "uncommitted changes") {
		t.Fatalf("session deletion failed to protect dirty work: %d %s", code, body)
	}
	if len(agents(third)) != 2 || !tmuxRunning(third) {
		t.Fatal("refused deletion changed the session")
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	act(map[string]string{"action": "delete", "id": third})
	if len(agents(third)) != 0 || tmuxRunning(third) {
		t.Fatal("session deletion left its agents or tmux session")
	}
	if _, err := os.Stat(thirdWorker.Dir); !os.IsNotExist(err) {
		t.Fatal("deleted session's worktree remains", err)
	}
	if paneSession(worker.Pane) != "hypercube-"+first || paneSession(otherWorker.Pane) != "hypercube-"+second {
		t.Fatal("session deletion affected another session")
	}
	// Starting a running session changes nothing.
	act(map[string]string{"action": "start", "id": first})
	if launches(lead.ID) != 1 || launches(worker.ID) != 1 {
		t.Fatal("start relaunched a running session")
	}
	call(otherLead.ID, "send_message", map[string]string{"to": "worker", "text": "only second worker"})
	until("session-local name delivery", func() bool { return strings.Contains(logOf(otherWorker.ID), "only second worker") })
	if strings.Contains(logOf(worker.ID), "only second worker") {
		t.Fatal("message crossed sessions")
	}
	call(otherLead.ID, "send_message", map[string]string{"to": lead.Address(), "text": "from the other lead"})
	until("lead-to-lead delivery", func() bool { return strings.Contains(logOf(lead.ID), "from the other lead") })
	// A turn that delivered messages leaves a session untitled. The first
	// prompt typed while idle titles it, and later prompts leave the title.
	until("lead idle after wake-up", func() bool { return agent(lead.ID).Activity == store.Idle })
	if sessionOf(first).Title != "" {
		t.Fatal("a wake-up titled the session")
	}
	tmux("send-keys", "-t", lead.Pane, "-l", "Fix the parser")
	tmux("send-keys", "-t", lead.Pane, "Enter")
	until("first prompt titles the session", func() bool { return sessionOf(first).Title == "Fix the parser" })
	tmux("send-keys", "-t", lead.Pane, "-l", "Something else")
	tmux("send-keys", "-t", lead.Pane, "Enter")
	until("second prompt handled", func() bool { return strings.Contains(logOf(lead.ID), `"kind":"handled","text":"Something else"`) })
	if sessionOf(first).Title != "Fix the parser" {
		t.Fatal("a later prompt changed the title")
	}
	until("lead's reply unseen", func() bool { return sessionOf(first).Unseen })
	act(map[string]string{"action": "seen", "id": first})
	if sessionOf(first).Unseen {
		t.Fatal("seen left the reply unseen")
	}
	// The view's terminal window does not take new workers.
	tmux("new-window", "-d", "-t", "=hypercube-"+first+":")
	call(lead.ID, "create_agent", map[string]string{"name": "extra", "profile": "worker", "dir": "."})
	until("extra worker", func() bool { list := agents(first); return len(list) == 3 && list[2].Waiter > 0 })
	extra := agents(first)[2]
	if tmux("display-message", "-p", "-t", extra.Pane, "#{window_id}") != tmux("display-message", "-p", "-t", lead.Pane, "#{window_id}") {
		t.Fatal("worker creation used the terminal window")
	}
	call(lead.ID, "remove_agent", map[string]string{"name": extra.Name})
	tmux("send-keys", "-t", worker.Pane, "-l", "draft stays here")
	call(lead.ID, "send_message", map[string]string{"to": worker.Name, "text": "wake while drafting"})
	until("idle wake-up", func() bool { return strings.Contains(logOf(worker.ID), "wake while drafting") })
	tmux("send-keys", "-t", worker.Pane, "Enter")
	until("draft preserved", func() bool { return strings.Contains(logOf(worker.ID), `"kind":"handled","text":"draft stays here"`) })
	tmux("send-keys", "-t", worker.Pane, "work", "Enter")
	until("busy worker", func() bool {
		return agent(worker.ID).Activity == store.Busy && strings.Contains(logOf(worker.ID), `"kind":"working"`)
	})
	call(lead.ID, "send_message", map[string]string{"to": worker.Name, "text": "change course during turn"})
	tmux("send-keys", "-t", worker.Pane, "tool", "Enter")
	until("mid-turn steering", func() bool { return strings.Contains(logOf(worker.ID), "change course during turn") })
	tmux("send-keys", "-t", worker.Pane, "fail", "Enter")
	until("failure reaches lead", func() bool { return strings.Contains(logOf(lead.ID), "test provider failure") })
	if strings.Contains(logOf(otherLead.ID), "test provider failure") {
		t.Fatal("failure reached another session's lead")
	}
	tmux("send-keys", "-t", worker.Pane, "exit", "Enter")
	until("resume after exit", func() bool { return launches(worker.ID) == 2 })
	if agent(worker.ID).Pane != worker.Pane || !strings.Contains(logOf(worker.ID), worker.ID+"-conversation") {
		t.Fatal("relaunch lost the pane or conversation")
	}
	// A stale pane in another session must not close its worker.
	stranger := otherWorker.Pane
	if err := db.Tx(func(tx *store.Tx) error {
		a, err := tx.Agent(worker.ID)
		if err != nil {
			return err
		}
		a.Pane = stranger
		return tx.SaveAgent(a)
	}); err != nil {
		t.Fatal(err)
	}
	call(lead.ID, "remove_agent", map[string]string{"name": worker.Name})
	if got := tmux("display-message", "-p", "-t", stranger, "#{pane_id}"); got != stranger {
		t.Fatalf("unrelated pane was closed: %q", got)
	}
	if out, err := exec.Command("tmux", "display-message", "-p", "-t", worker.Pane, "#{pane_id}").CombinedOutput(); err == nil && strings.TrimSpace(string(out)) != "" {
		t.Fatalf("removed agent's real pane remains: %s", out)
	}
	if _, err := os.Stat(worker.Dir); !os.IsNotExist(err) {
		t.Fatal("worker worktree remains")
	}
	if len(agents(first)) != 1 {
		t.Fatal("removed worker remains")
	}

	// A stopped session stays stopped when the view restarts, and keeps its
	// conversations and queued messages.
	act(map[string]string{"action": "stop", "id": second})
	if tmuxRunning(second) || !tmuxRunning(first) {
		t.Fatal("stop closed the wrong tmux session")
	}
	until("view lists the session stopped", func() bool {
		for _, s := range snapshot()[0].Sessions {
			if s.Running != (s.ID == first) {
				return false
			}
		}
		return true
	})
	if code, _ := post("/session", map[string]string{"action": "bogus", "id": second}); code != http.StatusBadRequest {
		t.Fatal("an unknown action was accepted")
	}
	if _, res, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(base, "http")+"/tmux?session="+second, nil); err == nil || res == nil || res.StatusCode != http.StatusConflict {
		t.Fatalf("terminals for a stopped session: %v %v", res, err)
	}
	queue(otherLead.ID, "queued while stopped")
	queue(otherWorker.ID, "other queued while stopped")
	stopView()
	startView()
	time.Sleep(500 * time.Millisecond)
	if tmuxRunning(second) || launches(otherLead.ID) != 1 {
		t.Fatal("restarting the view started a stopped session")
	}
	// Start resumes the conversations and waits for the user's prompt.
	otherLead, otherWorker = agent(otherLead.ID), agent(otherWorker.ID)
	act(map[string]string{"action": "start", "id": second})
	until("started session waiting", func() bool {
		return launches(otherWorker.ID) == 2 && agent(otherLead.ID).Waiter > otherLead.Waiter && agent(otherWorker.ID).Waiter > otherWorker.Waiter
	})
	if !sessionOf(second).AwaitingUser || !strings.Contains(logOf(otherWorker.ID), "--resume\",\""+otherWorker.ID+"-conversation") {
		t.Fatal("started session did not resume and await the user")
	}
	for _, a := range agents(second) {
		var lastLaunch []string
		for _, line := range strings.Split(logOf(a.ID), "\n") {
			var entry struct {
				Kind string
				Argv []string
			}
			if json.Unmarshal([]byte(line), &entry) == nil && entry.Kind == "launch" {
				lastLaunch = entry.Argv
			}
		}
		if len(lastLaunch) == 0 || slices.Contains(lastLaunch, session.Nudge) {
			t.Fatalf("restarted launch for %s: %v", a.ID, lastLaunch)
		}
	}
	time.Sleep(time.Second)
	if strings.Contains(logOf(otherLead.ID), "queued while stopped") || len(queued(otherLead.ID)) != 1 || len(queued(otherWorker.ID)) != 1 {
		t.Fatal("a started session delivered messages before a user prompt")
	}
	otherLead = agent(otherLead.ID)
	tmux("send-keys", "-t", otherLead.Pane, "continue", "Enter")
	until("started session released", func() bool {
		return strings.Contains(logOf(otherLead.ID), "queued while stopped") && strings.Contains(logOf(otherWorker.ID), "other queued while stopped")
	})

	// At logout the view and tmux end; at login only the sessions the user
	// left running start again, waiting for the user.
	act(map[string]string{"action": "stop", "id": second})
	queue(lead.ID, "queued at logout")
	lead = agent(lead.ID)
	stopView()
	tmux("kill-server")
	startView()
	until("running session restored", func() bool { return launches(lead.ID) == 2 && agent(lead.ID).Waiter > lead.Waiter })
	if tmuxRunning(second) || !sessionOf(first).AwaitingUser {
		t.Fatal("login restored the wrong sessions")
	}
	time.Sleep(time.Second)
	if strings.Contains(logOf(lead.ID), "queued at logout") {
		t.Fatal("a restored session delivered messages before a user prompt")
	}
	lead = agent(lead.ID)
	tmux("send-keys", "-t", lead.Pane, "continue", "Enter")
	until("restored session released", func() bool { return strings.Contains(logOf(lead.ID), "queued at logout") })

	// A new title shows to agents; the address stays.
	act(map[string]string{"action": "rename", "id": first, "title": "#{session_name} -triage;"})
	act(map[string]string{"action": "start", "id": second})
	until("second session running", func() bool { return tmuxRunning(second) && agent(otherLead.ID).Pane != "" })
	if listed := call(otherLead.ID, "list_agents", nil); !strings.Contains(listed, lead.Address()) || !strings.Contains(listed, "#{session_name} -triage;") {
		t.Fatal("list_agents missed the new title", listed)
	}
	tmux("send-keys", "-t", lead.Pane, "-l", "Another line")
	tmux("send-keys", "-t", lead.Pane, "Enter")
	until("prompt after rename", func() bool { return strings.Contains(logOf(lead.ID), `"kind":"handled","text":"Another line"`) })
	if got := sessionOf(first).Title; got != "#{session_name} -triage;" {
		t.Fatalf("a prompt replaced the user's title: %q", got)
	}
	// Deleting the last session leaves the workspace; deleting the workspace
	// removes it and its worktrees.
	act(map[string]string{"action": "delete", "id": second})
	call(lead.ID, "create_agent", map[string]string{"name": "last", "profile": "worker"})
	until("last worker", func() bool { list := agents(first); return len(list) == 2 && list[1].Waiter > 0 })
	lastWorker := agents(first)[1]
	act(map[string]string{"action": "delete", "id": first})
	until("workspace without sessions", func() bool { list := snapshot(); return len(list) == 1 && len(list[0].Sessions) == 0 })
	if _, err := os.Stat(lastWorker.Dir); !os.IsNotExist(err) {
		t.Fatal("deleted session's worktree remains", err)
	}
	second = act(map[string]string{"action": "new", "workspace": w.ID, "lead": "first"})
	secondLead := agents(second)[0]
	until("new session lead", ready(secondLead.ID))
	call(secondLead.ID, "create_agent", map[string]string{"name": "w", "profile": "worker"})
	until("new session worker", func() bool { list := agents(second); return len(list) == 2 && list[1].Waiter > 0 })
	finalWorker := agents(second)[1]
	if code, body := post("/workspace", map[string]string{"action": "delete", "id": w.ID}); code != http.StatusOK {
		t.Fatalf("deleting the workspace: %d %s", code, body)
	}
	until("no workspaces", func() bool { return len(snapshot()) == 0 })
	if tmuxRunning(second) {
		t.Fatal("the deleted workspace's session runs")
	}
	if _, err := os.Stat(finalWorker.Dir); !os.IsNotExist(err) {
		t.Fatal("deleted workspace's worktree remains", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".hypercube")); !os.IsNotExist(err) {
		t.Fatal("session worktree directories remain", err)
	}
}

// tmuxRunning reports whether the session's tmux session exists.
func tmuxRunning(session string) bool {
	return exec.Command("tmux", "has-session", "-t", "=hypercube-"+session).Run() == nil
}
