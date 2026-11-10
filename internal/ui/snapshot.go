package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kkestell/hypercube/internal/github"
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
	GitHub   *github.Status `json:"github,omitempty"`
	Sessions []shownSession `json:"sessions"`
}

type shownSession struct {
	ID        string        `json:"id"`
	Parent    string        `json:"parent"`
	Title     string        `json:"title"`
	Ready     bool          `json:"ready"`
	Ended     bool          `json:"ended"`
	Archived  int64         `json:"archived,omitempty"`
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
	// PullRequest is the open pull request for the session's branch.
	PullRequest *github.PullRequest `json:"pullRequest,omitempty"`
	Children    []shownSession      `json:"children"`
}

// showSession shows a top-level session with its children and, for a
// worktree session, its merge status. In a GitHub workspace (status is not
// nil) each session shows its open pull request.
func showSession(tx *store.Tx, s store.Session, root string, status *github.Status, merges map[string]workspace.Merge) (shownSession, error) {
	shown, err := show(tx, s)
	if err != nil {
		return shown, err
	}
	shown.PullRequest = pullRequest(s, root, status)
	shown.Worktree = s.Base != ""
	if m, ok := merges[s.ID]; ok {
		shown.Merge = &m
	}
	children, err := tx.Children(s.ID)
	if err != nil {
		return shown, err
	}
	for _, c := range children {
		child, err := show(tx, c)
		if err != nil {
			return shown, err
		}
		child.PullRequest = pullRequest(c, root, status)
		shown.Children = append(shown.Children, child)
	}
	return shown, nil
}

// pullRequest finds the open pull request from a session's branch, if any. A
// session in the checkout is on the checkout's current branch, which the
// session's saved branch may no longer name; a session in a managed worktree is
// on that worktree's branch. A pull request from a fork never matches.
func pullRequest(s store.Session, root string, status *github.Status) *github.PullRequest {
	if status == nil || s.Ended {
		return nil
	}
	var branch string
	switch {
	case within(s.Dir, filepath.Join(root, ".hypercube")):
		branch = s.Branch
	case within(s.Dir, root):
		branch = status.Branch
	}
	if branch == "" {
		return nil
	}
	for _, pr := range status.PullRequests {
		if !pr.Fork && pr.Branch == branch {
			return &pr
		}
	}
	return nil
}

// within reports whether path is dir or inside it.
func within(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// show shows one session. An unseen idle session carries its last reply.
func show(tx *store.Tx, s store.Session) (shownSession, error) {
	shown := shownSession{ID: s.ID, Parent: s.Parent, Title: s.Title, Ended: s.Ended, Archived: s.Archived,
		Ready: !s.Ended && s.Ready != 0, Unseen: s.Unseen, Harness: s.Harness, Model: s.Model, Effort: s.Effort, Dir: s.Dir,
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
// place, and the harnesses' usage limits. merges holds the worktree sessions'
// merge status by session ID, and statuses holds the GitHub workspaces' status
// by workspace ID.
func snapshot(db *store.DB, merges map[string]workspace.Merge, statuses map[string]github.Status) ([]shownWorkspace, []store.Limit, error) {
	list := []shownWorkspace{}
	var limits []store.Limit
	err := db.Read(func(tx *store.Tx) error {
		var err error
		if limits, err = tx.Limits(); err != nil {
			return err
		}
		workspaces, err := tx.Workspaces()
		if err != nil {
			return err
		}
		for _, w := range workspaces {
			shown := shownWorkspace{ID: w.ID, Path: w.Path, Name: filepath.Base(w.Path), NoGit: w.NoGit, Sessions: []shownSession{}}
			var status *github.Status
			if st, ok := statuses[w.ID]; ok {
				status = &st
				shown.GitHub = status
			}
			sessions, err := tx.Sessions(w.ID)
			if err != nil {
				return err
			}
			slices.Reverse(sessions)
			for _, s := range sessions {
				session, err := showSession(tx, s, w.Path, status, merges)
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
	return list, limits, err
}
