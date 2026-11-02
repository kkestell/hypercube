// Package session owns the lifecycle of workspaces, sessions, and workers:
// adding and deleting them, and starting and stopping a session's agents. The
// Runtime in runtime.go runs the agents.
package session

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// AddWorkspace adds the workspace at dir: the top of its Git checkout, or dir
// itself outside Git. It returns the existing workspace when the folder is
// already added.
func (r *Runtime) AddWorkspace(dir string) (store.Workspace, error) {
	root, err := workspace.At(dir)
	if err != nil {
		return store.Workspace{}, err
	}
	_, gitErr := workspace.Root(root)
	var w store.Workspace
	err = r.save(func(tx *write) error {
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
// profile, or the first one, starts it, and sends the lead the first prompt,
// which titles the session.
func (r *Runtime) Create(workspaceID, lead, text string, images ...harness.Image) (store.Session, store.Agent, error) {
	if err := checkPrompt(text, images); err != nil {
		return store.Session{}, store.Agent{}, err
	}
	cfg, err := harness.LoadConfig()
	if err != nil {
		return store.Session{}, store.Agent{}, err
	}
	profile, err := cfg.Lead(lead)
	if err != nil {
		return store.Session{}, store.Agent{}, err
	}
	s := store.Session{ID: store.NewID(), Workspace: workspaceID, Title: report.Title(text), Running: true}
	a := store.Agent{ID: store.NewID(), Session: s.ID, Name: "lead", Role: store.Lead,
		Harness: profile.Harness, Model: profile.Model, Effort: profile.Effort, Activity: store.Idle}
	err = r.save(func(tx *write) error {
		w, err := tx.Workspace(workspaceID)
		if err != nil {
			return err
		}
		if err := tx.AddSession(s); err != nil {
			return err
		}
		a.Dir = w.Path
		return tx.AddAgent(a)
	})
	if err != nil {
		return s, a, err
	}
	if err := r.start(s.ID, true); err != nil {
		return s, a, err
	}
	return s, a, r.Input(a.ID, text, images...)
}

// Start launches a stopped session's agents, resuming their conversations.
// They deliver no queued messages until the user prompts the session.
func (r *Runtime) Start(id string) error {
	return r.start(id, false)
}

func (r *Runtime) start(id string, fresh bool) error {
	var root string
	err := r.db.Read(func(tx *store.Tx) error {
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
	// The agents are registered before any plugin starts, so a second start
	// finds the session running.
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		return errClosing
	}
	if r.sessionLive(id) {
		r.mu.Unlock()
		return nil
	}
	var lives []*live
	var agents []store.Agent
	err = r.save(func(tx *write) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		s.Running, s.AwaitingUser = true, !fresh
		if err := tx.SaveSession(s); err != nil {
			return err
		}
		if err := tx.TouchSession(id, time.Now().UnixMilli()); err != nil {
			return err
		}
		agents, err = tx.Agents(id)
		if err != nil {
			return err
		}
		for i := range agents {
			a := &agents[i]
			a.Activity, a.Attention = store.Exited, false
			if a.Role == store.Lead {
				a.Branch = branch
			}
			if err := tx.SaveAgent(*a); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		for _, a := range agents {
			lives = append(lives, r.register(a, 0))
		}
	}
	r.mu.Unlock()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	errs := make([]error, len(lives))
	for i, l := range lives {
		wg.Go(func() {
			if err := r.launch(l); err != nil && !errors.Is(err, errStopped) {
				r.failed(l, err)
				errs[i] = fmt.Errorf("launching %s: %w", agents[i].Name, err)
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// Stop ends the session's agents. Their conversations and queued messages stay
// for the next Start.
func (r *Runtime) Stop(id string) error {
	r.mu.Lock()
	lives, err := r.stopLocked(id)
	r.mu.Unlock()
	stopClients(lives)
	r.Terminals.Stop(id)
	return err
}

// stopLocked marks the session stopped before releasing its agents, so a late
// exit cannot relaunch them. The caller holds mu.
func (r *Runtime) stopLocked(id string) ([]*live, error) {
	err := r.save(func(tx *write) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		s.Running = false
		return tx.SaveSession(s)
	})
	if err != nil {
		return nil, err
	}
	return r.detachLocked(func(l *live) bool { return l.session == id }, true), nil
}

// Delete stops the session and deletes it with its workers' worktrees and
// branches. It refuses, changing nothing, if a worker has uncommitted changes
// in its worktree or an unmerged branch. Workers without worktrees have their
// directories left alone.
func (r *Runtime) Delete(id string) error {
	var root string
	var workers []store.Agent
	err := r.db.Read(func(tx *store.Tx) error {
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
	stopClients(r.detach(id, false))
	// A worker can change its worktree before the session ends. Such a worker
	// stays, so that delete can run again after it is fixed.
	for _, a := range workers {
		if present {
			if err := workspace.RemoveWorktree(root, a.Dir, a.Branch); err != nil {
				return fmt.Errorf("%s: %w", a.Name, err)
			}
		}
		if err := r.save(func(tx *write) error { return tx.DeleteAgent(a.ID) }); err != nil {
			return err
		}
	}
	err = r.save(func(tx *write) error { return tx.DeleteSession(id) })
	// The session is gone, so its shell cannot restart.
	r.Terminals.Stop(id)
	return err
}

// DeleteWorkspace deletes each of the workspace's sessions, then the
// workspace.
func (r *Runtime) DeleteWorkspace(id string) error {
	var sessions []store.Session
	err := r.db.Read(func(tx *store.Tx) error {
		var err error
		sessions, err = tx.Sessions(id)
		return err
	})
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if err := r.Delete(s.ID); err != nil {
			return err
		}
	}
	return r.save(func(tx *write) error { return tx.DeleteWorkspace(id) })
}

// Rename changes a session's title. Its ID, and so its agents' addresses,
// stay the same.
func (r *Runtime) Rename(id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("a session needs a title")
	}
	return r.update(id, func(s *store.Session) { s.Title = title })
}

// Seen clears the session's unseen reply.
func (r *Runtime) Seen(id string) error {
	return r.update(id, func(s *store.Session) { s.Unseen = false })
}

// SetStatus records a worker's status on one line.
func (r *Runtime) SetStatus(agent, text string) error {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return errors.New("text is required")
	}
	return r.save(func(tx *write) error {
		a, err := tx.Agent(agent)
		if err != nil {
			return err
		}
		if a.Role != store.Worker {
			return errors.New("only workers have a status")
		}
		a.Status, a.StatusAt = text, time.Now().UnixMilli()
		return tx.SaveAgent(a)
	})
}

func (r *Runtime) update(id string, change func(*store.Session)) error {
	return r.save(func(tx *write) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		change(&s)
		return tx.SaveSession(s)
	})
}

// Restore starts every session the user left running, as after the app
// restarts. One failure does not stop the others.
func (r *Runtime) Restore() {
	var sessions []store.Session
	err := r.db.Read(func(tx *store.Tx) error {
		var err error
		sessions, err = tx.Sessions("")
		return err
	})
	if err != nil {
		logf("restore sessions: %v", err)
		return
	}
	for _, s := range sessions {
		if !s.Running {
			continue
		}
		if err := r.Start(s.ID); err != nil {
			logf("restore session %s: %v", s.ID, err)
		}
	}
}

// AddWorker adds a worker to the lead's running session and launches it. An
// optional first message is queued for it. Without dir, the worker gets its own
// branch and worktree from HEAD; with dir, it works directly in that directory.
func (r *Runtime) AddWorker(leadID, name string, profile harness.Profile, dir, message string) (store.Agent, error) {
	var lead store.Agent
	if err := r.db.Read(func(tx *store.Tx) error {
		var err error
		lead, err = tx.Agent(leadID)
		return err
	}); err != nil {
		return store.Agent{}, err
	}
	if lead.Role != store.Lead {
		return store.Agent{}, errors.New("only the lead can do this")
	}
	a := store.Agent{
		ID: store.NewID(), Session: lead.Session, Name: name, Role: store.Worker,
		Harness: profile.Harness, Model: profile.Model, Effort: profile.Effort, Activity: store.Exited,
	}
	root := lead.Dir
	r.mu.Lock()
	if r.live[leadID] == nil {
		r.mu.Unlock()
		return store.Agent{}, errors.New("the session is not running")
	}
	err := r.save(func(tx *write) error {
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
	var l *live
	if err == nil {
		l = r.register(a, 0)
	}
	r.mu.Unlock()
	if err != nil {
		return store.Agent{}, err
	}
	worktreeCreated := false
	if a.Branch != "" {
		err = workspace.AddWorktree(root, a.Dir, a.Branch)
		worktreeCreated = err == nil
	}
	if err == nil {
		err = r.launch(l)
	}
	if err == nil {
		return a, nil
	}
	cause := err
	r.mu.Lock()
	if r.current(l) {
		delete(r.live, l.id)
	}
	r.mu.Unlock()
	if worktreeCreated {
		if cleanup := workspace.RemoveWorktree(root, a.Dir, a.Branch); cleanup != nil {
			return a, fmt.Errorf("%w; worktree cleanup failed: %v; agent %s was kept for cleanup", cause, cleanup, a.Name)
		}
	}
	if cleanup := r.save(func(tx *write) error { return tx.DeleteAgent(a.ID) }); cleanup != nil {
		return a, fmt.Errorf("%w; removing the agent failed: %v", cause, cleanup)
	}
	return a, cause
}

// RemoveWorker ends a worker and removes it from the lead's session. A worker
// with a worktree also has its worktree and branch removed, which Git refuses
// for dirty worktrees and unmerged branches.
func (r *Runtime) RemoveWorker(leadID, name string) (store.Agent, error) {
	var lead, a store.Agent
	err := r.db.Read(func(tx *store.Tx) error {
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
	if err != nil {
		return a, err
	}
	r.mu.Lock()
	running := r.live[leadID] != nil
	r.mu.Unlock()
	switch {
	case a.Session != lead.Session:
		return a, errors.New("only workers in your session can be removed")
	case a.Role != store.Worker:
		return a, errors.New("only workers can be removed")
	case !running:
		return a, errors.New("the session is not running")
	}
	if a.Branch != "" {
		if err := workspace.RemoveWorktree(lead.Dir, a.Dir, a.Branch); err != nil {
			return a, err
		}
	}
	r.mu.Lock()
	lives := r.detachLocked(func(l *live) bool { return l.id == a.ID }, false)
	r.mu.Unlock()
	stopClients(lives)
	return a, r.save(func(tx *write) error { return tx.DeleteAgent(a.ID) })
}
