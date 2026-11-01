// Command hypercube-harness-codex runs the Codex CLI as a Hypercube agent.
// Hypercube runs `launch`, which prints a `run` command. `run` starts Codex
// and waits for messages while Codex is idle; Codex runs `hook` through the
// hooks that launch configures.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

func main() {
	args := os.Args[1:]
	var err error
	switch {
	case len(args) == 1 && args[0] == "launch":
		err = launch(os.Stdin, os.Stdout)
	case len(args) == 3 && args[0] == "hook":
		err = hook(args[1], args[2], os.Stdin, os.Stdout)
	case len(args) > 2 && args[0] == "run":
		err = run(args[1], args[2:])
	default:
		err = fmt.Errorf("usage: hypercube-harness-codex [launch | hook <hypercube> <event> | run <hypercube> <codex argv>]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-harness-codex:", err)
		os.Exit(1)
	}
}

// sessionVar names the file that holds the ID of the agent's Codex session.
// Codex saves a new session from its first prompt.
const sessionVar = "HYPERCUBE_CODEX_SESSION"

// Codex hook events, the Hypercube reports they make, and the keys Codex uses
// for them in hook trust.
var events = []struct{ name, report, key string }{
	{"SessionStart", "start", "session_start"},
	{"UserPromptSubmit", "prompt", "user_prompt_submit"},
	{"PostToolUse", "tool", "post_tool_use"},
	{"Stop", "stop", "stop"},
}

func launch(stdin io.Reader, stdout io.Writer) error {
	var in harness.LaunchInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	command := func(args ...string) string {
		quoted := []string{plugin.ShellQuote(self)}
		for _, arg := range args {
			quoted = append(quoted, plugin.ShellQuote(arg))
		}
		return strings.Join(quoted, " ")
	}
	var hooks, trust []string
	for _, e := range events {
		cmd := command("hook", in.Hypercube, e.name)
		hooks = append(hooks, fmt.Sprintf("%s=[{hooks=[{type=\"command\",command=%s}]}]", e.name, jsonString(cmd)))
		key := fmt.Sprintf("/<session-flags>/config.toml:%s:0:0", e.key)
		trust = append(trust, fmt.Sprintf("%s={trusted_hash=%s}", jsonString(key), jsonString(hookHash(e.key, cmd))))
	}
	var env, inherited []string
	for k, v := range in.Env {
		env = append(env, jsonString(k)+"="+jsonString(v))
	}
	// Codex starts MCP servers with a few variables of its own environment;
	// hypercube mcp needs the rest, such as its configuration and tmux paths.
	for _, item := range os.Environ() {
		if k, _, _ := strings.Cut(item, "="); k != "" {
			inherited = append(inherited, jsonString(k))
		}
	}
	argv := []string{self, "run", in.Hypercube, "codex"}
	if in.Conversation != "" {
		argv = append(argv, "resume")
	}
	argv = append(argv, "--model", in.Model, "-c", "model_reasoning_effort="+jsonString(in.Effort),
		"--approve-for-me", "-c", "sandbox_workspace_write.network_access=true",
		"-c", fmt.Sprintf("mcp_servers.hypercube={command=%s,args=[\"mcp\"],env={%s},env_vars=[%s],default_tools_approval_mode=\"approve\"}", jsonString(in.Hypercube), strings.Join(env, ","), strings.Join(inherited, ",")),
		"-c", fmt.Sprintf("hooks={%s,state={%s}}", strings.Join(hooks, ","), strings.Join(trust, ",")))
	// The sandbox keeps Git metadata read-only unless it is a writable root
	// itself. A worktree's metadata is in its own Git directory and in its
	// parent checkout's.
	if out, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir", "--git-dir").Output(); err == nil {
		dirs := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i, dir := range dirs {
			if i == 0 || dir != dirs[0] {
				argv = append(argv, "--add-dir", dir)
			}
		}
	}
	argv = append(argv, "--")
	if in.Conversation != "" {
		argv = append(argv, in.Conversation)
	}
	if in.Prompt != "" {
		argv = append(argv, in.Prompt)
	}
	session := filepath.Join(in.StateDir, "codex", in.Agent+".session")
	if err := os.MkdirAll(filepath.Dir(session), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(session, []byte(in.Conversation), 0o644); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(map[string]any{"argv": argv, "env": map[string]string{sessionVar: session}})
}

// hookHash is the hash Codex records when a user trusts a command hook with
// default settings in a matcher group of its own, so the hooks launch
// configures run without a trust prompt or an edit to the user's settings.
func hookHash(event, command string) string {
	identity := map[string]any{"event_name": event, "hooks": []any{map[string]any{"type": "command", "command": command, "async": false, "timeout": 600}}}
	sum := sha256.Sum256([]byte(jsonString(identity)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

type hookInput struct {
	SessionID      string  `json:"session_id"`
	AgentID        *string `json:"agent_id"`
	TranscriptPath string  `json:"transcript_path"`
	Prompt         string  `json:"prompt"`
}

// hook reports a Codex hook event and answers Codex with the context
// Hypercube returns.
func hook(hypercube, event string, stdin io.Reader, stdout io.Writer) error {
	name := ""
	for _, e := range events {
		if e.name == event {
			name = e.report
		}
	}
	if name == "" {
		return fmt.Errorf("unknown hook event %q", event)
	}
	var in hookInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return err
	}
	// Native subagents have their own conversations.
	if in.AgentID != nil {
		return nil
	}
	// Usage only feeds the user's view, so failing to read it must not stop
	// the hook from delivering messages.
	if (event == "PostToolUse" || event == "Stop") && in.TranscriptPath != "" {
		_ = usage(hypercube, in.TranscriptPath)
	}
	body := map[string]string{}
	if event == "UserPromptSubmit" {
		body["conversation"] = in.SessionID
		body["text"] = in.Prompt
		if err := os.WriteFile(os.Getenv(sessionVar), []byte(in.SessionID), 0o644); err != nil {
			return err
		}
	}
	var r plugin.Response
	if err := plugin.Report(hypercube, name, body, &r); err != nil || r.Context == "" {
		return err
	}
	if event == "Stop" {
		return json.NewEncoder(stdout).Encode(map[string]string{"decision": "block", "reason": r.Context})
	}
	return json.NewEncoder(stdout).Encode(map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": event, "additionalContext": r.Context}})
}

// usage reports the context use in the newest token count of the session's
// transcript. Codex records no cost. Token counts follow each model response,
// so the newest is near the end.
func usage(hypercube, transcript string) error {
	f, err := os.Open(transcript)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.NewSectionReader(f, max(0, info.Size()-1<<20), info.Size()))
	if err != nil {
		return err
	}
	lines := bytes.Split(data, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var entry struct {
			Payload struct {
				Type string `json:"type"`
				Info *struct {
					Last struct {
						Input int `json:"input_tokens"`
					} `json:"last_token_usage"`
					Window int `json:"model_context_window"`
				} `json:"info"`
			} `json:"payload"`
		}
		if !bytes.Contains(lines[i], []byte(`"token_count"`)) || json.Unmarshal(lines[i], &entry) != nil || entry.Payload.Type != "token_count" || entry.Payload.Info == nil {
			continue
		}
		return plugin.Report(hypercube, "usage", map[string]int{"context": entry.Payload.Info.Last.Input, "window": entry.Payload.Info.Window}, &plugin.Response{})
	}
	return nil
}

// run runs Codex and, while it runs, waits for messages for the agent. When
// `hypercube wait` prints messages for the idle agent, run queues them in the
// session, which starts a turn. Codex fires no hooks while it is idle, even
// after resuming a session, so the waiter is a child of this process, and ends
// with it.
func run(hypercube string, argv []string) error {
	codex := exec.Command(argv[0], argv[1:]...)
	codex.Stdin, codex.Stdout, codex.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Codex handles the terminal's signals itself. Ignoring them through a
	// handler leaves Codex's own handling unchanged.
	signal.Notify(make(chan os.Signal, 1), syscall.SIGINT, syscall.SIGQUIT)
	if err := codex.Start(); err != nil {
		return err
	}
	go func() {
		if err := waitLoop(hypercube); err != nil {
			fmt.Fprintln(os.Stderr, "hypercube-harness-codex:", err)
		}
	}()
	err := codex.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	return err
}

// waitLoop delivers messages until a newer waiter replaces it. A new session
// waits for its first prompt.
func waitLoop(hypercube string) error {
	for {
		session, err := os.ReadFile(os.Getenv(sessionVar))
		if err != nil {
			return err
		}
		if len(session) == 0 {
			time.Sleep(time.Second)
			continue
		}
		var out bytes.Buffer
		cmd := exec.Command(hypercube, "wait")
		cmd.Stdout, cmd.Stderr = &out, io.Discard
		err = cmd.Run()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 2 {
			return err
		}
		// A new session may have replaced the one the waiter started in.
		if session, err = os.ReadFile(os.Getenv(sessionVar)); err != nil {
			return err
		}
		if err := queue(string(session), strings.TrimSpace(out.String())); err != nil {
			return err
		}
	}
}

// queue adds a message to a Codex session, which starts a turn when the
// session is idle and follows the current turn otherwise.
func queue(session, text string) error {
	cmd := exec.Command("codex", "queue", "--thread", session, "--message", text)
	cmd.Stdout, cmd.Stderr = io.Discard, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex queue: %w", err)
	}
	return nil
}

// jsonString marshals as Codex does: object keys sorted, HTML characters
// unescaped.
func jsonString(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return strings.TrimSuffix(b.String(), "\n")
}
