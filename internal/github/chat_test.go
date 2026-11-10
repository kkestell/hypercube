package github

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCheckRepo(t *testing.T) {
	t.Parallel()
	checkout := func(t *testing.T, args ...[]string) string {
		t.Helper()
		dir := t.TempDir()
		for _, a := range append([][]string{{"init", "-q", "-b", "main"}}, args...) {
			if out, err := exec.Command("git", append([]string{"-C", dir}, a...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", a, err, out)
			}
		}
		return dir
	}
	origin := checkout(t, []string{"remote", "add", "origin", "https://github.com/O/R.git"})
	gitlab := checkout(t, []string{"remote", "add", "origin", "https://gitlab.com/o/r.git"})
	tracking := checkout(t,
		[]string{"remote", "add", "origin", "https://github.com/me/r.git"},
		[]string{"remote", "add", "upstream", "https://github.com/o/r.git"},
		[]string{"config", "branch.main.remote", "upstream"})
	plain := t.TempDir()
	for _, c := range []struct {
		name, dir, repo, want string
	}{
		{"origin", origin, "o/r", ""},
		{"another repository", origin, "o/other", origin + " is a checkout of O/R, not o/other"},
		{"no GitHub remote", gitlab, "o/r", gitlab + " has no GitHub remote"},
		{"not Git", plain, "o/r", plain + " is not a Git checkout"},
		{"branch tracking upstream", tracking, "o/r", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := CheckRepo(c.dir, c.repo)
			if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
				t.Errorf("CheckRepo(%s, %s) = %v, want %q", c.dir, c.repo, err, c.want)
			}
		})
	}
}
