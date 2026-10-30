// Command hypercube runs a team of coding agents in tmux.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	case len(os.Args) >= 2 && os.Args[1] == "new":
		err = newSessionArgs(os.Args[2:])
	case len(os.Args) == 3 && os.Args[1] == "delete-session":
		var root, name string
		root, name, err = existingWorkspaceAt(".")
		if err == nil {
			err = deleteTeam(root, store.Path(name), os.Args[2], "")
		}
	case (len(os.Args) == 4 || len(os.Args) == 6) && os.Args[1] == "window":
		err = windowAction(os.Args[2:])
		if err != nil && len(os.Args) == 6 {
			_ = exec.Command("tmux", "display-message", "-t", os.Args[5], strings.ReplaceAll(err.Error(), "#", "##")).Run()
		}
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
		err = fmt.Errorf("usage: hypercube [new [-d] [name] | delete-session name | start dir | stop [dir | --all] | delete [dir | --all] | status | running | mcp | hook <event> | wait | statusline [--limits]]")
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

// known is a workspace with a state file. Its root is the lead's
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
			if a.Role == store.Lead {
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
	lock, err := store.LockFile(path + ".session.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	if tmux.HasSession(name) && !tmux.WorkspaceSession(name, path) {
		return errors.New("refusing to stop a tmux session not owned by this workspace")
	}
	if !tmux.HasSession(name) {
		return fmt.Errorf("no session for %s", workspace.Short(root))
	}
	return tmux.KillSession(name)
}

// status prints every workspace with a state file: its directory, whether its
// session is running, its number of agents, and when it was last active.
// Every hook writes the state, so the state file's modification time is the
// last activity.
func status() error {
	type row struct {
		dir, session string
		agents       int
		modified     time.Time
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
		r := row{dir: k.name, session: "stopped", agents: len(k.state.Agents), modified: info.ModTime()}
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
	fmt.Fprintln(w, "WORKSPACE\tSESSION\tAGENTS\tACTIVE")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", r.dir, r.session, r.agents, ago(r.modified))
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
	return deleteTeam(root, store.Path(name), "", "")
}

// An empty team deletes the workspace. Otherwise only that window and its
// team's resources are removed, with the same worktree safety checks.
func deleteTeam(root, path, team, expectedWindow string) error {
	name := store.Workspace(path)
	lock, err := store.LockFile(path + ".session.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	if tmux.HasSession(name) && !tmux.WorkspaceSession(name, path) {
		return errors.New("refusing to delete a tmux session not owned by this workspace")
	}
	s, err := store.Load(path)
	if err != nil {
		return err
	}
	if team != "" {
		if s.TeamLead(team) == nil {
			return fmt.Errorf("no session %q", team)
		}
		window, err := tmux.FindWindow(name, team, path)
		if err != nil {
			return err
		}
		if expectedWindow != "" && window != expectedWindow {
			return errors.New("the window no longer belongs to this session")
		}
	}
	var workers []store.Agent
	var problems []string
	for _, a := range s.Agents {
		if a.Role != store.Worker || a.Branch == "" || team != "" && a.Team != team {
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
		return fmt.Errorf("not deleting: problem worktrees:\n  %s", strings.Join(problems, "\n  "))
	}

	if team != "" {
		if err := tmux.KillWindow(name, team, path); err != nil {
			return err
		}
	} else if tmux.HasSession(name) {
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
	if team != "" {
		empty := false
		if err := store.Update(path, func(s *store.State) error {
			var remove []string
			for _, a := range s.Agents {
				if a.Team == team {
					remove = append(remove, a.ID)
				}
			}
			for _, id := range remove {
				s.Remove(id)
			}
			empty = len(s.Agents) == 0
			return nil
		}); err != nil {
			return err
		}
		if !empty {
			return nil
		}
	}
	return store.Delete(path)
}

// start attaches to the workspace, starting its saved sessions when necessary.
func start() error { return startWorkspace(".", true) }

func startWorkspace(dir string, attach bool) error {
	return openWorkspace(dir, "", false, false, attach)
}

func newSessionArgs(args []string) error {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	detached := flags.Bool("d", false, "create the session without attaching")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("usage: hypercube new [-d] [name]")
	}
	return openWorkspace(".", flags.Arg(0), true, !*detached, !*detached)
}

var sessionName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// A workspace owns one tmux session. Each Hypercube session is a team in its
// own marked window; names and worker lifecycle are local to that team.
func openWorkspace(dir, newName string, create, selectNew, attach bool) error {
	if create && newName != "" && !sessionName.MatchString(newName) {
		return errors.New("session names must be lowercase letters, digits, and hyphens")
	}
	root, name, err := workspaceAt(dir)
	if err != nil {
		return err
	}
	path := store.Path(name)
	lock, err := store.LockFile(path + ".session.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	if tmux.HasSession(name) && !tmux.WorkspaceSession(name, path) {
		return errors.New("a tmux session with this name belongs to something else")
	}
	s, err := store.Load(path)
	if err != nil {
		return err
	}
	add := create || len(s.Agents) == 0
	if add {
		if !create {
			newName = store.DefaultTeam
		} else if newName == "" {
			for n := 1; ; n++ {
				newName = fmt.Sprintf("session-%d", n)
				if s.TeamLead(newName) == nil {
					break
				}
			}
		}
		if s.TeamLead(newName) != nil {
			return fmt.Errorf("session %q already exists; select its window with Ctrl-b w", newName)
		}
	}
	missing := map[string]bool{}
	for _, a := range s.Agents {
		if a.Role != store.Lead {
			continue
		}
		window, err := tmux.FindWindow(name, a.Team, path)
		if err != nil {
			return err
		}
		missing[a.Team] = window == ""
	}
	if add {
		missing[newName] = true
	}
	var launch []store.Agent
	needsLaunch := false
	for _, absent := range missing {
		needsLaunch = needsLaunch || absent
	}
	if needsLaunch {
		cfg, err := harness.LoadConfig()
		if err != nil {
			return err
		}
		branch, branchErr := workspace.Branch(root)
		err = store.Update(path, func(s *store.State) error {
			s.NoGit = branchErr != nil
			if add {
				o := cfg.Lead
				s.Agents = append(s.Agents, store.Agent{ID: store.NewID(), Name: "lead", Role: store.Lead,
					Harness: o.Harness, Model: o.Model, Effort: o.Effort, Dir: root, Team: newName})
			}
			for i := range s.Agents {
				a := &s.Agents[i]
				if !missing[a.Team] {
					continue
				}
				a.Target, a.QuickExits, a.Attention = "", 0, false
				if a.Role == store.Lead {
					a.Branch = branch
				}
				launch = append(launch, *a)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	sort.SliceStable(launch, func(i, j int) bool {
		return launch[i].Role == store.Lead && launch[j].Role != store.Lead
	})
	for _, a := range launch {
		prompt := mcp.Nudge
		if a.Role == store.Lead {
			prompt = ""
		}
		if _, err := harness.Launch(a, path, prompt); err != nil {
			// Keep healthy windows running and the failed team's state for retry.
			if cleanup := tmux.KillWindow(name, a.Team, path); cleanup != nil {
				return fmt.Errorf("launching %s: %w; closing its window: %v", a.Name, err, cleanup)
			}
			return fmt.Errorf("launching %s in session %s: %w", a.Name, a.Team, err)
		}
	}
	if len(launch) > 0 {
		if err := tmux.ConfigureSession(name, filepath.Base(root)); err != nil {
			return err
		}
	}
	if create {
		window, err := tmux.FindWindow(name, newName, path)
		if err != nil {
			return err
		}
		if window == "" {
			return fmt.Errorf("session %q has no tmux window", newName)
		}
		if selectNew {
			if err := tmux.SelectWindow(window); err != nil {
				return err
			}
		}
	}
	lock.Close()
	if attach {
		return tmux.Attach(name)
	}
	return nil
}

// These commands run from tmux bindings, using its workspace marker instead
// of a worker's current directory.
func windowAction(args []string) error {
	s, err := store.Load(args[1])
	if err != nil {
		return err
	}
	for _, a := range s.Agents {
		if a.Role != store.Lead {
			continue
		}
		if args[0] == "new" && len(args) == 2 {
			return openWorkspace(a.Dir, "", true, true, false)
		}
		if args[0] == "delete" && len(args) == 4 && a.Team == args[2] {
			return deleteTeam(a.Dir, args[1], args[2], args[3])
		}
	}
	return errors.New("no matching Hypercube session")
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
	lock, err := store.LockFile(path + ".session.lock")
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
