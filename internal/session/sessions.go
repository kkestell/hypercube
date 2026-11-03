package session

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

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

// Create starts a top-level session with the first prompt. With worktree,
// the session runs in its own worktree on a new branch from the checkout's
// HEAD.
func (r *Runtime) Create(workspaceID, profileName string, worktree bool, text string, images ...harness.Image) (store.Session, error) {
	if err := checkPrompt(text, images); err != nil {
		return store.Session{}, err
	}
	cfg, err := harness.LoadConfig()
	if err != nil {
		return store.Session{}, err
	}
	profile, err := cfg.Profile(profileName)
	if err != nil {
		return store.Session{}, err
	}
	var w store.Workspace
	if err := r.db.Read(func(tx *store.Tx) error { var err error; w, err = tx.Workspace(workspaceID); return err }); err != nil {
		return store.Session{}, err
	}
	s := store.Session{ID: store.NewID(), Workspace: workspaceID, Title: report.Title(text), Running: true,
		Harness: profile.Harness, Model: profile.Model, Effort: profile.Effort, Activity: store.Idle, Dir: w.Path}
	if worktree {
		if w.NoGit {
			return store.Session{}, errors.New("a worktree session needs a Git checkout")
		}
		s.Dir, s.Branch = workspace.SessionDir(w.Path, s.ID), workspace.SessionBranch(s.ID)
		if err := workspace.AddWorktree(w.Path, s.Dir, s.Branch); err != nil {
			return store.Session{}, err
		}
		if s.Base, err = workspace.Head(s.Dir); err != nil {
			return store.Session{}, errors.Join(err, workspace.RemoveWorktree(w.Path, s.Dir, s.Branch))
		}
	}
	if err := r.save(func(tx *write) error { return tx.AddSession(s) }); err != nil {
		if worktree {
			err = errors.Join(err, workspace.RemoveWorktree(w.Path, s.Dir, s.Branch))
		}
		return s, err
	}
	if worktree {
		r.refreshMerges(workspaceID)
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
	var dir string
	err := r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		if s.Parent != "" {
			return errors.New("a child session starts and stops with its parent")
		}
		dir = s.Dir
		return nil
	})
	if err != nil {
		return err
	}
	branch, _ := workspace.Branch(dir)
	// The family is registered before any plugin starts, so a second start
	// finds it running.
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
	err = r.save(func(tx *write) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		s.Running, s.AwaitingUser = true, !fresh
		// A worktree session's branch is fixed when it is created.
		if s.Base == "" {
			s.Branch = branch
		}
		children, err := tx.Children(id)
		if err != nil {
			return err
		}
		for _, member := range append([]store.Session{s}, children...) {
			if member.Ended {
				continue
			}
			if err := tx.SaveSession(member); err != nil {
				return err
			}
			l, err := r.decide(tx, member, id, 0)
			if err != nil {
				return err
			}
			lives = append(lives, l)
		}
		return nil
	})
	if err == nil {
		for _, l := range lives {
			r.live[l.id] = l
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
				errs[i] = fmt.Errorf("launching %s: %w", l.id, err)
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
	r.stopLaunches(lives)
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
		children, err := tx.Children(id)
		if err != nil {
			return err
		}
		// A stopped session waits on nobody, so its family drops the flag now
		// instead of when each agent exits.
		for _, c := range append(children, s) {
			c.Attention = false
			if err := tx.SaveSession(c); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.selectLaunches(func(l *live) bool { return l.root == id }), nil
}

// Delete deletes a session. An ended child loses only its row and chat. A
// top-level session stops and is deleted with its children and their
// worktrees and branches, and, for a worktree session, its own worktree and
// branch. It refuses, changing nothing, if one of those worktrees has
// uncommitted changes or an unmerged branch. Children without worktrees have
// their directories left alone.
func (r *Runtime) Delete(id string) error {
	var s store.Session
	var root string
	var worktrees []store.Session
	err := r.db.Read(func(tx *store.Tx) error {
		var err error
		s, err = tx.Session(id)
		if err != nil || s.Parent != "" {
			return err
		}
		w, err := tx.Workspace(s.Workspace)
		if err != nil {
			return err
		}
		root = w.Path
		children, err := tx.Children(id)
		if err != nil {
			return err
		}
		for _, c := range children {
			if !c.Ended && c.Branch != "" {
				worktrees = append(worktrees, c)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if s.Parent != "" {
		if !s.Ended {
			return errors.New("a child session must be ended before it can be deleted")
		}
		return r.save(func(tx *write) error { return tx.DeleteSession(id) })
	}
	// A checkout that moved or disappeared took its worktrees with it.
	_, statErr := os.Stat(root)
	present := !errors.Is(statErr, os.ErrNotExist)
	// Children's worktrees belong to the session's directory. When a worktree
	// session's directory is gone, the checkout still holds their branches.
	childRoot := s.Dir
	if _, err := os.Stat(s.Dir); errors.Is(err, os.ErrNotExist) {
		childRoot = root
	}
	var problems []string
	if present {
		if s.Base != "" {
			if err := workspace.Removable(root, s.Dir, s.Branch); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", s.Title, err))
			}
		}
		for _, c := range worktrees {
			if err := workspace.Removable(childRoot, c.Dir, c.Branch); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", c.Name, err))
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
	lives := r.selectLaunches(func(l *live) bool { return l.root == id })
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.deleting, id)
		r.mu.Unlock()
	}()
	r.stopLaunches(lives)
	r.stopShells(id)
	// A child can change its worktree before the family stops. That child stays, so that delete can run again after it is fixed.
	for _, c := range worktrees {
		if present {
			if err := workspace.RemoveWorktree(childRoot, c.Dir, c.Branch); err != nil {
				return fmt.Errorf("%s: %w", c.Name, err)
			}
		}
		if err := r.save(func(tx *write) error { return tx.DeleteSession(c.ID) }); err != nil {
			return err
		}
	}
	if present && s.Base != "" {
		if err := workspace.RemoveWorktree(root, s.Dir, s.Branch); err != nil {
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

// selectLaunches marks launches as stopping but leaves their readers attached.
// The caller holds mu.
func (r *Runtime) selectLaunches(match func(*live) bool) []*live {
	var lives []*live
	for _, l := range r.live {
		if match(l) {
			l.stopping = true
			lives = append(lives, l)
		}
	}
	return lives
}
func (r *Runtime) stopLaunches(lives []*live) {
	var wg sync.WaitGroup
	for _, l := range lives {
		wg.Go(func() {
			r.mu.Lock()
			client := l.client
			r.mu.Unlock()
			if client != nil {
				_ = client.Stop()
			}
			<-l.done
		})
	}
	wg.Wait()
}

// Quit ends every agent and shell while preserving the sessions' running marks.
func (r *Runtime) Quit() {
	r.mu.Lock()
	if r.detached {
		r.mu.Unlock()
		return
	}
	r.closing = true
	lives := r.selectLaunches(func(*live) bool { return true })
	r.mu.Unlock()
	r.Terminals.Close()
	r.stopLaunches(lives)
}

// Detach finishes writes and releases readers, leaving each holder running.
func (r *Runtime) Detach() {
	r.mu.Lock()
	r.closing = true
	r.detached = true
	lives := make([]*live, 0, len(r.live))
	for _, l := range r.live {
		lives = append(lives, l)
	}
	r.mu.Unlock()
	for _, l := range lives {
		l.send.Lock()
		if l.client != nil {
			_ = l.client.Close()
		}
		l.send.Unlock()
	}
	for _, l := range lives {
		<-l.done
	}
}
