package session

import (
	"errors"
	"strings"
	"sync"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harnessclient"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// AddWorkspace adds the workspace at dir: the top of its Git checkout, or dir
// itself outside Git. It returns the existing workspace when the folder is
// already added, refreshing only its Git classification without moving it.
func (r *Runtime) AddWorkspace(dir string) (store.Workspace, error) {
	dir, err := workspace.Resolve("/", dir)
	if err != nil {
		return store.Workspace{}, err
	}
	root, err := workspace.Root(dir)
	noGit := errors.Is(err, workspace.ErrNotGit)
	if noGit {
		root = dir
	} else if err != nil {
		return store.Workspace{}, err
	}
	var w store.Workspace
	err = r.save(func(tx *write) error {
		list, err := tx.Workspaces()
		if err != nil {
			return err
		}
		// Prefer a saved input folder over its checkout root: old records may
		// name subfolders, and their sessions must keep that identity and path.
		for _, path := range []string{dir, root} {
			for _, existing := range list {
				if existing.Path == path {
					w = existing
					w.NoGit = noGit
					return tx.SetWorkspaceNoGit(w.ID, noGit)
				}
			}
		}
		w = store.Workspace{ID: store.NewID(), Path: root, NoGit: noGit}
		return tx.AddWorkspace(w)
	})
	return w, err
}

// Create starts a top-level session with the settings and the first prompt.
// With worktree, the session runs in its own worktree on a new branch from
// the checkout's HEAD.
func (r *Runtime) Create(workspaceID string, settings harness.Settings, worktree bool, text string, images ...harness.Image) (store.Session, error) {
	return r.create(workspaceID, settings, worktree, "", text, images...)
}

// CreateSubmission retains the created session even if its first input fails,
// and correlates that input with the browser's provisional prompt.
func (r *Runtime) CreateSubmission(workspaceID string, settings harness.Settings, worktree bool, submission, text string, images ...harness.Image) (store.Session, error) {
	if err := CheckSubmissionID(submission); err != nil {
		return store.Session{}, err
	}
	return r.create(workspaceID, settings, worktree, submission, text, images...)
}

func (r *Runtime) create(workspaceID string, settings harness.Settings, worktree bool, submission, text string, images ...harness.Image) (store.Session, error) {
	if err := checkPrompt(text, images); err != nil {
		return store.Session{}, err
	}
	if err := settings.Check(); err != nil {
		return store.Session{}, err
	}
	if _, err := harnessclient.Find(r.programs.Plugins, settings.Harness); err != nil {
		return store.Session{}, err
	}
	r.mu.Lock()
	if r.workspaceDeleting[workspaceID] {
		r.mu.Unlock()
		return store.Session{}, errWorkspaceDeleting
	}
	r.workspaceCreates[workspaceID]++
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.workspaceCreates[workspaceID]--
		if r.workspaceCreates[workspaceID] == 0 {
			delete(r.workspaceCreates, workspaceID)
		}
		r.mu.Unlock()
	}()
	var w store.Workspace
	if err := r.db.Read(func(tx *store.Tx) error { var err error; w, err = tx.Workspace(workspaceID); return err }); err != nil {
		return store.Session{}, err
	}
	s := store.Session{ID: store.NewID(), Workspace: workspaceID, Title: report.Title(text),
		Harness: settings.Harness, Model: settings.Model, Effort: settings.Effort, Activity: store.Idle, Dir: w.Path}
	if worktree {
		if w.NoGit {
			return store.Session{}, errors.New("a worktree session needs a Git checkout")
		}
		s.Dir, s.Branch = workspace.SessionDir(w.Path, s.ID), workspace.SessionBranch(s.ID)
		if err := workspace.AddWorktree(w.Path, s.Dir, s.Branch); err != nil {
			return store.Session{}, err
		}
		var err error
		if s.Base, err = workspace.Head(s.Dir); err != nil {
			return store.Session{}, errors.Join(err, workspace.RemoveWorktree(w.Path, s.Dir, s.Branch))
		}
	}
	if err := r.save(func(tx *write) error { return tx.AddSession(s) }); err != nil {
		if worktree {
			err = errors.Join(err, workspace.RemoveWorktree(w.Path, s.Dir, s.Branch))
		}
		return store.Session{}, err
	}
	if worktree {
		r.refreshMerges(workspaceID)
	}
	return s, r.input(s.ID, submission, text, images...)
}

// DeleteWorkspace deletes each of the workspace's sessions, then the
// workspace. It refuses while a top-level creation or another deletion is in
// progress, blocking new top-level sessions after its membership snapshot.
func (r *Runtime) DeleteWorkspace(id string) error {
	var sessions []store.Session
	r.mu.Lock()
	if r.workspaceDeleting[id] {
		r.mu.Unlock()
		return errWorkspaceDeleting
	}
	if r.workspaceCreates[id] != 0 {
		r.mu.Unlock()
		return errWorkspaceCreating
	}
	r.workspaceDeleting[id] = true
	err := r.db.Read(func(tx *store.Tx) error {
		var err error
		sessions, err = tx.Sessions(id)
		return err
	})
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.workspaceDeleting, id)
		r.mu.Unlock()
	}()
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if err := r.Delete(s.ID); err != nil {
			return err
		}
	}
	r.Terminals.Stop(workspaceShellOwner(id))
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

// Quit ends every agent and shell. Each session wakes again when next used.
func (r *Runtime) Quit() {
	r.mu.Lock()
	if r.detached {
		r.mu.Unlock()
		return
	}
	r.closing = true
	r.stopIdleLocked()
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
	r.stopIdleLocked()
	lives := make([]*live, 0, len(r.live))
	for _, l := range r.live {
		lives = append(lives, l)
	}
	r.mu.Unlock()
	for _, l := range lives {
		// Closing releases a sender blocked on plugin input. It must not
		// wait behind that sender's lock.
		r.mu.Lock()
		client := l.client
		r.mu.Unlock()
		if client != nil {
			_ = client.Close()
		}
	}
	for _, l := range lives {
		<-l.done
	}
}
