// Package githubtest stands in for the GitHub CLI and GitHub's GraphQL API in
// tests.
package githubtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// GH writes a stand-in gh executable that knows the logins and returns the
// path. Its auth status lists them, its auth token for LOGIN prints
// "token-LOGIN" and fails for the login "broken", and it fails when GH_TOKEN is
// set.
func GH(t testing.TB, logins ...string) string {
	t.Helper()
	var hosts []string
	for _, login := range logins {
		hosts = append(hosts, fmt.Sprintf(`{"login":%q,"state":"success","active":false,"host":"github.com"}`, login))
	}
	script := fmt.Sprintf(`#!/bin/sh
if [ -n "$GH_TOKEN" ]; then echo "GH_TOKEN is set" >&2; exit 1; fi
case "$1 $2" in
"auth status") echo '{"hosts":{"github.com":[%s]}}' ;;
"auth token")
	while [ $# -gt 0 ]; do
		if [ "$1" = "--user" ]; then
			if [ "$2" = broken ]; then echo "no token for broken" >&2; exit 1; fi
			echo "token-$2"; exit 0
		fi
		shift
	done
	exit 1 ;;
*) exit 1 ;;
esac
`, strings.Join(hosts, ","))
	path := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// API is a GraphQL server. A test sets the permission each bearer token sees
// and the pull requests every token reads.
type API struct {
	URL string

	mu          sync.Mutex
	permissions map[string]string
	failing     bool
	revoked     map[string]bool
	requests    int
	pulls       string
}

// NewAPI starts the server and stops it when the test ends. Every account
// starts without access to any repository.
func NewAPI(t testing.TB) *API {
	t.Helper()
	a := &API{permissions: map[string]string{}, revoked: map[string]bool{}, pulls: pullsAnswer(0)}
	server := httptest.NewServer(a)
	t.Cleanup(server.Close)
	a.URL = server.URL
	return a
}

// SetPermission sets the viewerPermission the token's account sees, such as
// "WRITE". An empty permission answers NOT_FOUND.
func (a *API) SetPermission(token, permission string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissions[token] = permission
}

// Revoke makes the server answer 401 for the token.
func (a *API) Revoke(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revoked[token] = true
}

// Fail makes the server answer 500 while failing is true.
func (a *API) Fail(failing bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failing = failing
}

// SetPullRequests sets the open pull requests that every token reads: their
// total count and the nodes of the first page, each shaped like GitHub's
// pull request fields.
func (a *API) SetPullRequests(total int, nodes ...map[string]any) {
	data, err := json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{
		"pullRequests": map[string]any{"totalCount": total, "nodes": nodes}}}})
	if err != nil {
		panic(err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pulls = string(data)
}

func pullsAnswer(total int) string {
	return fmt.Sprintf(`{"data":{"repository":{"pullRequests":{"totalCount":%d,"nodes":[]}}}}`, total)
}

// Requests is the number of permission queries the server has received.
func (a *API) Requests() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.requests
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	pulls := strings.Contains(string(body), "pullRequests")
	if !pulls {
		a.requests++
	}
	if a.failing {
		http.Error(w, "unavailable", http.StatusInternalServerError)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "bearer ")
	if a.revoked[token] {
		http.Error(w, "bad credentials", http.StatusUnauthorized)
		return
	}
	if pulls {
		_, _ = w.Write([]byte(a.pulls))
		return
	}
	permission := a.permissions[token]
	if permission == "" {
		_, _ = w.Write([]byte(`{"data":{"repository":null},"errors":[{"type":"NOT_FOUND","message":"not found"}]}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"repository": map[string]string{"viewerPermission": permission}}})
}
