package github

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kkestell/hypercube/internal/workspace"
)

// ErrNotPullRequestRepo means a clone was asked for a repository that none of
// the account's pull requests are in.
var ErrNotPullRequestRepo = errors.New("not a pull request repository")

// Clone clones repo, an owner/name that one of login's pull requests is in,
// into dir with login's token. It records login in the clone's own Git
// configuration so the new workspace uses that account.
func (w *Watcher) Clone(ctx context.Context, repo, login, dir string) error {
	if !w.hasPullRequestIn(login, repo) {
		return fmt.Errorf("%w for %s", ErrNotPullRequestRepo, login)
	}
	token, err := w.token(ctx, login)
	if err != nil {
		return fmt.Errorf("reading the GitHub token for %s: %w", login, err)
	}
	return workspace.Clone(ctx, "https://github.com/"+repo+".git", dir,
		[]string{"credential.githubAccount=" + login}, []string{"GH_TOKEN=" + token})
}

func (w *Watcher) hasPullRequestIn(login, repo string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.ContainsFunc(w.accounts[login].PullRequests, func(pr PullRequest) bool { return pr.Repo == repo })
}
