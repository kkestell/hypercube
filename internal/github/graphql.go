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
