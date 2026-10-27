// Command hypercube runs a team of coding agents in tmux.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/hook"
	"github.com/kkestell/hypercube/internal/limits"
	"github.com/kkestell/hypercube/internal/mcp"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/tmux"
	"github.com/kkestell/hypercube/internal/workspace"
)

func main() {
	var err error
	switch {
	case len(os.Args) == 1:
		err = start()
	case len(os.Args) <= 3 && (os.Args[1] == "stop" || os.Args[1] == "down"):
		err = stopOrDown(os.Args[1], os.Args[2:])
	case len(os.Args) == 2 && os.Args[1] == "status":
		err = status()
	case len(os.Args) == 2 && os.Args[1] == "statusline":
		err = limits.Statusline(os.Stdin, os.Stdout)
	case len(os.Args) == 2 && os.Args[1] == "limits":
		fmt.Println(limits.Render())
	case len(os.Args) == 2 && os.Args[1] == "mcp":
		err = mcp.Serve(os.Stdin, os.Stdout)
	case len(os.Args) == 3 && os.Args[1] == "hook":
		err = hook.Run(os.Args[2], os.Stdin, os.Stdout)
	default:
		err = fmt.Errorf("usage: hypercube [stop [dir | --all] | down [dir | --all] | status | mcp | hook <event> | statusline | limits]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube:", err)
		os.Exit(1)
	}
}

// workspaceAt returns the root and name of the workspace containing dir. The
// root is the top of the checkout, or dir itself outside Git.
func workspaceAt(dir string) (root, name string, err error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	if root, err = workspace.Resolve(wd, dir); err != nil {
		return "", "", err
	}
	if top, err := workspace.Root(root); err == nil {
		root = top
	}
	return root, workspace.Name(root), nil
}

// known is a workspace with a state file. Its root is the orchestrator's
// directory.
type known struct {
	root, name, path string
	state            store.State
}

// workspaces returns every workspace with a state file.
func workspaces() ([]known, error) {
	paths, err := filepath.Glob(filepath.Join(store.Dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	var list []known
	for _, path := range paths {
		s, err := store.Load(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		k := known{name: store.Workspace(path), path: path, state: s}
		for _, a := range s.Agents {
			if a.Role == store.Orchestrator {
				k.root = a.Dir
			}
		}
		list = append(list, k)
	}
	return list, nil
}

// stopOrDown runs stop or down on the workspace containing the given
// directory, by default the working directory. With --all, it runs on every
// workspace it applies to, after the user confirms.
func stopOrDown(cmd string, args []string) error {
	run := stop
	if cmd == "down" {
		run = down
	}
	if len(args) == 0 || args[0] != "--all" {
		dir := "."
		if len(args) == 1 {
			dir = args[0]
		}
		root, name, err := workspaceAt(dir)
		if err != nil {
			return err
		}
		return run(root, name)
	}

	list, err := workspaces()
	if err != nil {
		return err
	}
	var targets []known
	for _, k := range list {
		// Only running sessions can be stopped.
		if cmd == "down" || tmux.HasSession(k.name) {
			targets = append(targets, k)
		}
	}
	verb := map[string]string{"stop": "stop", "down": "take down"}[cmd]
	if len(targets) == 0 {
		fmt.Printf("No workspaces to %s.\n", verb)
		return nil
	}
	for _, k := range targets {
		fmt.Println(workspace.Short(k.root))
	}
	fmt.Printf("%s all of these? [y/N] ", strings.ToUpper(verb[:1])+verb[1:])
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
		return fmt.Errorf("%s cancelled", cmd)
	}
	var failed []string
	for _, k := range targets {
		if err := run(k.root, k.name); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", workspace.Short(k.root), err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%s failed for:\n%s", cmd, strings.Join(failed, "\n"))
	}
	return nil
}

// stop kills the workspace's session, which ends its agents.
func stop(root, name string) error {
	if !tmux.HasSession(name) {
		return fmt.Errorf("no session for %s", workspace.Short(root))
	}
	return tmux.KillSession(name)
}

// status prints every workspace with a state file: its directory, whether its
// session is running, its number of agents, when it was created, and when it
// was last active. The lock file is created with the state and never written,
// so its modification time is the creation time. Every hook writes the state,
// so the state file's modification time is the last activity.
func status() error {
	type row struct {
		dir, session      string
		agents            int
		created, modified time.Time
	}
	list, err := workspaces()
	if err != nil {
		return err
	}
	var rows []row
	for _, k := range list {
		info, err := os.Stat(k.path)
		if err != nil {
			return err
		}
		r := row{dir: k.name, session: "stopped", agents: len(k.state.Agents), created: info.ModTime(), modified: info.ModTime()}
		if lock, err := os.Stat(strings.TrimSuffix(k.path, ".json") + ".lock"); err == nil {
			r.created = lock.ModTime()
		}
		if k.root != "" {
			r.dir = workspace.Short(k.root)
		}
		if tmux.HasSession(k.name) {
			r.session = "running"
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].modified.After(rows[j].modified) })

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WORKSPACE\tSESSION\tAGENTS\tCREATED\tACTIVE")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n", r.dir, r.session, r.agents, ago(r.created), ago(r.modified))
	}
	return w.Flush()
}

// ago describes how long before now t was.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// down stops the workspace and deletes its state, worker worktrees, and
// worker branches. It refuses, changing nothing, if a worker has uncommitted
// changes in its worktree or an unmerged branch. Workers without worktrees
// have their directories left alone.
func down(root, name string) error {
	path := store.Path(name)
	s, err := store.Load(path)
	if err != nil {
		return err
	}
	var workers []store.Agent
	var problems []string
	for _, a := range s.Agents {
		if a.Role != store.Worker || a.Branch == "" {
			continue
		}
		workers = append(workers, a)
		if err := workspace.Removable(root, a.Dir, a.Branch); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", a.Name, err))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("not taking down the workspace:\n  %s", strings.Join(problems, "\n  "))
	}

	if tmux.HasSession(name) {
		if err := tmux.KillSession(name); err != nil {
			return err
		}
	}
	// A worker can change its worktree before the session ends. Keep such a
	// worker in the state so that down can be run again after it is fixed.
	for _, a := range workers {
		if err := workspace.RemoveWorktree(root, a.Dir, a.Branch); err != nil {
			return fmt.Errorf("%s: %w", a.Name, err)
		}
		err := store.Update(path, func(s *store.State) error { s.Remove(a.ID); return nil })
		if err != nil {
			return err
		}
	}
	return store.Delete(path)
}

// start attaches to the workspace's session, creating it and launching the
// team if it is not running.
func start() error {
	root, name, err := workspaceAt(".")
	if err != nil {
		return err
	}
	if tmux.HasSession(name) {
		return tmux.Attach(name)
	}
	cfg, err := harness.LoadConfig()
	if err != nil {
		return err
	}
	branch, err := workspace.Branch(root)
	repo := err == nil

	path := store.Path(name)
	var agents []store.Agent
	err = store.Update(path, func(s *store.State) error {
		if len(s.Agents) == 0 {
			s.NoGit = !repo
			o := cfg.Orchestrator
			s.Agents = append(s.Agents, store.Agent{
				ID: store.NewID(), Name: "orchestrator", Role: store.Orchestrator,
				Harness: o.Harness, Model: o.Model, Effort: o.Effort, Dir: root,
			})
		}
		for i := range s.Agents {
			a := &s.Agents[i]
			if a.Role == store.Orchestrator {
				a.Branch, a.Activity = branch, store.Idle
			} else {
				a.Activity = store.Busy
			}
		}
		agents = s.Agents
		return nil
	})
	if err != nil {
		return err
	}

	// The orchestrator is first so that it gets the left pane. Workers start a
	// turn to pick up messages queued while they were down.
	targets := map[string]string{}
	for _, a := range agents {
		prompt := mcp.Nudge
		if a.Role == store.Orchestrator {
			prompt = ""
		}
		target, err := harness.Launch(a, path, prompt)
		if err != nil {
			return fmt.Errorf("launching %s: %w", a.Name, err)
		}
		targets[a.ID] = target
	}
	status, err := harness.StatusRight()
	if err != nil {
		return err
	}
	if err := tmux.ConfigureSession(name, workspace.Short(root), status); err != nil {
		return err
	}
	err = store.Update(path, func(s *store.State) error {
		for i := range s.Agents {
			s.Agents[i].Target = targets[s.Agents[i].ID]
		}
		return nil
	})
	if err != nil {
		return err
	}
	return tmux.Attach(name)
}
