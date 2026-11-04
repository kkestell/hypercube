package mcp

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kkestell/hypercube/internal/session"
)

func TestOpenFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "guide.md"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, "notes"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	db, rt := family(t, false, dir, "")
	client := connect(t, serve(t, db, rt).URL, "session-1")
	for _, tc := range []struct {
		name, path string
		lines      []map[string]int
		want       []session.LineRange
		view       string
		fail       bool
	}{
		{name: "relative", path: "docs/../docs/guide.md", want: []session.LineRange{}},
		{name: "absolute", path: filepath.Join(dir, "docs", "guide.md"), want: []session.LineRange{}},
		{name: "omitted end", path: "docs/guide.md", lines: []map[string]int{{"start": 1}}, want: []session.LineRange{{Start: 1, End: 1}}},
		{name: "ranges", path: "docs/guide.md", lines: []map[string]int{{"start": 1, "end": 2}, {"start": 3, "end": 3}}, want: []session.LineRange{{Start: 1, End: 2}, {Start: 3, End: 3}}},
		{name: "outside", path: outside, view: real, want: []session.LineRange{}},
		{name: "parent", path: "../" + filepath.Base(filepath.Dir(outside)) + "/outside", view: real, want: []session.LineRange{}},
		{name: "symlink outside", path: "escape", view: real, want: []session.LineRange{}},
		{name: "home", path: "~/notes", view: filepath.Join(realHome, "notes"), want: []session.LineRange{}},
		{name: "missing", path: "missing", fail: true},
		{name: "folder", path: "docs", fail: true},
		{name: "zero start", path: "docs/guide.md", lines: []map[string]int{{"start": 0, "end": 1}}, fail: true},
		{name: "reversed", path: "docs/guide.md", lines: []map[string]int{{"start": 2, "end": 1}}, fail: true},
		{name: "past last line", path: "docs/guide.md", lines: []map[string]int{{"start": 4}}, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"path": tc.path}
			if tc.lines != nil {
				args["lines"] = tc.lines
			}
			text, failed := callTool(t, client, "open_file", args)
			if failed != tc.fail {
				t.Fatalf("open_file: %s (failed %v)", text, failed)
			}
			opens := rt.TakeFileOpens("session-1")
			if tc.fail {
				if len(opens) != 0 {
					t.Fatalf("refused request queued: %+v", opens)
				}
				return
			}
			if tc.view == "" {
				tc.view = "docs/guide.md"
			}
			want := []session.FileOpen{{Path: tc.view, Lines: tc.want}}
			if !reflect.DeepEqual(opens, want) {
				t.Fatalf("opens = %+v, want %+v", opens, want)
			}
		})
	}
}
