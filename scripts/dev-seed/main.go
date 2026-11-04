// dev-seed prepares the preview workspace before the dev app accepts browsers.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: dev-seed <repo>")
	}
	if err := seed(os.Args[1]); err != nil {
		log.Fatal(err)
	}
}

func seed(repo string) error {
	db, err := store.Open()
	if err != nil {
		return err
	}
	defer db.Close()
	var exists bool
	if err := db.Read(func(tx *store.Tx) error {
		_, err := tx.Session("dev-review")
		exists = err == nil
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}); err != nil || exists {
		return err
	}

	other := filepath.Join(filepath.Dir(repo), "notes")
	if err := os.MkdirAll(other, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(other, "ideas.md"), []byte("# Ideas\n\n- Review the preview.\n"), 0644); err != nil {
		return err
	}
	rt := session.New(db, "")
	w, err := rt.AddWorkspace(repo)
	if err != nil {
		return err
	}
	w2, err := rt.AddWorkspace(other)
	if err != nil {
		return err
	}

	branch := workspace.ChildBranch("dev-preview", "design")
	dir := workspace.WorktreeDir(repo, "dev-preview", "design")
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		if err := workspace.AddWorktree(repo, dir, branch); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "design.md"), []byte("# Design notes\n\nA small improvement for the preview.\n"), 0644); err != nil {
			return err
		}
		if err := git(dir, "add", "design.md"); err != nil {
			return err
		}
		if err := git(dir, "-c", "user.name=hypercube-dev", "-c", "user.email=dev@localhost", "commit", "-m", "Add design notes"); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "draft.md"), []byte("A work in progress.\n"), 0644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(repo, "README.md"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	_, err = f.WriteString("\nPreview change: a short note for review.\n")
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(repo, "docs", "usage.md")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Join(repo, "notes"), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(repo, "notes", "review.md"), []byte("# Review notes\n\nCheck the sidebar and transcript.\n"), 0644); err != nil {
		return err
	}

	now := time.Now().Add(-5 * time.Minute).UnixMilli()
	sessions := []store.Session{
		{ID: "dev-preview", Workspace: w.ID, Title: "Polish the workspace", Running: true, Harness: "test", Model: "preview", Effort: "low", Dir: repo, Activity: store.Idle},
		{ID: "dev-design", Workspace: w.ID, Parent: "dev-preview", Name: "design", Title: "Explore a design", Harness: "test", Model: "preview", Effort: "high", Dir: dir, Branch: branch, Activity: store.Idle},
		{ID: "dev-review", Workspace: w.ID, Title: "Review the changes", Running: true, Unseen: true, Harness: "test", Model: "review", Effort: "high", Dir: repo, Activity: store.Idle},
		{ID: "dev-notes", Workspace: w2.ID, Title: "Plan the next steps", Running: true, Harness: "test", Model: "planning", Effort: "medium", Dir: other, Activity: store.Idle},
	}
	items := map[string][]harness.Item{
		"dev-preview": {
			{ID: "sample:user", Kind: harness.KindUser, Text: "Please review the **workspace** and suggest a small improvement.", At: now, Done: true},
			{ID: "sample:working", Kind: harness.KindState, State: harness.Working, At: now + 1000, Done: true},
			{ID: "sample:reason", Kind: harness.KindReasoning, Text: "I'll inspect the existing files and keep the change focused.", At: now + 2000, Done: true},
			{ID: "sample:tool", Kind: harness.KindTool, Tool: "shell", Input: json.RawMessage("{\"command\":\"git status --short\"}"), Output: " M README.md\n D docs/usage.md\n?? notes/", At: now + 3000, Done: true},
			{ID: "sample:reply", Kind: harness.KindText, Text: "## Review\n\nThe workspace is ready for a focused change.\n\n```go\nfmt.Println(\"Hello, preview!\")\n```\n\n- Check the modified file.\n- Review the new notes.", At: now + 12000, Done: true},
			{ID: "sample:idle", Kind: harness.KindState, State: harness.Idle, Final: []string{"sample:reply"}, At: now + 13000, Done: true},
		},
		"dev-review": {{ID: "review:user", Kind: harness.KindUser, Text: "What did you find?", Done: true}, {ID: "review:error", Kind: harness.KindError, Text: "A sample command failed; retrying succeeded.", Done: true}, {ID: "review:reply", Kind: harness.KindText, Text: "The changes look good. I left a few **review notes** for you.", Done: true, Images: []harness.Image{{MIMEType: "image/png", Data: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/aWQAAAAASUVORK5CYII="}}}},
		"dev-notes":  {{ID: "notes:user", Kind: harness.KindUser, Text: "Help me plan the next iteration.", Done: true}, {ID: "notes:question", Kind: harness.KindAttention, Text: "Which area should we tackle first?", Choices: []string{"Sidebar", "Transcript"}, Free: true}},
		"dev-design": {{ID: "design:user", Kind: harness.KindUser, From: "dev-preview", Text: "Explore a design in your worktree.", Done: true}, {ID: "design:reply", Kind: harness.KindText, Text: "I committed the design notes and started a draft.", Done: true}},
	}
	return db.Tx(func(tx *store.Tx) error {
		for _, s := range sessions {
			if _, err := tx.Session(s.ID); errors.Is(err, store.ErrNotFound) {
				if err := tx.AddSession(s); err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else if err := tx.SaveSession(s); err != nil {
				return err
			}
			for i, item := range items[s.ID] {
				data, err := json.Marshal(item)
				if err != nil {
					return err
				}
				if _, err := tx.PutEntry(s.ID, store.Entry{Seq: int64(i + 1), Item: item.ID, Data: data}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func git(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %s: %w", args, out, err)
	}
	return nil
}
