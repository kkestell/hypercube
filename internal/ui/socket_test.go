package ui

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

func TestFileOpensReachTheViewingPage(t *testing.T) {
	a := newApp(t)
	change(t, a.db, func(tx *store.Tx) error {
		return tx.AddSession(store.Session{ID: "other", Workspace: a.session.Workspace, Dir: a.session.Dir})
	})
	p := a.page()
	p.until("initial state", func() bool { return p.state.Build != "" })
	request := session.FileOpen{Path: "guide.md", Lines: []session.LineRange{{Start: 1, End: 1}}}
	a.rt.OpenFile(a.session.ID, request)
	// A second transcript follows any opens from the first selection.
	p.send(`{"select":"other"}`)
	p.until("other session transcript", func() bool { return p.resets == 1 })
	p.send(`{"select":"other"}`)
	p.until("other session selected again", func() bool { return p.resets == 2 })
	if len(p.opens) != 0 {
		t.Fatalf("open sent to another session's page: %+v", p.opens)
	}
	p.send(`{"select":"` + a.session.ID + `"}`)
	p.until("queued open", func() bool { return len(p.opens) == 1 })
	if p.resets != 3 || p.opens[0].Session != a.session.ID || p.opens[0].Path != request.Path {
		t.Fatalf("open before selecting its session or wrong open: %+v, resets %d", p.opens, p.resets)
	}
	a.rt.OpenFile(a.session.ID, session.FileOpen{Path: "second.go", Lines: []session.LineRange{}})
	p.until("live open", func() bool { return len(p.opens) == 2 })
	if p.opens[1].Path != "second.go" || p.opens[1].Lines == nil {
		t.Fatalf("live open: %+v", p.opens[1])
	}
	q := a.page()
	q.send(`{"select":"` + a.session.ID + `"}`)
	q.until("second page transcript", func() bool { return q.resets == 1 })
	// The second selection's transcript follows any opens from the first.
	q.send(`{"select":"` + a.session.ID + `"}`)
	q.until("second selection", func() bool { return q.resets == 2 })
	if len(q.opens) != 0 {
		t.Fatalf("requests delivered twice: %+v", q.opens)
	}
	if len(a.rt.TakeFileOpens(a.session.ID)) != 0 {
		t.Fatal("requests remained queued")
	}
}

func TestRequestsReachPagesViewingOtherSessions(t *testing.T) {
	a := newApp(t)
	p := a.page()
	p.until("initial state", func() bool { return p.state.Build != "" })
	a.input("!ask", http.StatusOK)
	p.until("the question", func() bool {
		return len(p.state.Requests) == 1 && p.state.Requests[0].Item.Text == "Proceed?"
	})
	q := url.Values{"action": {"answer"}, "id": {a.session.ID}, "request": {p.state.Requests[0].Item.ID}, "choice": {"yes"}}
	a.post("/session?"+q.Encode(), "", http.StatusOK)
	p.until("the answered question", func() bool { return len(p.state.Requests) == 0 })
	if p.resets != 0 {
		t.Fatalf("transcripts sent to a page without a selection: %d", p.resets)
	}
}
