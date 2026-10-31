// Command hypercube-harness-claude runs Claude Code CLI as a Hypercube agent.
// Hypercube runs `launch`; Claude runs `hook`, `wait`, and `statusline`
// through the hooks and status line that launch configures.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func main() {
	args := os.Args[1:]
	var err error
	switch {
	case len(args) == 1 && args[0] == "launch":
		err = launch(os.Stdin, os.Stdout)
	case len(args) == 3 && args[0] == "hook":
		err = hook(args[1], args[2], os.Stdin, os.Stdout)
	case len(args) == 2 && args[0] == "wait":
		err = wait(args[1], os.Stdin)
	case (len(args) == 2 || len(args) == 3 && args[2] == "--limits") && args[0] == "statusline":
		_ = report(args[1], "answered", map[string]string{}, &response{})
		err = statusline(len(args) == 3, os.Stdin, os.Stdout)
	default:
		err = fmt.Errorf("usage: hypercube-harness-claude [launch | hook <hypercube> <event> | wait <hypercube> | statusline <hypercube> [--limits]]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-harness-claude:", err)
		os.Exit(1)
	}
}

type launchInput struct {
	Hypercube string            `json:"hypercube"`
	Env       map[string]string `json:"env"`
	Role      string            `json:"role"`
	Model     string            `json:"model"`
	Effort    string            `json:"effort"`
	Session   string            `json:"session"`
	Prompt    string            `json:"prompt"`
}

// Claude hook events and the Hypercube reports they make.
var reports = map[string]string{
	"SessionStart":       "start",
	"UserPromptSubmit":   "prompt",
	"PostToolUse":        "tool",
	"Stop":               "stop",
	"StopFailure":        "failed",
	"PermissionRequest":  "approval",
	"PostToolUseFailure": "toolfailed",
	"PermissionDenied":   "denied",
}

func launch(stdin io.Reader, stdout io.Writer) error {
	var in launchInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	command := func(args ...string) string {
		quoted := []string{shellQuote(self)}
		for _, arg := range args {
			quoted = append(quoted, shellQuote(arg))
		}
		return strings.Join(quoted, " ")
	}
	hooks := map[string]any{}
	for event := range reports {
		commands := []any{map[string]any{"type": "command", "command": command("hook", in.Hypercube, event)}}
		// The waiter starts a turn in the idle session when messages arrive.
		if event == "SessionStart" || event == "Stop" || event == "StopFailure" {
			commands = append(commands, map[string]any{"type": "command", "command": command("wait", in.Hypercube), "asyncRewake": true, "timeout": 20 * 24 * 60 * 60})
		}
		hooks[event] = []any{map[string]any{"hooks": commands}}
	}
	status := []string{"statusline", in.Hypercube}
	if in.Role == "lead" {
		status = append(status, "--limits")
	}
	argv := []string{"claude",
		"--mcp-config", jsonString(map[string]any{"mcpServers": map[string]any{"hypercube": map[string]any{"command": in.Hypercube, "args": []string{"mcp"}, "env": in.Env}}}),
		"--settings", jsonString(map[string]any{"hooks": hooks, "statusLine": map[string]any{"type": "command", "command": command(status...)}}),
		"--model", in.Model, "--effort", in.Effort, "--permission-mode", "auto"}
	if in.Session != "" {
		argv = append(argv, "--resume", in.Session)
	}
	if in.Prompt != "" {
		argv = append(argv, in.Prompt)
	}
	return json.NewEncoder(stdout).Encode(map[string]any{"argv": argv, "env": map[string]string{}})
}

type hookInput struct {
	SessionID            string          `json:"session_id"`
	AgentID              *string         `json:"agent_id"`
	Error                string          `json:"error"`
	ErrorDetails         string          `json:"error_details"`
	LastAssistantMessage string          `json:"last_assistant_message"`
	ToolName             string          `json:"tool_name"`
	ToolInput            json.RawMessage `json:"tool_input"`
	Reason               string          `json:"reason"`
}

type response struct {
	Context string `json:"context"`
}

// hook reports a Claude hook event and answers Claude with the context
// Hypercube returns.
func hook(hypercube, event string, stdin io.Reader, stdout io.Writer) error {
	name, ok := reports[event]
	if !ok {
		return fmt.Errorf("unknown hook event %q", event)
	}
	var in hookInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return err
	}
	// Hypercube logs failed calls of its own tools and auto mode denials of
	// any tool, native subagents' included.
	if event == "PostToolUseFailure" || event == "PermissionDenied" {
		if event == "PostToolUseFailure" && !strings.HasPrefix(in.ToolName, "mcp__hypercube__") {
			return nil
		}
		return report(hypercube, name, map[string]any{"tool": in.ToolName, "input": in.ToolInput, "error": in.Error, "reason": in.Reason}, &response{})
	}
	// Native subagents have their own conversations. Only their approvals
	// concern the agent.
	if in.AgentID != nil && event != "PermissionRequest" {
		return nil
	}
	body := map[string]any{}
	switch event {
	case "UserPromptSubmit":
		body["session"] = in.SessionID
	case "StopFailure":
		for _, text := range []string{in.ErrorDetails, in.LastAssistantMessage, in.Error} {
			if strings.TrimSpace(text) != "" {
				body["error"] = text
				break
			}
		}
	}
	var r response
	if err := report(hypercube, name, body, &r); err != nil || r.Context == "" {
		return err
	}
	if event == "Stop" {
		return json.NewEncoder(stdout).Encode(map[string]string{"decision": "block", "reason": r.Context})
	}
	return json.NewEncoder(stdout).Encode(map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": event, "additionalContext": r.Context}})
}

func report(hypercube, event string, body any, r *response) error {
	cmd := exec.Command(hypercube, "report", event)
	cmd.Stdin, cmd.Stderr = strings.NewReader(jsonString(body)), os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("hypercube report %s: %w", event, err)
	}
	return json.Unmarshal(bytes.TrimSpace(out), r)
}

// wait becomes `hypercube wait` with its stdout on stderr: asyncRewake starts
// a turn with stderr when the hook exits with status 2. Replacing this process
// keeps Claude as the waiter's parent, so the waiter ends with Claude.
func wait(hypercube string, stdin io.Reader) error {
	var in hookInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return err
	}
	if in.AgentID != nil {
		return nil
	}
	if err := syscall.Dup2(2, 1); err != nil {
		return err
	}
	return syscall.Exec(hypercube, []string{hypercube, "wait"}, os.Environ())
}

// statusline renders Claude's status line from the JSON Claude sends. Claude
// redraws it after an approval dialog closes, which is when main reports the
// approval answered.
func statusline(limits bool, input io.Reader, out io.Writer) error {
	var status map[string]any
	_ = json.NewDecoder(input).Decode(&status)
	value := func(parts ...string) any {
		var v any = status
		for _, part := range parts {
			obj, _ := v.(map[string]any)
			v = obj[part]
		}
		return v
	}
	text := func(v any) string { s, _ := v.(string); return s }
	number := func(v any) float64 { n, _ := v.(float64); return n }
	paint := func(color, text string) string { return "\x1b[" + color + "m" + text + "\x1b[0m" }
	line := paint("38;2;218;96;0", "✳︎ ") + paint("38;2;218;96;0", text(value("model", "display_name"))) + paint("38;2;255;255;255", " :: ")
	if value("thinking", "enabled") == true {
		line += paint("38;2;255;151;69", text(value("effort", "level")))
	}
	line += fmt.Sprintf(" %s/%s (%d%%)", tokens(number(value("context_window", "total_input_tokens"))), tokens(number(value("context_window", "context_window_size"))), int(number(value("context_window", "used_percentage"))))
	if limits {
		line += fmt.Sprintf(" | 5h %s | 7d %s", bar(number(value("rate_limits", "five_hour", "used_percentage"))), bar(number(value("rate_limits", "seven_day", "used_percentage"))))
	}
	_, err := fmt.Fprint(out, line)
	return err
}

func tokens(n float64) string {
	if n < 0 {
		n = 0
	}
	whole := int64(n)
	if whole < 1000 {
		return fmt.Sprint(whole)
	}
	unit, scale := "k", int64(1000)
	if whole >= 1000000 {
		unit, scale = "M", 1000000
	}
	if tenth := whole % scale / (scale / 10); tenth != 0 {
		return fmt.Sprintf("%d.%d%s", whole/scale, tenth, unit)
	}
	return fmt.Sprintf("%d%s", whole/scale, unit)
}

func bar(percent float64) string {
	filled := (int(math.Max(0, math.Min(100, percent)))*10 + 50) / 100
	return strings.Repeat("▪", filled) + strings.Repeat("·", 10-filled)
}

func jsonString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
