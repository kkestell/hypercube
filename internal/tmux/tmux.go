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

func HasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", "="+name).Run() == nil
}

// NewPane starts argv in a new pane of the session's window, labels the pane
// with name, and returns the pane ID. If the session does not exist, it
// creates the session with this as its first pane. The first pane fills the
// left 40% of the window and the rest are stacked on the right.
func NewPane(session, name, dir string, env map[string]string, argv []string) (string, error) {
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
		args = []string{"new-session", "-d", "-s", session, "-n", "hypercube"}
	}
	args = append(args, "-c", dir, "-P", "-F", "#{pane_id}")
	for k, v := range env {
		args = append(args, "-e", k+"="+v)
	}
	pane, err := run(append(args, argv...)...)
	if err != nil {
		return "", err
	}
	_, err = run("set-option", "-p", "-t", pane, "@hypercube_agent", name, ";",
		"set-option", "-w", "-t", window, "pane-border-status", "top", ";",
		"set-option", "-w", "-t", window, "pane-border-format", " #{@hypercube_agent} ", ";",
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
