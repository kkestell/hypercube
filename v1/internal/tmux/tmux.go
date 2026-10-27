// Package tmux runs the tmux commands Hypercube needs.
package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// HasSession reports whether the named session exists.
func HasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", "="+name).Run() == nil
}

// NewPane starts argv in a new pane of the session's window, labels the pane
// with label, and returns the pane ID. If the session does not exist, it
// creates the session with this as its first pane. The first pane fills the
// left 40% of the window and the rest are stacked on the right.
func NewPane(session, label, dir string, env map[string]string, argv []string) (string, error) {
	window := "=" + session + ":"
	var args []string
	if HasSession(session) {
		panes, err := run("list-panes", "-t", window, "-F", "#{pane_id}")
		if err != nil {
			return "", err
		}
		ids := strings.Split(panes, "\n")
		args = []string{"split-window", "-d", "-v", "-t", ids[len(ids)-1]}
	} else {
		// Server options go first so that they are on before the harness
		// starts. Extended keys let Shift+Enter reach the harnesses.
		args = []string{
			"set-option", "-s", "extended-keys", "on", ";",
			"set-option", "-s", "extended-keys-format", "csi-u", ";",
			"set-option", "-s", "terminal-features[99]", "xterm*:extkeys", ";",
			"set-option", "-s", "focus-events", "on", ";",
			"new-session", "-d", "-s", session, "-n", "hypercube"}
	}
	args = append(args, "-c", dir, "-P", "-F", "#{pane_id}")
	for k, v := range env {
		args = append(args, "-e", k+"="+v)
	}
	pane, err := run(append(args, argv...)...)
	if err != nil {
		return "", err
	}
	_, err = run("set-option", "-p", "-t", pane, "@hypercube_agent", label, ";",
		"set-option", "-w", "-t", window, "pane-border-status", "top", ";",
		"set-option", "-w", "-t", window, "pane-border-format", "#[fg=black,bg=colour117]#{?pane_active,#[bold],} #{@hypercube_agent} #[default]", ";",
		"set-option", "-w", "-t", window, "pane-border-lines", "heavy", ";",
		"set-option", "-w", "-t", window, "pane-border-style", "fg=colour33", ";",
		"set-option", "-w", "-t", window, "pane-active-border-style", "fg=colour33", ";",
		"set-option", "-w", "-t", window, "main-pane-width", "40%", ";",
		"select-layout", "-t", window, "main-vertical")
	return pane, err
}

// KillPane closes the pane and lays out the remaining panes again.
func KillPane(session, target string) error {
	_, err := run("kill-pane", "-t", target, ";",
		"select-layout", "-t", "="+session+":", "main-vertical")
	return err
}

// ConfigureSession turns on the mouse and passthrough, which lets harness
// notifications reach the terminal, and styles the status bar like the pane
// labels with left and right on it. left is plain text; right is a format.
func ConfigureSession(session, left, right string) error {
	target := "=" + session + ":"
	_, err := run("set-option", "-t", target, "mouse", "on", ";",
		"set-option", "-w", "-t", target, "allow-passthrough", "on", ";",
		"set-option", "-t", target, "status-style", "bg=colour117,fg=black", ";",
		"set-option", "-t", target, "status-left", " "+strings.ReplaceAll(left, "#", "##")+" ", ";",
		"set-option", "-t", target, "status-left-length", "120", ";",
		"set-option", "-w", "-t", target, "window-status-format", "", ";",
		"set-option", "-w", "-t", target, "window-status-current-format", "", ";",
		"set-option", "-t", target, "status-right", right, ";",
		"set-option", "-t", target, "status-right-length", "120")
	return err
}

// SetGlobal sets a global user option.
func SetGlobal(option, value string) error {
	_, err := run("set-option", "-g", option, value)
	return err
}

// Global returns a global user option, or "" if it is unset.
func Global(option string) string {
	out, _ := run("show-options", "-gqv", option)
	return out
}

// KillSession closes the session and every pane in it.
func KillSession(session string) error {
	_, err := run("kill-session", "-t", "="+session)
	return err
}

// SendKeys types text into the target's pane and presses Enter.
func SendKeys(target, text string) error {
	if _, err := run("send-keys", "-t", target, "-l", text); err != nil {
		return err
	}
	time.Sleep(200 * time.Millisecond)
	_, err := run("send-keys", "-t", target, "Enter")
	return err
}

// Attach attaches the terminal to the session, or switches to it when
// already inside tmux.
func Attach(session string) error {
	verb := "attach-session"
	if os.Getenv("TMUX") != "" {
		verb = "switch-client"
	}
	cmd := exec.Command("tmux", verb, "-t", "="+session)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
