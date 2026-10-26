package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsRepo(t *testing.T) {
	dir := t.TempDir()
	if IsRepo(dir) {
		t.Errorf("IsRepo(%s) = true before git init", dir)
	}
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	for _, d := range []string{dir, sub} {
		if !IsRepo(d) {
			t.Errorf("IsRepo(%s) = false after git init", d)
		}
	}
}

func TestResolve(t *testing.T) {
	base, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	for _, d := range []string{filepath.Join(base, "rel"), filepath.Join(home, "src")} {
		os.Mkdir(d, 0o755)
	}
	os.WriteFile(filepath.Join(base, "file"), nil, 0o644)

	for dir, want := range map[string]string{
		"rel":                         filepath.Join(base, "rel"),
		"~/src":                       filepath.Join(home, "src"),
		filepath.Join(home, "src"):    filepath.Join(home, "src"),
		filepath.Join(base, "rel/.."): base,
	} {
		if got, err := Resolve(base, dir); err != nil || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", dir, got, err, want)
		}
	}
	for _, dir := range []string{"missing", "file"} {
		if got, err := Resolve(base, dir); err == nil {
			t.Errorf("Resolve(%q) = %q, want an error", dir, got)
		}
	}
}
