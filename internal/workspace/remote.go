package workspace

import (
	"errors"
	"os/exec"
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
