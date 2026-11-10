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
	"regexp"
	"slices"
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
// and the pull requests, later pages of their review threads, and required
// check failures every token reads.
type API struct {
	URL string

	mu          sync.Mutex
	permissions map[string]string
	failing     bool
	revoked     map[string]bool
	requests    int
	pulls       string
	failedURLs  []string
	threadPages map[threadPage]any
	threadReads []string
}

// threadPage names the review threads of a pull request after a cursor.
type threadPage struct{ id, after string }

// NewAPI starts the server and stops it when the test ends. Every account
// starts without access to any repository.
func NewAPI(t testing.TB) *API {
	t.Helper()
	a := &API{permissions: map[string]string{}, revoked: map[string]bool{}, pulls: pullsAnswer(), threadPages: map[threadPage]any{}}
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

// SetPullRequests sets what every token reads: the nodes of the pull requests
// that involve the token's account and of the ones whose review was requested
// from it or its team, each shaped like GitHub's pull request fields, and the
// URLs of the ones requested from the account itself.
func (a *API) SetPullRequests(involved, requested []map[string]any, direct ...string) {
	var urls []map[string]string
	for _, u := range direct {
		urls = append(urls, map[string]string{"url": u})
	}
	data, err := json.Marshal(map[string]any{"data": map[string]any{
		"involved":  map[string]any{"nodes": involved},
		"requested": map[string]any{"nodes": requested},
		"direct":    map[string]any{"nodes": urls}}})
	if err != nil {
		panic(err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pulls = string(data)
}

// SetRequiredFailures sets the pull requests, by URL such as
// https://github.com/owner/name/pull/12, that have a failed required check.
func (a *API) SetRequiredFailures(urls ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failedURLs = urls
}

// SetThreadPage sets the page of review threads, shaped like GitHub's
// reviewThreads connection, that follows the cursor after in the pull request
// with node ID id.
func (a *API) SetThreadPage(id, after string, page map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.threadPages[threadPage{id, after}] = page
}

// ThreadReads lists the node IDs of the pull requests whose further review
// threads the server has been asked for, in order.
func (a *API) ThreadReads() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.threadReads)
}

// threadFields finds the pull request fields of a review thread query.
var threadFields = regexp.MustCompile(`(p\d+):node\(id:\$(i\d+)\)\{\.\.\.on PullRequest\{reviewThreads\(first:\d+,after:\$(c\d+)\)`)

// threadsAnswer answers a review thread query from the pages set for each pull
// request's ID and cursor variables.
func (a *API) threadsAnswer(query string, variables map[string]string) string {
	data := map[string]any{}
	for _, m := range threadFields.FindAllStringSubmatch(query, -1) {
		id := variables[m[2]]
		a.threadReads = append(a.threadReads, id)
		page, ok := a.threadPages[threadPage{id, variables[m[3]]}]
		if !ok {
			data[m[1]] = nil
			continue
		}
		data[m[1]] = map[string]any{"reviewThreads": page}
	}
	out, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		panic(err)
	}
	return string(out)
}

// aliases finds the repository and pull request fields of a required check
// query.
var aliases = regexp.MustCompile(`(r\d+):repository\(owner:"([^"]*)",name:"([^"]*)"\)|(p\d+):pullRequest\(number:(\d+)\)`)

// requiredAnswer answers a required check query: a failed required check for
// each pull request whose URL is in the set.
func (a *API) requiredAnswer(query string) string {
	data := map[string]map[string]any{}
	var alias, owner, name string
	for _, m := range aliases.FindAllStringSubmatch(query, -1) {
		if m[1] != "" {
			alias, owner, name = m[1], m[2], m[3]
			data[alias] = map[string]any{}
			continue
		}
		if !slices.Contains(a.failedURLs, fmt.Sprintf("https://github.com/%s/%s/pull/%s", owner, name, m[5])) {
			continue
		}
		data[alias][m[4]] = map[string]any{"statusCheckRollup": map[string]any{"contexts": map[string]any{
			"nodes": []map[string]any{{"conclusion": "FAILURE", "isRequired": true}}}}}
	}
	out, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		panic(err)
	}
	return string(out)
}

func pullsAnswer() string {
	return `{"data":{"involved":{"nodes":[]},"requested":{"nodes":[]},"direct":{"nodes":[]}}}`
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
	var request struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}
	_ = json.Unmarshal(body, &request)
	required := strings.Contains(request.Query, "isRequired")
	pulls := strings.Contains(request.Query, "involved:search")
	threads := !pulls && strings.Contains(request.Query, "reviewThreads")
	if !pulls && !required && !threads {
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
	if required {
		_, _ = w.Write([]byte(a.requiredAnswer(request.Query)))
		return
	}
	if pulls {
		_, _ = w.Write([]byte(a.pulls))
		return
	}
	if threads {
		_, _ = w.Write([]byte(a.threadsAnswer(request.Query, request.Variables)))
		return
	}
	permission := a.permissions[token]
	if permission == "" {
		_, _ = w.Write([]byte(`{"data":{"repository":null},"errors":[{"type":"NOT_FOUND","message":"not found"}]}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"repository": map[string]string{"viewerPermission": permission}}})
}
