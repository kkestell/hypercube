package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/github"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
)

func TestHTTPSubmissionCorrelation(t *testing.T) {
	t.Parallel()
	a := newApp(t)
	h := Handler(a.db, configFile(a.db), a.rt, &github.Watcher{})
	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "http://localhost:4280"+path, strings.NewReader(body))
		r.Header.Set("Origin", "http://localhost:4280")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	const id = "submission-12345678-1234-1234-1234-123456789abc"
	w := post("/session?action=input&id="+a.session.ID, `{"text":"hello","submission":"`+id+`","images":[{"mime_type":"image/png","data":"iVBORw0KGgo="}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("input: %d %s", w.Code, w.Body)
	}
	items, err := a.rt.Transcript(a.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.ID == id {
			found = item.Kind == harness.KindUser && item.Text == "hello" && len(item.Images) == 1
		}
	}
	if !found {
		t.Fatalf("correlated user item missing: %+v", items)
	}
	for _, bad := range []string{"host-existing", "submission-1234", strings.Repeat("x", 1000)} {
		body, _ := json.Marshal(map[string]string{"text": "bad", "submission": bad})
		if w := post("/session?action=input&id="+a.session.ID, string(body)); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid correlation accepted: %d %s", w.Code, w.Body)
		}
	}
	for _, test := range []struct {
		name, workspace, text string
		status                int
		created               bool
	}{
		{"accepted", a.session.Workspace, "hello", http.StatusOK, true},
		{"first input refused", a.session.Workspace, "!reject", http.StatusConflict, true},
		{"workspace missing", "missing", "hello", http.StatusConflict, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := post("/session?action=new&workspace="+test.workspace+"&harness=test&model=m&effort=low", `{"text":"`+test.text+`","submission":"`+id+`"}`)
			var result map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatalf("creation response: %d %s", w.Code, w.Body)
			}
			if w.Code != test.status || (result["session"] != "") != test.created || (result["error"] != "") != (test.status != http.StatusOK) {
				t.Fatalf("creation result: %d %v", w.Code, result)
			}
			if test.status == http.StatusOK {
				items, err := a.rt.Transcript(result["session"])
				if err != nil || !slices.ContainsFunc(items, func(item session.TranscriptItem) bool { return item.ID == id && item.Kind == harness.KindUser }) {
					t.Fatalf("created transcript: %+v, %v", items, err)
				}
			}
		})
	}
}
