// Package github finds the GitHub account each workspace uses and gives new
// agents and shells its token. It reads accounts from the GitHub CLI and
// repositories from the workspaces' Git remotes.
package github

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

// Status is a GitHub workspace's repository and account.
type Status struct {
	Repo    string   `json:"repo"`              // owner/name
	URL     string   `json:"url"`               // https://github.com/owner/name
	Account string   `json:"account"`           // empty until the user chooses
	Choices []string `json:"choices,omitempty"` // accounts the user chooses among; set only when several qualify
}

// Watcher keeps each GitHub workspace's status and account token current. The
// zero value is ready to use; Run drives it.
type Watcher struct {
	DB       *store.DB
	GH       string        // gh executable; "gh" when empty
	API      string        // GraphQL endpoint; GitHub's when empty
	Interval time.Duration // between passes; one minute when zero

	// mu guards statuses, tokens, changed, and refresh.
	mu       sync.Mutex
	statuses map[string]Status // by workspace ID
	tokens   map[string]string // by workspace ID
	changed  chan struct{}
	refresh  chan struct{}
	// detected belongs to Run's goroutine.
	detected map[string]detection
}

// detection is the accounts that could push to a workspace's repository.
type detection struct {
	key     string // the repository and the logins asked
	pushers []string
}

// Run makes a pass now and then each interval, or soon after Refresh, until
// ctx ends.
func (w *Watcher) Run(ctx context.Context) {
	interval := w.Interval
	if interval == 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	refresh := w.refreshChannel()
	for {
		w.pass(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-refresh:
		}
	}
}

func (w *Watcher) refreshChannel() chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.refresh == nil {
		w.refresh = make(chan struct{}, 1)
	}
	return w.refresh
}

// Refresh asks for a pass soon. It never blocks.
func (w *Watcher) Refresh() {
	select {
	case w.refreshChannel() <- struct{}{}:
	default:
	}
}

// Env returns the environment that gives a workspace's agents and shells its
// account's token, or nil when it has none.
func (w *Watcher) Env(workspace string) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if token := w.tokens[workspace]; token != "" {
		return []string{"GH_TOKEN=" + token}
	}
	return nil
}

// Statuses returns each GitHub workspace's status by workspace ID.
func (w *Watcher) Statuses() map[string]Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	return maps.Clone(w.statuses)
}

// Changed returns a channel that closes at the next change to the statuses.
func (w *Watcher) Changed() <-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.changed == nil {
		w.changed = make(chan struct{})
	}
	return w.changed
}

// publish replaces the statuses and tokens, waking watchers when a status
// differs. The caller does not hold mu.
func (w *Watcher) publish(statuses map[string]Status, tokens map[string]string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	differs := !maps.EqualFunc(w.statuses, statuses, func(a, b Status) bool { return reflect.DeepEqual(a, b) })
	w.statuses, w.tokens = statuses, tokens
	if differs {
		if w.changed != nil {
			close(w.changed)
		}
		w.changed = make(chan struct{})
	}
}

// pass resolves every workspace's repository and account.
func (w *Watcher) pass(ctx context.Context) {
	logins, err := w.logins(ctx)
	if err != nil || len(logins) == 0 {
		if err != nil && ctx.Err() == nil && !errors.Is(err, exec.ErrNotFound) {
			fmt.Fprintln(os.Stderr, "hypercube: reading GitHub accounts:", err)
		}
		if ctx.Err() == nil {
			w.publish(nil, nil)
		}
		return
	}
	var workspaces []store.Workspace
	if err := w.DB.Read(func(tx *store.Tx) (err error) { workspaces, err = tx.Workspaces(); return }); err != nil {
		fmt.Fprintln(os.Stderr, "hypercube: reading workspaces for GitHub:", err)
		return
	}
	w.mu.Lock()
	old, oldTokens := w.statuses, w.tokens // replaced, never edited
	w.mu.Unlock()
	statuses, tokens := map[string]Status{}, map[string]string{}
	passTokens := map[string]string{} // by login, read at most once a pass
	token := func(login string) (string, error) {
		if t, ok := passTokens[login]; ok {
			return t, nil
		}
		t, err := w.token(ctx, login)
		if err != nil {
			return "", err
		}
		passTokens[login] = t
		return t, nil
	}
	detected := map[string]detection{}
	for _, ws := range workspaces {
		if ws.NoGit {
			continue
		}
		_, remote, err := workspace.Remote(ws.Path)
		if err != nil {
			continue
		}
		owner, name, ok := parseRepo(remote)
		if !ok {
			continue
		}
		repo := owner + "/" + name
		status := Status{Repo: repo, URL: "https://github.com/" + repo}
		if saved, _ := workspace.Config(ws.Path, "credential.githubAccount"); slices.Contains(logins, saved) {
			status.Account = saved
		} else {
			key := repo + "\n" + strings.Join(logins, ",")
			d, ok := w.detected[ws.ID]
			if !ok || d.key != key {
				pushers, err := w.pushers(ctx, logins, owner, name, token)
				if err != nil {
					if ctx.Err() == nil {
						fmt.Fprintf(os.Stderr, "hypercube: finding GitHub accounts that can push to %s: %v\n", repo, err)
					}
					// Keep what the last pass knew.
					if prev, ok := old[ws.ID]; ok {
						statuses[ws.ID], tokens[ws.ID] = prev, oldTokens[ws.ID]
					}
					continue
				}
				d = detection{key, pushers}
			}
			detected[ws.ID] = d
			candidates := d.pushers
			if len(candidates) == 0 {
				candidates = logins
			}
			if len(candidates) == 1 {
				status.Account = candidates[0]
			} else {
				status.Choices = candidates
			}
		}
		if status.Account != "" {
			t, err := token(status.Account)
			if err != nil {
				fmt.Fprintf(os.Stderr, "hypercube: reading the GitHub token for %s: %v\n", status.Account, err)
			} else {
				tokens[ws.ID] = t
			}
		}
		statuses[ws.ID] = status
	}
	if ctx.Err() != nil {
		return
	}
	w.detected = detected
	w.publish(statuses, tokens)
}

// pushers lists the logins whose accounts can push to the repository.
func (w *Watcher) pushers(ctx context.Context, logins []string, owner, name string, token func(string) (string, error)) ([]string, error) {
	var pushers []string
	for _, login := range logins {
		t, err := token(login)
		if err != nil {
			return nil, err
		}
		ok, err := w.canPush(ctx, t, owner, name)
		if err != nil {
			return nil, err
		}
		if ok {
			pushers = append(pushers, login)
		}
	}
	return pushers, nil
}
