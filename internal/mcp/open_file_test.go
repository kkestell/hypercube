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
	db, rt := family(t, false, dir, "")
	client := connect(t, serve(t, db, rt).URL, "session-1")
	for _, tc := range []struct {
		name, path string
		lines      []map[string]int
		want       []session.LineRange
		fail       bool
	}{
		{name: "relative", path: "docs/../docs/guide.md", want: []session.LineRange{}},
		{name: "absolute", path: filepath.Join(dir, "docs", "guide.md"), want: []session.LineRange{}},
		{name: "omitted end", path: "docs/guide.md", lines: []map[string]int{{"start": 1}}, want: []session.LineRange{{Start: 1, End: 1}}},
		{name: "ranges", path: "docs/guide.md", lines: []map[string]int{{"start": 1, "end": 2}, {"start": 3, "end": 3}}, want: []session.LineRange{{Start: 1, End: 2}, {Start: 3, End: 3}}},
		{name: "parent", path: "../outside", fail: true},
		{name: "outside", path: outside, fail: true},
		{name: "symlink outside", path: "escape", fail: true},
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
			want := []session.FileOpen{{Path: "docs/guide.md", Lines: tc.want}}
			if !reflect.DeepEqual(opens, want) {
				t.Fatalf("opens = %+v, want %+v", opens, want)
			}
		})
	}
}
