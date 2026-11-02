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

// Create starts a top-level session with the first prompt.
func (r *Runtime) Create(workspaceID, profileName, text string, images ...harness.Image) (store.Session, error) {
	if err := checkPrompt(text, images); err != nil {
		return store.Session{}, err
	}
	cfg, err := harness.LoadConfig()
	if err != nil {
		return store.Session{}, err
	}
	profile, err := cfg.Lead(profileName)
	if err != nil {
		return store.Session{}, err
	}
	s := store.Session{ID: store.NewID(), Workspace: workspaceID, Title: report.Title(text), Running: true,
		Harness: profile.Harness, Model: profile.Model, Effort: profile.Effort, Activity: store.Idle}
	err = r.save(func(tx *write) error {
		w, err := tx.Workspace(workspaceID)
		if err != nil {
			return err
		}
		s.Dir = w.Path
		return tx.AddSession(s)
	})
	if err != nil {
		return s, err
	}
	if err := r.start(s.ID, true); err != nil {
		return s, err
	}
	return s, r.Input(s.ID, text, images...)
}

// Start launches a stopped family, resuming its conversations.
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
		if s.Parent != "" {
			return errors.New("a child session starts and stops with its parent")
		}
		root = s.Dir
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
	var sessions []store.Session
	err = r.save(func(tx *write) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		s.Running, s.AwaitingUser, s.Branch = true, !fresh, branch
		s.LastActive = time.Now().UnixMilli()
		if err := tx.SaveSession(s); err != nil {
			return err
		}
		children, err := tx.Children(id)
		if err != nil {
			return err
		}
		sessions = append([]store.Session{s}, children...)
		active := sessions[:0]
		for _, a := range sessions {
			if a.Ended {
				continue
			}
			a.Activity, a.Attention = store.Exited, false
			if err := tx.SaveSession(a); err != nil {
				return err
			}
			if err := r.loadSeq(tx.Tx, a.ID); err != nil {
				return err
			}
			active = append(active, a)
		}
		sessions = active
		return nil
	})
	if err == nil {
		for _, a := range sessions {
			lives = append(lives, r.register(a.ID, id, 0))
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
				errs[i] = fmt.Errorf("launching %s: %w", sessions[i].ID, err)
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// Stop stops the family. Its conversations and queued messages stay
// for the next Start.
func (r *Runtime) Stop(id string) error {
	r.mu.Lock()
	lives, err := r.stopLocked(id)
	r.mu.Unlock()
	stopClients(lives)
	if err == nil {
		r.stopShells(id)
	}
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
		if s.Parent != "" {
			return errors.New("a child session starts and stops with its parent")
		}
		s.Running = false
		return tx.SaveSession(s)
	})
	if err != nil {
		return nil, err
	}
	return r.detachLocked(func(l *live) bool { return l.root == id }, true), nil
}

// Delete stops the session and deletes it with its child sessions' worktrees and
// branches. It refuses, changing nothing, if a child has uncommitted changes
// in its worktree or an unmerged branch. Children without worktrees have their
// directories left alone.
func (r *Runtime) Delete(id string) error {
	var root string
	var children []store.Session
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
		if s.Parent != "" {
			if !s.Ended {
				return errors.New("a child session must be ended before it can be deleted")
			}
			return nil
		}
		allChildren, err := tx.Children(id)
		if err != nil {
			return err
		}
		for _, a := range allChildren {
			if !a.Ended && a.Branch != "" {
				children = append(children, a)
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
		for _, a := range children {
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
	lives := r.detachLocked(func(l *live) bool { return l.root == id }, false)
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.deleting, id)
		r.mu.Unlock()
	}()
	stopClients(lives)
	r.stopShells(id)
	// A child can change its worktree before the family stops. That child stays, so that delete can run again after it is fixed.
	for _, a := range children {
		if present {
			if err := workspace.RemoveWorktree(root, a.Dir, a.Branch); err != nil {
				return fmt.Errorf("%s: %w", a.Name, err)
			}
		}
		if err := r.save(func(tx *write) error { return tx.DeleteSession(a.ID) }); err != nil {
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

// Rename changes a session's title. Its ID stays the same.
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

// Shell opens the running session's shell in its own directory.
func (r *Runtime) Shell(id string, cols, rows int) (*terminal.Shell, error) {
	var dir string
	err := r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		running, err := tx.Running(s)
		if err != nil {
			return err
		}
		if !running {
			return errors.New("the session is not running")
		}
		dir = s.Dir
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.Terminals.Open(id, dir, cols, rows)
}

// stopShells ends the root's shell and all its children's shells.
func (r *Runtime) stopShells(id string) {
	var children []store.Session
	if err := r.db.Read(func(tx *store.Tx) error { var err error; children, err = tx.Children(id); return err }); err != nil {
		logf("reading child shells: %v", err)
	}
	r.Terminals.Stop(id)
	for _, c := range children {
		r.Terminals.Stop(c.ID)
	}
}

// detachLocked removes the matching running agents from the runtime, saving
// their unfinished items, and returns them for stopClients. With exited set it
// also marks the agents exited. The caller holds mu.
func (r *Runtime) detachLocked(match func(*live) bool, exited bool) []*live {
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
				if err := tx.putItem(l.id, e.seq, e.item); err != nil {
					return err
				}
			}
			if exited {
				if err := markExited(tx, l.id); err != nil {
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
