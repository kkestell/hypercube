package session

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// Delete deletes a session. An ended child loses only its row and chat. A
// top-level session stops and is deleted with its children and their managed
// worktrees and branches. Ordinary refusals of worktrees whose work removal
// would lose leave the family running; work written while stopping is checked
// again and kept.
func (r *Runtime) Delete(id string) error {
	return r.removeFamily(id, false)
}

// Archive stops a top-level family and removes its worktrees, keeping its chats.
func (r *Runtime) Archive(id string) error {
	return r.removeFamily(id, true)
}

// Unarchive returns an archived top-level session to the session list. A
// worktree session gets its worktree back, on its branch created again at the
// tip recorded when it was archived, keeping its starting commit. Its children
// stay ended, and it wakes when next used.
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
	base, start := s.Base, ""
	if base != "" {
		// Without its recorded tip, the branch starts again from HEAD. Archiving
		// removed the branch only after finding its commits on HEAD, its starting
		// commit, or a remote branch, so none are lost.
		if s.Tip != "" && workspace.HasCommit(checkout, s.Tip) {
			start = s.Tip
		}
		if err := workspace.AddWorktree(checkout, s.Dir, s.Branch, start); err != nil {
			return fmt.Errorf("cannot recreate the session's worktree: %w", err)
		}
		if start == "" {
			if base, err = workspace.Head(s.Dir); err != nil {
				return errors.Join(err, workspace.RemoveWorktree(checkout, s.Dir, s.Branch, ""))
			}
		}
	}
	err = r.save(func(tx *write) error {
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		s.Ended, s.Archived, s.Base, s.Tip = false, 0, base, ""
		return tx.SaveSession(s)
	})
	if err != nil {
		if base != "" {
			// The archived row still records the restored tip, so the rollback
			// keeps it and removes only a worktree without uncommitted files.
			err = errors.Join(err, workspace.RemoveWorktree(checkout, s.Dir, s.Branch, cmp.Or(start, base)))
		}
		return err
	}
	if base != "" {
		r.refreshMerges(s.Workspace)
	}
	return nil
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
	// Creation owns setup and failed-start rollback, even after its launch
	// ends. Let it finish before checking or removing any family resources.
	r.waitChildCreations(children)
	err = r.db.Read(func(tx *store.Tx) error {
		var err error
		children, err = tx.Children(id)
		return err
	})
	if err != nil {
		return err
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
	tip := ""
	if present {
		if err := checkWorktrees(trees); err != nil {
			return err
		}
		if archive && s.Base != "" {
			if tip, err = workspace.BranchTip(root, s.Branch); err != nil {
				return err
			}
		}
	}
	for _, tree := range trees {
		if present {
			if err := workspace.RemoveWorktree(tree.root, tree.session.Dir, tree.session.Branch, tree.session.Base); err != nil {
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
	if err := r.recordRemovedFamily(id, children, archive, tip); err != nil {
		return err
	}
	if s.Base != "" {
		r.forgetMerge(id)
	}
	return nil
}

func (r *Runtime) waitChildCreations(children []store.Session) {
	var pending []chan struct{}
	r.mu.Lock()
	for _, child := range children {
		if done := r.childCreates[child.ID]; done != nil {
			pending = append(pending, done)
		}
	}
	r.mu.Unlock()
	for _, done := range pending {
		<-done
	}
}

// recordRemovedFamily finishes persistence after all managed worktrees are gone.
// Worktree children were recorded individually so partial cleanup is retained.
// An archived session keeps its branch's tip for unarchiving.
func (r *Runtime) recordRemovedFamily(id string, children []store.Session, archive bool, tip string) error {
	return r.save(func(tx *write) error {
		if !archive {
			return tx.DeleteSession(id)
		}
		for _, child := range children {
			if child.Branch == "" && !child.Ended {
				if err := recordEnded(tx, child.ID); err != nil {
					return err
				}
			}
		}
		s, err := tx.Session(id)
		if err != nil {
			return err
		}
		s.Tip = tip
		if err := tx.SaveSession(s); err != nil {
			return err
		}
		return recordEnded(tx, id)
	})
}

func recordRemoved(tx *write, id string, archive bool) error {
	if archive {
		return recordEnded(tx, id)
	}
	return tx.DeleteSession(id)
}

type managedWorktree struct {
	root    string
	session store.Session
}

// familyWorktrees orders children before the worktree whose branch they merge
// into.
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
		if err := workspace.Removable(tree.root, tree.session.Dir, tree.session.Branch, tree.session.Base); err != nil {
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

// recordEnded ends a session whose agent and worktree are gone, keeping its
// chat and dropping its undelivered messages.
func recordEnded(tx *write, id string) error {
	s, err := tx.Session(id)
	if err != nil {
		return err
	}
	s.Ended, s.Archived, s.Attention = true, time.Now().UnixMilli(), false
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
		if err := workspace.Removable(parent.Dir, child.Dir, child.Branch, ""); err != nil {
			return child, err
		}
	}
	r.mu.Lock()
	lives := r.selectLaunches(func(l *live) bool { return l.id == child.ID })
	r.mu.Unlock()
	r.stopLaunches(lives)
	r.Terminals.Stop(child.ID)
	if child.Branch != "" {
		if err := workspace.RemoveWorktree(parent.Dir, child.Dir, child.Branch, ""); err != nil {
			return child, err
		}
	}
	err = r.save(func(tx *write) error {
		if err := recordEnded(tx, child.ID); err != nil {
			return err
		}
		var err error
		child, err = tx.Session(child.ID)
		return err
	})
	r.Deliver(parentID)
	return child, err
}
