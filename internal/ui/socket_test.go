package ui

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"

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

func TestTranscriptImagesLoadByLink(t *testing.T) {
	a := newApp(t)
	p := a.page()
	p.send(`{"select":"` + a.session.ID + `"}`)
	p.until("the transcript", func() bool { return p.resets == 1 })
	a.input("!image", http.StatusOK)
	var links []string
	p.until("the image item", func() bool {
		for _, it := range p.items[a.session.ID] {
			if it.Kind == harness.KindText && len(it.Images) == 1 {
				links = it.Images
				return true
			}
		}
		return false
	})
	p.send(`{"select":"` + a.session.ID + `"}`)
	p.until("the transcript again", func() bool { return p.resets == 2 })
	if strings.Contains(p.lastMsg, "iVBORw0KGgo") || !strings.Contains(p.lastMsg, "/transcript-image?") {
		t.Fatalf("transcript carries image data or no link: %s", p.lastMsg)
	}

	get := func(link string) *http.Response {
		res, err := http.Get(a.server.URL + link)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	res := get(links[0])
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("image: %d %q %q", res.StatusCode, res.Header.Get("Content-Type"), body)
	}
	if res := get(strings.Replace(links[0], "index=0", "index=1", 1)); res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing image: status %d", res.StatusCode)
	}
}
