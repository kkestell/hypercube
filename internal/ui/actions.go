package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/github"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

// maxInput is the largest input body: 10 MiB of images in base64, and text.
const maxInput = 16 << 20

// workspaceAction adds a workspace in the given folder and returns its ID,
// clones a pull request's repository into a folder and adds it, deletes a
// workspace, or saves the GitHub account a workspace uses.
func (s *server) workspaceAction(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch q.Get("action") {
	case "new":
		dir := q.Get("path")
		if dir == "" {
			http.Error(w, "a path is required", http.StatusBadRequest)
			return
		}
		added, err := s.rt.AddWorkspace(dir)
		if refused(w, err) {
			return
		}
		s.gh.Refresh()
		fmt.Fprint(w, added.ID)
	case "clone":
		dir, ok := absolutePath(q.Get("path"))
		if !ok {
			http.Error(w, "the folder must be an absolute path", http.StatusBadRequest)
			return
		}
		if err := s.gh.Clone(r.Context(), q.Get("repo"), q.Get("account"), dir); errors.Is(err, github.ErrNotPullRequestRepo) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		} else if refused(w, err) {
			return
		}
		added, err := s.rt.AddWorkspace(dir)
		if refused(w, err) {
			return
		}
		s.gh.Refresh()
		fmt.Fprint(w, added.ID)
	case "github-account":
		id, account := q.Get("id"), q.Get("account")
		if !slices.Contains(s.gh.Statuses()[id].Choices, account) {
			http.Error(w, "not an account to choose", http.StatusBadRequest)
			return
		}
		if refused(w, s.db.Tx(func(tx *store.Tx) error { return tx.SetGitHubAccount(id, account) })) {
			return
		}
		s.gh.Refresh()
	case "delete":
		refused(w, s.rt.DeleteWorkspace(q.Get("id")))
	default:
		http.Error(w, "unknown workspace action", http.StatusBadRequest)
	}
}

// input is the JSON body of a prompt: its text and images.
type input struct {
	Submission string          `json:"submission"`
	Text       string          `json:"text"`
	Images     []harness.Image `json:"images"`
}

// readInput reads and decodes a prompt's body, answering the request and
// returning false when it is too large or malformed.
func readInput(w http.ResponseWriter, r *http.Request) (input, bool) {
	var in input
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxInput))
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return in, false
	}
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, "malformed input: "+err.Error(), http.StatusBadRequest)
		return in, false
	}
	if err := session.CheckSubmissionID(in.Submission); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return in, false
	}
	return in, true
}

// sessionAction creates a session in a workspace from its first prompt and
// returns the session ID, or acts on a session.
func (s *server) sessionAction(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	var err error
	switch q.Get("action") {
	case "new":
		in, ok := readInput(w, r)
		if !ok {
			return
		}
		settings := harness.Settings{Harness: q.Get("harness"), Model: q.Get("model"), Effort: q.Get("effort")}
		created, err := s.rt.CreateSubmission(q.Get("workspace"), settings, q.Get("worktree") == "1", q.Get("start"), in.Submission, in.Text, in.Images...)
		writeCreated(w, created.ID, err)
		return
	case "review":
		s.startReview(w, r)
		return
	case "delete":
		err = s.rt.Delete(id)
	case "archive":
		err = s.rt.Archive(id)
	case "unarchive":
		err = s.rt.Unarchive(id)
	case "rename":
		err = s.rt.Rename(id, q.Get("title"))
	case "seen":
		err = s.rt.Seen(id)
	case "input":
		in, ok := readInput(w, r)
		if !ok {
			return
		}
		err = s.rt.InputSubmission(id, in.Submission, in.Text, in.Images...)
	case "merge":
		err = s.rt.Merge(id, q.Get("target"))
	case "selection":
		err = s.rt.Select(id, harness.Selection{Model: q.Get("model"), Effort: q.Get("effort")})
	case "interrupt":
		err = s.rt.Interrupt(id)
	case "restart":
		err = s.rt.Restart(id)
	case "answer":
		var body []byte
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		choices := q["choice"]
		if len(choices) == 1 && choices[0] == "" {
			choices = nil
		}
		err = s.rt.Answer(id, q.Get("request"), choices, string(body))
	case "start-proposal":
		_, err = s.rt.StartProposal(id, q.Get("proposal"))
	case "dismiss-proposal":
		err = s.rt.DismissProposal(id, q.Get("proposal"))
	case "allow":
		scope := q.Get("scope")
		if scope != "session" && scope != "all" {
			http.Error(w, "unknown scope", http.StatusBadRequest)
			return
		}
		err = s.rt.AlwaysAllow(id, q.Get("request"), scope == "all")
	default:
		http.Error(w, "unknown session action", http.StatusBadRequest)
		return
	}
	refused(w, err)
}

// startReview fetches a pull request's head commit, starts a worktree session
// at it with the review model and prompt, and answers like a new session.
// Failures before creation are plain text.
func (s *server) startReview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	number, err := strconv.Atoi(q.Get("number"))
	if err != nil || number <= 0 {
		http.Error(w, "a pull request number is required", http.StatusBadRequest)
		return
	}
	workspaceID := q.Get("workspace")
	var dir string
	err = s.db.Read(func(tx *store.Tx) error {
		ws, err := tx.Workspace(workspaceID)
		dir = ws.Path
		return err
	})
	if refused(w, err) {
		return
	}
	cfg, err := config.Load()
	if refused(w, err) {
		return
	}
	settings, err := reviewSettings(cfg)
	if refused(w, err) {
		return
	}
	commit, url, err := s.gh.FetchPullRequest(r.Context(), workspaceID, dir, number)
	if refused(w, err) {
		return
	}
	created, err := s.rt.CreateSubmission(workspaceID, settings, true, commit, "", reviewPrompt(cfg.ReviewPrompt, number, url))
	writeCreated(w, created.ID, err)
}

// writeCreated answers a session creation with the session's ID and the error,
// if any, as JSON. Creation can fail after saving the session, which then still
// belongs to the page; an empty ID means nothing was created.
func writeCreated(w http.ResponseWriter, id string, err error) {
	w.Header().Set("Content-Type", "application/json")
	result := map[string]string{"session": id}
	if err != nil {
		result["error"] = err.Error()
		w.WriteHeader(http.StatusConflict)
	}
	_ = json.NewEncoder(w).Encode(result)
}

// reviewSettings is the model a review starts with: the review selection, the
// default selection, or else the first listed model at its default effort.
func reviewSettings(cfg config.Config) (harness.Settings, error) {
	if cfg.ReviewSelection != nil {
		return *cfg.ReviewSelection, nil
	}
	if cfg.DefaultSelection != nil {
		return *cfg.DefaultSelection, nil
	}
	if len(cfg.Models) == 0 {
		return harness.Settings{}, errors.New("Choose a model in Settings to start a review")
	}
	m := cfg.Models[0]
	return harness.Settings{Harness: m.Harness, Model: m.Model, Effort: m.DefaultEffort}, nil
}

// reviewPrompt fills a review prompt's {number} and {url} with the pull
// request's number and link.
func reviewPrompt(prompt string, number int, url string) string {
	return strings.NewReplacer("{number}", strconv.Itoa(number), "{url}", url).Replace(prompt)
}

// grants returns saved always-allow choices.
func (s *server) grants(w http.ResponseWriter, r *http.Request) {
	var grants []store.Grant
	err := s.db.Read(func(tx *store.Tx) (err error) { grants, err = tx.Grants(); return })
	if refused(w, err) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if grants == nil {
		grants = []store.Grant{}
	}
	_ = json.NewEncoder(w).Encode(grants)
}

// deleteGrant removes one choice; approvals read grants from the database.
func (s *server) deleteGrant(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("harness") == "" || q.Get("tool") == "" {
		http.Error(w, "missing grant", http.StatusBadRequest)
		return
	}
	refused(w, s.db.Tx(func(tx *store.Tx) error { return tx.DeleteGrant(q.Get("session"), q.Get("harness"), q.Get("tool")) }))
}

// absolutePath cleans an absolute path or one that starts with ~/, which
// stands for the home folder.
func absolutePath(path string) (string, bool) {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		path = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(path) {
		return "", false
	}
	return filepath.Clean(path), true
}
