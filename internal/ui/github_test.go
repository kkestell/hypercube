package ui

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/github"
	"github.com/kkestell/hypercube/internal/github/githubtest"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

func TestSnapshotShowsGitHubStatusAndPullRequests(t *testing.T) {
	t.Parallel()
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		for _, w := range []store.Workspace{{ID: "gh", Path: "/src/w"}, {ID: "plain", Path: "/src/plain"}} {
			if err := tx.AddWorkspace(w); err != nil {
				return err
			}
		}
		for _, s := range []store.Session{
			{ID: "checkout", Workspace: "gh", Dir: "/src/w", Branch: "old"}, // its saved branch is stale
			{ID: "worktree", Workspace: "gh", Dir: "/src/w/.hypercube/wt", Branch: "wt-branch"},
			{ID: "named", Workspace: "gh", Dir: "/src/w/.hypercube/nm", Branch: "feature/login"},
			{ID: "nobranch", Workspace: "gh", Dir: "/src/w/.hypercube/nb"},
			{ID: "fork", Workspace: "gh", Dir: "/src/w/.hypercube/fk", Branch: "fork-branch"},
			{ID: "ended", Workspace: "gh", Dir: "/src/w", Ended: true},
			{ID: "elsewhere", Workspace: "gh", Dir: "/elsewhere"},
			{ID: "child", Workspace: "gh", Parent: "checkout", Name: "c", Dir: "/src/w", Activity: store.Idle},
			{ID: "plain-session", Workspace: "plain", Dir: "/src/plain", Branch: "feature"},
		} {
			if err := tx.AddSession(s); err != nil {
				return err
			}
		}
		return nil
	})
	statuses := map[string]github.Status{"gh": {Repo: "o/r", Branch: "feature", PullRequests: []github.PullRequest{
		{Number: 4, Branch: "old"},
		{Number: 5, Branch: "feature"},
		{Number: 8, Branch: "wt-branch"},
		{Number: 10, Branch: "feature/login"},
		{Number: 9, Branch: "fork-branch", Fork: true},
	}}}
	list, _, err := snapshot(db, nil, nil, statuses)
	if err != nil {
		t.Fatal(err)
	}
	pulls := map[string]int{}
	var walk func(sessions []shownSession)
	walk = func(sessions []shownSession) {
		for _, s := range sessions {
			if s.PullRequest != nil {
				pulls[s.ID] = s.PullRequest.Number
			}
			walk(s.Children)
		}
	}
	for _, w := range list {
		walk(w.Sessions)
		if has := w.GitHub != nil; has != (w.ID == "gh") {
			t.Errorf("workspace %s has GitHub status: %v", w.ID, has)
		}
	}
	want := map[string]int{"checkout": 5, "worktree": 8, "named": 10, "child": 5}
	if len(pulls) != len(want) {
		t.Fatalf("pull requests by session = %v, want %v", pulls, want)
	}
	for id, number := range want {
		if pulls[id] != number {
			t.Fatalf("pull requests by session = %v, want %v", pulls, want)
		}
	}
}

func TestStateCarriesGitHubStatusAndAccountChoice(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GH_TOKEN", "")
	db := open(t)
	dir, bare := t.TempDir(), t.TempDir()
	for _, args := range [][]string{
		{"-C", bare, "init", "-q", "--bare", "-b", "main"},
		{"-C", dir, "init", "-q", "-b", "main"},
		{"-C", dir, "remote", "add", "origin", "https://github.com/o/r.git"},
		{"-C", dir, "config", "url." + bare + ".insteadOf", "https://github.com/o/r.git"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	change(t, db, func(tx *store.Tx) error { return tx.AddWorkspace(store.Workspace{ID: "w", Path: dir}) })
	api := githubtest.NewAPI(t)
	api.SetPermission("token-alice", "WRITE")
	api.SetPermission("token-bob", "ADMIN")
	watcher := &github.Watcher{DB: db, GH: githubtest.GH(t, "alice", "bob"), API: api.URL, Interval: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { watcher.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	a := &app{t: t, db: db}
	a.server = httptest.NewServer(Handler(db, configFile(db), session.New(db, configFile(db), "", session.Programs{}), watcher))
	t.Cleanup(a.server.Close)
	p := a.page()
	p.until("the GitHub status", func() bool {
		return len(p.state.Workspaces) == 1 && p.state.Workspaces[0].GitHub != nil && len(p.state.Workspaces[0].GitHub.Choices) == 2
	})
	if got := p.state.Workspaces[0].GitHub; got.Repo != "o/r" || got.Account != "" {
		t.Fatalf("github = %+v", got)
	}

	p.until("the accounts", func() bool {
		return len(p.state.Accounts) == 2 && p.state.Accounts[0].Login == "alice" && p.state.Accounts[1].Login == "bob"
	})

	a.post("/workspace?action=github-account&id=w&account=carol", "", 400)
	a.post("/workspace?action=github-account&id=nope&account=alice", "", 400)
	a.post("/workspace?action=github-account&id=w&account=alice", "", 200)
	p.until("the chosen account", func() bool {
		return p.state.Workspaces[0].GitHub != nil && p.state.Workspaces[0].GitHub.Account == "alice"
	})
}

func TestCloneAction(t *testing.T) {
	root := t.TempDir()
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte("[url \""+root+"/\"]\n\tinsteadOf = https://github.com/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GH_TOKEN", "")
	bare := filepath.Join(root, "o", "r.git")
	seed := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "--bare", "-b", "main", bare},
		{"-C", seed, "init", "-q", "-b", "main"},
		{"-C", seed, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
		{"-C", seed, "push", "-q", bare, "main"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	db := open(t)
	change(t, db, func(tx *store.Tx) error { return tx.AddWorkspace(store.Workspace{ID: "w", Path: seed}) })
	api := githubtest.NewAPI(t)
	api.SetPullRequests([]map[string]any{
		{"number": 1, "title": "One", "url": "https://github.com/o/r/pull/1", "repository": map[string]string{"nameWithOwner": "o/r"}, "author": map[string]string{"login": "dana"}},
		{"number": 2, "title": "Two", "url": "https://github.com/o/missing/pull/2", "repository": map[string]string{"nameWithOwner": "o/missing"}, "author": map[string]string{"login": "dana"}},
	}, nil)
	watcher := &github.Watcher{DB: db, GH: githubtest.GH(t, "alice"), API: api.URL, Interval: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { watcher.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	a := &app{t: t, db: db}
	a.server = httptest.NewServer(Handler(db, configFile(db), session.New(db, configFile(db), "", session.Programs{}), watcher))
	t.Cleanup(a.server.Close)
	p := a.page()
	p.until("the accounts", func() bool {
		return len(p.state.Accounts) == 1 && len(p.state.Accounts[0].PullRequests) == 2
	})

	base, err := filepath.EvalSymlinks(t.TempDir()) // the workspace records the resolved path
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "new", "r")
	clone := "/workspace?action=clone&account=alice&repo=o/r&path=" + url.QueryEscape(target)
	a.post(clone, "", 200)
	p.until("the new workspace", func() bool {
		for _, w := range p.state.Workspaces {
			if w.Path == target {
				return true
			}
		}
		return false
	})
	for key, want := range map[string]string{"credential.githubAccount": "alice", "remote.origin.url": "https://github.com/o/r.git"} {
		out, err := exec.Command("git", "-C", target, "config", "--local", key).Output()
		if got := strings.TrimSpace(string(out)); err != nil || got != want {
			t.Fatalf("%s = %q, %v; want %q", key, got, err, want)
		}
	}

	a.post(clone, "", 409) // the folder now holds a repository
	other := url.QueryEscape(filepath.Join(t.TempDir(), "x"))
	a.post("/workspace?action=clone&account=alice&repo=o/s&path="+other, "", 400)
	a.post("/workspace?action=clone&account=carol&repo=o/r&path="+other, "", 400)
	a.post("/workspace?action=clone&account=alice&repo=o/r&path=r", "", 400)
	missing := filepath.Join(t.TempDir(), "missing")
	a.post("/workspace?action=clone&account=alice&repo=o/missing&path="+url.QueryEscape(missing), "", 409)
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a failed clone left %s: %v", missing, err)
	}
}
