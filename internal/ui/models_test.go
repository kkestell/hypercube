package ui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
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

func TestHarnessesAndModels(t *testing.T) {
	a := newApp(t)
	code, body := a.get("/harnesses")
	var list []harness.Harness
	if err := json.Unmarshal(body, &list); code != http.StatusOK || err != nil || len(list) != 1 || list[0].ID != "test" || !list[0].Installed {
		t.Fatalf("harnesses = %d %s", code, body)
	}
	for _, query := range []url.Values{
		{"harness": {"test"}, "workspace": {a.session.Workspace}, "model": {"m"}},
		{"harness": {"test"}, "session": {a.session.ID}, "model": {"m"}},
	} {
		code, body := a.get("/models?" + query.Encode())
		want, _ := harnesstest.Catalog(harness.ModelsRequest{Model: "m"})
		var got harness.Catalog
		if err := json.Unmarshal(body, &got); code != http.StatusOK || err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("models(%v) = %d %s", query, code, body)
		}
	}
	for _, query := range []url.Values{
		{"harness": {"test"}},
		{"harness": {"missing"}, "workspace": {a.session.Workspace}},
		{"harness": {"test"}, "workspace": {a.session.Workspace}, "model": {"gone"}},
	} {
		if code, body := a.get("/models?" + query.Encode()); code == http.StatusOK {
			t.Fatalf("models(%v) = %s", query, body)
		}
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
