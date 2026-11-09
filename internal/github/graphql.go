package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"
)

const defaultAPI = "https://api.github.com/graphql"

// errNotFound means the account cannot see the repository.
var errNotFound = errors.New("repository not found")

// errUnauthorized means GitHub rejected the token, such as a revoked login.
var errUnauthorized = errors.New("token rejected")

const permissionQuery = `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){viewerPermission}}`

// query posts a GraphQL query with the token and decodes its data.
func (w *Watcher) query(ctx context.Context, token, query string, variables map[string]any, data any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	api := w.API
	if api == "" {
		api = defaultAPI
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return errUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var answer struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		return fmt.Errorf("reading GitHub's answer: %w", err)
	}
	if len(answer.Errors) > 0 {
		if answer.Errors[0].Type == "NOT_FOUND" {
			return errNotFound
		}
		return errors.New(answer.Errors[0].Message)
	}
	return json.Unmarshal(answer.Data, data)
}

// canPush reports whether the token's account has write access to the
// repository. An account whose token GitHub rejects cannot.
func (w *Watcher) canPush(ctx context.Context, token, owner, name string) (bool, error) {
	var data struct {
		Repository struct {
			ViewerPermission string `json:"viewerPermission"`
		} `json:"repository"`
	}
	err := w.query(ctx, token, permissionQuery, map[string]any{"owner": owner, "name": name}, &data)
	if errors.Is(err, errNotFound) || errors.Is(err, errUnauthorized) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return slices.Contains([]string{"ADMIN", "MAINTAIN", "WRITE"}, data.Repository.ViewerPermission), nil
}

const pullRequestsQuery = `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){
  pullRequests(states:OPEN,first:50,orderBy:{field:UPDATED_AT,direction:DESC}){
    totalCount
    nodes{number title url isDraft isCrossRepository headRefName
      author{login} reviewDecision statusCheckRollup{state}}}}}`

// pullRequests lists the repository's most recently updated open pull requests,
// never nil, and counts all of them.
func (w *Watcher) pullRequests(ctx context.Context, token, owner, name string) ([]PullRequest, int, error) {
	var data struct {
		Repository struct {
			PullRequests struct {
				TotalCount int `json:"totalCount"`
				Nodes      []struct {
					Number            int    `json:"number"`
					Title             string `json:"title"`
					URL               string `json:"url"`
					IsDraft           bool   `json:"isDraft"`
					IsCrossRepository bool   `json:"isCrossRepository"`
					HeadRefName       string `json:"headRefName"`
					Author            *struct {
						Login string `json:"login"`
					} `json:"author"`
					ReviewDecision    string `json:"reviewDecision"`
					StatusCheckRollup *struct {
						State string `json:"state"`
					} `json:"statusCheckRollup"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}
	if err := w.query(ctx, token, pullRequestsQuery, map[string]any{"owner": owner, "name": name}, &data); err != nil {
		return nil, 0, err
	}
	prs := []PullRequest{}
	for _, n := range data.Repository.PullRequests.Nodes {
		pr := PullRequest{Number: n.Number, Title: n.Title, URL: n.URL, Branch: n.HeadRefName,
			Fork: n.IsCrossRepository, Draft: n.IsDraft, Review: n.ReviewDecision}
		if n.Author != nil {
			pr.Author = n.Author.Login
		}
		if n.StatusCheckRollup != nil {
			pr.Checks = n.StatusCheckRollup.State
		}
		prs = append(prs, pr)
	}
	return prs, data.Repository.PullRequests.TotalCount, nil
}
