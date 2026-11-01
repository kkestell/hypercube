// Package session owns the lifecycle of workspaces, sessions, and workers:
// adding and deleting them, and starting and stopping a session's tmux
// session. Each operation on a session holds that session's lock.
package session

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/tmux"
	"github.com/kkestell/hypercube/internal/workspace"
)

// Nudge is the initial prompt for a worker; later messages use harness wake-ups.
const Nudge = "Check your Hypercube messages."

// AddWorkspace adds the workspace at dir: the top of its Git checkout, or dir
// itself outside Git. It returns the existing workspace when the folder is
// already added.
func AddWorkspace(db *store.DB, dir string) (store.Workspace, error) {
	root, err := workspace.At("/", dir)
	if err != nil {
		return store.Workspace{}, err
	}
	_, gitErr := workspace.Root(root)
	var w store.Workspace
	err = db.Tx(func(tx *store.Tx) error {
		list, err := tx.Workspaces()
		if err != nil {
			return err
		}
		for _, existing := range list {
			if existing.Path == root {
				w = existing
				return nil
			}
		}
		w = store.Workspace{ID: store.NewID(), Path: root, NoGit: gitErr != nil}
		return tx.AddWorkspace(w)
	})
	return w, err
}

// Create adds a session to the workspace with a lead from the named lead
// profile, or the first one, and starts it.
func Create(db *store.DB, workspaceID, lead string) (store.Session, error) {
	cfg, err := harness.LoadConfig()
	if err != nil {
		return store.Session{}, err
	}
	profile, err := cfg.Lead(lead)
	if err != nil {
		return store.Session{}, err
	}
	s := store.Session{ID: store.NewID(), Workspace: workspaceID, Running: true}
	err = db.Tx(func(tx *store.Tx) error {
		w, err := tx.Workspace(workspaceID)
		if err != nil {
			return err
		}
		if err := tx.AddSession(s); err != nil {
			return err
		}
		return tx.AddAgent(store.Agent{ID: store.NewID(), Session: s.ID, Name: "lead", Role: store.Lead,
			Harness: profile.Harness, Model: profile.Model, Effort: profile.Effort, Dir: w.Path, Activity: store.Idle})
	})
	if err != nil {
		return s, err
	}
	return s, start(db, s.ID, true)
}

// Start launches a stopped session's agents, resuming their conversations.
// They deliver no queued messages until the user prompts the session.
func Start(db *store.DB, id string) error {
	return start(db, id, false)
}

func start(db *store.DB, id string, fresh bool) error {
	lock, err := store.Lock(id)
	if err != nil {
		return err
	}
	defer lock.Close()
	if tmux.Running(id) {
		return nil
	}
	var root string
	err = db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		w, err := tx.Workspace(s.Workspace)
		root = w.Path
		return err
	})
	if err != nil {
		return err
	}
	branch, _ := workspace.Branch(root)
	var launch []store.Agent
	err = db.Tx(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		s.Running, s.AwaitingUser = true, !fresh
		if err := tx.SaveSession(s); err != nil {
			return err
		}
		agents, err := tx.Agents(id)
		if err != nil {
			return err
		}
		for _, a := range agents {
			a.Pane, a.Attention, a.QuickExits = "", false, 0
			if a.Role == store.Lead {
				a.Branch = branch
			}
			if err := tx.SaveAgent(a); err != nil {
				return err
			}
			launch = append(launch, a)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Agents lists the lead first, and the lead creates the tmux session.
	for _, a := range launch {
		if _, err := harness.Launch(a, ""); err != nil {
			if cleanup := tmux.KillSession(id); cleanup != nil {
				return fmt.Errorf("launching %s: %w; closing the session: %v", a.Name, err, cleanup)
			}
			return fmt.Errorf("launching %s: %w", a.Name, err)
		}
	}
	return nil
}

// Stop closes the session's tmux session, which ends its agents. Their
// conversations and queued messages stay for the next Start.
func Stop(db *store.DB, id string) error {
	lock, err := store.Lock(id)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := tmux.KillSession(id); err != nil {
		return err
	}
	return db.Tx(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		s.Running = false
		if err := tx.SaveSession(s); err != nil {
			return err
		}
		agents, err := tx.Agents(id)
		if err != nil {
			return err
		}
		for _, a := range agents {
			a.Activity, a.Attention, a.Pane = store.Exited, false, ""
			if err := tx.SaveAgent(a); err != nil {
				return err
			}
		}
		return nil
	})
}

// Delete stops the session and deletes it with its workers' worktrees and
// branches. It refuses, changing nothing, if a worker has uncommitted changes
// in its worktree or an unmerged branch. Workers without worktrees have their
// directories left alone.
func Delete(db *store.DB, id string) error {
	lock, err := store.Lock(id)
	if err != nil {
		return err
	}
	defer lock.Close()
	var root string
	var workers []store.Agent
	err = db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		w, err := tx.Workspace(s.Workspace)
		if err != nil {
			return err
		}
		root = w.Path
		agents, err := tx.Agents(id)
		for _, a := range agents {
			if a.Role == store.Worker && a.Branch != "" {
				workers = append(workers, a)
			}
		}
		return err
	})
	if err != nil {
		return err
	}
	// A checkout that moved or disappeared took its worktrees with it.
	_, statErr := os.Stat(root)
	present := !errors.Is(statErr, os.ErrNotExist)
	var problems []string
	for _, a := range workers {
		if !present {
			continue
		}
		if err := workspace.Removable(root, a.Dir, a.Branch); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", a.Name, err))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("not deleting: problem worktrees:\n  %s", strings.Join(problems, "\n  "))
	}
	if err := tmux.KillSession(id); err != nil {
		return err
	}
	// A worker can change its worktree before the session ends. Such a worker
	// stays, so that delete can run again after it is fixed.
	for _, a := range workers {
		if present {
			if err := workspace.RemoveWorktree(root, a.Dir, a.Branch); err != nil {
				return fmt.Errorf("%s: %w", a.Name, err)
			}
		}
		if err := db.Tx(func(tx *store.Tx) error { return tx.DeleteAgent(a.ID) }); err != nil {
			return err
		}
	}
	return db.Tx(func(tx *store.Tx) error { return tx.DeleteSession(id) })
}

// DeleteWorkspace deletes each of the workspace's sessions, then the
// workspace.
func DeleteWorkspace(db *store.DB, id string) error {
	var sessions []store.Session
	err := db.Read(func(tx *store.Tx) error {
		var err error
		sessions, err = tx.Sessions(id)
		return err
	})
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if err := Delete(db, s.ID); err != nil {
			return err
		}
	}
	return db.Tx(func(tx *store.Tx) error { return tx.DeleteWorkspace(id) })
}

// Rename changes a session's title. Its ID, and so its agents' addresses,
// stay the same.
func Rename(db *store.DB, id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("a session needs a title")
	}
	return update(db, id, func(s *store.Session) { s.Title = title })
}

// Seen clears the session's unseen reply.
func Seen(db *store.DB, id string) error {
	return update(db, id, func(s *store.Session) { s.Unseen = false })
}

func update(db *store.DB, id string, change func(*store.Session)) error {
	return db.Tx(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		change(&s)
		return tx.SaveSession(s)
	})
}

// Restore starts every session the user left running whose tmux session is
// missing, as after a login. One failure does not stop the others.
func Restore(db *store.DB) {
	var sessions []store.Session
	err := db.Read(func(tx *store.Tx) error {
		var err error
		sessions, err = tx.Sessions("")
		return err
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "restore sessions:", err)
		return
	}
	for _, s := range sessions {
		if !s.Running || tmux.Running(s.ID) {
			continue
		}
		if err := Start(db, s.ID); err != nil {
			fmt.Fprintf(os.Stderr, "restore session %s: %v\n", s.ID, err)
		}
	}
}

// AddWorker adds a worker to the lead's running session and launches it with
// an optional first message. Without dir, the worker gets its own branch and
// worktree from HEAD; with dir, it works directly in that directory.
func AddWorker(db *store.DB, leadID, name string, profile harness.Profile, dir, message string) (store.Agent, error) {
	var lead store.Agent
	if err := db.Read(func(tx *store.Tx) error {
		var err error
		lead, err = tx.Agent(leadID)
		return err
	}); err != nil {
		return store.Agent{}, err
	}
	if lead.Role != store.Lead {
		return store.Agent{}, errors.New("only the lead can do this")
	}
	lock, err := store.Lock(lead.Session)
	if err != nil {
		return store.Agent{}, err
	}
	defer lock.Close()
	if !tmux.Running(lead.Session) {
		return store.Agent{}, errors.New("the session is not running")
	}
	a := store.Agent{
		ID: store.NewID(), Session: lead.Session, Name: name, Role: store.Worker,
		Harness: profile.Harness, Model: profile.Model, Effort: profile.Effort, Activity: store.Busy,
	}
	root := lead.Dir
	err = db.Tx(func(tx *store.Tx) error {
		s, err := tx.Session(lead.Session)
		if err != nil {
			return err
		}
		w, err := tx.Workspace(s.Workspace)
		if err != nil {
			return err
		}
		if _, err := tx.Find(lead.Session, name); err == nil {
			return fmt.Errorf("an agent named %s already exists", name)
		}
		switch {
		case dir != "":
			if a.Dir, err = workspace.Resolve(root, dir); err != nil {
				return err
			}
		case w.NoGit:
			return errors.New("dir is required: this workspace is not a Git checkout, so workers cannot have worktrees")
		default:
			a.Dir, a.Branch = workspace.WorktreeDir(root, a.Address()), workspace.WorkerBranch(a.Address())
		}
		if err := tx.AddAgent(a); err != nil {
			return err
		}
		if message != "" {
			return tx.Queue(store.Message{From: leadID, To: a.ID, Text: message})
		}
		return nil
	})
	if err != nil {
		return store.Agent{}, err
	}
	worktreeCreated := false
	if a.Branch != "" {
		err = workspace.AddWorktree(root, a.Dir, a.Branch)
		worktreeCreated = err == nil
	}
	var pane string
	if err == nil {
		pane, err = harness.Launch(a, Nudge)
	}
	if err == nil {
		return a, nil
	}
	cause := err
	if pane != "" && tmux.Owns(pane, a.ID) {
		if closeErr := tmux.KillPane(pane); closeErr != nil {
			return a, fmt.Errorf("%w; closing the pane failed: %v; agent %s was kept for cleanup", cause, closeErr, a.Name)
		}
	}
	if worktreeCreated {
		if cleanup := workspace.RemoveWorktree(root, a.Dir, a.Branch); cleanup != nil {
			return a, fmt.Errorf("%w; worktree cleanup failed: %v; agent %s was kept for cleanup", cause, cleanup, a.Name)
		}
	}
	if cleanup := db.Tx(func(tx *store.Tx) error { return tx.DeleteAgent(a.ID) }); cleanup != nil {
		return a, fmt.Errorf("%w; removing the agent failed: %v", cause, cleanup)
	}
	return a, cause
}

// RemoveWorker closes a worker's pane and removes it from the lead's session.
// A worker with a worktree also has its worktree and branch removed, which
// Git refuses for dirty worktrees and unmerged branches.
func RemoveWorker(db *store.DB, leadID, name string) (store.Agent, error) {
	var lead, a store.Agent
	read := func() error {
		return db.Read(func(tx *store.Tx) error {
			var err error
			if lead, err = tx.Agent(leadID); err != nil {
				return err
			}
			if lead.Role != store.Lead {
				return errors.New("only the lead can do this")
			}
			a, err = tx.Find(lead.Session, name)
			return err
		})
	}
	if err := read(); err != nil {
		return a, err
	}
	lock, err := store.Lock(lead.Session)
	if err != nil {
		return a, err
	}
	defer lock.Close()
	if err := read(); err != nil {
		return a, err
	}
	switch {
	case a.Session != lead.Session:
		return a, errors.New("only workers in your session can be removed")
	case a.Role != store.Worker:
		return a, errors.New("only workers can be removed")
	case !tmux.Running(lead.Session):
		return a, errors.New("the session is not running")
	}
	if a.Branch != "" {
		if err := workspace.RemoveWorktree(lead.Dir, a.Dir, a.Branch); err != nil {
			return a, err
		}
	}
	pane, err := tmux.FindPane(a.Session, a.ID)
	if err != nil {
		return a, err
	}
	if pane != "" {
		if err := tmux.KillPane(pane); err != nil {
			return a, fmt.Errorf("closing %s: %w", a.Name, err)
		}
	}
	return a, db.Tx(func(tx *store.Tx) error { return tx.DeleteAgent(a.ID) })
}

// Exited relaunches an agent whose pane died, resuming its conversation, unless
// it exited three times within ten seconds of launching.
func Exited(db *store.DB, agentID, pane string) error {
	var a store.Agent
	if err := db.Read(func(tx *store.Tx) error {
		var err error
		a, err = tx.Agent(agentID)
		return err
	}); err != nil {
		return err
	}
	lock, err := store.Lock(a.Session)
	if err != nil {
		return err
	}
	defer lock.Close()
	if !tmux.Running(a.Session) || !tmux.Dead(pane, agentID) {
		return nil
	}
	restart := false
	err = db.Tx(func(tx *store.Tx) error {
		a, err := tx.Agent(agentID)
		if err != nil || a.Pane != pane || a.Activity == store.Exited {
			return err
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
		return tx.SaveAgent(a)
	})
	if err != nil || !restart {
		return err
	}
	if err := tmux.Restart(pane, agentID); err != nil {
		_ = db.Tx(func(tx *store.Tx) error {
			a, e := tx.Agent(agentID)
			if e != nil {
				return e
			}
			a.Activity = store.Exited
			return tx.SaveAgent(a)
		})
		return err
	}
	return nil
}
