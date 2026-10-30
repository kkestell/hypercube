// Package tmux delegates terminal rendering, input, scrollback, and pane
// lifetimes to tmux. Hypercube changes options only in its own sessions.
package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kkestell/hypercube/internal/store"
)

func run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func HasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", "="+name).Run() == nil
}

// NewPane allocates the pane before launching its harness, so even an immediate
// launch failure remains visible. The caller serializes workspace lifecycle
// operations. The temporary sleep is replaced after the pane is configured.
func NewPane(session, team, label, dir string, env map[string]string, argv []string) (pane string, err error) {
	first := !HasSession(session)
	window, err := FindWindow(session, team, env["HYPERCUBE_STATE"])
	if err != nil {
		return "", err
	}
	newWindow := window == ""
	var args []string
	if first {
		args = []string{"new-session", "-d", "-s", session, "-n", team}
	} else if newWindow {
		args = []string{"new-window", "-d", "-t", "=" + session + ":", "-n", team}
	} else {
		panes, err := run("list-panes", "-t", window, "-F", "#{pane_id}")
		if err != nil {
			return "", err
		}
		ids := strings.Split(panes, "\n")
		args = []string{"split-window", "-d", "-v", "-t", ids[len(ids)-1]}
	}
	args = append(args, "-c", dir, "-P", "-F", "#{pane_id}", "/bin/sleep", "60")
	pane, err = run(args...)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_, _ = run("kill-pane", "-t", pane)
		}
	}()
	window, err = run("display-message", "-p", "-t", pane, "#{window_id}")
	if err != nil {
		return pane, err
	}
	exe, err := os.Executable()
	if err != nil {
		return pane, err
	}
	died := "run-shell -b " + quote(quote(exe)+" exited "+quote(env["HYPERCUBE_STATE"])+" '#{pane_id}' '#{pane_dead_status}'")
	_, err = run("set-option", "-p", "-t", pane, "@hypercube_id", env["HYPERCUBE_AGENT_ID"], ";",
		"set-option", "-p", "-t", pane, "@hypercube_label", strings.ReplaceAll(label, "#", "##"), ";",
		"set-option", "-w", "-t", window, "remain-on-exit", "on", ";",
		"set-option", "-w", "-t", window, "pane-border-status", "top", ";",
		"set-option", "-w", "-t", window, "pane-border-format", " #{?pane_active,#[bold],}#{@hypercube_label} #{@hypercube_activity}#{?pane_dead, · exited #{pane_dead_status},} #[default]", ";",
		"set-option", "-w", "-t", window, "main-pane-width", "40%", ";",
		"set-option", "-w", "-t", window, "@hypercube_state", env["HYPERCUBE_STATE"], ";",
		"set-option", "-w", "-t", window, "@hypercube_team", team, ";",
		"set-option", "-w", "-t", window, "automatic-rename", "off", ";",
		"set-option", "-w", "-t", window, "allow-rename", "off", ";",
		"set-option", "-w", "-t", window, "extended-keys", "always", ";",
		"set-option", "-w", "-t", window, "window-status-format", " #I #W"+attention("black")+" ", ";",
		"set-option", "-w", "-t", window, "window-status-current-format", "#[fg=green,bg=black,bold] #I #W"+attention("green")+" #[fg=black,bg=green,nobold]", ";",
		"set-option", "-w", "-t", window, "window-status-separator", "", ";",
		"set-option", "-t", "="+session+":", "@hypercube_state", env["HYPERCUBE_STATE"], ";",
		"set-hook", "-w", "-t", window, "pane-died", died, ";",
		"select-layout", "-t", window, "main-vertical")
	if err != nil {
		return pane, err
	}
	if first {
		var options []string
		for key, value := range env {
			if key == "HYPERCUBE_AGENT_ID" {
				continue
			}
			if len(options) != 0 {
				options = append(options, ";")
			}
			options = append(options, "set-environment", "-t", "="+session+":", key, value)
		}
		if len(options) != 0 {
			if _, err = run(options...); err != nil {
				return pane, err
			}
		}
	}
	err = Respawn(pane, dir, env, argv)
	if err == nil && newWindow {
		_, err = run("select-pane", "-t", pane)
	}
	return pane, err
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
func Owns(session, pane, id, state string) bool {
	if pane == "" || !strings.HasPrefix(pane, "%") {
		return false
	}
	got, err := run("display-message", "-p", "-t", pane, "#{session_name}\n#{@hypercube_id}\n#{@hypercube_state}")
	return err == nil && got == session+"\n"+id+"\n"+state
}

func KillPane(session, target string) error {
	if target == "" {
		return nil
	}
	window, err := run("display-message", "-p", "-t", target, "#{window_id}")
	if err != nil {
		return err
	}
	if _, err := run("kill-pane", "-t", target); err != nil {
		return err
	}
	if HasSession(session) {
		_, err := run("select-layout", "-t", window, "main-vertical")
		return err
	}
	return nil
}

func ConfigureSession(session, left string) error {
	_, err := run("set-option", "-t", "="+session+":", "mouse", "on", ";",
		"set-option", "-t", "="+session+":", "status-style", "fg=black,bg=green", ";",
		"set-option", "-t", "="+session+":", "status-left", "#[bold] "+strings.ReplaceAll(left, "#", "##")+" #[nobold]", ";",
		"set-option", "-t", "="+session+":", "status-left-length", "100", ";",
		"set-option", "-t", "="+session+":", "status-right", "")
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	newCommand := `#{q:@hypercube_exe} window new #{q:@hypercube_state}`
	// c replaces tmux's new-window only in Hypercube windows.
	// Cleanup must survive tmux exiting when its last window is removed.
	deleteCommand := `nohup #{q:@hypercube_exe} window delete #{q:@hypercube_state} #{@hypercube_team} #{window_id} >/dev/null 2>&1 &`
	owned := `#{&&:#{@hypercube_state},#{@hypercube_team}}`
	_, err = run("set-option", "-t", "="+session+":", "@hypercube_exe", exe, ";",
		"bind-key", "c", "if-shell", "-F", owned, "run-shell -b "+quote(newCommand), "new-window", ";",
		"bind-key", "X", "if-shell", "-F", owned,
		"confirm-before -p 'Delete current Hypercube session? (y/n)' "+quote("run-shell -b "+quote(deleteCommand)))
	return err
}

func Activity(session, pane, id, state, activity string, attention bool) {
	if !Owns(session, pane, id, state) {
		return
	}
	text := "#[fg=colour244]●#[default]"
	if activity == "busy" {
		text = "#[fg=green]●#[default]"
	}
	if activity == "exited" {
		text = ""
	}
	if attention {
		text += " #[fg=red]●#[default]"
	}
	flag := ""
	if attention {
		flag = "1"
	}
	_, _ = run("set-option", "-p", "-t", pane, "@hypercube_activity", text, ";",
		"set-option", "-p", "-t", pane, "@hypercube_attention", flag)
}

// attention marks a window name in the status line with a red dot for each of
// its panes awaiting approval, then restores the given foreground. The window
// formats name their colors because #[default] leaves the status line's.
func attention(fg string) string {
	return "#{P:#{?@hypercube_attention, #[fg=red]●#[fg=" + fg + "],}}"
}

func KillSession(session string) error { _, err := run("kill-session", "-t", "="+session); return err }

func Attach(session string) error {
	verb := "attach-session"
	if os.Getenv("TMUX") != "" {
		verb = "switch-client"
	}
	cmd := exec.Command("tmux", verb, "-t", "="+session)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// WorkspaceSession refuses a name collision with a session Hypercube did not
// create, even if that session happens to have the expected name.
func WorkspaceSession(session, state string) bool {
	got, err := run("show-options", "-qv", "-t", "="+session+":", "@hypercube_state")
	return err == nil && got == state
}

// FindWindow uses live markers rather than a window's index or editable name.
func FindWindow(session, team, state string) (string, error) {
	if !HasSession(session) {
		return "", nil
	}
	if !WorkspaceSession(session, state) {
		return "", fmt.Errorf("tmux session %q belongs to something else", session)
	}
	rows, err := run("list-windows", "-t", "="+session, "-F", "#{window_id}\t#{@hypercube_team}\t#{@hypercube_state}")
	if err != nil {
		return "", err
	}
	var found string
	for _, row := range strings.Split(rows, "\n") {
		fields := strings.Split(row, "\t")
		if len(fields) != 3 || fields[1] != team || fields[2] != state {
			continue
		}
		if found != "" {
			return "", fmt.Errorf("multiple tmux windows claim session %q", team)
		}
		found = fields[0]
	}
	return found, nil
}

func SelectWindow(window string) error {
	_, err := run("select-window", "-t", window)
	return err
}

func KillWindow(session, team, state string) error {
	window, err := FindWindow(session, team, state)
	if err != nil || window == "" {
		return err
	}
	_, err = run("kill-window", "-t", window)
	return err
}

// Restart reuses the pane's original command and launch environment.
func Restart(pane, id, state string) error {
	_, err := run("respawn-pane", "-k", "-t", pane, "-e", "HYPERCUBE_AGENT_ID="+id, "-e", "HYPERCUBE_STATE="+state)
	return err
}

// FindPane looks up the agent by its live pane marker. A saved pane number can
// be stale; it must neither close someone else's pane nor leave ours behind.
func FindPane(session, id, state string) (string, error) {
	if !WorkspaceSession(session, state) {
		return "", nil
	}
	rows, err := run("list-panes", "-s", "-t", "="+session, "-F", "#{pane_id}\t#{@hypercube_id}")
	if err != nil {
		return "", err
	}
	for _, row := range strings.Split(rows, "\n") {
		pane, agent, _ := strings.Cut(row, "\t")
		if agent == id && Owns(session, pane, id, state) {
			return pane, nil
		}
	}
	return "", nil
}

// RefreshActivity is best effort: a closed pane must not make a delivered
// message fail. Read the newest state after committing the hook or wake-up.
func RefreshActivity(path, id string) {
	s, err := store.Load(path)
	if err != nil {
		return
	}
	a, err := s.Agent(id)
	if err != nil {
		return
	}
	Activity(store.Workspace(path), a.Target, id, path, a.Activity, a.Attention)
}

// Dead guards against a queued pane-died hook arriving after a later respawn.
func Dead(session, pane, id, state string) bool {
	if !Owns(session, pane, id, state) {
		return false
	}
	got, err := run("display-message", "-p", "-t", pane, "#{pane_dead}")
	return err == nil && got == "1"
}
