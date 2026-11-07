package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

// maxInput is the largest input body: 10 MiB of images in base64, and text.
const maxInput = 16 << 20

// workspaceAction adds a workspace in the given folder, or one the user picks,
// and returns its ID, or deletes a workspace.
func (s *server) workspaceAction(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch q.Get("action") {
	case "new":
		dir := q.Get("path")
		if dir == "" {
			// activate brings the picker in front of the browser.
			out, err := exec.Command("osascript", "-e", "activate", "-e", `POSIX path of (choose folder with prompt "Choose the workspace's folder:")`).Output()
			var exit *exec.ExitError
			if errors.As(err, &exit) && strings.Contains(string(exit.Stderr), "(-128)") {
				return // The user cancelled.
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			dir = strings.TrimSpace(string(out))
		}
		added, err := s.rt.AddWorkspace(dir)
		if refused(w, err) {
			return
		}
		fmt.Fprint(w, added.ID)
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
		created, err := s.rt.CreateSubmission(q.Get("workspace"), settings, q.Get("worktree") == "1", in.Submission, in.Text, in.Images...)
		w.Header().Set("Content-Type", "application/json")
		result := map[string]string{"session": created.ID}
		if err != nil {
			result["error"] = err.Error()
			w.WriteHeader(http.StatusConflict)
		}
		_ = json.NewEncoder(w).Encode(result)
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
