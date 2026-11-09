package github

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/github/githubtest"
	"github.com/kkestell/hypercube/internal/store"
)

// stand is a watcher over an isolated store, a stand-in gh, and a stand-in API.
type stand struct {
	t *testing.T
	w *Watcher
	// api answers permission queries by bearer token.
	api *githubtest.API
	// bares holds each workspace's remote repository by workspace ID.
	bares map[string]string
	dirs  map[string]string
}

func newStand(t *testing.T, logins ...string) *stand {
	t.Helper()
	// A developer's own settings and tokens must not decide any case.
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GH_TOKEN", "outer")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	api := githubtest.NewAPI(t)
	return &stand{t: t, api: api, bares: map[string]string{}, dirs: map[string]string{}, w: &Watcher{DB: db, GH: githubtest.GH(t, logins...), API: api.URL, Interval: time.Hour}}
}

// git runs git in dir.
func (s *stand) git(dir string, args ...string) {
	s.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		s.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// workspace adds a workspace whose repository has the remote, and returns its
// ID. A local bare repository answers for the remote's URL, so fetches stay
// off the network.
func (s *stand) workspace(id, remote string, setup ...[]string) string {
	s.t.Helper()
	dir := s.t.TempDir()
	git := func(args ...string) {
		s.t.Helper()
		s.git(dir, args...)
	}
	git("init", "-q", "-b", "main")
	if remote != "" {
		bare := s.t.TempDir()
		s.git(bare, "init", "-q", "--bare", "-b", "main")
		s.bares[id], s.dirs[id] = bare, dir
		git("remote", "add", "origin", remote)
		git("config", "url."+bare+".insteadOf", remote)
	}
	for _, args := range setup {
		git(args...)
	}
	err := s.w.DB.Tx(func(tx *store.Tx) error { return tx.AddWorkspace(store.Workspace{ID: id, Path: dir}) })
	if err != nil {
		s.t.Fatal(err)
	}
	return id
}

// run starts the watcher and stops it when the test ends.
func (s *stand) run() {
	s.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.w.Run(ctx); close(done) }()
	s.t.Cleanup(func() { cancel(); <-done })
}

// until waits on Changed until the statuses satisfy check.
func (s *stand) until(what string, check func(map[string]Status) bool) {
	s.t.Helper()
	timeout := time.After(20 * time.Second)
	for {
		changed := s.w.Changed()
		if check(s.w.Statuses()) {
			return
		}
		select {
		case <-changed:
		case <-timeout:
			s.t.Fatalf("timed out waiting for %s; statuses %+v", what, s.w.Statuses())
		}
	}
}

func (s *stand) resolved(id string) func(map[string]Status) bool {
	return func(m map[string]Status) bool { return m[id].Account != "" || len(m[id].Choices) > 0 }
}

const remote = "https://github.com/o/r.git"

func TestConfiguredAccountSkipsPermissionQuery(t *testing.T) {
	s := newStand(t, "alice", "bob")
	id := s.workspace("w", remote, []string{"config", "credential.githubAccount", "bob"})
	s.run()
	s.until("bob chosen", s.resolved(id))
	got := s.w.Statuses()[id]
	got.Updated = 0 // the time of the pull request query
	if want := (Status{Repo: "o/r", URL: "https://github.com/o/r", Account: "bob", Branch: "main", PullRequests: []PullRequest{}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("status = %+v, want %+v", got, want)
	}
	if got := s.w.Env(id); !reflect.DeepEqual(got, []string{"GH_TOKEN=token-bob"}) {
		t.Fatalf("Env = %v", got)
	}
	if n := s.api.Requests(); n != 0 {
		t.Fatalf("%d permission queries", n)
	}
}

func TestOneAccountThatCanPush(t *testing.T) {
	s := newStand(t, "alice", "bob", "carol")
	s.api.SetPermission("token-bob", "MAINTAIN")
	s.api.SetPermission("token-carol", "READ")
	id := s.workspace("w", remote)
	s.run()
	s.until("bob chosen", s.resolved(id))
	if got := s.w.Statuses()[id]; got.Account != "bob" || got.Choices != nil {
		t.Fatalf("status = %+v", got)
	}
	if got := s.w.Env(id); !reflect.DeepEqual(got, []string{"GH_TOKEN=token-bob"}) {
		t.Fatalf("Env = %v", got)
	}
}

func TestSeveralAccountsThatCanPush(t *testing.T) {
	s := newStand(t, "alice", "bob", "carol")
	s.api.SetPermission("token-alice", "WRITE")
	s.api.SetPermission("token-carol", "ADMIN")
	id := s.workspace("w", remote)
	s.run()
	s.until("a choice", s.resolved(id))
	got := s.w.Statuses()[id]
	if got.Account != "" || !reflect.DeepEqual(got.Choices, []string{"alice", "carol"}) {
		t.Fatalf("status = %+v", got)
	}
	if env := s.w.Env(id); env != nil {
		t.Fatalf("Env = %v", env)
	}
}

func TestRevokedAccountDoesNotBlockDetection(t *testing.T) {
	s := newStand(t, "alice", "bob")
	s.api.Revoke("token-alice")
	s.api.SetPermission("token-bob", "WRITE")
	id := s.workspace("w", remote)
	s.run()
	s.until("bob chosen", s.resolved(id))
	if got := s.w.Statuses()[id]; got.Account != "bob" || got.Choices != nil {
		t.Fatalf("status = %+v", got)
	}
}

func TestNoAccountCanPush(t *testing.T) {
	t.Run("one login", func(t *testing.T) {
		s := newStand(t, "alice")
		id := s.workspace("w", remote)
		s.run()
		s.until("alice chosen", s.resolved(id))
		if got := s.w.Statuses()[id]; got.Account != "alice" {
			t.Fatalf("status = %+v", got)
		}
	})
	t.Run("several logins", func(t *testing.T) {
		s := newStand(t, "alice", "bob")
		id := s.workspace("w", remote)
		s.run()
		s.until("a choice", s.resolved(id))
		if got := s.w.Statuses()[id]; !reflect.DeepEqual(got.Choices, []string{"alice", "bob"}) {
			t.Fatalf("status = %+v", got)
		}
	})
}

func TestWorkspacesWithoutGitHub(t *testing.T) {
	s := newStand(t, "alice")
	good := s.workspace("good", remote)
	s.workspace("gitlab", "https://gitlab.com/o/r.git")
	s.workspace("bare", "")
	err := s.w.DB.Tx(func(tx *store.Tx) error {
		return tx.AddWorkspace(store.Workspace{ID: "plain", Path: filepath.Join(t.TempDir(), "plain"), NoGit: true})
	})
	if err != nil {
		t.Fatal(err)
	}
	s.run()
	s.until("the GitHub workspace", s.resolved(good))
	if got := s.w.Statuses(); len(got) != 1 {
		t.Fatalf("statuses = %+v", got)
	}
}

func TestGHMissing(t *testing.T) {
	s := newStand(t, "alice")
	// gh is found on PATH, resolves the workspace, then disappears.
	dir := filepath.Dir(s.w.GH)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	s.w.GH = "hypercube-test-gh"
	if err := os.Rename(filepath.Join(dir, "gh"), filepath.Join(dir, s.w.GH)); err != nil {
		t.Fatal(err)
	}
	id := s.workspace("w", remote)
	s.run()
	s.until("alice chosen", s.resolved(id))
	if err := os.Remove(filepath.Join(dir, s.w.GH)); err != nil {
		t.Fatal(err)
	}
	s.w.Refresh()
	s.until("the status cleared", func(m map[string]Status) bool { return len(m) == 0 })
	if got := s.w.Env(id); got != nil {
		t.Fatalf("Env = %v", got)
	}
}

func TestFailedPermissionQueryIsRetried(t *testing.T) {
	s := newStand(t, "alice", "bob")
	s.api.SetPermission("token-bob", "WRITE")
	s.api.Fail(true)
	id := s.workspace("w", remote)
	// A workspace with a configured account resolves in the same pass, so its
	// status shows the first pass is over.
	pass := s.workspace("pass", "https://github.com/o/other.git", []string{"config", "credential.githubAccount", "alice"})
	s.run()
	s.until("the first pass", s.resolved(pass))
	if got := s.w.Statuses()[id]; got.Repo != "" {
		t.Fatalf("status after failure = %+v", got)
	}
	s.api.Fail(false)
	s.w.Refresh()
	s.until("bob chosen after recovery", s.resolved(id))
	if got := s.w.Statuses()[id]; got.Account != "bob" {
		t.Fatalf("status = %+v", got)
	}
}

func TestDetectionRepeatsOnlyWhenInputsChange(t *testing.T) {
	s := newStand(t, "alice", "bob")
	s.api.SetPermission("token-bob", "WRITE")
	first := s.workspace("first", remote)
	s.run()
	s.until("the first workspace", s.resolved(first))
	// A pass visits the first workspace before the second, so the second's
	// status shows a pass has covered the first again.
	before := s.api.Requests()
	second := s.workspace("second", "https://github.com/o/other.git")
	s.w.Refresh()
	s.until("the second workspace", s.resolved(second))
	if got := s.api.Requests() - before; got != 2 {
		t.Fatalf("the second workspace took %d permission queries, want 2", got)
	}
}

// queried waits until the workspace's pull requests have been read.
func (s *stand) queried(id string) func(map[string]Status) bool {
	return func(m map[string]Status) bool { return m[id].Updated != 0 }
}

func TestAheadBehindAfterFetch(t *testing.T) {
	s := newStand(t, "alice")
	id := s.workspace("w", remote)
	dir, bare := s.dirs[id], s.bares[id]
	s.git(dir, "commit", "-q", "--allow-empty", "-m", "init")
	s.git(dir, "push", "-q", "-u", "origin", "main")
	// Another clone pushes a commit that the workspace has not fetched.
	other := t.TempDir()
	s.git(other, "clone", "-q", bare, "clone")
	s.git(other+"/clone", "commit", "-q", "--allow-empty", "-m", "elsewhere")
	s.git(other+"/clone", "push", "-q", "origin", "main")
	s.run()
	s.until("the pull requests", s.queried(id))
	got := s.w.Statuses()[id]
	if got.Branch != "main" || got.Upstream != "origin/main" || got.Ahead != 0 || got.Behind != 1 {
		t.Fatalf("status = %+v", got)
	}
}

func TestPullRequestsMapAndKeepOnFailure(t *testing.T) {
	s := newStand(t, "alice")
	s.api.SetPullRequests(3,
		map[string]any{"number": 7, "title": "Add it", "url": "https://github.com/o/r/pull/7", "isDraft": false,
			"isCrossRepository": false, "headRefName": "feature", "author": map[string]string{"login": "dana"},
			"reviewDecision": "APPROVED", "statusCheckRollup": map[string]string{"state": "SUCCESS"}},
		map[string]any{"number": 5, "title": "Ghost", "url": "https://github.com/o/r/pull/5", "isDraft": true,
			"isCrossRepository": true, "headRefName": "fork-branch", "author": nil,
			"reviewDecision": nil, "statusCheckRollup": nil})
	id := s.workspace("w", remote)
	s.run()
	s.until("the pull requests", s.queried(id))
	got := s.w.Statuses()[id]
	want := []PullRequest{
		{Number: 7, Title: "Add it", URL: "https://github.com/o/r/pull/7", Author: "dana", Branch: "feature", Review: "APPROVED", Checks: "SUCCESS"},
		{Number: 5, Title: "Ghost", URL: "https://github.com/o/r/pull/5", Branch: "fork-branch", Fork: true, Draft: true},
	}
	if !reflect.DeepEqual(got.PullRequests, want) || got.Open != 3 || got.Error != "" {
		t.Fatalf("status = %+v", got)
	}

	s.api.Fail(true)
	s.w.Refresh()
	s.until("the failure", func(m map[string]Status) bool { return m[id].Error != "" })
	failed := s.w.Statuses()[id]
	if !reflect.DeepEqual(failed.PullRequests, want) || failed.Open != 3 || failed.Updated != got.Updated {
		t.Fatalf("status after failure = %+v", failed)
	}
}

func TestPendingChoiceDoesNotFetchOrQuery(t *testing.T) {
	s := newStand(t, "alice", "bob")
	id := s.workspace("w", remote)
	dir, bare := s.dirs[id], s.bares[id]
	s.git(dir, "commit", "-q", "--allow-empty", "-m", "init")
	s.git(dir, "push", "-q", "-u", "origin", "main")
	other := t.TempDir()
	s.git(other, "clone", "-q", bare, "clone")
	s.git(other+"/clone", "commit", "-q", "--allow-empty", "-m", "elsewhere")
	s.git(other+"/clone", "push", "-q", "origin", "main")
	s.run()
	s.until("a choice", s.resolved(id))
	got := s.w.Statuses()[id]
	if got.PullRequests == nil || len(got.PullRequests) != 0 || got.Updated != 0 || got.Error != "" || got.Behind != 0 {
		t.Fatalf("status = %+v", got)
	}
}

func TestTokenFailureIsReported(t *testing.T) {
	s := newStand(t, "broken")
	id := s.workspace("w", remote)
	s.run()
	s.until("the account", s.resolved(id))
	got := s.w.Statuses()[id]
	if got.Account != "broken" || got.Updated != 0 || !strings.Contains(got.Error, "no token for broken") {
		t.Fatalf("status = %+v", got)
	}
}

func TestSavedChoiceDecidesBetweenPushers(t *testing.T) {
	s := newStand(t, "alice", "bob")
	s.api.SetPermission("token-alice", "WRITE")
	s.api.SetPermission("token-bob", "WRITE")
	id := s.workspace("w", remote)
	s.run()
	s.until("a choice", s.resolved(id))
	if got := s.w.Statuses()[id]; got.Account != "" || s.w.Env(id) != nil {
		t.Fatalf("status = %+v", got)
	}

	choose := func(account string) {
		t.Helper()
		err := s.w.DB.Tx(func(tx *store.Tx) error { return tx.SetGitHubAccount(id, account) })
		if err != nil {
			t.Fatal(err)
		}
		s.w.Refresh()
	}
	choose("bob")
	s.until("bob saved", func(m map[string]Status) bool { return m[id].Account == "bob" })
	if got := s.w.Statuses()[id]; !reflect.DeepEqual(got.Choices, []string{"alice", "bob"}) {
		t.Fatalf("choices = %v", got.Choices)
	}
	if got := s.w.Env(id); !reflect.DeepEqual(got, []string{"GH_TOKEN=token-bob"}) {
		t.Fatalf("Env = %v", got)
	}

	// A saved account that is not a candidate is ignored.
	choose("carol")
	s.until("the choice dropped", func(m map[string]Status) bool { return m[id].Account == "" })
}
