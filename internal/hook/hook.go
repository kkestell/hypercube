// Package hook handles harness hook events. Claude Code and
// the OpenCode plugin share the response format.
package hook

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
	SessionID            string          `json:"session_id"`
	AgentID              *string         `json:"agent_id"`
	Error                string          `json:"error"`
	ErrorDetails         string          `json:"error_details"`
	LastAssistantMessage string          `json:"last_assistant_message"`
	ToolName             string          `json:"tool_name"`
	ToolInput            json.RawMessage `json:"tool_input"`
	Reason               string          `json:"reason"`
}

type specific struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

type response struct {
	Decision           string    `json:"decision,omitempty"`
	Reason             string    `json:"reason,omitempty"`
	HookSpecificOutput *specific `json:"hookSpecificOutput,omitempty"`
}

// Run handles one hook event. Processes outside a Hypercube agent do nothing.
func Run(event string, stdin io.Reader, stdout io.Writer) error {
	id, path := os.Getenv("HYPERCUBE_AGENT_ID"), os.Getenv("HYPERCUBE_STATE")
	if id == "" || path == "" {
		return nil
	}
	var in input
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return err
	}

	if event == "PostToolUseFailure" || event == "PermissionDenied" {
		return logTool(event, path, id, in)
	}
	if in.AgentID != nil && event != "PermissionRequest" {
		return nil
	}

	var resp *response
	err := store.Update(path, func(s *store.State) error {
		a, err := s.Agent(id)
		if err != nil {
			return err
		}
		switch event {
		case "SessionStart":
			var profiles []harness.Profile
			if a.Role == store.Lead {
				cfg, err := harness.LoadConfig()
				if err != nil {
					return err
				}
				profiles = cfg.Profiles
			}
			resp = context(event, facts(s, a, profiles))
		case "UserPromptSubmit":
			// Claude Code saves no conversation until the first prompt, so a
			// session ID recorded earlier may not be resumable.
			if in.SessionID != "" {
				a.SessionID = in.SessionID
			}
			a.Activity = store.Busy
			resp = context(event, Format(s, s.Take(id)))
		case "PostToolUse":
			resp = context(event, Format(s, s.Take(id)))
		case "PermissionRequest":
			a.Attention = true
		case "StopFailure":
			if a.Role == store.Worker {
				if lead := s.TeamLead(a.Team); lead != nil {
					s.Messages = append(s.Messages, store.Message{From: id, To: lead.ID, Text: failure(in)})
				}
			}
			a.Activity = store.Idle
		case "Stop":
			if text := Format(s, s.Take(id)); text != "" {
				resp = &response{Decision: "block", Reason: text}
			} else {
				a.Activity = store.Idle
			}
		default:
			return fmt.Errorf("unknown hook event %q", event)
		}
		// Writing the response inside the update keeps taken messages queued
		// if the harness cannot read them.
		if resp == nil {
			return nil
		}
		return json.NewEncoder(stdout).Encode(resp)
	})
	if err == nil {
		tmux.RefreshActivity(path, id)
	}
	return err
}

// logTool appends failed Hypercube tool calls and auto mode denials of any
// tool to tools.log beside the state file, as JSON lines for tuning tool
// descriptions and allow rules. Native subagents' calls are logged too.
func logTool(event, path, id string, in input) error {
	if event == "PostToolUseFailure" && !hypercubeTool(in.ToolName) {
		return nil
	}
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
	defer f.Close()
	msg, detail := "tool failed", slog.String("error", in.Error)
	if event == "PermissionDenied" {
		msg, detail = "tool denied", slog.String("reason", in.Reason)
	}
	slog.New(slog.NewJSONHandler(f, nil)).Info(msg,
		"workspace", store.Workspace(path), "session", a.Team, "agent", a.Name,
		"harness", a.Harness, "model", a.Model, "conversation", in.SessionID, "dir", a.Dir,
		"tool", in.ToolName, "input", in.ToolInput, detail)
	return f.Close()
}

// hypercubeTool reports whether a harness tool name is one of Hypercube's MCP
// tools: mcp__hypercube__send_message in Claude Code, hypercube_send_message
// in OpenCode.
func hypercubeTool(name string) bool {
	return strings.HasPrefix(name, "mcp__hypercube__") || strings.HasPrefix(name, "hypercube_")
}

func context(event, text string) *response {
	if text == "" {
		return nil
	}
	return &response{HookSpecificOutput: &specific{HookEventName: event, AdditionalContext: text}}
}

// format renders messages as the model sees them.
// Senders cannot see the recipient's chat, so agent messages say how to reply.
func Format(s *store.State, messages []store.Message) string {
	var parts []string
	for _, m := range messages {
		if a, err := s.Agent(m.From); err == nil {
			parts = append(parts, fmt.Sprintf("Message from %s:\n%s\n\n%s cannot see your chat. To answer, use send_message to %s.", a.Address(), m.Text, a.Address(), a.Address()))
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
	fmt.Fprintf(&b, "Your Hypercube session is %q. Agents are addressed as session/name; a bare name means an agent in your session.\n", a.Team)
	if a.Role == store.Lead {
		b.WriteString("Worker tools apply only to this session. You can also message the leads of other sessions.\n")
	} else {
		b.WriteString("Messaging applies only to this session.\n")
	}
	fmt.Fprintf(&b, "Your working directory is %s%s.\n\nTeam:\n", a.Dir, onBranch(a.Branch))
	var leads []string
	for _, t := range s.Agents {
		if t.Team == a.Team {
			fmt.Fprintf(&b, "- %s: %s, %s, %s%s\n", t.Address(), t.Role, t.Harness, t.Dir, onBranch(t.Branch))
		} else if t.Role == store.Lead {
			leads = append(leads, t.Address())
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

const reportBack = ` When you finish, or need something, send the lead a short summary: what you did, the result, and anything unresolved. Leave details in your commits or files. A message you send is the only way other agents see your work's status.
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

Worker profiles. Pass the name of the one that fits the task to create_agent as profile:
`)
	for _, p := range profiles {
		fmt.Fprintf(b, "- %s (%s, %s, %s): %s\n", p.Name, p.Harness, p.Model, p.Effort, p.Description)
	}
}

func failure(in input) string {
	detail := ""
	for _, text := range []string{in.ErrorDetails, in.LastAssistantMessage, in.Error} {
		if text = strings.TrimSpace(text); text != "" {
			detail = text
			break
		}
	}
	if detail == "" {
		return "My turn failed."
	}
	chars := []rune(detail)
	if len(chars) > 2000 {
		detail = string(chars[:2000]) + "…"
	}
	return "My turn failed: " + detail
}
