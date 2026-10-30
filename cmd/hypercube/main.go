// Command hypercube runs a team of coding agents in tmux.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/hook"
	"github.com/kkestell/hypercube/internal/mcp"
	"github.com/kkestell/hypercube/internal/statusline"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/tmux"
	"github.com/kkestell/hypercube/internal/wait"
	"github.com/kkestell/hypercube/internal/workspace"
)

func main() {
	var err error
	switch {
	case len(os.Args) == 1:
		err = start()
	case len(os.Args) <= 3 && (os.Args[1] == "stop" || os.Args[1] == "delete"):
		err = stopOrDelete(os.Args[1], os.Args[2:])
	case len(os.Args) == 2 && os.Args[1] == "status":
		err = status()
	case len(os.Args) == 3 && os.Args[1] == "start":
		err = startWorkspace(os.Args[2], false)
	case len(os.Args) == 2 && os.Args[1] == "running":
		err = running()
	case (len(os.Args) == 2 || len(os.Args) == 3 && os.Args[2] == "--limits") && os.Args[1] == "statusline":
		err = statusline.Run(len(os.Args) == 3, os.Stdin, os.Stdout)
	case len(os.Args) == 2 && os.Args[1] == "wait":
		var delivered bool
		delivered, err = wait.Run(os.Stdin, os.Stderr)
		if err == nil && delivered {
			os.Exit(2)
		}
	case len(os.Args) == 3 && os.Args[1] == "agent":
		err = runAgent(os.Args[2])
	case len(os.Args) == 5 && os.Args[1] == "exited":
		err = exited(os.Args[2], os.Args[3], os.Args[4])
	case len(os.Args) == 2 && os.Args[1] == "mcp":
		err = mcp.Serve(os.Stdin, os.Stdout)
	case len(os.Args) == 3 && os.Args[1] == "hook":
		err = hook.Run(os.Args[2], os.Stdin, os.Stdout)
	default:
		err = fmt.Errorf("usage: hypercube [start dir | stop [dir | --all] | delete [dir | --all] | status | running | mcp | hook <event> | wait | statusline [--limits]]")
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

// stopOrDelete runs stop or delete on the workspace containing the given
// directory, by default the working directory. With --all, it runs on every
// workspace it applies to, after the user confirms.
func stopOrDelete(cmd string, args []string) error {
	run := stop
	if cmd == "delete" {
		run = deleteWorkspace
	}
	if len(args) == 0 || args[0] != "--all" {
		dir := "."
		if len(args) == 1 {
			dir = args[0]
		}
		root, name, err := existingWorkspaceAt(dir)
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
		if cmd == "delete" || tmux.HasSession(k.name) {
			targets = append(targets, k)
		}
	}
	if len(targets) == 0 {
		fmt.Printf("No workspaces to %s.\n", cmd)
		return nil
	}
	for _, k := range targets {
		fmt.Println(workspace.Short(k.root))
	}
	fmt.Printf("%s all of these? [y/N] ", strings.ToUpper(cmd[:1])+cmd[1:])
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
	path := store.Path(name)
	lock, err := store.LockFile(path+".session.lock", true)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := legacyStopped(path); err != nil {
		return err
	}
	if tmux.HasSession(name) && !tmux.WorkspaceSession(name, path) {
		return errors.New("refusing to stop a tmux session not owned by this workspace")
	}
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

// deleteWorkspace stops the workspace and deletes its state, worker worktrees, and
// worker branches. It refuses, changing nothing, if a worker has uncommitted
// changes in its worktree or an unmerged branch. Workers without worktrees
// have their directories left alone.
func deleteWorkspace(root, name string) error {
	path := store.Path(name)
	lock, err := store.LockFile(path+".session.lock", true)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := legacyStopped(path); err != nil {
		return err
	}
	if tmux.HasSession(name) && !tmux.WorkspaceSession(name, path) {
		return errors.New("refusing to delete a tmux session not owned by this workspace")
	}
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
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := workspace.Removable(root, a.Dir, a.Branch); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", a.Name, err))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("not deleting the workspace:\n  %s", strings.Join(problems, "\n  "))
	}

	if tmux.HasSession(name) {
		if err := tmux.KillSession(name); err != nil {
			return err
		}
	}
	// A worker can change its worktree before the session ends. Keep such a
	// worker in the state so that delete can be run again after it is fixed.
	for _, a := range workers {
		if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
			if err := workspace.RemoveWorktree(root, a.Dir, a.Branch); err != nil {
				return fmt.Errorf("%s: %w", a.Name, err)
			}
		}
		err := store.Update(path, func(s *store.State) error { s.Remove(a.ID); return nil })
		if err != nil {
			return err
		}
	}
	return store.Delete(path)
}

// start attaches to the workspace, starting its team when necessary.
func start() error { return startWorkspace(".", true) }

func startWorkspace(dir string, attach bool) error {
	root, name, err := workspaceAt(dir)
	if err != nil {
		return err
	}
	path := store.Path(name)
	lock, err := store.LockFile(path+".session.lock", true)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := legacyStopped(path); err != nil {
		return err
	}
	if tmux.HasSession(name) {
		if !tmux.WorkspaceSession(name, path) {
			return errors.New("a tmux session with this name belongs to something else")
		}
		lock.Close()
		if attach {
			return tmux.Attach(name)
		}
		return nil
	}
	cfg, err := harness.LoadConfig()
	if err != nil {
		return err
	}
	branch, branchErr := workspace.Branch(root)
	var agents []store.Agent
	err = store.Update(path, func(s *store.State) error {
		if err := oneTeam(s); err != nil {
			return err
		}
		s.NoGit = branchErr != nil
		if len(s.Agents) == 0 {
			o := cfg.Orchestrator
			s.Agents = append(s.Agents, store.Agent{ID: store.NewID(), Name: "orchestrator", Role: store.Orchestrator,
				Harness: o.Harness, Model: o.Model, Effort: o.Effort, Dir: root, Team: store.DefaultTeam})
		}
		for i := range s.Agents {
			a := &s.Agents[i]
			a.Target, a.QuickExits, a.Attention = "", 0, false
			if a.Role == store.Orchestrator {
				a.Branch = branch
			}
		}
		agents = append([]store.Agent(nil), s.Agents...)
		return nil
	})
	if err != nil {
		return err
	}
	sort.SliceStable(agents, func(i, j int) bool {
		return agents[i].Role == store.Orchestrator && agents[j].Role != store.Orchestrator
	})
	for _, a := range agents {
		prompt := mcp.Nudge
		if a.Role == store.Orchestrator {
			prompt = ""
		}
		if _, err := harness.Launch(a, path, prompt); err != nil {
			// A partially launched team must not remain hidden after a failed start.
			if tmux.WorkspaceSession(name, path) {
				_ = tmux.KillSession(name)
			}
			return fmt.Errorf("launching %s: %w", a.Name, err)
		}
	}
	if err := tmux.ConfigureSession(name, workspace.Short(root)); err != nil {
		_ = tmux.KillSession(name)
		return err
	}
	lock.Close()
	if attach {
		return tmux.Attach(name)
	}
	return nil
}

func oneTeam(s *store.State) error {
	if len(s.Tasks) != 0 {
		return errors.New("this workspace has scheduled tasks; this version supports one team without scheduled tasks; its saved state was left unchanged")
	}
	leads := 0
	for _, a := range s.Agents {
		if a.TeamOf() != store.DefaultTeam {
			return fmt.Errorf("this workspace has another team (%s); this version supports one team; its saved state was left unchanged", a.TeamOf())
		}
		if a.Role == store.Orchestrator {
			leads++
		}
	}
	if len(s.Agents) != 0 && leads != 1 {
		return errors.New("a workspace must have exactly one orchestrator")
	}
	return nil
}

// Avoid running two implementations against the same live workspace state.
func legacyStopped(path string) error {
	hostLock := strings.TrimSuffix(path, ".json") + ".host.lock"
	if _, err := os.Stat(hostLock); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	lock, err := store.LockFile(hostLock, false)
	if err != nil {
		return err
	}
	if lock == nil {
		return errors.New("the workspace's previous host is still running; stop it with the installed binary before starting this version")
	}
	return lock.Close()
}

// runAgent replaces the wrapper with the harness. tmux owns its PTY and process
// lifetime; the pane-died hook handles relaunches using the saved conversation.
func runAgent(prompt string) error {
	id, path, pane := os.Getenv("HYPERCUBE_AGENT_ID"), os.Getenv("HYPERCUBE_STATE"), os.Getenv("TMUX_PANE")
	if id == "" || path == "" || !tmux.Owns(store.Workspace(path), pane, id, path) {
		return errors.New("agent must run in its own Hypercube pane")
	}
	var agent store.Agent
	err := store.Update(path, func(s *store.State) error {
		a, err := s.Agent(id)
		if err != nil {
			return err
		}
		a.Target, a.Launched, a.Attention = pane, time.Now().UnixMilli(), false
		a.Activity = store.Idle
		if prompt != "" {
			a.Activity = store.Busy
		}
		agent = *a
		return nil
	})
	if err != nil {
		return err
	}
	argv, env, err := harness.Command(agent, path, prompt)
	if err == nil {
		var binary string
		binary, err = exec.LookPath(argv[0])
		if err == nil {
			tmux.Activity(store.Workspace(path), pane, id, path, agent.Activity, false)
			var inherited []string
			for _, item := range os.Environ() {
				key, _, _ := strings.Cut(item, "=")
				if _, replaced := env[key]; !replaced {
					inherited = append(inherited, item)
				}
			}
			for key, value := range env {
				inherited = append(inherited, key+"="+value)
			}
			err = syscall.Exec(binary, argv, inherited)
		}
	}
	_ = store.Update(path, func(s *store.State) error {
		a, e := s.Agent(id)
		if e == nil {
			a.Activity = store.Exited
		}
		return e
	})
	return err
}

func exited(path, pane, status string) error {
	if _, err := strconv.Atoi(status); err != nil {
		return err
	}
	lock, err := store.LockFile(path+".session.lock", true)
	if err != nil {
		return err
	}
	defer lock.Close()
	if !tmux.WorkspaceSession(store.Workspace(path), path) {
		return nil
	}
	var agent store.Agent
	restart := false
	err = store.Update(path, func(s *store.State) error {
		for i := range s.Agents {
			a := &s.Agents[i]
			if a.Target != pane || !tmux.Dead(store.Workspace(path), pane, a.ID, path) {
				continue
			}
			if a.Activity == store.Exited {
				return nil
			}
			a.QuickExits++
			if time.Since(time.UnixMilli(a.Launched)) >= 10*time.Second {
				a.QuickExits = 0
			}
			a.Attention = false
			if a.QuickExits >= 3 {
				a.Activity = store.Exited
			} else {
				restart = true
			}
			agent = *a
			return nil
		}
		return nil
	})
	if err != nil {
		return err
	}
	if restart {
		if err := tmux.Restart(pane, agent.ID, path); err != nil {
			_ = store.Update(path, func(s *store.State) error {
				a, e := s.Agent(agent.ID)
				if e == nil {
					a.Activity = store.Exited
				}
				return e
			})
			return err
		}
	} else if agent.ID != "" {
		tmux.Activity(store.Workspace(path), pane, agent.ID, path, store.Exited, false)
	}
	return nil
}

func running() error {
	list, err := workspaces()
	if err != nil {
		return err
	}
	for _, k := range list {
		if tmux.WorkspaceSession(k.name, k.path) {
			fmt.Println(k.root)
		}
	}
	return nil
}

// Existing workspaces can be stopped or forgotten after their checkout moves
// or disappears; their recorded worktrees moved with that checkout.
func existingWorkspaceAt(dir string) (string, string, error) {
	root, name, err := workspaceAt(dir)
	if err == nil {
		return root, name, nil
	}
	original := err
	wd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	root, err = workspace.Spell(wd, dir)
	if err != nil {
		return "", "", err
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		return "", "", original
	}
	name = workspace.Name(root)
	if _, err := os.Stat(store.Path(name)); err != nil {
		return "", "", original
	}
	return root, name, nil
}
