package session

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// Delete deletes a session. An ended child loses only its row and chat. A
// top-level session stops and is deleted with its children and their managed
// worktrees and branches. Ordinary dirty or unmerged worktree refusals leave
// the family running; work written while stopping is checked again and kept.
func (r *Runtime) Delete(id string) error {
	return r.removeFamily(id, false)
}

// Archive stops a top-level family and removes its worktrees, keeping its chats.
func (r *Runtime) Archive(id string) error {
	return r.removeFamily(id, true)
}

// Unarchive returns an archived top-level session to the session list. A
// worktree session gets its worktree back, on its branch created again from
// the checkout's HEAD. Its children stay ended, and it wakes when next used.
func (r *Runtime) Unarchive(id string) error {
	var s store.Session
	var checkout string
	r.mu.Lock()
	err := r.db.Read(func(tx *store.Tx) error {
		var err error
		s, err = tx.Session(id)
		if err != nil {
			return err
		}
		switch {
		case r.deleting[id]:
			return errDeleting
		case s.Parent != "":
			return errors.New("only a top-level session can be unarchived")
		case !s.Ended:
			return errors.New("the session is not archived")
		}
		w, err := tx.Workspace(s.Workspace)
		checkout = w.Path
		return err
	})
	if err == nil {
		r.deleting[id] = true
	}
	r.mu.Unlock()
	if err != nil {
		return err
	}
	defer r.releaseRemoval(id)
	base := ""
	if s.Base != "" {
		if err := workspace.AddWorktree(checkout, s.Dir, s.Branch); err != nil {
			return err
		}
		if base, err = workspace.Head(s.Dir); err != nil {
			return err
		}
	}
	err = r.save(func(tx *write) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		s.Ended, s.Base = false, base
		return tx.SaveSession(s)
	})
	if err == nil && base != "" {
		r.refreshMerges(s.Workspace)
	}
	return err
}

// claimRemoval reads the family under the same lock as child creation and
// claims it before releasing the membership snapshot.
func (r *Runtime) claimRemoval(id string) (store.Session, string, []store.Session, error) {
	var s store.Session
	var root string
	var children []store.Session
	r.mu.Lock()
	err := r.db.Read(func(tx *store.Tx) error {
		var err error
		s, err = tx.Session(id)
		if err != nil {
			return err
		}
		if r.deleting[id] || r.deleting[s.Parent] {
			return errDeleting
		}
		if s.Parent != "" {
			return nil
		}
		w, err := tx.Workspace(s.Workspace)
		if err != nil {
			return err
		}
		root = w.Path
		children, err = tx.Children(id)
		if err != nil {
			return err
		}
		for _, c := range children {
			if r.deleting[c.ID] {
				return errDeleting
			}
		}
		return nil
	})
	if err == nil {
		r.deleting[id] = true
	}
	r.mu.Unlock()
	return s, root, children, err
}

func (r *Runtime) removeFamily(id string, archive bool) error {
	s, root, children, err := r.claimRemoval(id)
	if err != nil {
		return err
	}
	defer r.releaseRemoval(id)
	if s.Parent != "" {
		if archive {
			return errors.New("only a top-level session can be archived")
		}
		if !s.Ended {
			return errors.New("a child session must be ended before it can be deleted")
		}
		return r.save(func(tx *write) error { return tx.DeleteSession(id) })
	}
	if s.Ended {
		if archive {
			return errors.New("the session is already archived")
		}
		return r.save(func(tx *write) error { return tx.DeleteSession(id) })
	}
	trees := familyWorktrees(root, s, children)
	// A checkout that moved or disappeared took its worktrees with it.
	_, statErr := os.Stat(root)
	present := !errors.Is(statErr, os.ErrNotExist)
	if present {
		if err := checkWorktrees(trees); err != nil {
			return err
		}
	}
	r.mu.Lock()
	lives := r.selectLaunches(func(l *live) bool { return l.root == id })
	r.mu.Unlock()
	r.stopLaunches(lives)
	r.stopShells(id)
	// All writers have stopped. Check the whole family before removing any
	// worktree, including files written after the initial checks.
	if present {
		if err := checkWorktrees(trees); err != nil {
			return err
		}
	}
	for _, tree := range trees {
		if present {
			if err := workspace.RemoveWorktree(tree.root, tree.session.Dir, tree.session.Branch); err != nil {
				return err
			}
		}
		if tree.session.Parent == "" {
			continue
		}
		// A later worktree can still fail to remove. Record each completed
		// child now so it cannot resume in a directory that is already gone.
		if err := r.save(func(tx *write) error { return recordRemoved(tx, tree.session.ID, archive) }); err != nil {
			return err
		}
	}
	return r.recordRemovedFamily(id, children, archive)
}

// recordRemovedFamily finishes persistence after all managed worktrees are gone.
// Worktree children were recorded individually so partial cleanup is retained.
func (r *Runtime) recordRemovedFamily(id string, children []store.Session, archive bool) error {
	return r.save(func(tx *write) error {
		if archive {
			for _, child := range children {
				if child.Branch == "" && !child.Ended {
					if err := endArchived(tx, child.ID); err != nil {
						return err
					}
				}
			}
		}
		return recordRemoved(tx, id, archive)
	})
}

func recordRemoved(tx *write, id string, archive bool) error {
	if archive {
		return endArchived(tx, id)
	}
	return tx.DeleteSession(id)
}

type managedWorktree struct {
	root    string
	session store.Session
}

// familyWorktrees orders children before the worktree that contains them.
func familyWorktrees(root string, s store.Session, children []store.Session) []managedWorktree {
	childRoot := s.Dir
	if _, err := os.Stat(s.Dir); errors.Is(err, os.ErrNotExist) {
		childRoot = root
	}
	var trees []managedWorktree
	for _, c := range children {
		if !c.Ended && c.Branch != "" {
			trees = append(trees, managedWorktree{childRoot, c})
		}
	}
	if s.Base != "" {
		trees = append(trees, managedWorktree{root, s})
	}
	return trees
}

func checkWorktrees(trees []managedWorktree) error {
	var problems []string
	for _, tree := range trees {
		if err := workspace.Removable(tree.root, tree.session.Dir, tree.session.Branch); err != nil {
			name := tree.session.Title
			if tree.session.Parent != "" {
				name = tree.session.Name
			}
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("cannot remove worktrees:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

func endArchived(tx *write, id string) error {
	s, err := tx.Session(id)
	if err != nil {
		return err
	}
	s.Ended, s.Attention = true, false
	if err := tx.SaveSession(s); err != nil {
		return err
	}
	return tx.DropQueued(id)
}

// releaseRemoval ends a removal's claim on a session and, for a top-level
// session, on its children. Sessions that survive a refused removal can park
// again, and messages that reached them during the claim wake them.
func (r *Runtime) releaseRemoval(id string) {
	var held []string
	r.mu.Lock()
	delete(r.deleting, id)
	err := r.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(id)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		family := []store.Session{s}
		if s.Parent == "" {
			children, err := tx.Children(id)
			if err != nil {
				return err
			}
			family = append(family, children...)
		}
		for _, m := range family {
			if !m.Ended {
				held = append(held, m.ID)
			}
		}
		return nil
	})
	if err != nil {
		logf("reading the sessions %s's removal held: %v", id, err)
	}
	for _, m := range held {
		r.settleLocked(m)
	}
	r.mu.Unlock()
	for _, m := range held {
		r.Deliver(m)
	}
}

// EndChild stops its writers and removes its worktree, keeping its chat.
func (r *Runtime) EndChild(parentID, name string) (store.Session, error) {
	var parent, child store.Session
	r.mu.Lock()
	err := r.db.Read(func(tx *store.Tx) error {
		var err error
		parent, err = tx.Session(parentID)
		if err != nil {
			return err
		}
		if parent.Parent != "" {
			return errors.New("only a top-level session can do this")
		}
		children, err := tx.Children(parent.ID)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(children, func(c store.Session) bool { return c.Name == name })
		if i < 0 {
			return fmt.Errorf("you have no child session named %s", name)
		}
		child = children[i]
		if child.Ended {
			return errors.New("the child session has already ended")
		}
		if r.deleting[parentID] || r.deleting[child.ID] {
			return errDeleting
		}
		if r.live[parentID] == nil || r.live[parentID].stopping || r.closing {
			return errors.New("the session is not running")
		}
		return nil
	})
	if err == nil {
		r.deleting[child.ID] = true
	}
	r.mu.Unlock()
	if err != nil {
		return child, err
	}
	defer r.releaseRemoval(child.ID)
	if child.Branch != "" {
		if err := workspace.Removable(parent.Dir, child.Dir, child.Branch); err != nil {
			return child, err
		}
	}
	r.mu.Lock()
	lives := r.selectLaunches(func(l *live) bool { return l.id == child.ID })
	r.mu.Unlock()
	r.stopLaunches(lives)
	r.Terminals.Stop(child.ID)
	if child.Branch != "" {
		if err := workspace.RemoveWorktree(parent.Dir, child.Dir, child.Branch); err != nil {
			return child, err
		}
	}
	err = r.save(func(tx *write) error {
		var err error
		child, err = tx.Session(child.ID)
		if err != nil {
			return err
		}
		child.Ended, child.Attention = true, false
		if err := tx.SaveSession(child); err != nil {
			return err
		}
		return tx.DropQueued(child.ID)
	})
	r.Deliver(parentID)
	return child, err
}
