package github

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	return &stand{t: t, api: api, w: &Watcher{DB: db, GH: githubtest.GH(t, logins...), API: api.URL, Interval: time.Hour}}
}

// workspace adds a workspace whose repository has the remote, and returns its ID.
func (s *stand) workspace(id, remote string, setup ...[]string) string {
	s.t.Helper()
	dir := s.t.TempDir()
	git := func(args ...string) {
		s.t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			s.t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	if remote != "" {
		git("remote", "add", "origin", remote)
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
	if got := s.w.Statuses()[id]; !reflect.DeepEqual(got, Status{Repo: "o/r", URL: "https://github.com/o/r", Account: "bob"}) {
		t.Fatalf("status = %+v", got)
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
