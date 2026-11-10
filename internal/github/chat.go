package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/kkestell/hypercube/internal/workspace"
)

// PullRequestHead fetches login's open pull request number in repo into the
// checkout at dir with login's token and returns its head branch and commit.
// It reads the account's pull requests rather than the workspace's status, so
// it works for a workspace added before the next pass publishes its status.
func (w *Watcher) PullRequestHead(ctx context.Context, login, repo string, number int, dir string) (branch, commit string, err error) {
	branch, ok := w.openBranch(login, repo, number)
	if !ok {
		return "", "", fmt.Errorf("pull request #%d in %s is not open for %s", number, repo, login)
	}
	token, err := w.token(ctx, login)
	if err != nil {
		return "", "", fmt.Errorf("reading the GitHub token for %s: %w", login, err)
	}
	remote, _, err := workspace.Remote(dir)
	if err != nil {
		return "", "", err
	}
	commit, err = workspace.FetchPullRequest(ctx, dir, remote, number, []string{"GH_TOKEN=" + token})
	return branch, commit, err
}

func (w *Watcher) openBranch(login, repo string, number int) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, pr := range w.accounts[login].PullRequests {
		if pr.Number == number && strings.EqualFold(pr.Repo, repo) {
			return pr.Branch, true
		}
	}
	return "", false
}

// CheckRepo returns an error unless the checkout at dir is on GitHub as repo,
// read from its remote the way the watcher maps a workspace to a repository.
func CheckRepo(dir, repo string) error {
	if _, err := workspace.Root(dir); err != nil {
		return fmt.Errorf("%s is not a Git checkout", dir)
	}
	_, remote, err := workspace.Remote(dir)
	if err != nil {
		return err
	}
	owner, name, ok := parseRepo(remote)
	if !ok {
		return fmt.Errorf("%s has no GitHub remote", dir)
	}
	if found := owner + "/" + name; !strings.EqualFold(found, repo) {
		return fmt.Errorf("%s is a checkout of %s, not %s", dir, found, repo)
	}
	return nil
}
