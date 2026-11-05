package session

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

var childName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// CreateChild launches a child in a new worktree, or directly in dir.
func (r *Runtime) CreateChild(parentID, name, title string, profile harness.Profile, dir, message string) (store.Session, error) {
	if !childName.MatchString(name) {
		return store.Session{}, errors.New("name must be lowercase letters, digits, and hyphens")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return store.Session{}, errors.New("a session needs a title")
	}
	var parent store.Session
	if err := r.db.Read(func(tx *store.Tx) error { var err error; parent, err = tx.Session(parentID); return err }); err != nil {
		return store.Session{}, err
	}
	if parent.Parent != "" {
		return store.Session{}, errors.New("only a top-level session can do this")
	}
	child := store.Session{ID: store.NewID(), Workspace: parent.Workspace, Parent: parent.ID, Name: name, Title: title,
		Harness: profile.Harness, Model: profile.Model, Effort: profile.Effort, Activity: store.Exited}
	r.mu.Lock()
	if r.deleting[parentID] {
		r.mu.Unlock()
		return store.Session{}, errDeleting
	}
	if r.live[parentID] == nil || r.live[parentID].stopping || r.closing {
		r.mu.Unlock()
		return store.Session{}, errors.New("the session is not running")
	}
	var l *live
	err := r.save(func(tx *write) error {
		w, err := tx.Workspace(parent.Workspace)
		if err != nil {
			return err
		}
		children, err := tx.Children(parent.ID)
		if err != nil {
			return err
		}
		for _, c := range children {
			if c.Name == name {
				return fmt.Errorf("a child session with the name %s already exists; choose another", name)
			}
		}
		switch {
		case dir != "":
			child.Dir, err = workspace.Resolve(parent.Dir, dir)
			if err != nil {
				return err
			}
		case w.NoGit:
			return errors.New("dir is required: this workspace is not a Git checkout, so child sessions cannot have worktrees")
		default:
			child.Dir, child.Branch = workspace.WorktreeDir(parent.Dir, parent.ID, name), workspace.ChildBranch(parent.ID, name)
		}
		if err := tx.AddSession(child); err != nil {
			return err
		}
		if message != "" {
			if err := tx.Queue(store.Message{From: parentID, To: child.ID, Text: message}); err != nil {
				return err
			}
		}
		l, err = r.decide(tx, child, parent.ID, 0)
		return err
	})
	if err == nil {
		r.live[child.ID] = l
	}
	r.mu.Unlock()
	if err != nil {
		return store.Session{}, err
	}
	discard := func(err error) error {
		if cleanup := r.save(func(tx *write) error { return tx.DeleteSession(child.ID) }); cleanup != nil {
			return fmt.Errorf("%w; deleting the child session failed: %v", err, cleanup)
		}
		return err
	}
	if child.Branch != "" {
		if err := workspace.AddWorktree(parent.Dir, child.Dir, child.Branch); err != nil {
			// run never starts, so end the launch here and release anything
			// already stopping it.
			r.end(l, err)
			close(l.done)
			return store.Session{}, discard(err)
		}
	}
	if err := r.launch(l); err != nil {
		if child.Branch != "" {
			if cleanup := workspace.RemoveWorktree(parent.Dir, child.Dir, child.Branch); cleanup != nil {
				return store.Session{}, fmt.Errorf("%w; worktree cleanup failed: %v; child session %s was kept for cleanup", err, cleanup, name)
			}
		}
		return store.Session{}, discard(err)
	}
	return child, nil
}
