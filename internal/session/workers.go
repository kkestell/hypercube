package session

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

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
		if err := r.loadSeq(tx.Tx, a.ID); err != nil {
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
		if err = workspace.AddWorktree(root, a.Dir, a.Branch); err != nil {
			r.end(l, err)
		}
		worktreeCreated = err == nil
	}
	if err == nil {
		err = r.launch(l)
	}
	if err == nil {
		return a, nil
	}
	// A worker that cannot start leaves nothing behind.
	if worktreeCreated {
		if cleanup := workspace.RemoveWorktree(root, a.Dir, a.Branch); cleanup != nil {
			return store.Agent{}, fmt.Errorf("%w; worktree cleanup failed: %v; agent %s was kept for cleanup", err, cleanup, a.Name)
		}
	}
	if cleanup := r.save(func(tx *write) error { return tx.DeleteAgent(a.ID) }); cleanup != nil {
		return store.Agent{}, fmt.Errorf("%w; removing the agent failed: %v", err, cleanup)
	}
	return store.Agent{}, err
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
