package ui

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		name = filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package main\n")
	write("sub/b.md", "# B\n")
	for _, name := range []string{".git/config", ".DS_Store", ".svn/entries", ".hg/data", "Thumbs.db", ".hypercube/worker", "sub/.git/config"} {
		write(name, "hidden")
	}
	write("binary", "a\x00b")
	write("limit", strings.Repeat("a", maxFile))
	for name, size := range map[string]int64{"large": 1<<20 + 1, "over-limit": maxFile + 1} {
		write(name, "")
		if err := os.Truncate(filepath.Join(dir, name), size); err != nil {
			t.Fatal(err)
		}
	}
	for name, target := range map[string]string{"link": "sub", "escape": filepath.Join(t.TempDir(), "outside"), "broken": "missing"} {
		if name == "escape" {
			if err := os.WriteFile(target, []byte("outside"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
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
	h := handler(t, db)
	get := func(route, session, path, etag string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", "http://localhost:4280/"+route+"?"+url.Values{"session": {session}, "path": {path}}.Encode(), nil)
		if etag != "" {
			r.Header.Set("If-None-Match", etag)
		}
		res := httptest.NewRecorder()
		h.ServeHTTP(res, r)
		return res
	}
	listing := func(path string) map[string]bool {
		t.Helper()
		res := get("files", "s", path, "")
		if res.Code != 200 {
			t.Fatalf("listing %q = %d %s", path, res.Code, res.Body)
		}
		var entries []struct {
			Name string `json:"name"`
			Dir  bool   `json:"dir"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &entries); err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, entry := range entries {
			got[entry.Name] = entry.Dir
		}
		return got
	}
	root := listing("")
	for _, name := range []string{"a.go", "binary", "large", "limit", "over-limit", "escape", "broken"} {
		if isDir, ok := root[name]; !ok || isDir {
			t.Errorf("root[%q] = %v, %v; want file", name, isDir, ok)
		}
	}
	if !root["sub"] || !root["link"] || len(root) != 9 {
		t.Fatalf("root listing = %v", root)
	}
	for _, path := range []string{"sub", "link"} {
		got := listing(path)
		if isDir, ok := got["b.md"]; !ok || isDir || len(got) != 1 {
			t.Fatalf("listing %q = %v", path, got)
		}
	}
	res := get("file", "s", "a.go", "")
	etag := res.Header().Get("ETag")
	if res.Code != 200 || res.Body.String() != "package main\n" || etag == "" || res.Header().Get("Content-Type") != "text/plain; charset=utf-8" || res.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("read = %d %v %s", res.Code, res.Header(), res.Body)
	}
	if res := get("file", "s", "a.go", etag); res.Code != 304 || res.Body.Len() != 0 {
		t.Fatalf("unchanged = %d %s", res.Code, res.Body)
	}
	write("a.go", "package changed\n")
	if res := get("file", "s", "a.go", etag); res.Code != 200 || res.Header().Get("ETag") == etag || res.Body.String() != "package changed\n" {
		t.Fatalf("changed = %d %s", res.Code, res.Body)
	}
	for _, tc := range []struct {
		route, session, path string
		status               int
	}{
		{"file", "s", "..", 409}, {"files", "s", "..", 409},
		{"file", "s", "/etc/passwd", 409}, {"files", "s", "/etc", 409},
		{"file", "s", "escape", 409}, {"files", "s", "escape", 409},
		{"file", "s", "missing", 404}, {"files", "s", "missing", 404},
		{"file", "s", "binary", 415}, {"file", "s", "large", 413},
		{"file", "s", "over-limit", 413}, {"file", "s", "sub", 400},
		{"file", "unknown", "a.go", 404}, {"files", "unknown", "", 404},
		{"file", "s", "limit", 200}, {"file", "s", ".git/config", 200},
	} {
		res := get(tc.route, tc.session, tc.path, "")
		if res.Code != tc.status {
			t.Errorf("/%s session=%q path=%q = %d %s; want %d", tc.route, tc.session, tc.path, res.Code, res.Body, tc.status)
		}
	}
}

func TestChildFilesUseItsDirectory(t *testing.T) {
	a := newApp(t)
	child := a.child("files")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "child.txt"), []byte("child contents"), 0o644); err != nil {
		t.Fatal(err)
	}
	change(t, a.db, func(tx *store.Tx) error { child.Dir = dir; return tx.SaveSession(child) })
	h := handler(t, a.db)
	get := func(route string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/"+route+"?session="+child.ID+"&path=child.txt", nil))
		return res
	}
	if res := get("file"); res.Code != 200 || res.Body.String() != "child contents" {
		t.Fatalf("child file=%d %s", res.Code, res.Body)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if res := get("files"); res.Code != 404 {
		t.Fatalf("removed directory=%d %s", res.Code, res.Body)
	}
}
