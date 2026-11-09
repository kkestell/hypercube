package ui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func (a *app) get(path string) (int, []byte) {
	a.t.Helper()
	res, err := http.Get(a.server.URL + path)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	return res.StatusCode, body
}

func TestHarnesses(t *testing.T) {
	a := newApp(t)
	code, body := a.get("/harnesses")
	var harnesses []harness.Harness
	if err := json.Unmarshal(body, &harnesses); code != 200 || err != nil || len(harnesses) != 1 || harnesses[0].ID != "test" || !harnesses[0].Installed {
		t.Fatalf("harnesses = %d %s", code, body)
	}
}

func TestNewSessionAndSelection(t *testing.T) {
	a := newApp(t)
	query := url.Values{"action": {"new"}, "workspace": {a.session.Workspace}, "harness": {"test"}, "model": {"plain"}}
	a.post("/session?"+query.Encode(), `{"text":"hello"}`, http.StatusOK)
	query = url.Values{"action": {"selection"}, "id": {a.session.ID}, "model": {"next"}, "effort": {"high"}}
	a.post("/session?"+query.Encode(), "", http.StatusOK)
	if err := a.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(a.session.ID)
		if err == nil && (s.Model != "next" || s.Effort != "high") {
			t.Fatalf("selected %+v", s)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
