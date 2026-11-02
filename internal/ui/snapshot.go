package ui

import (
	"cmp"
	"path/filepath"
	"slices"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// shownWorkspace, shownSession, and shownAgent are what the browser view shows.
type shownWorkspace struct {
	ID       string         `json:"id"`
	Path     string         `json:"path"`
	Name     string         `json:"name"`
	Sessions []shownSession `json:"sessions"`
}

type shownSession struct {
	ID       string       `json:"id"`
	Title    string       `json:"title"`
	Running  bool         `json:"running"`
	Unseen   bool         `json:"unseen"`
	Reply    string       `json:"reply,omitempty"`
	ReplySeq int64        `json:"replySeq,omitempty"`
	Agents   []shownAgent `json:"agents"`
}

type shownAgent struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Role      string        `json:"role"`
	Harness   string        `json:"harness"`
	Model     string        `json:"model"`
	Effort    string        `json:"effort"`
	Dir       string        `json:"dir"`
	Activity  string        `json:"activity"`
	Attention bool          `json:"attention"`
	Status    string        `json:"status"`
	StatusAt  int64         `json:"statusAt"`
	Usage     harness.Usage `json:"usage"`
}

// snapshot reads everything the view shows in one transaction: workspaces by
// folder name, with running sessions first, each group by most recent activity.
func snapshot(db *store.DB) ([]shownWorkspace, error) {
	list := []shownWorkspace{}
	err := db.Read(func(tx *store.Tx) error {
		workspaces, err := tx.Workspaces()
		if err != nil {
			return err
		}
		for _, w := range workspaces {
			shown := shownWorkspace{ID: w.ID, Path: w.Path, Name: filepath.Base(w.Path), Sessions: []shownSession{}}
			sessions, err := tx.Sessions(w.ID)
			if err != nil {
				return err
			}
			slices.SortStableFunc(sessions, func(a, b store.Session) int {
				if a.Running != b.Running {
					if a.Running {
						return -1
					}
					return 1
				}
				return cmp.Or(cmp.Compare(b.LastActive, a.LastActive), cmp.Compare(b.Created, a.Created))
			})
			for _, s := range sessions {
				agents, err := tx.Agents(s.ID)
				if err != nil {
					return err
				}
				session := shownSession{ID: s.ID, Title: s.Title, Running: s.Running, Unseen: s.Unseen, Agents: []shownAgent{}}
				for _, a := range agents {
					session.Agents = append(session.Agents, shownAgent{a.ID, a.Name, a.Role, a.Harness, a.Model, a.Effort,
						a.Dir, a.Activity, a.Attention, a.Status, a.StatusAt, a.Usage})
					if s.Unseen && a.Role == store.Lead && a.Activity == store.Idle {
						if session.Reply, session.ReplySeq, err = tx.LastText(a.ID); err != nil {
							return err
						}
					}
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
