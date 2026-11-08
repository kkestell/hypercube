package ui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
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

func TestHarnessesAndModels(t *testing.T) {
	a := newApp(t)
	code, body := a.get("/harnesses")
	var harnesses []harness.Harness
	if err := json.Unmarshal(body, &harnesses); code != 200 || err != nil || len(harnesses) != 1 || harnesses[0].ID != "test" || !harnesses[0].Installed {
		t.Fatalf("harnesses = %d %s", code, body)
	}
	code, body = a.get("/models")
	if code != 200 || strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("models = %d %s", code, body)
	}
	a.post("/models", `{"harness":"uninstalled","model":"native/id","efforts":["low","high"],"default_effort":"high"}`, 200)
	_, body = a.get("/models")
	var list []store.ModelEntry
	if err := json.Unmarshal(body, &list); err != nil || len(list) != 1 || list[0].ID == "" {
		t.Fatalf("models = %s, %v", body, err)
	}
	entry := list[0]
	entry.Model = "edited"
	data, _ := json.Marshal(entry)
	a.post("/models", string(data), 200)
	for _, body := range []string{`{`, `{"harness":"x","model":"m","unknown":true}`, `{"harness":"x"}`, `{"harness":"x","model":"m","efforts":["low","high"]}`} {
		a.post("/models", body, 400)
	}
	a.post("/models", `{"id":"missing","harness":"x","model":"m"}`, 404)
	a.post("/models", `{"harness":"uninstalled","model":"edited"}`, 409)
	req, _ := http.NewRequest(http.MethodDelete, a.server.URL+"/models/"+entry.ID, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ = io.ReadAll(res.Body)
	if res.StatusCode != 200 || strings.TrimSpace(string(data)) != "[]" {
		t.Fatalf("delete = %d %s", res.StatusCode, data)
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
