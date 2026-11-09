package github

import (
	"context"
	"fmt"
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

// untilAccounts waits on Changed until the accounts satisfy check.
func (s *stand) untilAccounts(what string, check func([]Account) bool) {
	s.t.Helper()
	timeout := time.After(20 * time.Second)
	for {
		changed := s.w.Changed()
		if check(s.w.Accounts()) {
			return
		}
		select {
		case <-changed:
		case <-timeout:
			s.t.Fatalf("timed out waiting for %s; accounts %+v", what, s.w.Accounts())
		}
	}
}

// accountOf returns the login's account, or the zero value.
func (s *stand) accountOf(accounts []Account, login string) Account {
	for _, a := range accounts {
		if a.Login == login {
			return a
		}
	}
	return Account{}
}

// repoNode is a minimal pull request node in the repository.
func repoNode(repo string, number int) map[string]any {
	return map[string]any{"number": number, "title": fmt.Sprintf("PR %d", number),
		"url":        fmt.Sprintf("https://github.com/%s/pull/%d", repo, number),
		"repository": map[string]string{"nameWithOwner": repo}, "author": map[string]string{"login": "dana"}}
}

func TestPullRequestsMapAndKeepOnFailure(t *testing.T) {
	s := newStand(t, "alice")
	s.api.SetPullRequests([]map[string]any{
		{"number": 7, "title": "Add it", "url": "https://github.com/o/r/pull/7", "isDraft": false,
			"repository":        map[string]string{"nameWithOwner": "o/r"},
			"isCrossRepository": false, "headRefName": "feature", "author": map[string]string{"login": "dana"},
			"reviewDecision": "APPROVED", "mergeStateStatus": "CLEAN", "statusCheckRollup": map[string]string{"state": "SUCCESS"}},
		{"number": 5, "title": "Ghost", "url": "https://github.com/o/r/pull/5", "isDraft": true,
			"repository":        map[string]string{"nameWithOwner": "o/r"},
			"isCrossRepository": true, "headRefName": "fork-branch", "author": nil,
			"reviewDecision": nil, "statusCheckRollup": nil},
		repoNode("o/s", 2),
		{}, // a search result that is not a pull request
	}, nil)
	id := s.workspace("w", remote)
	s.run()
	s.until("the pull requests", s.queried(id))
	got := s.w.Statuses()[id]
	want := []PullRequest{
		{Number: 7, Title: "Add it", URL: "https://github.com/o/r/pull/7", Repo: "o/r", Author: "dana", Branch: "feature", Review: "APPROVED", Checks: "SUCCESS", Merge: "CLEAN"},
		{Number: 5, Title: "Ghost", URL: "https://github.com/o/r/pull/5", Repo: "o/r", Branch: "fork-branch", Fork: true, Draft: true},
	}
	if !reflect.DeepEqual(got.PullRequests, want) || got.Error != "" {
		t.Fatalf("status = %+v", got)
	}
	account := s.accountOf(s.w.Accounts(), "alice")
	if len(account.PullRequests) != 3 || account.PullRequests[2].Repo != "o/s" || account.Updated != got.Updated {
		t.Fatalf("account = %+v", account)
	}

	s.api.Fail(true)
	s.w.Refresh()
	s.until("the failure", func(m map[string]Status) bool { return m[id].Error != "" })
	failed := s.w.Statuses()[id]
	if !reflect.DeepEqual(failed.PullRequests, want) || failed.Updated != got.Updated {
		t.Fatalf("status after failure = %+v", failed)
	}
	kept := s.accountOf(s.w.Accounts(), "alice")
	if len(kept.PullRequests) != 3 || kept.Updated != account.Updated || kept.Error == "" {
		t.Fatalf("account after failure = %+v", kept)
	}
}

func TestRepositoryMatchIgnoresCase(t *testing.T) {
	s := newStand(t, "alice")
	s.api.SetPullRequests([]map[string]any{repoNode("O/R", 1)}, nil)
	id := s.workspace("w", remote)
	s.run()
	s.until("the pull requests", s.queried(id))
	if got := s.w.Statuses()[id].PullRequests; len(got) != 1 || got[0].Repo != "O/R" {
		t.Fatalf("pull requests = %+v", got)
	}
}

func TestEveryLoginIsRead(t *testing.T) {
	s := newStand(t, "alice", "bob")
	s.api.SetPullRequests([]map[string]any{repoNode("o/r", 1)}, nil)
	s.workspace("w", remote) // pending a choice, so it uses neither account
	s.run()
	s.untilAccounts("both accounts", func(as []Account) bool {
		return s.accountOf(as, "alice").Updated != 0 && s.accountOf(as, "bob").Updated != 0
	})
	accounts := s.w.Accounts()
	if len(accounts) != 2 || accounts[0].Login != "alice" || accounts[1].Login != "bob" ||
		accounts[0].PullRequests[0].URL != accounts[1].PullRequests[0].URL {
		t.Fatalf("accounts = %+v", accounts)
	}
}

func TestDirectAndTeamRequestsAreKeyedByURL(t *testing.T) {
	s := newStand(t, "alice")
	direct, team := repoNode("o/r", 1), repoNode("o/s", 1)
	s.api.SetPullRequests(nil, []map[string]any{direct, team}, "https://github.com/o/r/pull/1")
	s.workspace("w", remote)
	s.run()
	s.untilAccounts("the account", func(as []Account) bool { return s.accountOf(as, "alice").Updated != 0 })
	got := s.w.Accounts()[0].PullRequests
	if len(got) != 2 || got[0].Requested != "user" || got[1].Requested != "team" {
		t.Fatalf("pull requests = %+v", got)
	}
}

func TestRequiredFailuresAcrossRepositories(t *testing.T) {
	s := newStand(t, "alice")
	var nodes []map[string]any
	for _, repo := range []string{"o/r", "o/s"} {
		for _, n := range []int{1, 2} {
			node := repoNode(repo, n)
			node["author"] = map[string]string{"login": "alice"}
			node["statusCheckRollup"] = map[string]string{"state": "FAILURE"}
			nodes = append(nodes, node)
		}
	}
	s.api.SetPullRequests(nodes, nil)
	s.api.SetRequiredFailures("https://github.com/o/s/pull/1", "https://github.com/o/r/pull/2")
	s.workspace("w", remote)
	s.run()
	s.untilAccounts("the account", func(as []Account) bool { return s.accountOf(as, "alice").Updated != 0 })
	failed := map[string]bool{}
	for _, pr := range s.w.Accounts()[0].PullRequests {
		failed[pr.URL] = pr.RequiredFailed
	}
	want := map[string]bool{"https://github.com/o/r/pull/1": false, "https://github.com/o/r/pull/2": true,
		"https://github.com/o/s/pull/1": true, "https://github.com/o/s/pull/2": false}
	if !reflect.DeepEqual(failed, want) {
		t.Fatalf("required failures = %v", failed)
	}
}

// person and bot are comment authors; at is a time on the test's clock.
func person(login string) map[string]string {
	return map[string]string{"__typename": "User", "login": login}
}
func bot(login string) map[string]string {
	return map[string]string{"__typename": "Bot", "login": login}
}
func at(minute int) string {
	return time.Date(2026, 1, 1, 0, minute, 0, 0, time.UTC).Format(time.RFC3339)
}

func said(author map[string]string, minute int) map[string]any {
	return map[string]any{"author": author, "createdAt": at(minute)}
}

func thread(resolved bool, first, last map[string]any) map[string]any {
	return map[string]any{"isResolved": resolved, "first": map[string]any{"nodes": []any{first}}, "last": map[string]any{"nodes": []any{last}}}
}

func TestPullRequestsDescribeTheAccountsPart(t *testing.T) {
	type pr = map[string]any
	node := func(number int, author string, fields pr) pr {
		n := pr{"number": number, "author": map[string]string{"login": author}, "headRefOid": "head",
			"url":        fmt.Sprintf("https://github.com/o/r/pull/%d", number),
			"repository": map[string]string{"nameWithOwner": "o/r"},
			"commits":    pr{"nodes": []pr{{"commit": pr{"committedDate": at(10)}}}}}
		for k, v := range fields {
			n[k] = v
		}
		return n
	}
	review := func(state string, minute int, commit string) pr {
		return pr{"nodes": []pr{{"state": state, "submittedAt": at(minute), "commit": pr{"oid": commit}}}}
	}
	opinion := func(login, state string) pr {
		return pr{"nodes": []pr{{"author": map[string]string{"login": login}, "state": state}}}
	}
	threads := func(ts ...map[string]any) pr { return pr{"nodes": ts} }
	conversation := func(cs ...map[string]any) pr { return pr{"nodes": cs} }
	cases := []struct {
		name string
		node pr
		want PullRequest
	}{
		{"own with a failed required check, a reply, and a newer comment", node(1, "alice", pr{
			"statusCheckRollup": pr{"state": "FAILURE"}, "mergeStateStatus": "DIRTY",
			"reviewThreads": threads(thread(false, said(person("bob"), 1), said(person("bob"), 2)),
				thread(true, said(person("bob"), 1), said(person("bob"), 2)),
				thread(false, said(bot("copilot"), 1), said(bot("copilot"), 1))),
			"comments":       conversation(said(person("carol"), 11), said(bot("codecov"), 12)),
			"reviewRequests": pr{"totalCount": 2}}),
			PullRequest{Checks: "FAILURE", Merge: "DIRTY", RequiredFailed: true, Unanswered: 2, PendingReviews: 2}},
		{"own with comments answered by a reply or a push", node(2, "alice", pr{
			"reviewThreads": threads(thread(false, said(person("bob"), 1), said(person("alice"), 2))),
			"comments":      conversation(said(person("bob"), 3))}),
			PullRequest{}},
		{"own with a comment answered by a reply", node(3, "alice", pr{
			"comments": conversation(said(person("bob"), 11), said(person("alice"), 12))}),
			PullRequest{}},
		{"own with changes requested, then a push", node(9, "alice", pr{"reviewDecision": "CHANGES_REQUESTED",
			"latestOpinionatedReviews": pr{"nodes": []pr{{"author": map[string]string{"login": "bob"}, "state": "CHANGES_REQUESTED", "commit": pr{"oid": "old"}}}}}),
			PullRequest{Review: "CHANGES_REQUESTED", Responded: true}},
		{"own with changes requested at its head", node(10, "alice", pr{"reviewDecision": "CHANGES_REQUESTED",
			"latestOpinionatedReviews": pr{"nodes": []pr{{"author": map[string]string{"login": "bob"}, "state": "CHANGES_REQUESTED", "commit": pr{"oid": "head"}}}}}),
			PullRequest{Review: "CHANGES_REQUESTED"}},
		{"review requested from the account", node(4, "bob", nil),
			PullRequest{Requested: "user"}},
		{"review requested from the account's team", node(5, "bob", nil),
			PullRequest{Requested: "team"}},
		{"changes requested, then a push", node(6, "bob", pr{
			"latestOpinionatedReviews": opinion("alice", "CHANGES_REQUESTED"), "reviews": review("CHANGES_REQUESTED", 20, "old")}),
			PullRequest{Reviewed: "CHANGES_REQUESTED", Responded: true}},
		{"changes requested, then nothing", node(7, "bob", pr{
			"latestOpinionatedReviews": opinion("alice", "CHANGES_REQUESTED"), "reviews": review("CHANGES_REQUESTED", 20, "head"),
			"comments": conversation(said(person("carol"), 21))}),
			PullRequest{Reviewed: "CHANGES_REQUESTED"}},
		{"changes requested, then commented, then nothing", node(11, "bob", pr{
			"latestOpinionatedReviews": opinion("alice", "CHANGES_REQUESTED"), "reviews": review("COMMENTED", 30, "head"),
			"comments": conversation(said(person("bob"), 25))}),
			PullRequest{Reviewed: "CHANGES_REQUESTED"}},
		{"reviewed, with a deleted author and a bot comment since", node(12, "bob", pr{
			"author": nil, "reviews": review("COMMENTED", 20, "head"), "comments": conversation(said(bot("codecov"), 21))}),
			PullRequest{}},
		{"commented, then the author replied in the account's thread", node(8, "bob", pr{
			"reviews":       review("COMMENTED", 20, "head"),
			"reviewThreads": threads(thread(false, said(person("alice"), 20), said(person("bob"), 21)), thread(false, said(person("carol"), 1), said(person("carol"), 2)))}),
			PullRequest{Responded: true, Unanswered: 1}},
	}
	s := newStand(t, "alice")
	var nodes []map[string]any
	for _, c := range cases {
		nodes = append(nodes, c.node)
	}
	// Review requests reach the list even when they are not among the
	// involved pull requests.
	var involved, requested []map[string]any
	for _, n := range nodes {
		if n["number"] == 4 || n["number"] == 5 {
			requested = append(requested, n)
		} else {
			involved = append(involved, n)
		}
	}
	s.api.SetPullRequests(involved, requested, "https://github.com/o/r/pull/4")
	s.api.SetRequiredFailures("https://github.com/o/r/pull/1")
	id := s.workspace("w", remote)
	s.run()
	s.until("the pull requests", s.queried(id))
	byNumber := map[int]PullRequest{}
	for _, pr := range s.w.Statuses()[id].PullRequests {
		byNumber[pr.Number] = pr
	}
	if len(byNumber) != len(cases) {
		t.Fatalf("pull requests = %+v", byNumber)
	}
	for _, c := range cases {
		g := byNumber[c.node["number"].(int)]
		g.Number, g.Author, g.URL, g.Repo = 0, "", "", ""
		if !reflect.DeepEqual(g, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, g, c.want)
		}
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
	s.api.SetPullRequests([]map[string]any{repoNode("o/r", 1)}, nil)
	s.run()
	s.until("a choice", s.resolved(id))
	s.untilAccounts("both accounts", func(as []Account) bool { return s.accountOf(as, "alice").Updated != 0 })
	got := s.w.Statuses()[id]
	if got.PullRequests == nil || len(got.PullRequests) != 0 || got.Updated != 0 || got.Error != "" || got.Behind != 0 {
		t.Fatalf("status = %+v", got)
	}
}

func TestTokenFailureIsReported(t *testing.T) {
	s := newStand(t, "broken", "alice")
	s.api.SetPullRequests([]map[string]any{repoNode("o/r", 1)}, nil)
	id := s.workspace("w", remote, []string{"config", "credential.githubAccount", "broken"})
	s.run()
	s.until("the account", s.resolved(id))
	s.untilAccounts("the other account", func(as []Account) bool { return s.accountOf(as, "alice").Updated != 0 })
	got := s.w.Statuses()[id]
	if got.Account != "broken" || got.Updated != 0 || strings.Count(got.Error, "no token for broken") != 1 {
		t.Fatalf("status = %+v", got)
	}
	broken := s.accountOf(s.w.Accounts(), "broken")
	if broken.Updated != 0 || !strings.Contains(broken.Error, "no token for broken") || len(broken.PullRequests) != 0 {
		t.Fatalf("account = %+v", broken)
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
