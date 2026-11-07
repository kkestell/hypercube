package ui

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestOpenInApp(t *testing.T) {
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: "/workspace with spaces"}); err != nil {
			return err
		}
		return tx.AddSession(store.Session{ID: "s", Workspace: "w", Dir: "/worktree with spaces"})
	})
	bin := t.TempDir()
	output := filepath.Join(bin, "args")
	t.Setenv("PATH", bin)
	t.Setenv("OPEN_ARGS", output)
	if err := os.WriteFile(filepath.Join(bin, "open"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$OPEN_ARGS\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	h := handler(t, db)
	for _, tc := range []struct {
		kind, id, app, path, want string
		status                    int
	}{
		{"workspace", "w", "vscode", "", "-a\nVisual Studio Code\n/workspace with spaces\n", 200},
		{"session", "s", "finder", "", "/worktree with spaces\n", 200},
		{"session", "s", "terminal", "", "-a\nTerminal\n/worktree with spaces\n", 200},
		{"path", "s", "finder", "src/file with spaces.go", "-R\n/worktree with spaces/src/file with spaces.go\n", 200},
		{"path", "s", "finder", "src/folder", "-R\n/worktree with spaces/src/folder\n", 200},
		{"path", "s", "finder", "/elsewhere/file.go", "-R\n/elsewhere/file.go\n", 200},
		{"path", "s", "finder", "-filename.go", "-R\n/worktree with spaces/-filename.go\n", 200},
		{"path", "s", "finder", "", "", 400},
		{"path", "s", "terminal", "src", "", 400},
		{"path", "missing", "finder", "src", "", 404},
		{"session", "missing", "finder", "", "", 404},
		{"workspace", "w", "other", "", "", 400},
		{"other", "/tmp", "finder", "", "", 400},
	} {
		t.Run(strings.Join([]string{tc.kind, tc.id, tc.app}, "_"), func(t *testing.T) {
			os.Remove(output)
			q := url.Values{"kind": {tc.kind}, "id": {tc.id}, "app": {tc.app}, "path": {tc.path}}
			req := httptest.NewRequest("POST", "http://localhost:4280/open?"+q.Encode(), nil)
			req.Header.Set("Origin", "http://localhost:4280")
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != tc.status {
				t.Fatalf("status %d: %s", res.Code, res.Body.String())
			}
			got, err := os.ReadFile(output)
			if tc.want == "" && os.IsNotExist(err) {
				return
			}
			if err != nil || string(got) != tc.want {
				t.Fatalf("arguments %q, error %v; want %q", got, err, tc.want)
			}
		})
	}
}
