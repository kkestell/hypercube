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
func NewPane(state, team, title, label, dir string, env map[string]string, argv []string) (pane string, err error) {
	session := store.Workspace(state)
	first := !HasSession(session)
	window, err := FindWindow(session, team, state)
	if err != nil {
		return "", err
	}
	newWindow := window == ""
	var args []string
	if first {
		args = []string{"new-session", "-d", "-s", session, "-n", literal(title)}
	} else if newWindow {
		args = []string{"new-window", "-d", "-t", "=" + session + ":", "-n", literal(title)}
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
	died := "run-shell -b " + quote(quote(exe)+" exited "+quote(state)+" '#{pane_id}'")
	_, err = run("set-option", "-p", "-t", pane, "@hypercube_id", env["HYPERCUBE_AGENT_ID"], ";",
		"set-option", "-p", "-t", pane, "@hypercube_label", strings.ReplaceAll(label, "#", "##"), ";",
		"set-option", "-w", "-t", window, "remain-on-exit", "on", ";",
		"set-option", "-w", "-t", window, "main-pane-width", "40%", ";",
		"set-option", "-w", "-t", window, "@hypercube_state", state, ";",
		"set-option", "-w", "-t", window, "@hypercube_team", team, ";",
		"set-option", "-w", "-t", window, "automatic-rename", "off", ";",
		"set-option", "-w", "-t", window, "allow-rename", "off", ";",
		"set-option", "-w", "-t", window, "extended-keys", "always", ";",
		"set-option", "-t", "="+session+":", "@hypercube_state", state, ";",
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

// ConfigureSession names the workspace and clears a session's unseen reply
// when the view switches to it.
func ConfigureSession(session, label string) error {
	target := "=" + session + ":"
	// A window becomes visible when it is selected or when a client attaches or
	// switches to this workspace; either clears its unseen reply.
	seen := "set-option -w -u @hypercube_replied"
	_, err := run("set-option", "-t", target, "@hypercube_workspace", label, ";",
		"set-hook", "-t", target, "session-window-changed", seen, ";",
		"set-hook", "-t", target, "client-session-changed", seen)
	return err
}

// Activity gives the browser view the agent's activity, status, and usage as
// pane options.
func Activity(session, pane, state string, a store.Agent) {
	if !Owns(session, pane, a.ID, state) {
		return
	}
	flag := ""
	if a.Attention {
		flag = "1"
	}
	busy := ""
	if a.Activity == store.Busy {
		busy = "1"
	}
	usage := ""
	if a.Usage != (store.Usage{}) {
		usage = fmt.Sprintf("%d %d %g", a.Usage.Context, a.Usage.Window, a.Usage.Cost)
	}
	_, _ = run("set-option", "-p", "-t", pane, "@hypercube_attention", flag, ";",
		"set-option", "-p", "-t", pane, "@hypercube_busy", busy, ";",
		"set-option", "-p", "-t", pane, "@hypercube_status", a.Status, ";",
		"set-option", "-p", "-t", pane, "@hypercube_status_at", fmt.Sprint(a.StatusAt), ";",
		"set-option", "-p", "-t", pane, "@hypercube_usage", usage)
}

// markReply flags the lead's window when the lead goes from busy to idle while
// no client is viewing that window. It must decide at reply time: a reply
// watched in place would otherwise appear unseen after the user switches away.
// Run it before Activity replaces the pane's busy flag.
func markReply(session, pane, id, state string) {
	if !Owns(session, pane, id, state) {
		return
	}
	got, err := run("display-message", "-p", "-t", pane, "#{@hypercube_busy} #{&&:#{session_attached},#{window_active}}")
	if err != nil || got != "1 0" {
		return
	}
	_, _ = run("set-option", "-w", "-t", pane, "@hypercube_replied", "1")
}

func KillSession(session string) error { _, err := run("kill-session", "-t", "="+session); return err }

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

// RenameWindow shows a session's new title.
func RenameWindow(window, title string) error {
	_, err := run("rename-window", "-t", window, "--", literal(title))
	return err
}

// literal keeps a window name as typed: tmux expands formats in window names
// and reads an argument's trailing semicolon as a command separator.
func literal(name string) string {
	name = strings.ReplaceAll(name, "#", "##")
	if strings.HasSuffix(name, ";") {
		name = strings.TrimSuffix(name, ";") + `\;`
	}
	return name
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
	session := store.Workspace(path)
	if a.Role == store.Lead && a.Activity == store.Idle {
		markReply(session, a.Target, id, path)
	}
	Activity(session, a.Target, path, *a)
}

// Dead guards against a queued pane-died hook arriving after a later respawn.
func Dead(session, pane, id, state string) bool {
	if !Owns(session, pane, id, state) {
		return false
	}
	got, err := run("display-message", "-p", "-t", pane, "#{pane_dead}")
	return err == nil && got == "1"
}
