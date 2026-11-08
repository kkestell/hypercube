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
	write(t, filepath.Join(root, "folder/nested/untracked"), "new")
	changes, err := Changes(filepath.Join(root, "folder"), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Files) != 2 || changes.Files[0].Path != "nested/untracked" || changes.Files[1].Path != "untracked" {
		t.Fatalf("folder changes = %+v", changes)
	}
	changes, err = Changes(root, root)
	if err != nil {
		t.Fatal(err)
	}
	if changes.Branch != "main" || len(changes.Files) != 6 {
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
	changes, err = Changes(root, root)
	if err != nil || len(changes.Files) != 0 {
		t.Fatalf("clean = %+v, %v", changes, err)
	}
	run(root, "checkout", "--detach", "-q")
	changes, err = Changes(root, root)
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
	changes, err := Changes(root, root)
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
				changes, err = Changes(root, root)
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

func TestBranchChanges(t *testing.T) {
	checkout, run := gitRepo(t)
	for path, text := range map[string]string{
		"modified": "base\n", "reverted": "base\n", "deleted": "old\n", "recreated": "old\n",
		"identical": "old\n", "binary": "a\x00b", "sub/a:[1]": "old\n", "mode": "same\n", "renamed": "rename\n",
	} {
		write(t, filepath.Join(checkout, path), text)
	}
	run(checkout, "add", ".")
	run(checkout, "commit", "-qm", "common")
	feature := filepath.Join(t.TempDir(), "feature")
	run(checkout, "worktree", "add", "-qb", "feature", feature)
	write(t, filepath.Join(checkout, "checkout-only"), "not feature work\n")
	run(checkout, "add", ".")
	run(checkout, "commit", "-qm", "checkout advances independently")
	write(t, filepath.Join(feature, "modified"), "committed\n")
	write(t, filepath.Join(feature, "reverted"), "committed\n")
	write(t, filepath.Join(feature, "added"), "committed\n")
	write(t, filepath.Join(feature, "vanished"), "committed\n")
	run(feature, "rm", "deleted", "recreated", "identical")
	run(feature, "mv", "renamed", "new-name")
	run(feature, "add", ".")
	run(feature, "commit", "-qm", "feature changes")
	write(t, filepath.Join(feature, "modified"), "staged\n")
	run(feature, "add", "modified")
	write(t, filepath.Join(feature, "modified"), "working\nmore\n")
	write(t, filepath.Join(feature, "added"), "edited addition\n")
	write(t, filepath.Join(feature, "reverted"), "base\n")
	write(t, filepath.Join(feature, "recreated"), "new\n")
	write(t, filepath.Join(feature, "identical"), "old\n")
	write(t, filepath.Join(feature, "binary"), "c\x00d")
	write(t, filepath.Join(feature, "sub/a:[1]"), "literal\n")
	write(t, filepath.Join(feature, "empty"), "")
	if err := os.Remove(filepath.Join(feature, "vanished")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(feature, "link")); err != nil {
		t.Fatal(err)
	}
	run(feature, "config", "core.filemode", "true")
	if err := os.Chmod(filepath.Join(feature, "mode"), 0o755); err != nil {
		t.Fatal(err)
	}
	indexName, err := git(feature, "rev-parse", "--git-path", "index")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(indexName)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := BranchChanges(feature, feature, checkout)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"modified": "modified", "added": "added", "deleted": "deleted", "recreated": "modified", "binary": "modified", "sub/a:[1]": "modified", "empty": "untracked", "link": "untracked", "mode": "modified", "renamed": "deleted", "new-name": "added"}
	if changes.Branch != "feature" || changes.Target != "main" || len(changes.Files) != len(want) {
		t.Fatalf("changes = %+v", changes)
	}
	seen := map[string]bool{}
	for _, file := range changes.Files {
		if seen[file.Path] || want[file.Path] != file.Status {
			t.Errorf("unexpected/duplicate file: %+v", file)
		}
		seen[file.Path] = true
		if file.Path == "recreated" && (file.Added == nil || *file.Added != 1 || file.Removed == nil || *file.Removed != 1) {
			t.Errorf("recreated counts = %+v", file)
		}
		if file.Path == "binary" && file.Added != nil {
			t.Errorf("binary counts = %+v", file)
		}
	}
	for path, fragment := range map[string]string{"modified": "-base\n+working\n+more\n", "added": "+edited addition\n", "recreated": "-old\n+new\n", "identical": "", "empty": "new file mode", "link": "+missing", "sub/a:[1]": "-old\n+literal\n", "mode": "old mode"} {
		patch, err := BranchDiff(feature, checkout, path, 4096)
		if err != nil || (fragment == "" && patch != "") || (fragment != "" && !strings.Contains(patch, fragment)) {
			t.Errorf("%s patch = %q, %v", path, patch, err)
		}
	}
	after, err := os.ReadFile(indexName)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("branch reads rewrote real index")
	}
	sub, err := BranchChanges(filepath.Join(feature, "sub"), feature, checkout)
	if err != nil || len(sub.Files) != 1 || sub.Files[0].Path != "a:[1]" {
		t.Fatalf("sub = %+v, %v", sub, err)
	}
	// Moving the checkout to the feature commit changes the live base immediately.
	run(checkout, "checkout", "--detach", "-q", "feature")
	changes, err = BranchChanges(feature, feature, checkout)
	if err != nil || changes.Target != "detached HEAD" {
		t.Fatalf("moved target = %+v, %v", changes, err)
	}
	for _, file := range changes.Files {
		if file.Path == "new-name" || file.Path == "deleted" {
			t.Errorf("committed path remained after base moved: %+v", file)
		}
	}
}
