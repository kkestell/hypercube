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
	"github.com/kkestell/hypercube/internal/terminal"
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
	if r.deleting[id] {
		r.mu.Unlock()
		return errDeleting
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
			if err := r.loadSeq(tx.Tx, a.ID); err != nil {
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
		if err != nil {
			return err
		}
		for _, a := range agents {
			if a.Role == store.Worker && a.Branch != "" {
				workers = append(workers, a)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// A checkout that moved or disappeared took its worktrees with it.
	_, statErr := os.Stat(root)
	present := !errors.Is(statErr, os.ErrNotExist)
	var problems []string
	if present {
		for _, a := range workers {
			if err := workspace.Removable(root, a.Dir, a.Branch); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", a.Name, err))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("not deleting: problem worktrees:\n  %s", strings.Join(problems, "\n  "))
	}
	// Waiting for the plugins releases mu, so the mark keeps Start from
	// resuming the session meanwhile.
	r.mu.Lock()
	r.deleting[id] = true
	lives := r.detachLocked(func(l *live) bool { return l.session == id }, false)
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.deleting, id)
		r.mu.Unlock()
	}()
	stopClients(lives)
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

// Shell returns the running session's shell, starting it in the workspace
// folder with the given size if the session has none.
func (r *Runtime) Shell(session string, cols, rows int) (*terminal.Shell, error) {
	var dir string
	err := r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(session)
		if err != nil {
			return err
		}
		if !s.Running {
			return errors.New("the session is not running")
		}
		w, err := tx.Workspace(s.Workspace)
		dir = w.Path
		return err
	})
	if err != nil {
		return nil, err
	}
	return r.Terminals.Open(session, dir, cols, rows)
}

// detachLocked removes the matching running agents from the runtime, saving
// their unfinished items, and returns them for stopClients. With ended set it
// also marks the agents ended. The caller holds mu.
func (r *Runtime) detachLocked(match func(*live) bool, ended bool) []*live {
	var lives []*live
	for id, l := range r.live {
		if match(l) {
			lives = append(lives, l)
			delete(r.live, id)
		}
	}
	err := r.save(func(tx *write) error {
		for _, l := range lives {
			for _, e := range l.drain() {
				if err := tx.putItem(l, e.seq, e.item); err != nil {
					return err
				}
			}
			if ended {
				if err := markEnded(tx, l.id); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		logf("saving agents' transcripts: %v", err)
	}
	return lives
}

// stopClients ends the plugins of detached agents, in parallel.
func stopClients(lives []*live) {
	var wg sync.WaitGroup
	for _, l := range lives {
		if l.client != nil {
			wg.Go(func() { l.client.Shutdown(shutdownWait) })
		}
	}
	wg.Wait()
}

// Shutdown ends every agent's plugin for the app's exit. Sessions keep the
// running mark, so Restore starts them again.
func (r *Runtime) Shutdown() {
	r.mu.Lock()
	r.closing = true
	lives := r.detachLocked(func(*live) bool { return true }, true)
	r.mu.Unlock()
	r.Terminals.Close()
	stopClients(lives)
}
