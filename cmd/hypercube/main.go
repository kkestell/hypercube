// Command hypercube runs a team of coding agents in tmux.
package main

import (
	"fmt"
	"os"
	"strings"

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
	case len(os.Args) == 2 && os.Args[1] == "stop":
		err = stop()
	case len(os.Args) == 2 && os.Args[1] == "down":
		err = down()
	case len(os.Args) == 2 && os.Args[1] == "statusline":
		err = limits.Statusline(os.Stdin, os.Stdout)
	case len(os.Args) == 2 && os.Args[1] == "limits":
		fmt.Println(limits.Render())
	case len(os.Args) == 2 && os.Args[1] == "mcp":
		err = mcp.Serve(os.Stdin, os.Stdout)
	case len(os.Args) == 3 && os.Args[1] == "hook":
		err = hook.Run(os.Args[2], os.Stdin, os.Stdout)
	default:
		err = fmt.Errorf("usage: hypercube [stop | down | mcp | hook <event> | statusline | limits]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube:", err)
		os.Exit(1)
	}
}

// current returns the root and name of the workspace containing the working
// directory. The root is the top of the checkout, or the working directory
// itself outside Git.
func current() (root, name string, err error) {
	root, err = os.Getwd()
	if err != nil {
		return "", "", err
	}
	if workspace.IsRepo(root) {
		if root, err = workspace.Root(root); err != nil {
			return "", "", err
		}
	}
	return root, workspace.Name(root), nil
}

// stop kills the workspace's session, which ends its agents.
func stop() error {
	_, name, err := current()
	if err != nil {
		return err
	}
	if !tmux.HasSession(name) {
		return fmt.Errorf("no session for this workspace")
	}
	return tmux.KillSession(name)
}

// down stops the workspace and deletes its state, worker worktrees, and
// worker branches. It refuses, changing nothing, if a worker has uncommitted
// changes or an unmerged branch. A workspace outside Git has no worktrees, so
// down leaves its workers' directories alone.
func down() error {
	root, name, err := current()
	if err != nil {
		return err
	}
	path := store.Path(name)
	s, err := store.Load(path)
	if err != nil {
		return err
	}
	var workers []store.Agent
	var problems []string
	for _, a := range s.Agents {
		if a.Role != store.Worker || s.NoGit {
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
	root, name, err := current()
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
	repo := workspace.IsRepo(root)
	var branch string
	if repo {
		if branch, err = workspace.Branch(root); err != nil {
			return err
		}
	}

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
