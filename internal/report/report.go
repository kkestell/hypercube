// Package report handles reports from a harness plugin about its agent's
// conversation, and answers with text for the model.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/tmux"
)

type input struct {
	Session string          `json:"session"`
	Error   string          `json:"error"`
	Tool    string          `json:"tool"`
	Input   json.RawMessage `json:"input"`
	Reason  string          `json:"reason"`
	Text    string          `json:"text"`
	store.Usage
}

// response holds text for the model to read now. At stop, it continues the
// turn.
type response struct {
	Context string `json:"context,omitempty"`
}

// Run handles one report. Reports from outside a Hypercube agent get an empty
// response and change nothing.
func Run(event string, stdin io.Reader, stdout io.Writer) error {
	id, path := os.Getenv("HYPERCUBE_AGENT_ID"), os.Getenv("HYPERCUBE_STATE")
	if id == "" || path == "" {
		return json.NewEncoder(stdout).Encode(response{})
	}
	var in input
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return err
	}
	if event == "toolfailed" || event == "denied" {
		if err := logTool(event, path, id, in); err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(response{})
	}
	// The status line reports answered and usage on every redraw, so skip the
	// lock when nothing changes.
	if event == "answered" || event == "usage" {
		s, err := store.Load(path)
		if err != nil {
			return err
		}
		if a, err := s.Agent(id); err != nil || event == "answered" && !a.Attention || event == "usage" && a.Usage == in.Usage {
			return json.NewEncoder(stdout).Encode(response{})
		}
	}

	err := store.Update(path, func(s *store.State) error {
		a, err := s.Agent(id)
		if err != nil {
			return err
		}
		var resp response
		switch event {
		case "start":
			var profiles []harness.Profile
			if a.Role == store.Lead {
				cfg, err := harness.LoadConfig()
				if err != nil {
					return err
				}
				profiles = cfg.Workers()
			}
			resp.Context = facts(s, a, profiles)
		case "prompt":
			if lead := s.TeamLead(a.Team); lead != nil {
				lead.AwaitingUser = false
			}
			// A harness may save no conversation until the first prompt, so
			// a session ID seen earlier may not be resumable.
			if in.Session != "" {
				a.SessionID = in.Session
			}
			// A turn that delivered messages starts after Wait has marked the
			// agent busy, so only a prompt typed while idle titles the session.
			if a.Role == store.Lead && a.Title == "" && a.Activity != store.Busy {
				if title := title(in.Text); title != "" {
					a.Title = title
					// The window name is cosmetic; the saved title is what counts.
					if window, err := tmux.FindWindow(store.Workspace(path), a.Team, path); err == nil && window != "" {
						_ = tmux.RenameWindow(window, title)
					}
				}
			}
			a.Activity = store.Busy
			resp.Context = Format(s, s.Take(id))
		case "tool":
			resp.Context = Format(s, s.Take(id))
		case "stop":
			resp.Context = Format(s, s.Take(id))
			if resp.Context == "" {
				a.Activity = store.Idle
			}
		case "failed":
			if a.Role == store.Worker {
				if lead := s.TeamLead(a.Team); lead != nil {
					s.Messages = append(s.Messages, store.Message{From: id, To: lead.ID, Text: failure(in.Error)})
				}
			}
			a.Activity = store.Idle
		case "approval":
			// An empty response leaves the decision to the harness's own
			// dialog.
			a.Attention = true
		case "answered":
			a.Attention = false
		case "usage":
			a.Usage = in.Usage
		default:
			return fmt.Errorf("unknown report %q", event)
		}
		// Writing the response inside the update keeps taken messages queued
		// if the plugin cannot read them.
		return json.NewEncoder(stdout).Encode(resp)
	})
	if err == nil {
		tmux.RefreshActivity(path, id)
	}
	return err
}

// maxTitle is the longest session title, in characters.
const maxTitle = 200

// title makes a session title from a prompt: its first line with text, with
// runs of whitespace collapsed and cut to maxTitle characters.
func title(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		if chars := []rune(line); len(chars) > maxTitle {
			line = string(chars[:maxTitle-1]) + "…"
		}
		return line
	}
	return ""
}

// logTool appends a failed Hypercube tool call or an auto mode denial to
// tools.log beside the state file, as a JSON line for tuning tool descriptions
// and allow rules.
func logTool(event, path, id string, in input) error {
	s, err := store.Load(path)
	if err != nil {
		return err
	}
	a, err := s.Agent(id)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(filepath.Dir(path), "tools.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	msg, detail := "tool failed", slog.String("error", in.Error)
	if event == "denied" {
		msg, detail = "tool denied", slog.String("reason", in.Reason)
	}
	slog.New(slog.NewJSONHandler(f, nil)).Info(msg,
		"workspace", store.Workspace(path), "session", a.Team, "agent", a.Name,
		"harness", a.Harness, "model", a.Model, "conversation", a.SessionID, "dir", a.Dir,
		"tool", in.Tool, "input", in.Input, detail)
	return f.Close()
}

// Format renders messages as the model sees them.
// Senders cannot see the recipient's chat, so agent messages say how to reply.
func Format(s *store.State, messages []store.Message) string {
	var parts []string
	for _, m := range messages {
		if a, err := s.Agent(m.From); err == nil {
			parts = append(parts, fmt.Sprintf("Message from %s:\n%s\n\n%s cannot see your chat. To answer, use the hypercube send_message tool to %s.", a.Address(), m.Text, a.Address(), a.Address()))
		} else {
			parts = append(parts, fmt.Sprintf("Message from %s:\n%s", m.From, m.Text))
		}
	}
	return strings.Join(parts, "\n\n")
}

// facts describes the agent's place in the team. The lead also gets
// the configured worker profiles.
func facts(s *store.State, a *store.Agent, profiles []harness.Profile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, the %s in a Hypercube team of coding agents.\n", a.Address(), a.Role)
	fmt.Fprintf(&b, "Your Hypercube session is %q, titled %q. Agents are addressed as session/name, where session is the session's ID; a bare name means an agent in your session.\n", a.Team, s.Title(a.Team))
	if a.Role == store.Lead {
		b.WriteString("Worker tools apply only to this session. You can also message the leads of other sessions.\n")
	} else {
		b.WriteString("Messaging applies only to this session.\n")
	}
	b.WriteString("Reach Hypercube agents only with the hypercube tools. Any other agent tools you have manage only your own subagents. Messages from Hypercube agents arrive in your conversation by themselves, so end your turn instead of waiting for them.\n")
	fmt.Fprintf(&b, "Your working directory is %s%s.\n\nTeam:\n", a.Dir, onBranch(a.Branch))
	var leads []string
	for _, t := range s.Agents {
		if t.Team == a.Team {
			fmt.Fprintf(&b, "- %s: %s, %s, %s%s\n", t.Address(), t.Role, t.Harness, t.Dir, onBranch(t.Branch))
		} else if t.Role == store.Lead {
			leads = append(leads, fmt.Sprintf("%s (%s)", t.Address(), s.Title(t.Team)))
		}
	}
	if a.Role == store.Lead {
		b.WriteString("\nOther Hypercube sessions in this workspace share your original checkout, files, Git index, and branch. Coordinate shared changes with their leads and the user; use worker worktrees for independent edits. list_agents shows the current leads.\n")
		if len(leads) > 0 {
			fmt.Fprintf(&b, "Other leads: %s\n", strings.Join(leads, ", "))
		}
	}
	switch {
	case a.Role == store.Lead && s.NoGit:
		b.WriteString(`
Your working directory is not a Git checkout. Each worker works directly in the directory you give it, usually its own Git checkout. Tell each worker how to handle branches and commits in its directory. Workers given the same directory share one checkout. Remove a worker when its work is done. You decide how to divide the work, coordinate the workers, and judge when it is done. The user talks to you in this session.
`)
		writeTeamwork(&b, profiles)
	case a.Role == store.Lead:
		b.WriteString(`
You work in the original checkout. Unless the user says otherwise, give a worker its own worktree, by omitting dir, when it will change files. It commits on its own branch in its worktree. Merge its branch into your branch from your checkout, then remove the worker when its work is merged. A worker that will not change files, such as one doing research or review, needs no worktree: pass dir "." to run it in your checkout, and remove it when it is done. When the user asks for changes made directly in a directory, pass that directory, and tell the worker how to handle branches and commits there. Workers given the same directory share one checkout. You decide how to divide the work, coordinate the workers, and judge when it is done. The user talks to you in this session.
`)
		writeTeamwork(&b, profiles)
	case a.Branch != "":
		b.WriteString(`
Commit your work on your branch. The lead merges it from its checkout.` + reportBack)
	default:
		b.WriteString(`
You work directly in your working directory. Handle branches and commits as the lead tells you.` + reportBack)
	}
	return b.String()
}

func onBranch(branch string) string {
	if branch == "" {
		return ""
	}
	return " on branch " + branch
}

const reportBack = ` Use the hypercube set_status tool often to keep the user up to date: when you start, when you move to a new step, and when you find something that changes the plan, say in a sentence or two what you are doing now. When you finish, or need something, send the lead a short summary: what you did, the result, and anything unresolved. Leave details in your commits or files. A message you send is the only way other agents see your work's status.
`

// writeTeamwork writes the lead's guidance on parallel work and the
// configured worker profiles.
func writeTeamwork(b *strings.Builder, profiles []harness.Profile) {
	b.WriteString(`
Teamwork:
- Do small or tightly coupled tasks yourself. Create workers when the work splits into long, self-contained parts. A team uses several times the tokens of a single agent.
- Run independent work in parallel and coupled work in sequence. Give each file that several tasks touch to one worker.
- A worker's first message states the objective, the expected result, the files and sources to start from, and the boundaries: what to leave alone and when to stop and report.
- Choose the cheapest profile that fits the task. When a worker's result fails its checks, give the task to a stronger profile.
- Before you merge a change, run the build and tests. A non-trivial change also needs a review by a worker that did not write it. A trivial change needs none.
- Ask workers for conclusions, not transcripts, to keep your context for coordination.

Worker profiles. The role says which work each profile suits: plan, work, or review. Pass the name of the one that fits the task to create_agent as profile:
`)
	for _, p := range profiles {
		fmt.Fprintf(b, "- %s (%s; %s, %s, %s): %s\n", p.Name, p.Role, p.Harness, p.Model, p.Effort, p.Description)
	}
}

func failure(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return "My turn failed."
	}
	chars := []rune(detail)
	if len(chars) > 2000 {
		detail = string(chars[:2000]) + "…"
	}
	return "My turn failed: " + detail
}
