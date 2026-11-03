package ui

import (
	"cmp"
	"path/filepath"
	"slices"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// shownWorkspace, shownSession are what the browser view shows.
type shownWorkspace struct {
	ID       string         `json:"id"`
	Path     string         `json:"path"`
	Name     string         `json:"name"`
	NoGit    bool           `json:"noGit"`
	Sessions []shownSession `json:"sessions"`
}

type shownSession struct {
	ID        string        `json:"id"`
	Parent    string        `json:"parent"`
	Title     string        `json:"title"`
	Running   bool          `json:"running"`
	Ended     bool          `json:"ended"`
	Unseen    bool          `json:"unseen"`
	Reply     string        `json:"reply,omitempty"`
	ReplySeq  int64         `json:"replySeq,omitempty"`
	Harness   string        `json:"harness"`
	Model     string        `json:"model"`
	Effort    string        `json:"effort"`
	Dir       string        `json:"dir"`
	Activity  string        `json:"activity"`
	Attention bool          `json:"attention"`
	Usage     harness.Usage `json:"usage"`
	// Worktree marks a top-level session that runs in its own worktree.
	Worktree bool             `json:"worktree"`
	Merge    *workspace.Merge `json:"merge,omitempty"`
	Children []shownSession   `json:"children"`
}

// showSession shows a top-level session with its children and, for a
// worktree session, its merge status. A child runs while its parent runs,
// until it ends.
func showSession(tx *store.Tx, s store.Session, merges map[string]workspace.Merge) (shownSession, error) {
	shown, err := show(tx, s, s.Running)
	if err != nil {
		return shown, err
	}
	shown.Worktree = s.Base != ""
	if m, ok := merges[s.ID]; ok {
		shown.Merge = &m
	}
	children, err := tx.Children(s.ID)
	if err != nil {
		return shown, err
	}
	for _, c := range children {
		child, err := show(tx, c, s.Running && !c.Ended)
		if err != nil {
			return shown, err
		}
		shown.Children = append(shown.Children, child)
	}
	return shown, nil
}

// show shows one session. An unseen idle session carries its last reply.
func show(tx *store.Tx, s store.Session, running bool) (shownSession, error) {
	shown := shownSession{ID: s.ID, Parent: s.Parent, Title: s.Title, Running: running, Ended: s.Ended,
		Unseen: s.Unseen, Harness: s.Harness, Model: s.Model, Effort: s.Effort, Dir: s.Dir,
		Activity: s.Activity, Attention: s.Attention, Usage: s.Usage, Children: []shownSession{}}
	if s.Unseen && s.Activity == store.Idle {
		var err error
		shown.Reply, shown.ReplySeq, err = tx.LastText(s.ID)
		if err != nil {
			return shown, err
		}
	}
	return shown, nil
}

// snapshot reads everything the view shows in one transaction: workspaces by
// folder name, each with its sessions newest first, so a session keeps its
// place. merges holds the worktree sessions' merge status by session ID.
func snapshot(db *store.DB, merges map[string]workspace.Merge) ([]shownWorkspace, error) {
	list := []shownWorkspace{}
	err := db.Read(func(tx *store.Tx) error {
		workspaces, err := tx.Workspaces()
		if err != nil {
			return err
		}
		for _, w := range workspaces {
			shown := shownWorkspace{ID: w.ID, Path: w.Path, Name: filepath.Base(w.Path), NoGit: w.NoGit, Sessions: []shownSession{}}
			sessions, err := tx.Sessions(w.ID)
			if err != nil {
				return err
			}
			slices.Reverse(sessions)
			for _, s := range sessions {
				session, err := showSession(tx, s, merges)
				if err != nil {
					return err
				}
				shown.Sessions = append(shown.Sessions, session)
			}
			list = append(list, shown)
		}
		return nil
	})
	slices.SortStableFunc(list, func(a, b shownWorkspace) int { return cmp.Compare(a.Name, b.Name) })
	return list, err
}
