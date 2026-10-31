// Package harness launches agents through harness plugins. A plugin is an
// executable named hypercube-harness-<name> beside hypercube. It builds the
// harness's command line from per-launch arguments, environment, and
// overrides, without changing the user's settings, credentials, or HOME.
package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/tmux"
	"github.com/kkestell/hypercube/internal/workspace"
)

// LaunchInput is what `<plugin> launch` reads on stdin.
type LaunchInput struct {
	// Hypercube is the executable the harness runs for `mcp`, `report`, and
	// `wait`.
	Hypercube string `json:"hypercube"`
	// Env is the environment for Hypercube processes the harness starts.
	Env     map[string]string `json:"env"`
	Agent   string            `json:"agent"`
	Role    string            `json:"role"`
	Model   string            `json:"model"`
	Effort  string            `json:"effort"`
	Session string            `json:"session"`
	Prompt  string            `json:"prompt"`
	// StateDir is Hypercube's state directory, where a plugin may keep files.
	StateDir string `json:"state_dir"`
}

// LaunchOutput is what `<plugin> launch` prints: the harness command and the
// environment to add to it.
type LaunchOutput struct {
	Argv []string          `json:"argv"`
	Env  map[string]string `json:"env"`
}

func plugin(name string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	path := filepath.Join(filepath.Dir(exe), "hypercube-harness-"+name)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("harness %q has no plugin: install %s", name, path)
	}
	return path, nil
}

// Command runs the agent's plugin to get the harness command and environment.
func Command(a store.Agent, statePath, prompt string) ([]string, map[string]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	path, err := plugin(a.Harness)
	if err != nil {
		return nil, nil, err
	}
	in, err := json.Marshal(LaunchInput{
		Hypercube: exe, Env: map[string]string{"HYPERCUBE_AGENT_ID": a.ID, "HYPERCUBE_STATE": statePath},
		Agent: a.ID, Role: a.Role, Model: a.Model, Effort: a.Effort, Session: a.SessionID, Prompt: prompt,
		StateDir: filepath.Dir(statePath),
	})
	if err != nil {
		return nil, nil, err
	}
	cmd := exec.Command(path, "launch")
	cmd.Stdin, cmd.Stderr = bytes.NewReader(in), os.Stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("%s launch: %w", filepath.Base(path), err)
	}
	var out LaunchOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, nil, fmt.Errorf("%s launch: %w", filepath.Base(path), err)
	}
	if len(out.Argv) == 0 {
		return nil, nil, fmt.Errorf("%s launch printed no command", filepath.Base(path))
	}
	return out.Argv, out.Env, nil
}

// Launch starts a short-lived wrapper in a tmux pane. The wrapper records the
// pane and current launch state, then replaces itself with the native harness.
func Launch(a store.Agent, statePath, prompt string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if _, err := plugin(a.Harness); err != nil {
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
