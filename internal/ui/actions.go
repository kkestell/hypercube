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
	Text   string          `json:"text"`
	Images []harness.Image `json:"images"`
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
	return in, true
}

// sessionAction creates a session in a workspace from its first prompt and
// returns the session ID, or starts, stops, deletes, renames, or marks
// seen a session.
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
		created, err := s.rt.Create(q.Get("workspace"), q.Get("profile"), q.Get("worktree") == "1", in.Text, in.Images...)
		if refused(w, err) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"session": created.ID})
		return
	case "start":
		err = s.rt.Start(id)
	case "stop":
		err = s.rt.Stop(id)
	case "delete":
		err = s.rt.Delete(id)
	case "rename":
		err = s.rt.Rename(id, q.Get("title"))
	case "seen":
		err = s.rt.Seen(id)
	case "input":
		in, ok := readInput(w, r)
		if !ok {
			return
		}
		err = s.rt.Input(id, in.Text, in.Images...)
	case "interrupt":
		err = s.rt.Interrupt(id)
	case "answer":
		var body []byte
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		err = s.rt.Answer(id, q.Get("request"), q.Get("choice"), string(body))
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
