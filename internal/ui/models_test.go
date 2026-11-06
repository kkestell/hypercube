package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

func TestModelsCacheAndRefresh(t *testing.T) {
	a := newApp(t)
	log := filepath.Join(t.TempDir(), "models.log")
	t.Setenv("HYPERCUBE_TEST_MODELS_LOG", log)
	query := url.Values{"harness": {"test"}, "workspace": {a.session.Workspace}}
	get := func(wantCalls int) []byte {
		t.Helper()
		code, body := a.get("/models?" + query.Encode())
		if code != http.StatusOK {
			t.Fatalf("models = %d %s", code, body)
		}
		requests, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		if got := bytes.Count(requests, []byte("\n")); got != wantCalls {
			t.Fatalf("plugin calls = %d, want %d", got, wantCalls)
		}
		return body
	}
	fresh := get(1)
	get(1)
	// Seed a visibly older catalog to show that refresh replaces the saved value.
	stale := `{"models":[{"id":"old"}]}`
	change(t, a.db, func(tx *store.Tx) error { return tx.SaveCatalog("test", a.session.Dir, "", stale) })
	if got := get(1); string(bytes.TrimSpace(got)) != stale {
		t.Fatalf("cached catalog = %s", got)
	}
	query.Set("refresh", "1")
	if got := get(2); !bytes.Equal(got, fresh) {
		t.Fatalf("refreshed catalog = %s, want %s", got, fresh)
	}
	query.Del("refresh")
	if got := get(2); !bytes.Equal(got, fresh) {
		t.Fatalf("saved refresh = %s, want %s", got, fresh)
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
