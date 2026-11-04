package workspace

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestChanges(t *testing.T) {
	root, run := gitRepo(t)
	for name, text := range map[string]string{"modified": "one\ntwo\n", "deleted": "old\n", "binary": "a\x00b"} {
		write(t, filepath.Join(root, name), text)
	}
	run(root, "add", ".")
	run(root, "commit", "-qm", "files")
	write(t, filepath.Join(root, "modified"), "one\nstaged\n")
	run(root, "add", "modified")
	write(t, filepath.Join(root, "modified"), "one\nnew\nthird\n")
	write(t, filepath.Join(root, "binary"), "c\x00d")
	if err := os.Remove(filepath.Join(root, "deleted")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "added"), "staged\n")
	run(root, "add", "added")
	write(t, filepath.Join(root, "folder/untracked"), "new")
	changes, err := Changes(root)
	if err != nil {
		t.Fatal(err)
	}
	if changes.Branch != "main" || len(changes.Files) != 5 {
		t.Fatalf("changes = %+v", changes)
	}
	got := map[string]Change{}
	for _, file := range changes.Files {
		got[file.Path] = file
	}
	for _, tc := range []struct {
		path, status   string
		added, removed int
	}{
		{"modified", "modified", 2, 1}, {"deleted", "deleted", 0, 1}, {"added", "added", 1, 0},
	} {
		file := got[tc.path]
		if file.Status != tc.status || file.Added == nil || file.Removed == nil || *file.Added != tc.added || *file.Removed != tc.removed {
			t.Errorf("%s = %+v", tc.path, file)
		}
	}
	if got["folder/untracked"].Status != "untracked" {
		t.Errorf("untracked = %+v", got)
	}
	if binary := got["binary"]; binary.Status != "modified" || binary.Added != nil || binary.Removed != nil {
		t.Errorf("binary = %+v", binary)
	}
	run(root, "add", ".")
	run(root, "commit", "-qm", "changes")
	changes, err = Changes(root)
	if err != nil || len(changes.Files) != 0 {
		t.Fatalf("clean = %+v, %v", changes, err)
	}
	run(root, "checkout", "--detach", "-q")
	changes, err = Changes(root)
	if err != nil || changes.Branch != "" {
		t.Fatalf("detached = %+v, %v", changes, err)
	}
}

func TestChangesUnborn(t *testing.T) {
	_, run := gitRepo(t)
	root := t.TempDir()
	run(root, "init", "-q", "-b", "main")
	write(t, filepath.Join(root, "staged"), "one\n")
	run(root, "add", "staged")
	write(t, filepath.Join(root, "untracked"), "two\n")
	changes, err := Changes(root)
	if err != nil || changes.Branch != "main" || len(changes.Files) != 2 {
		t.Fatalf("unborn = %+v, %v", changes, err)
	}
	for _, file := range changes.Files {
		if file.Path == "staged" && (file.Status != "added" || file.Added == nil || *file.Added != 1) {
			t.Errorf("staged = %+v", file)
		}
		if file.Path == "untracked" && file.Status != "untracked" {
			t.Errorf("untracked = %+v", file)
		}
	}
	patch, err := Diff(root, "staged", 4096)
	if err != nil || !strings.Contains(patch, "+one\n") {
		t.Fatalf("unborn diff = %q, %v", patch, err)
	}
}

func TestDiff(t *testing.T) {
	root, run := gitRepo(t)
	text := "first\n" + strings.Repeat("middle\n", 12) + "last\n\n"
	write(t, filepath.Join(root, "x[1].txt"), text)
	write(t, filepath.Join(root, "x1.txt"), "other\n")
	run(root, "add", ".")
	run(root, "commit", "-qm", "files")
	write(t, filepath.Join(root, "x[1].txt"), strings.Replace(text, "first", "changed", 1))
	write(t, filepath.Join(root, "x1.txt"), "unrelated\n")
	patch, err := Diff(root, "x[1].txt", 4096)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(patch, "\n@@ ") != 1 || !strings.Contains(patch, "-first\n+changed\n middle\n") || !strings.HasSuffix(patch, " last\n \n") || strings.Contains(patch, "unrelated") {
		t.Fatalf("patch = %q", patch)
	}
	if _, err := Diff(root, "x[1].txt", 20); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("bounded diff = %v", err)
	}
	write(t, filepath.Join(root, "new file"), "untracked\n")
	if err := os.Symlink("missing target", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, line string }{{"new file", "+untracked\n"}, {"link", "+missing target\n"}} {
		patch, err := Diff(root, tc.path, 4096)
		if err != nil || !strings.Contains(patch, "new file mode") || !strings.Contains(patch, tc.line) {
			t.Errorf("%s = %q, %v", tc.path, patch, err)
		}
	}
	run(root, "add", ".")
	run(root, "commit", "-qm", "changes")
	patch, err = Diff(root, "x[1].txt", 4096)
	if err != nil || patch != "" {
		t.Fatalf("committed = %q, %v", patch, err)
	}
}

func TestChangesAndDiffLeaveIndexUnchanged(t *testing.T) {
	for _, operation := range []string{"changes", "diff"} {
		t.Run(operation, func(t *testing.T) {
			root, run := gitRepo(t)
			write(t, filepath.Join(root, "same"), "unchanged\n")
			write(t, filepath.Join(root, "changed"), "before\n")
			run(root, "add", ".")
			run(root, "commit", "-qm", "files")
			index := filepath.Join(root, ".git", "index")
			before, err := os.ReadFile(index)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(filepath.Join(root, "same"))
			if err != nil {
				t.Fatal(err)
			}
			// A clean file with changed stat data makes Git refresh its index normally.
			stamp := info.ModTime().Add(time.Second)
			if err := os.Chtimes(filepath.Join(root, "same"), stamp, stamp); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(root, "changed"), "after\n")
			t.Setenv("GIT_OPTIONAL_LOCKS", "1")
			if operation == "changes" {
				var changes ChangesResult
				changes, err = Changes(root)
				if err == nil && (len(changes.Files) != 1 || changes.Files[0].Path != "changed") {
					t.Fatalf("content changes = %+v; want only changed", changes.Files)
				}
			} else {
				_, err = Diff(root, "same", 4096)
			}
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(index)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("read-only polling rewrote the index")
			}
		})
	}
}

func TestDiffMetadataOnly(t *testing.T) {
	root, run := gitRepo(t)
	run(root, "config", "core.filemode", "true")
	write(t, filepath.Join(root, "mode"), "contents\n")
	run(root, "add", ".")
	run(root, "commit", "-qm", "file")
	if err := os.Chmod(filepath.Join(root, "mode"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "empty"), "")
	for _, tc := range []struct{ path, metadata string }{
		{"mode", "old mode 100644\nnew mode 100755\n"},
		{"empty", "new file mode 100644\n"},
	} {
		patch, err := Diff(root, tc.path, 4096)
		if err != nil || !strings.Contains(patch, tc.metadata) || strings.Contains(patch, "\n@@ ") {
			t.Errorf("%s patch = %q, %v", tc.path, patch, err)
		}
	}
}
