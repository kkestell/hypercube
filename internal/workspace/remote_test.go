package workspace

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestFetchAndTracking(t *testing.T) {
	isolateGit(t)
	bare := t.TempDir()
	root, run := gitRepo(t)
	run(bare, "init", "-q", "--bare", "-b", "main")
	run(root, "remote", "add", "origin", bare)
	run(root, "push", "-q", "-u", "origin", "main")
	if up, ahead, behind, err := Tracking(root); err != nil || up != "origin/main" || ahead != 0 || behind != 0 {
		t.Fatalf("Tracking = %q, %d, %d, %v", up, ahead, behind, err)
	}

	// Another clone pushes two commits; this one commits one.
	other := t.TempDir()
	run(other, "clone", "-q", bare, "clone")
	clone := other + "/clone"
	run(clone, "commit", "-q", "--allow-empty", "-m", "one")
	run(clone, "commit", "-q", "--allow-empty", "-m", "two")
	run(clone, "push", "-q", "origin", "main")
	run(root, "commit", "-q", "--allow-empty", "-m", "local")
	if _, ahead, behind, _ := Tracking(root); ahead != 1 || behind != 0 {
		t.Fatalf("before the fetch: ahead %d, behind %d", ahead, behind)
	}
	if err := Fetch(context.Background(), root, "origin", nil); err != nil {
		t.Fatal(err)
	}
	if _, ahead, behind, err := Tracking(root); err != nil || ahead != 1 || behind != 2 {
		t.Fatalf("after the fetch: ahead %d, behind %d, %v", ahead, behind, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "FETCH_HEAD")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("FETCH_HEAD: %v", err)
	}

	run(root, "checkout", "-q", "-b", "local-only")
	if up, ahead, behind, err := Tracking(root); err != nil || up != "" || ahead != 0 || behind != 0 {
		t.Fatalf("Tracking without upstream = %q, %d, %d, %v", up, ahead, behind, err)
	}
	if err := Fetch(context.Background(), root, "missing", nil); err == nil {
		t.Fatal("fetching a missing remote succeeded")
	}
}

func TestFetchPullRequest(t *testing.T) {
	isolateGit(t)
	bare := t.TempDir()
	root, run := gitRepo(t)
	run(bare, "init", "-q", "--bare", "-b", "main")
	run(root, "remote", "add", "origin", bare)
	// The pull request's commit exists only in the remote.
	fork, _ := gitRepo(t)
	run(fork, "commit", "-q", "--allow-empty", "-m", "pull request")
	want, err := Head(fork)
	if err != nil {
		t.Fatal(err)
	}
	run(fork, "push", "-q", bare, "HEAD:refs/pull/7/head")

	got, err := FetchPullRequest(context.Background(), root, "origin", 7, nil)
	if err != nil || got != want {
		t.Fatalf("FetchPullRequest = %q, %v; want %q", got, err, want)
	}
	if _, err := FetchPullRequest(context.Background(), root, "origin", 8, nil); err == nil {
		t.Fatal("fetching a missing pull request succeeded")
	}
	if refs, err := git(root, "for-each-ref", "refs/hypercube/"); err != nil || refs != "" {
		t.Fatalf("refs left behind: %q, %v", refs, err)
	}
}

func TestCloneRecordsConfigBeforeFetching(t *testing.T) {
	isolateGit(t)
	bare := t.TempDir()
	root, run := gitRepo(t)
	run(bare, "init", "-q", "--bare", "-b", "main")
	run(root, "push", "-q", bare, "main")

	dir := filepath.Join(t.TempDir(), "new", "clone")
	if err := Clone(context.Background(), bare, dir, []string{"credential.githubAccount=alice"}, "secret"); err != nil {
		t.Fatal(err)
	}
	if got, err := Config(dir, "credential.githubAccount"); err != nil || got != "alice" {
		t.Fatalf("credential.githubAccount = %q, %v", got, err)
	}
	if got, err := Config(dir, "remote.origin.url"); err != nil || got != bare {
		t.Fatalf("remote.origin.url = %q, %v", got, err)
	}
	config, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil || strings.Contains(string(config), "secret") {
		t.Fatalf("config holds the token: %v\n%s", err, config)
	}
	if err := Clone(context.Background(), bare, dir, nil, ""); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("cloning into a non-empty folder = %v", err)
	}
}

func TestCloneAuthenticatesWithTokenOnly(t *testing.T) {
	// A configured helper must neither answer nor store the token.
	store := filepath.Join(t.TempDir(), "credentials")
	global := filepath.Join(t.TempDir(), "gitconfig")
	write(t, global, "[credential]\n\thelper = store --file "+store+"\n")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	served := t.TempDir()
	root, run := gitRepo(t)
	bare := filepath.Join(served, "repo.git")
	run(served, "init", "-q", "--bare", "-b", "main", bare)
	run(root, "push", "-q", bare, "main")
	run(bare, "update-server-info")
	files := http.FileServer(http.Dir(served))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, password, ok := r.BasicAuth(); !ok || password != "secret" {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	dir := filepath.Join(t.TempDir(), "clone")
	if err := Clone(context.Background(), server.URL+"/repo.git", dir, nil, "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the credential store was written: %v", err)
	}
	config, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil || strings.Contains(string(config), "secret") || strings.Contains(string(config), "helper") {
		t.Fatalf("config holds the token or a helper: %v\n%s", err, config)
	}
}
