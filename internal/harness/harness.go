// Package harness builds the launch command for each harness.
package harness

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/tmux"
	"github.com/kkestell/hypercube/internal/workspace"
)

var events = []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "Stop"}

//go:embed opencode.js
var opencodePlugin string

// Command returns the argv and environment that launch an agent's harness.
// A non-empty prompt starts the first turn.
func Command(a store.Agent, statePath, prompt string) ([]string, map[string]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	env := map[string]string{"HYPERCUBE_AGENT_ID": a.ID, "HYPERCUBE_STATE": statePath}
	mcp := map[string]any{"command": exe, "args": []string{"mcp"}, "env": env}
	hook := func(event string) string { return shellQuote(exe) + " hook " + event }
	// Claude Code's hooks setting, which Ox also reads.
	hooks := map[string]any{}
	for _, e := range events {
		hooks[e] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hook(e)}}}}
	}

	switch a.Harness {
	case "claude":
		argv := []string{"claude",
			"--mcp-config", jsonString(map[string]any{"mcpServers": map[string]any{"hypercube": mcp}}),
			"--settings", jsonString(map[string]any{"hooks": hooks,
				"statusLine": map[string]any{"type": "command", "command": shellQuote(exe) + " statusline"}}),
			"--model", a.Model, "--effort", a.Effort, "--dangerously-skip-permissions"}
		if a.SessionID != "" {
			argv = append(argv, "--resume", a.SessionID)
		}
		if prompt != "" {
			argv = append(argv, prompt)
		}
		return argv, env, nil

	case "codex":
		// Codex keys folder trust on the main checkout, which covers worktrees.
		// A directory outside Git is trusted as it is.
		root := a.Dir
		if workspace.IsRepo(a.Dir) {
			if root, err = workspace.MainRoot(a.Dir); err != nil {
				return nil, nil, err
			}
		}
		argv := []string{"codex"}
		if a.SessionID != "" {
			argv = append(argv, "resume")
		}
		argv = append(argv, "--dangerously-bypass-approvals-and-sandbox", "--dangerously-bypass-hook-trust",
			"-c", "model="+strconv.Quote(a.Model),
			"-c", "model_reasoning_effort="+strconv.Quote(a.Effort),
			"-c", fmt.Sprintf("mcp_servers.hypercube={command=%q,args=[\"mcp\"],env={HYPERCUBE_AGENT_ID=%q,HYPERCUBE_STATE=%q},env_vars=[\"TMUX\"]}", exe, a.ID, statePath),
			"-c", fmt.Sprintf("projects={%q={trust_level=\"trusted\"}}", root))
		for _, e := range events {
			argv = append(argv, "-c", fmt.Sprintf("hooks.%s=[{hooks=[{type=\"command\",command=%q}]}]", e, hook(e)))
		}
		if a.SessionID != "" {
			argv = append(argv, a.SessionID)
		}
		if prompt != "" {
			argv = append(argv, prompt)
		}
		return argv, env, nil

	case "opencode":
		plugin := filepath.Join(store.Dir(), "opencode-plugin")
		if err := os.MkdirAll(plugin, 0o755); err != nil {
			return nil, nil, err
		}
		if err := os.WriteFile(filepath.Join(plugin, "index.js"), []byte(strings.ReplaceAll(opencodePlugin, "HYPERCUBE_EXE", strconv.Quote(exe))), 0o644); err != nil {
			return nil, nil, err
		}
		env["OPENCODE_CONFIG_CONTENT"] = jsonString(map[string]any{
			"model":   a.Model,
			"agents":  map[string]any{"build": map[string]any{"model": a.Model + "#" + a.Effort}},
			"mcp":     map[string]any{"servers": map[string]any{"hypercube": map[string]any{"type": "local", "command": []string{exe, "mcp"}, "environment": env}}},
			"plugins": []string{plugin},
		})
		argv := []string{"opencode", "--standalone", "--auto"}
		if a.SessionID != "" {
			argv = append(argv, "--session", a.SessionID)
		}
		if prompt != "" {
			argv = append(argv, "--prompt", prompt)
		}
		return argv, env, nil

	case "ox":
		env["OX_SETTINGS"] = jsonString(map[string]any{
			"model": a.Model, "effort": a.Effort, "mode": "auto",
			"mcpServers": map[string]any{"hypercube": mcp},
			"hooks":      hooks,
		})
		argv := []string{"ox"}
		if a.SessionID != "" {
			argv = append(argv, "--resume", a.SessionID)
		}
		if prompt != "" {
			argv = append(argv, "--", prompt)
		}
		return argv, env, nil
	}
	return nil, nil, fmt.Errorf("unknown harness %q", a.Harness)
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// StatusRight is the tmux status bar format that shows usage limits.
func StatusRight() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return "#(" + shellQuote(exe) + " limits) ", nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Launch starts the agent's harness in a new pane of the workspace's tmux
// session and returns the pane ID.
func Launch(a store.Agent, statePath, prompt string) (string, error) {
	argv, env, err := Command(a, statePath, prompt)
	if err != nil {
		return "", err
	}
	label := fmt.Sprintf("%s · %s %s %s · %s", a.Name, a.Harness, a.Model, a.Effort, workspace.Short(a.Dir))
	return tmux.NewPane(store.Workspace(statePath), label, a.Dir, env, argv)
}
