package github

import (
	"context"
	"errors"
	"fmt"

	"github.com/kkestell/hypercube/internal/workspace"
)

// FetchPullRequest fetches pull request number's head commit into the
// workspace's checkout at dir with the workspace's account, and returns the
// commit ID and the pull request's URL.
func (w *Watcher) FetchPullRequest(ctx context.Context, workspaceID, dir string, number int) (commit, url string, err error) {
	status, ok := w.Statuses()[workspaceID]
	if !ok {
		return "", "", errors.New("the workspace has no GitHub repository")
	}
	remote, _, err := workspace.Remote(dir)
	if err != nil {
		return "", "", err
	}
	commit, err = workspace.FetchPullRequest(ctx, dir, remote, number, w.Env(workspaceID))
	if err != nil {
		return "", "", err
	}
	return commit, fmt.Sprintf("%s/pull/%d", status.URL, number), nil
}
