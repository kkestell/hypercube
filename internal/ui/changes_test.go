package ui

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

func TestBranchChangesRoutes(t *testing.T) {
	checkout := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	write := func(dir, path, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, path), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(checkout, "init", "-qb", "main")
	write(checkout, "file", "base\n")
	run(checkout, "add", ".")
	run(checkout, "commit", "-qm", "base")
	base, err := workspace.Head(checkout)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(t.TempDir(), "parent")
	child := filepath.Join(t.TempDir(), "child")
	run(checkout, "worktree", "add", "-qb", "parent", parent)
	write(parent, "file", "parent\n")
	run(parent, "commit", "-qam", "parent")
	run(parent, "worktree", "add", "-qb", "child", child)
	write(child, "file", "child\n")
	run(child, "commit", "-qam", "child")
	write(child, "new", "one\ntwo")
	write(child, "binary", "a\x00b")
	write(child, "large", strings.Repeat("x", maxFile+1))
	missing := filepath.Join(t.TempDir(), "gone")
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: checkout}); err != nil {
			return err
		}
		for _, session := range []store.Session{
			{ID: "parent", Workspace: "w", Dir: parent, Base: base},
			{ID: "child", Workspace: "w", Parent: "parent", Dir: child, Base: base},
			{ID: "shared", Workspace: "w", Dir: checkout},
			{ID: "missing", Workspace: "w", Dir: missing, Base: base},
		} {
			if err := tx.AddSession(session); err != nil {
				return err
			}
		}
		return nil
	})
	h := handler(t, db)
	get := func(route string, params url.Values) *httptest.ResponseRecorder {
		t.Helper()
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/"+route+"?"+params.Encode(), nil))
		return res
	}
	params := url.Values{"session": {"child"}, "comparison": {"branch"}, "base": {"HEAD"}, "checkout": {parent}}
	res := get("changes", params)
	var changes workspace.ChangesResult
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &changes) != nil {
		t.Fatalf("changes = %d %s", res.Code, res.Body)
	}
	if changes.Branch != "child" || changes.Target != "main" || len(changes.Files) != 4 {
		t.Fatalf("changes = %+v", changes)
	}
	for _, file := range changes.Files {
		if file.Path == "new" && (file.Added == nil || *file.Added != 2) {
			t.Errorf("new counts = %+v", file)
		}
	}
	params.Set("path", "file")
	res = get("diff", params)
	if res.Code != 200 || !strings.Contains(res.Body.String(), "-base\n+child\n") {
		t.Fatalf("child diff used wrong owner: %d %s", res.Code, res.Body)
	}
	for _, tc := range []struct {
		route, query string
		status       int
	}{
		{"changes", "session=child", 200},
		{"changes", "session=child&comparison=unknown", 400},
		{"diff", "session=child&comparison=unknown&path=file", 400},
		{"changes", "workspace=w&comparison=branch", 409},
		{"diff", "session=shared&comparison=branch&path=file", 409},
		{"changes", "session=missing&comparison=branch", 404},
		{"diff", "session=child&comparison=branch&path=../file", 400},
		{"diff", "session=child&comparison=branch&path=binary", 415},
		{"diff", "session=child&comparison=branch&path=large", 413},
	} {
		query, _ := url.ParseQuery(tc.query)
		res := get(tc.route, query)
		if res.Code != tc.status {
			t.Errorf("%s %s = %d %s", tc.route, tc.query, res.Code, res.Body)
		}
	}
	// An unrelated checkout history must fail visibly, not compare with HEAD.
	run(checkout, "checkout", "--orphan", "unrelated")
	run(checkout, "commit", "-qm", "unrelated")
	for _, route := range []string{"changes", "diff"} {
		res := get(route, params)
		if res.Code < 400 {
			t.Errorf("unrelated %s = %d %s", route, res.Code, res.Body)
		}
	}
}

func TestChangesRoutes(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("tracked", "before\n")
	write("binary", "old\x00bytes")
	run("add", ".")
	run("commit", "-qm", "files")
	write("tracked", "after\n")
	write("binary", "new\x00bytes")
	write("sub/new.txt", "first\nlast")
	write("sub/new.bin", "\x00new")
	write("large", strings.Repeat("x", maxFile))
	if err := os.Symlink("missing", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	// Git lists an untracked nested repository as a directory, not its contents.
	run("init", "-q", filepath.Join(dir, "nested"))
	write("nested/file", "nested contents")
	write("vanished", "temporary")
	// Remove a file after status has listed it but before the HTTP route counts it.
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$*\" in *--raw*) rm -f \"$HYPERCUBE_TEST_VANISHED\";; esac\nexec \"$HYPERCUBE_TEST_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HYPERCUBE_TEST_GIT", realGit)
	t.Setenv("HYPERCUBE_TEST_VANISHED", filepath.Join(dir, "vanished"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	outside := t.TempDir()
	removed := filepath.Join(t.TempDir(), "removed")
	if err := os.Mkdir(removed, 0o755); err != nil {
		t.Fatal(err)
	}
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: dir}); err != nil {
			return err
		}
		for id, path := range map[string]string{"root": dir, "sub": filepath.Join(dir, "sub"), "outside": outside, "removed": removed} {
			if err := tx.AddSession(store.Session{ID: id, Workspace: "w", Dir: path}); err != nil {
				return err
			}
		}
		return nil
	})
	if err := os.Remove(removed); err != nil {
		t.Fatal(err)
	}
	h := handler(t, db)
	get := func(route, session, path string) *httptest.ResponseRecorder {
		t.Helper()
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/"+route+"?"+url.Values{"session": {session}, "path": {path}}.Encode(), nil))
		return res
	}
	list := func(session string) (string, map[string]workspace.Change) {
		t.Helper()
		res := get("changes", session, "")
		if res.Code != 200 {
			t.Fatalf("changes = %d %s", res.Code, res.Body)
		}
		var changes workspace.ChangesResult
		if err := json.Unmarshal(res.Body.Bytes(), &changes); err != nil {
			t.Fatal(err)
		}
		files := map[string]workspace.Change{}
		for _, file := range changes.Files {
			files[file.Path] = file
		}
		return changes.Branch, files
	}
	branch, files := list("root")
	if branch != "main" || len(files) != 7 {
		t.Fatalf("changes = %s %+v", branch, files)
	}
	if file, ok := files["nested/"]; !ok || file.Status != "untracked" || file.Added != nil || file.Removed != nil {
		t.Errorf("nested repository = %+v, %v", file, ok)
	}
	if _, ok := files["vanished"]; ok {
		t.Error("disappearing untracked file was retained")
	}
	for path, count := range map[string]int{"sub/new.txt": 2, "link": 1} {
		file := files[path]
		if file.Status != "untracked" || file.Added == nil || *file.Added != count || file.Removed == nil || *file.Removed != 0 {
			t.Errorf("%s = %+v", path, file)
		}
	}
	if file := files["sub/new.bin"]; file.Added != nil || file.Removed != nil {
		t.Errorf("binary counts = %+v", file)
	}
	if _, sub := list("sub"); len(sub) != 2 || sub["new.txt"].Status != "untracked" || sub["new.bin"].Status != "untracked" {
		t.Errorf("subfolder changes = %+v", sub)
	}
	for _, tc := range []struct {
		route, session, path string
		status               int
	}{
		{"changes", "outside", "", 200}, {"changes", "removed", "", 404},
		{"diff", "root", "tracked", 200}, {"diff", "root", "binary", 415},
		{"diff", "root", "large", 413}, {"file", "sub", "new.bin", 415},
		{"diff", "sub", "new.txt", 200}, {"diff", "removed", "tracked", 404},
		{"diff", "sub", "../tracked", 400}, {"diff", "sub", "", 400},
	} {
		res := get(tc.route, tc.session, tc.path)
		if res.Code != tc.status {
			t.Errorf("%+v = %d %s", tc, res.Code, res.Body)
		}
		if tc.route == "changes" && tc.session == "outside" && res.Body.String() != "null\n" {
			t.Errorf("outside = %s", res.Body)
		}
		if tc.route == "diff" && tc.session == "root" && tc.path == "tracked" && (!strings.Contains(res.Body.String(), "-before\n+after\n") || res.Header().Get("Cache-Control") != "no-cache") {
			t.Errorf("patch = %s", res.Body)
		}
	}
}
