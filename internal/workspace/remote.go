package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Config returns the value of a Git setting in dir, or "" when it is unset.
func Config(dir, key string) (string, error) {
	out, err := git(dir, "config", "--get", key)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", nil
	}
	return out, err
}

// Remote returns the name and configured URL of the remote the current branch
// tracks, or of origin when the branch tracks none; both are empty when
// neither has a URL. It reads the URL setting, not the URL Git would fetch,
// which a url.insteadOf rule can rewrite.
func Remote(dir string) (name, url string, err error) {
	branch, err := git(dir, "branch", "--show-current")
	if err != nil {
		return "", "", err
	}
	candidates := []string{"origin"}
	if branch != "" {
		tracked, err := Config(dir, "branch."+branch+".remote")
		if err != nil {
			return "", "", err
		}
		// A branch tracking "." follows a local branch, not a remote.
		if tracked != "" && tracked != "." {
			candidates = []string{tracked, "origin"}
		}
	}
	for _, name := range candidates {
		url, err := Config(dir, "remote."+name+".url")
		if err != nil {
			return "", "", err
		}
		if url != "" {
			return name, url, nil
		}
	}
	return "", "", nil
}

// Fetch fetches the remote into dir's remote-tracking branches, with extra
// environment variables such as a token, until ctx ends. It leaves FETCH_HEAD
// alone so that it cannot disturb a fetch and merge an agent is running.
func Fetch(ctx context.Context, dir, remote string, env []string) error {
	return fetch(ctx, dir, env, remote)
}

// FetchPullRequest fetches pull request number's head commit from the GitHub
// remote into dir and returns the commit ID. It fetches into a temporary ref,
// rather than FETCH_HEAD, so that it cannot disturb a fetch an agent is
// running, and deletes that ref before returning.
func FetchPullRequest(ctx context.Context, dir, remote string, number int, env []string) (string, error) {
	ref := "refs/hypercube/fetch/" + rand.Text()
	refspec := fmt.Sprintf("+refs/pull/%d/head:%s", number, ref)
	if err := fetch(ctx, dir, env, remote, refspec); err != nil {
		return "", err
	}
	commit, err := git(dir, "rev-parse", "--verify", ref+"^{commit}")
	if _, deleteErr := git(dir, "update-ref", "-d", ref); deleteErr != nil {
		return "", errors.Join(err, deleteErr)
	}
	return commit, err
}

// fetch runs git fetch in dir with extra environment variables until ctx ends,
// leaving FETCH_HEAD alone.
func fetch(ctx context.Context, dir string, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"fetch", "--quiet", "--no-write-fetch-head"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), env...), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &gitError{append([]string{"fetch"}, args...), strings.TrimSpace(stderr.String()), err}
	}
	return nil
}

// Clone clones url into dir with each config entry (such as
// credential.githubAccount=alice) written into the new repository before Git
// fetches, and with extra environment variables such as a token. Cancelling
// ctx interrupts Git, which then removes the folder it created.
func Clone(ctx context.Context, url, dir string, config, env []string) error {
	args := []string{"clone", "--quiet"}
	for _, entry := range config {
		args = append(args, "--config", entry)
	}
	args = append(args, url, dir)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.Env = append(append(os.Environ(), env...), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &gitError{args, strings.TrimSpace(stderr.String()), err}
	}
	return nil
}

// Tracking returns the branch the current branch tracks, such as origin/main,
// and how many commits the current branch is ahead of and behind it. A branch
// without an upstream gives "" and zeros.
func Tracking(dir string) (upstream string, ahead, behind int, err error) {
	upstream, err = git(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return "", 0, 0, nil
	}
	out, err := git(dir, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return "", 0, 0, err
	}
	counts := strings.Fields(out)
	if len(counts) != 2 {
		return "", 0, 0, fmt.Errorf("unexpected rev-list output %q", out)
	}
	if ahead, err = strconv.Atoi(counts[0]); err != nil {
		return "", 0, 0, err
	}
	if behind, err = strconv.Atoi(counts[1]); err != nil {
		return "", 0, 0, err
	}
	return upstream, ahead, behind, nil
}
