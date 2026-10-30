// Command hypercube runs a team of coding agents in the mux.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/hook"
	"github.com/kkestell/hypercube/internal/mcp"
	"github.com/kkestell/hypercube/internal/mux"
	"github.com/kkestell/hypercube/internal/statusline"
	"github.com/kkestell/hypercube/internal/store"
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
			err = deleteTeam(root, store.Path(name), os.Args[2])
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
		var code int
		code, err = runAgent(os.Args[2])
		if err == nil {
			// A harness killed by a signal reports -1, which the pane shows as 255.
			os.Exit(code)
		}
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
		if cmd == "delete" {
			targets = append(targets, k)
			continue
		}
		session, err := mux.Workspace(k.state)
		if err != nil {
			return err
		}
		if session != "" {
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
	s, err := store.Load(path)
	if err != nil {
		return err
	}
	session, err := mux.Workspace(s)
	if err != nil {
		return err
	}
	if session == "" {
		return fmt.Errorf("no session for %s", workspace.Short(root))
	}
	return mux.Kill(session)
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
		session, err := mux.Workspace(k.state)
		if err != nil {
			return err
		}
		if session != "" {
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
	return deleteTeam(root, store.Path(name), "")
}

// An empty team deletes the workspace. Otherwise only that window and its
// team's resources are removed, with the same worktree safety checks.
func deleteTeam(root, path, team string) error {
	lock, err := store.LockFile(path + ".session.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	s, err := store.Load(path)
	if err != nil {
		return err
	}
	if team != "" && s.TeamLead(team) == nil {
		return fmt.Errorf("no session %q", team)
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

	var running string
	if team != "" {
		running, err = mux.Team(s, team)
	} else {
		running, err = mux.Workspace(s)
	}
	if err != nil {
		return err
	}
	if err := mux.Kill(running); err != nil {
		return err
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

// A workspace owns one mux session. Each Hypercube session is a team in its
// own window; names and worker lifecycle are local to that team.
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
			return fmt.Errorf("session %q already exists", newName)
		}
	}
	session, err := mux.Workspace(s)
	if err != nil {
		return err
	}
	windows := map[string]string{}
	missing := map[string]bool{}
	for _, a := range s.Agents {
		if a.Role != store.Lead {
			continue
		}
		windows[a.Team], err = mux.Team(s, a.Team)
		if err != nil {
			return err
		}
		missing[a.Team] = windows[a.Team] == ""
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
				a.Target, a.Attention = "", false
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
		prompt, launchSession, window := mcp.Nudge, "", windows[a.Team]
		if a.Role == store.Lead {
			prompt, launchSession, window = "", session, ""
		}
		reply, err := harness.Launch(a, path, prompt, launchSession, window)
		if a.Role == store.Lead {
			// The reply names the window even when the launch fails after
			// spawning, so the cleanup below closes it.
			windows[a.Team] = reply.Window
			if err == nil {
				err = mux.Label(reply.Window, a.Team)
			}
			if err == nil && session == "" {
				session = reply.Session
				err = mux.Label(session, filepath.Base(root))
			}
		}
		if err != nil {
			// Keep healthy windows running and the failed team's state for retry.
			if cleanup := mux.Kill(windows[a.Team]); cleanup != nil {
				return fmt.Errorf("launching %s: %w; closing its window: %v", a.Name, err, cleanup)
			}
			return fmt.Errorf("launching %s in session %s: %w", a.Name, a.Team, err)
		}
	}
	if create && selectNew {
		if err := mux.Select(windows[newName]); err != nil {
			return err
		}
	}
	lock.Close()
	if attach {
		return mux.Attach(session)
	}
	return nil
}

// runAgent runs the harness as a child and relaunches it with its saved
// conversation until it exits quickly three times in a row.
func runAgent(prompt string) (int, error) {
	id, path := os.Getenv("HYPERCUBE_AGENT_ID"), os.Getenv("HYPERCUBE_STATE")
	if id == "" || path == "" {
		return 0, errors.New("agent must run in its own Hypercube pane")
	}
	// Ctrl-C reaches the harness through the process group. Catch it here
	// rather than ignore it: an ignored signal stays ignored in the child.
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	quickExits := 0
	for {
		var agent store.Agent
		err := store.Update(path, func(s *store.State) error {
			a, err := s.Agent(id)
			if err != nil {
				return err
			}
			a.Attention = false
			a.Activity = store.Idle
			if prompt != "" {
				a.Activity = store.Busy
			}
			agent = *a
			return nil
		})
		if err != nil {
			return 0, err
		}
		argv, env, err := harness.Command(agent, path, prompt)
		if err != nil {
			markExited(path, id)
			return 0, err
		}
		mux.RefreshAttention(path, id)
		cmd := exec.Command(argv[0], argv[1:]...)
		for _, item := range os.Environ() {
			key, _, _ := strings.Cut(item, "=")
			if _, replaced := env[key]; !replaced {
				cmd.Env = append(cmd.Env, item)
			}
		}
		for key, value := range env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		started := time.Now()
		err = cmd.Run()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			markExited(path, id)
			return 0, err
		}
		quickExits++
		if time.Since(started) >= 10*time.Second {
			quickExits = 0
		}
		if quickExits >= 3 {
			markExited(path, id)
			mux.RefreshAttention(path, id)
			return cmd.ProcessState.ExitCode(), nil
		}
	}
}

func markExited(path, id string) {
	_ = store.Update(path, func(s *store.State) error {
		a, err := s.Agent(id)
		if err == nil {
			a.Activity, a.Attention = store.Exited, false
		}
		return err
	})
}

func running() error {
	list, err := workspaces()
	if err != nil {
		return err
	}
	for _, k := range list {
		session, err := mux.Workspace(k.state)
		if err != nil {
			return err
		}
		if session != "" {
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
