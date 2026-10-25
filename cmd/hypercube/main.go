// Command hypercube runs a team of coding agents in tmux.
package main

import (
	"fmt"
	"os"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/hook"
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
	case len(os.Args) == 2 && os.Args[1] == "mcp":
		err = mcp.Serve(os.Stdin, os.Stdout)
	case len(os.Args) == 3 && os.Args[1] == "hook":
		err = hook.Run(os.Args[2], os.Stdin, os.Stdout)
	default:
		err = fmt.Errorf("usage: hypercube [mcp | hook <event>]")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube:", err)
		os.Exit(1)
	}
}

// start attaches to the workspace's session, creating it and launching the
// team if it is not running.
func start() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := workspace.Root(cwd)
	if err != nil {
		return err
	}
	name := workspace.Name(root)
	if tmux.HasSession(name) {
		return tmux.Attach(name)
	}
	cfg, err := harness.LoadConfig()
	if err != nil {
		return err
	}
	branch, err := workspace.Branch(root)
	if err != nil {
		return err
	}

	path := store.Path(name)
	var agents []store.Agent
	err = store.Update(path, func(s *store.State) error {
		if len(s.Agents) == 0 {
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
