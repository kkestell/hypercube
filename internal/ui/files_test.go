package ui

import (
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
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
	outside := filepath.Join(t.TempDir(), "outside")
	for name, target := range map[string]string{"link": "sub", "escape": outside, "broken": "missing"} {
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
		body, err := json.Marshal([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		res := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "http://localhost:4280/files/tree?session=s", strings.NewReader(string(body)))
		r.Header.Set("Origin", "http://localhost:4280")
		h.ServeHTTP(res, r)
		if res.Code != 200 {
			t.Fatalf("listing %q = %d %s", path, res.Code, res.Body)
		}
		var tree map[string][]struct {
			Name string `json:"name"`
			Dir  bool   `json:"dir"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &tree); err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, entry := range tree[path] {
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
	batch := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "http://localhost:4280/files/tree?session=s", strings.NewReader(`["","sub","missing"]`))
	r.Header.Set("Origin", "http://localhost:4280")
	h.ServeHTTP(batch, r)
	var tree map[string][]struct {
		Name string `json:"name"`
		Dir  bool   `json:"dir"`
	}
	if batch.Code != 200 || json.Unmarshal(batch.Body.Bytes(), &tree) != nil || len(tree[""]) != 9 ||
		len(tree["sub"]) != 1 || tree["missing"] != nil {
		t.Fatalf("batch listing = %d %s", batch.Code, batch.Body)
	}
	invalid := httptest.NewRecorder()
	r = httptest.NewRequest("POST", "http://localhost:4280/files/tree?session=s", strings.NewReader(`[".."]`))
	r.Header.Set("Origin", "http://localhost:4280")
	h.ServeHTTP(invalid, r)
	if invalid.Code != 400 {
		t.Fatalf("invalid batch path = %d %s", invalid.Code, invalid.Body)
	}
	write("replaced/child", "before")
	if err := os.RemoveAll(filepath.Join(dir, "replaced")); err != nil {
		t.Fatal(err)
	}
	write("replaced", "now a file")
	partial := httptest.NewRecorder()
	r = httptest.NewRequest("POST", "http://localhost:4280/files/tree?session=s",
		strings.NewReader(`["","replaced","escape"]`))
	r.Header.Set("Origin", "http://localhost:4280")
	h.ServeHTTP(partial, r)
	var refreshed map[string]json.RawMessage
	decodeErr := json.Unmarshal(partial.Body.Bytes(), &refreshed)
	replaced, recorded := refreshed["replaced"]
	var rootEntries []fileEntry
	rootErr := json.Unmarshal(refreshed[""], &rootEntries)
	if partial.Code != 200 || decodeErr != nil ||
		!recorded || string(replaced) != "null" || rootErr != nil || len(rootEntries) != 10 {
		t.Fatalf("partial batch = %d %s", partial.Code, partial.Body)
	}
	var failure struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(refreshed["escape"], &failure); err != nil || failure.Error == "" {
		t.Fatalf("unreadable folder has no error: %s", partial.Body)
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
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aX1sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	// Images must not inherit the text viewer's binary check or size limit.
	picture := string(image) + strings.Repeat("\x00", maxFile)
	write("sub/picture #1.png", picture)
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><circle r="1"/></svg>`
	write("sub/drawing.svg", svg)
	for _, tc := range []struct{ path, content, mime string }{
		{"sub/picture #1.png", picture, "image/png"},
		{"sub/drawing.svg", svg, "image/svg+xml"},
	} {
		res := get("image", "s", tc.path, "")
		if res.Code != 200 || res.Body.String() != tc.content || res.Header().Get("Content-Type") != tc.mime {
			t.Errorf("image %q = %d %v (%d bytes)", tc.path, res.Code, res.Header(), res.Body.Len())
		}
		if res.Header().Get("Content-Security-Policy") != "sandbox" || res.Header().Get("X-Content-Type-Options") != "nosniff" || res.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("image headers = %v", res.Header())
		}
	}
	for _, tc := range []struct {
		route, session, path string
		status               int
	}{
		{"file", "s", "..", 409},
		{"file", "s", outside, 200}, {"file", "s", filepath.Join(dir, "a.go"), 200},
		{"file", "s", "escape", 409},
		{"file", "s", "missing", 404},
		{"file", "s", "binary", 415}, {"file", "s", "large", 413},
		{"file", "s", "over-limit", 413}, {"file", "s", "sub", 400},
		{"file", "unknown", "a.go", 404},
		{"file", "s", "limit", 200}, {"file", "s", ".git/config", 200},
		{"image", "s", "../outside.png", 409}, {"image", "s", filepath.Join(dir, "sub", "drawing.svg"), 200}, {"image", "s", outside, 415},
		{"image", "s", "escape", 409}, {"image", "s", "missing.png", 404},
		{"image", "s", "sub", 400}, {"image", "s", "a.go", 415},
		{"image", "unknown", "sub/drawing.svg", 404},
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

func TestMentionFiles(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %s: %v", out, err)
		}
	}
	git("init", "-q")
	for name, content := range map[string]string{".gitignore": "ignored/\n*.tmp\n", "src/a.go": "a", "ignored/a": "b", "skip.tmp": "c", "other.txt": "d"} {
		name = filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "src/a.go")
	if err := os.WriteFile(filepath.Join(dir, "conflict.go"), []byte("conflict"), 0644); err != nil {
		t.Fatal(err)
	}
	hash, err := exec.Command("git", "-C", dir, "hash-object", "-w", "conflict.go").Output()
	if err != nil {
		t.Fatal(err)
	}
	index := exec.Command("git", "-C", dir, "update-index", "--index-info")
	h := strings.TrimSpace(string(hash))
	index.Stdin = strings.NewReader("100644 " + h + " 1\tconflict.go\n100644 " + h + " 2\tconflict.go\n100644 " + h + " 3\tconflict.go\n")
	if out, err := index.CombinedOutput(); err != nil {
		t.Fatalf("index: %s: %v", out, err)
	}
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: dir}); err != nil {
			return err
		}
		return tx.AddSession(store.Session{ID: "s", Workspace: "w", Dir: dir})
	})
	handler := handler(t, db)
	for _, query := range []string{"session=s", "workspace=w"} {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/mention-files?"+query, nil))
		var files []string
		if res.Code != 200 {
			t.Fatalf("%s: %d %s", query, res.Code, res.Body)
		}
		if err := json.Unmarshal(res.Body.Bytes(), &files); err != nil {
			t.Fatal(err)
		}
		got := strings.Join(files, ",")
		if got != ".gitignore,other.txt,conflict.go,src/a.go" {
			t.Errorf("%s: %s", query, got)
		}
	}
}

func TestMentionFilesOutsideGit(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"visible/a.go", ".hypercube/secret", ".svn/config", "visible/Thumbs.db"} {
		name = filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "unreadable"), 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(dir, "unreadable"), 0700) })
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "directory-link")); err != nil {
		t.Fatal(err)
	}
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: dir}); err != nil {
			return err
		}
		return tx.AddSession(store.Session{ID: "s", Workspace: "w", Dir: dir})
	})
	h := handler(t, db)
	request := func() *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/mention-files?session=s", nil))
		return res
	}
	res := request()
	if res.Code != 200 || strings.TrimSpace(res.Body.String()) != `["visible/a.go"]` {
		t.Fatalf("fallback = %d %s", res.Code, res.Body)
	}
	// A broken worktree marker is a Git error, not permission to fall back.
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	if res := request(); res.Code == 200 {
		t.Fatalf("broken Git returned fallback: %s", res.Body)
	}
}

func TestMentionFilesCapsGitResults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: elsewhere"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	output := filepath.Join(bin, "output")
	if err := os.WriteFile(output, []byte(strings.Repeat("a\x00", maxMentionFiles+1)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\n/bin/cat \"$MENTION_TEST_OUTPUT\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("MENTION_TEST_OUTPUT", output)
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: dir}); err != nil {
			return err
		}
		return tx.AddSession(store.Session{ID: "s", Workspace: "w", Dir: dir})
	})
	res := httptest.NewRecorder()
	handler(t, db).ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/mention-files?session=s", nil))
	if res.Code != 200 {
		t.Fatalf("listing = %d %s", res.Code, res.Body)
	}
	var files []string
	if err := json.Unmarshal(res.Body.Bytes(), &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != maxMentionFiles {
		t.Fatalf("got %d files, want %d", len(files), maxMentionFiles)
	}
}
