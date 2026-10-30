// Package harness builds per-launch harness configuration without changing
// the user's settings, credentials, or HOME.
package harness

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/tmux"
	"github.com/kkestell/hypercube/internal/workspace"
)

var events = []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "Stop", "StopFailure", "PermissionRequest", "PostToolUseFailure", "PermissionDenied"}

//go:embed opencode.js
var opencodePlugin string

// Command returns the command and environment for one harness launch.
func Command(a store.Agent, statePath, prompt string) ([]string, map[string]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	env := map[string]string{"HYPERCUBE_AGENT_ID": a.ID, "HYPERCUBE_STATE": statePath}
	var argv []string
	switch a.Harness {
	case "claude":
		hooks := map[string]any{}
		for _, event := range events {
			commands := []any{map[string]any{"type": "command", "command": shellQuote(exe) + " hook " + event}}
			if event == "SessionStart" || event == "Stop" || event == "StopFailure" {
				commands = append(commands, map[string]any{"type": "command", "command": shellQuote(exe) + " wait", "asyncRewake": true, "timeout": 20 * 24 * 60 * 60})
			}
			hooks[event] = []any{map[string]any{"hooks": commands}}
		}
		statusline := shellQuote(exe) + " statusline"
		if a.Role == store.Lead {
			statusline += " --limits"
		}
		argv = []string{"claude", "--mcp-config", jsonString(map[string]any{"mcpServers": map[string]any{"hypercube": map[string]any{"command": exe, "args": []string{"mcp"}, "env": env}}}),
			"--settings", jsonString(map[string]any{"hooks": hooks, "statusLine": map[string]any{"type": "command", "command": statusline}}),
			"--model", a.Model, "--effort", a.Effort, "--permission-mode", "auto"}
		if a.SessionID != "" {
			argv = append(argv, "--resume", a.SessionID)
		}
	case "opencode":
		plugin, err := writePlugin(exe, statePath)
		if err != nil {
			return nil, nil, err
		}
		session := a.SessionID
		if session == "" {
			session = "ses_hypercube_" + a.ID
		}
		env["OPENCODE_CONFIG_CONTENT"] = jsonString(map[string]any{
			"model":   a.Model,
			"agents":  map[string]any{"build": map[string]any{"model": a.Model + "#" + a.Effort}},
			"mcp":     map[string]any{"servers": map[string]any{"hypercube": map[string]any{"type": "local", "command": []string{exe, "mcp"}, "environment": map[string]string{"HYPERCUBE_AGENT_ID": a.ID, "HYPERCUBE_STATE": statePath}, "codemode": false}}},
			"plugins": []string{plugin},
		})
		env["OPENCODE_CLI_CONFIG_CONTENT"] = `{"tabs":{"mode":"off"}}`
		env["HYPERCUBE_SESSION_ID"] = session
		argv = []string{"opencode", "--standalone", "--auto", "--session", session}
		if prompt != "" {
			argv = append(argv, "--prompt")
		}
	default:
		return nil, nil, fmt.Errorf("agent %q has unsupported harness %q; use claude or opencode", a.Name, a.Harness)
	}
	if prompt != "" {
		argv = append(argv, prompt)
	}
	return argv, env, nil
}

func writePlugin(exe, statePath string) (string, error) {
	dir := filepath.Join(filepath.Dir(statePath), "opencode-plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "index.js")
	source := []byte(strings.ReplaceAll(opencodePlugin, "HYPERCUBE_EXE", jsonString(exe)))
	if before, err := os.ReadFile(path); err == nil && bytes.Equal(before, source) {
		return dir, nil
	}
	file, err := os.CreateTemp(dir, "index-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(source); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return dir, os.Rename(file.Name(), path)
}

func jsonString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// Launch starts a short-lived wrapper in a tmux pane. The wrapper records the
// pane and current launch state, then replaces itself with the native harness.
func Launch(a store.Agent, statePath, prompt string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	label := fmt.Sprintf("%s · %s %s %s · %s", a.Name, a.Harness, a.Model, a.Effort, workspace.Short(a.Dir))
	env := map[string]string{}
	for _, item := range os.Environ() {
		key, value, _ := strings.Cut(item, "=")
		if key != "TMUX" && key != "TMUX_PANE" {
			env[key] = value
		}
	}
	env["HYPERCUBE_AGENT_ID"], env["HYPERCUBE_STATE"] = a.ID, statePath
	return tmux.NewPane(store.Workspace(statePath), a.Team, label, a.Dir, env, []string{exe, "agent", prompt})
}
