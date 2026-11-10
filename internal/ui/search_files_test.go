package ui

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kkestell/hypercube/internal/store"
)

func TestSearchFiles(t *testing.T) {
	t.Parallel()
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

func TestSearchFilesLimitsPreviewAndSkipsLargeFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	line := strings.Repeat("界", 300) + strings.Repeat("needle", 1000)
	for name, content := range map[string]string{
		"minified.js": line,
		"large.js":    "needle" + strings.Repeat("x", maxFile),
	} {
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
	if len(got) != 1 || got[0].Path != "minified.js" || len(got[0].Lines) != 1 {
		t.Fatalf("results: %#v", got)
	}
	parts := got[0].Lines[0].Parts
	if utf8.RuneCountInString(strings.Join(parts, "")) != 200 || parts[1] != "needle" || len(parts) > 35 {
		t.Fatalf("preview: %#v", parts)
	}
}

func TestSearchFilesKeepsPathOrderAndLineLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Repeat("needle\n", 300)), 0644); err != nil {
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
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &got) != nil {
		t.Fatalf("%d %s", res.Code, res.Body)
	}
	if len(got) != 4 || got[0].Path != "a" || got[1].Path != "b" || got[2].Path != "c" || got[3].Path != "d" ||
		len(got[0].Lines) != 300 || len(got[1].Lines) != 300 || len(got[2].Lines) != 300 || len(got[3].Lines) != 100 {
		t.Fatalf("results: %#v", got)
	}
}
