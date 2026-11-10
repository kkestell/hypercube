package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestBranchesAndWorktreeParameters(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
		{"branch", "feature"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "git", Path: repo}); err != nil {
			return err
		}
		return tx.AddWorkspace(store.Workspace{ID: "plain", Path: t.TempDir(), NoGit: true})
	})
	h := handler(t, db)
	do := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://localhost:4280"+path, strings.NewReader(`{"text":"hi"}`))
		r.Header.Set("Origin", "http://localhost:4280")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	w := do("GET", "/branches?workspace=git")
	var got struct {
		Current       string
		Local, Remote []string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK {
		t.Fatalf("branches = %d %s", w.Code, w.Body)
	}
	if got.Current != "main" || len(got.Local) != 2 || !reflect.DeepEqual(got.Remote, []string{}) {
		t.Errorf("branches = %+v", got)
	}
	if w := do("GET", "/branches?workspace=plain"); w.Code != http.StatusConflict {
		t.Errorf("branches of a non-Git workspace = %d", w.Code)
	}
	for _, query := range []string{"branch=x", "start=HEAD"} {
		if w := do("POST", "/session?action=new&workspace=git&harness=test&model=m&effort=low&"+query); w.Code != http.StatusBadRequest {
			t.Errorf("new with %s and no worktree = %d", query, w.Code)
		}
	}
}
