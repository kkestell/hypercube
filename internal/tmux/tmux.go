// Package tmux delegates terminal rendering, input, scrollback, and pane
// lifetimes to tmux. Each running Hypercube session has its own tmux session,
// and Hypercube changes options only in those.
package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Name is the tmux session of a Hypercube session.
func Name(session string) string { return "hypercube-" + session }

// Running reports whether the Hypercube session's tmux session exists. The
// session's marker keeps a user's own tmux session with that name from being
// mistaken for it.
func Running(session string) bool {
	got, err := run("show-options", "-qv", "-t", "="+Name(session)+":", "@hypercube_session")
	return err == nil && got == session
}

// NewSession creates the Hypercube session's tmux session with the launch
// environment and starts its first pane, the lead's.
func NewSession(session, dir string, env map[string]string, argv []string) (string, error) {
	args := []string{"new-session", "-d", "-s", Name(session), "-c", dir}
	for key, value := range env {
		if key != "HYPERCUBE_AGENT_ID" {
			args = append(args, "-e", key+"="+value)
		}
	}
	args = append(args, "-P", "-F", "#{pane_id}", "/bin/sleep", "60")
	pane, err := run(args...)
	if err != nil {
		return "", err
	}
	if _, err := run("set-option", "-t", "="+Name(session)+":", "@hypercube_session", session); err != nil {
		return pane, err
	}
	return pane, start(pane, dir, env, argv)
}

// AddPane splits the session's agent window for another agent.
func AddPane(session, dir string, env map[string]string, argv []string) (string, error) {
	// The agent window is the one with marked panes; the view's terminal
	// window has none.
	rows, err := run("list-panes", "-s", "-t", "="+Name(session), "-F", "#{pane_id}\t#{@hypercube_agent}")
	if err != nil {
		return "", err
	}
	var last string
	for _, row := range strings.Split(rows, "\n") {
		if id, agent, _ := strings.Cut(row, "\t"); agent != "" {
			last = id
		}
	}
	if last == "" {
		return "", fmt.Errorf("session %s has no agent window", session)
	}
	pane, err := run("split-window", "-d", "-v", "-t", last, "-c", dir, "-P", "-F", "#{pane_id}", "/bin/sleep", "60")
	if err != nil {
		return "", err
	}
	if err := start(pane, dir, env, argv); err != nil {
		_, _ = run("kill-pane", "-t", pane)
		return pane, err
	}
	return pane, nil
}

// start marks and configures a placeholder pane, so even an immediate launch
// failure remains visible, then replaces its sleep with the agent.
func start(pane, dir string, env map[string]string, argv []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	died := "run-shell -b " + quote(quote(exe)+" exited '#{@hypercube_agent}' '#{pane_id}'")
	_, err = run("set-option", "-p", "-t", pane, "@hypercube_agent", env["HYPERCUBE_AGENT_ID"], ";",
		"set-option", "-w", "-t", pane, "remain-on-exit", "on", ";",
		"set-option", "-w", "-t", pane, "extended-keys", "always", ";",
		"set-option", "-w", "-t", pane, "main-pane-width", "40%", ";",
		"set-hook", "-w", "-t", pane, "pane-died", died, ";",
		"select-layout", "-t", pane, "main-vertical")
	if err != nil {
		return err
	}
	return Respawn(pane, dir, env, argv)
}

func Respawn(pane, dir string, env map[string]string, argv []string) error {
	args := []string{"respawn-pane", "-k", "-t", pane, "-c", dir}
	for key, value := range env {
		args = append(args, "-e", key+"="+value)
	}
	_, err := run(append(args, argv...)...)
	return err
}

// Owns checks identity at use time rather than trusting a saved pane number,
// since a tmux server restart can reuse pane IDs.
func Owns(pane, agent string) bool {
	if !strings.HasPrefix(pane, "%") {
		return false
	}
	got, err := run("display-message", "-p", "-t", pane, "#{@hypercube_agent}\n#{session_name}\n#{@hypercube_session}")
	lines := strings.Split(got, "\n")
	return err == nil && len(lines) == 3 && lines[0] == agent && lines[1] == Name(lines[2])
}

// FindPane looks up the agent's pane by its live marker in the session.
func FindPane(session, agent string) (string, error) {
	if !Running(session) {
		return "", nil
	}
	rows, err := run("list-panes", "-s", "-t", "="+Name(session), "-F", "#{pane_id}\t#{@hypercube_agent}")
	if err != nil {
		return "", err
	}
	for _, row := range strings.Split(rows, "\n") {
		pane, id, _ := strings.Cut(row, "\t")
		if id == agent && Owns(pane, agent) {
			return pane, nil
		}
	}
	return "", nil
}

// KillPane closes a pane and lays out the rest of its window again.
func KillPane(pane string) error {
	window, err := run("display-message", "-p", "-t", pane, "#{window_id}")
	if err != nil {
		return err
	}
	if _, err := run("kill-pane", "-t", pane); err != nil {
		return err
	}
	_, _ = run("select-layout", "-t", window, "main-vertical")
	return nil
}

// KillSession closes the Hypercube session's tmux session, if it is running.
func KillSession(session string) error {
	if !Running(session) {
		return nil
	}
	_, err := run("kill-session", "-t", "="+Name(session))
	return err
}

// Restart reuses the pane's original command and launch environment.
func Restart(pane, agent string) error {
	_, err := run("respawn-pane", "-k", "-t", pane, "-e", "HYPERCUBE_AGENT_ID="+agent)
	return err
}

// Dead guards against a queued pane-died hook arriving after a later respawn.
func Dead(pane, agent string) bool {
	if !Owns(pane, agent) {
		return false
	}
	got, err := run("display-message", "-p", "-t", pane, "#{pane_dead}")
	return err == nil && got == "1"
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
