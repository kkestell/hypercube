package workspace

import "testing"

func isolateGit(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func TestConfig(t *testing.T) {
	isolateGit(t)
	root, run := gitRepo(t)
	if got, err := Config(root, "credential.githubAccount"); err != nil || got != "" {
		t.Fatalf("unset Config = %q, %v", got, err)
	}
	run(root, "config", "credential.githubAccount", "me")
	if got, err := Config(root, "credential.githubAccount"); err != nil || got != "me" {
		t.Fatalf("set Config = %q, %v", got, err)
	}
}

func TestRemote(t *testing.T) {
	isolateGit(t)
	for _, tc := range []struct {
		name  string
		setup func(root string, run func(dir string, args ...string))
		want  [2]string
	}{
		{"no remotes", func(string, func(string, ...string)) {}, [2]string{}},
		{"origin without upstream", func(root string, run func(string, ...string)) {
			run(root, "remote", "add", "origin", "https://github.com/o/r.git")
			run(root, "remote", "add", "other", "https://github.com/o/other.git")
		}, [2]string{"origin", "https://github.com/o/r.git"}},
		{"tracking another remote", func(root string, run func(string, ...string)) {
			run(root, "remote", "add", "origin", "https://github.com/o/r.git")
			run(root, "remote", "add", "fork", "https://github.com/me/r.git")
			run(root, "config", "branch.main.remote", "fork")
		}, [2]string{"fork", "https://github.com/me/r.git"}},
		{"tracking a remote without a URL", func(root string, run func(string, ...string)) {
			run(root, "remote", "add", "origin", "https://github.com/o/r.git")
			run(root, "config", "branch.main.remote", "gone")
		}, [2]string{"origin", "https://github.com/o/r.git"}},
		{"tracking a local branch", func(root string, run func(string, ...string)) {
			run(root, "remote", "add", "origin", "https://github.com/o/r.git")
			run(root, "config", "branch.main.remote", ".")
		}, [2]string{"origin", "https://github.com/o/r.git"}},
		{"detached HEAD", func(root string, run func(string, ...string)) {
			run(root, "remote", "add", "origin", "https://github.com/o/r.git")
			run(root, "checkout", "-q", "--detach")
		}, [2]string{"origin", "https://github.com/o/r.git"}},
		{"insteadOf is not applied", func(root string, run func(string, ...string)) {
			run(root, "remote", "add", "origin", "https://github.com/o/r.git")
			run(root, "config", "url./tmp/local.insteadOf", "https://github.com/")
		}, [2]string{"origin", "https://github.com/o/r.git"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, run := gitRepo(t)
			tc.setup(root, run)
			name, url, err := Remote(root)
			if err != nil || [2]string{name, url} != tc.want {
				t.Fatalf("Remote = %q, %q, %v; want %q", name, url, err, tc.want)
			}
		})
	}
}
