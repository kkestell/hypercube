package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// run runs gh without the tokens this process may have inherited, which would
// hide every account but one.
func (w *Watcher) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, cmpGH(w.GH), args...)
	cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, "GH_TOKEN=") || strings.HasPrefix(kv, "GITHUB_TOKEN=")
	})
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && len(exit.Stderr) > 0 {
			return nil, fmt.Errorf("gh %s: %w: %s", args[0], err, strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("gh %s: %w", args[0], err)
	}
	return out, nil
}

func cmpGH(gh string) string {
	if gh == "" {
		return "gh"
	}
	return gh
}

// logins lists every account gh knows on github.com, whatever its state:
// gh tests each token over the network to set the state.
func (w *Watcher) logins(ctx context.Context) ([]string, error) {
	out, err := w.run(ctx, "auth", "status", "--json", "hosts", "--hostname", "github.com")
	if err != nil {
		return nil, err
	}
	var status struct {
		Hosts map[string][]struct {
			Login string `json:"login"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("reading gh auth status: %w", err)
	}
	var logins []string
	for _, account := range status.Hosts["github.com"] {
		logins = append(logins, account.Login)
	}
	slices.Sort(logins)
	return slices.Compact(logins), nil
}

// token reads the login's token from gh.
func (w *Watcher) token(ctx context.Context, login string) (string, error) {
	out, err := w.run(ctx, "auth", "token", "--hostname", "github.com", "--user", login)
	return strings.TrimSpace(string(out)), err
}
