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
)

func TestWorkspaceFilesUseMainCheckout(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", args, output, err)
		}
	}
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-qb", "main")
	write("main.txt", "before\n")
	git("add", ".")
	git("commit", "-qm", "initial")
	write("main.txt", "checkout needle\n")
	write("index.html", `<img src="picture.svg">`)
	write("picture.svg", `<svg xmlns="http://www.w3.org/2000/svg"/>`)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "other.txt"), []byte("session needle"), 0o644); err != nil {
		t.Fatal(err)
	}
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: dir}); err != nil {
			return err
		}
		return tx.AddSession(store.Session{ID: "s", Workspace: "w", Dir: other})
	})
	h := handler(t, db)
	request := func(route string, params url.Values) *httptest.ResponseRecorder {
		t.Helper()
		method, body := "GET", ""
		if route == "files/tree" {
			method, body = "POST", `[""]`
		}
		r := httptest.NewRequest(method, "http://localhost:4280/"+route+"?"+params.Encode(), strings.NewReader(body))
		r.Header.Set("Origin", "http://localhost:4280")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, r)
		return res
	}
	for _, tc := range []struct{ route, path, want string }{
		{"files/tree", "", `"name":"main.txt"`},
		{"file", "main.txt", "checkout needle"},
		{"mention-files", "", `"main.txt"`},
		{"search-files", "", `"path":"main.txt"`},
		{"changes", "", `"branch":"main"`},
		{"diff", "main.txt", "+checkout needle"},
	} {
		t.Run(tc.route, func(t *testing.T) {
			res := request(tc.route, url.Values{"workspace": {"w"}, "path": {tc.path}, "q": {"needle"}})
			if res.Code != 200 || !strings.Contains(res.Body.String(), tc.want) || strings.Contains(res.Body.String(), "other.txt") {
				t.Fatalf("%s = %d %s", tc.route, res.Code, res.Body)
			}
			missing := request(tc.route, url.Values{"workspace": {"missing"}, "path": {tc.path}})
			if missing.Code != 404 {
				t.Fatalf("missing owner = %d %s", missing.Code, missing.Body)
			}
		})
	}
	git("switch", "-qc", "feature")
	res := request("changes", url.Values{"workspace": {"w"}})
	if res.Code != 200 || !strings.Contains(res.Body.String(), `"branch":"feature"`) {
		t.Fatalf("changed branch = %d %s", res.Code, res.Body)
	}
	res = request("file", url.Values{"session": {"s"}, "path": {"other.txt"}})
	if res.Code != 200 || res.Body.String() != "session needle" {
		t.Fatalf("session directory = %d %s", res.Code, res.Body)
	}
	// HTML and its relative resources share the same workspace owner and directory.
	res = request("file", url.Values{"workspace": {"w"}, "path": {"index.html"}})
	var file struct {
		Preview filePreview `json:"preview"`
	}
	if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &file) != nil {
		t.Fatalf("HTML metadata = %d %s", res.Code, res.Body)
	}
	preview, err := url.Parse(file.Preview.URL)
	if err != nil || !strings.HasPrefix(preview.Path, "/workspace-preview/w/") {
		t.Fatalf("preview = %s, %v", file.Preview.URL, err)
	}
	for _, address := range []string{file.Preview.URL, preview.ResolveReference(&url.URL{Path: "picture.svg"}).String()} {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280"+address, nil))
		if res.Code != 200 || res.Header().Get("Content-Security-Policy") != "sandbox" {
			t.Fatalf("preview %s = %d %s", address, res.Code, res.Body)
		}
	}
	res = request("image", url.Values{"workspace": {"w"}, "path": {"picture.svg"}})
	if res.Code != 200 {
		t.Fatalf("Markdown image = %d %s", res.Code, res.Body)
	}
	missingPreview := httptest.NewRecorder()
	h.ServeHTTP(missingPreview, httptest.NewRequest("GET", "http://localhost:4280/workspace-preview/missing"+filepath.ToSlash(filepath.Join(dir, "picture.svg")), nil))
	if missingPreview.Code != 404 {
		t.Fatalf("missing preview owner = %d", missingPreview.Code)
	}
}
