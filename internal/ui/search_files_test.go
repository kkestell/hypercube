package ui

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestSearchFiles(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", out, err)
	}
	for name, content := range map[string]string{".gitignore": "ignored\n", "ignored": "needle", "a.txt": "first\nA NEEDLE and needle.\n", "binary": "needle\x00hidden"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: dir}); err != nil {
			return err
		}
		return tx.AddSession(store.Session{ID: "s", Workspace: "w", Dir: dir})
	})
	res := httptest.NewRecorder()
	handler(t, db).ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/search-files?session=s&q=needle", nil))
	var got []searchFile
	if res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "a.txt" || len(got[0].Lines) != 1 || got[0].Lines[0].Line != 2 || len(got[0].Lines[0].Parts) != 5 || got[0].Lines[0].Parts[1] != "NEEDLE" {
		t.Fatalf("results: %#v", got)
	}
}
