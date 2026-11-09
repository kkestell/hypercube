package ui

import (
	"context"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/github"
	"github.com/kkestell/hypercube/internal/github/githubtest"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

func TestSnapshotShowsGitHubStatusAndPullRequests(t *testing.T) {
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
		{Number: 9, Branch: "fork-branch", Fork: true},
	}}}
	list, _, err := snapshot(db, nil, statuses)
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
	want := map[string]int{"checkout": 5, "worktree": 8, "child": 5}
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
	a.server = httptest.NewServer(Handler(db, session.New(db, "", session.Programs{}), watcher))
	t.Cleanup(a.server.Close)
	p := a.page()
	p.until("the GitHub status", func() bool {
		return len(p.state.Workspaces) == 1 && p.state.Workspaces[0].GitHub != nil && len(p.state.Workspaces[0].GitHub.Choices) == 2
	})
	if got := p.state.Workspaces[0].GitHub; got.Repo != "o/r" || got.Account != "" {
		t.Fatalf("github = %+v", got)
	}

	a.post("/workspace?action=github-account&id=w&account=carol", "", 400)
	a.post("/workspace?action=github-account&id=nope&account=alice", "", 400)
	a.post("/workspace?action=github-account&id=w&account=alice", "", 200)
	p.until("the chosen account", func() bool {
		return p.state.Workspaces[0].GitHub != nil && p.state.Workspaces[0].GitHub.Account == "alice"
	})
}
