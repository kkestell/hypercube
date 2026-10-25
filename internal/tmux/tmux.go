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

// NewWindow starts argv in a new window and returns the window ID. If the
// session does not exist, it creates the session with this as its first
// window.
func NewWindow(session, name, dir string, env map[string]string, argv []string) (string, error) {
	var args []string
	if HasSession(session) {
		args = []string{"new-window", "-d", "-t", "=" + session + ":"}
	} else {
		args = []string{"new-session", "-d", "-s", session}
	}
	args = append(args, "-n", name, "-c", dir, "-P", "-F", "#{window_id}")
	for k, v := range env {
		args = append(args, "-e", k+"="+v)
	}
	return run(append(args, argv...)...)
}

func KillWindow(target string) error {
	_, err := run("kill-window", "-t", target)
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
