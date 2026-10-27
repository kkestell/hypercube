// Package hook handles harness hook events. Claude Code and Codex share the
// response format, which Ox also reads, and the OpenCode plugin reads the same
// format.
package hook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

type input struct {
	SessionID string `json:"session_id"`
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

	var resp *response
	err := store.Update(path, func(s *store.State) error {
		a, err := s.Agent(id)
		if err != nil {
			return err
		}
		switch event {
		case "SessionStart":
			var profiles []harness.Profile
			if a.Role == store.Orchestrator {
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
			resp = context(event, format(s, s.Take(id)))
		case "PostToolUse":
			resp = context(event, format(s, s.Take(id)))
		case "Stop":
			if text := format(s, s.Take(id)); text != "" {
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
	return err
}

func context(event, text string) *response {
	if text == "" {
		return nil
	}
	return &response{HookSpecificOutput: &specific{HookEventName: event, AdditionalContext: text}}
}

// format renders messages as the model sees them.
func format(s *store.State, messages []store.Message) string {
	var parts []string
	for _, m := range messages {
		name := m.From
		if a, err := s.Agent(m.From); err == nil {
			name = a.Name
		}
		parts = append(parts, fmt.Sprintf("Message from %s (%s):\n%s", name, m.From, m.Text))
	}
	return strings.Join(parts, "\n\n")
}

// facts describes the agent's place in the team. The orchestrator also gets
// the configured worker profiles.
func facts(s *store.State, a *store.Agent, profiles []harness.Profile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s (%s), the %s in a Hypercube team of coding agents.\n", a.Name, a.ID, a.Role)
	fmt.Fprintf(&b, "Your working directory is %s%s.\n\nTeam:\n", a.Dir, onBranch(a.Branch))
	for _, t := range s.Agents {
		fmt.Fprintf(&b, "- %s (%s): %s, %s, %s%s\n", t.Name, t.ID, t.Role, t.Harness, t.Dir, onBranch(t.Branch))
	}
	b.WriteString(messageTools)
	if s.NoGit {
		b.WriteString(`- create_agent(name, profile, dir, message?) creates a worker that works directly in dir, ` + dirArg + ` Its profile is one of the worker profiles below. The optional message is delivered on the worker's first turn. Orchestrator only.
- remove_agent(id) closes a worker's pane and removes it from the team. Its directory is left as it is. Orchestrator only.
`)
	} else {
		b.WriteString(`- create_agent(name, profile, dir?, message?) creates a worker. Without dir, the worker gets its own branch and worktree from the orchestrator's HEAD, with submodules checked out and copies of the gitignored paths the checkout's .worktreeinclude lists. With dir, ` + dirArg + ` the worker works directly in it, and Hypercube creates no branch or worktree. Its profile is one of the worker profiles below. The optional message is delivered on the worker's first turn. Orchestrator only.
- remove_agent(id) closes a worker's pane and removes it from the team. A worker with a worktree also has its worktree and branch removed, and Git refuses dirty worktrees and unmerged branches. A worker without one has its directory left as it is. Orchestrator only.
`)
	}
	switch {
	case a.Role == store.Orchestrator && s.NoGit:
		b.WriteString(`
Your working directory is not a Git checkout. Each worker works directly in the directory you give it, usually its own Git checkout. Tell each worker how to handle branches and commits in its directory. Workers given the same directory share one checkout. Remove a worker when its work is done. You decide how to divide the work, coordinate the workers, and judge when it is done. The user talks to you in this session.
`)
		writeTeamwork(&b, profiles)
	case a.Role == store.Orchestrator:
		b.WriteString(`
You work in the original checkout. Unless the user says otherwise, give a worker its own worktree, by omitting dir, when it will change files. It commits on its own branch in its worktree. Merge its branch into your branch from your checkout, then remove the worker when its work is merged. A worker that will not change files, such as one doing research or review, needs no worktree: pass dir "." to run it in your checkout, and remove it when it is done. When the user asks for changes made directly in a directory, pass that directory, and tell the worker how to handle branches and commits there. Workers given the same directory share one checkout. You decide how to divide the work, coordinate the workers, and judge when it is done. The user talks to you in this session.
`)
		writeTeamwork(&b, profiles)
	case a.Branch != "":
		b.WriteString(`
Commit your work on your branch. The orchestrator merges it from its checkout.` + reportBack)
	default:
		b.WriteString(`
You work directly in your working directory. Handle branches and commits as the orchestrator tells you.` + reportBack)
	}
	return b.String()
}

func onBranch(branch string) string {
	if branch == "" {
		return ""
	}
	return " on branch " + branch
}

const dirArg = "an existing directory given as an absolute path, a path starting with ~/, or a path relative to the orchestrator's working directory,"

const reportBack = ` When you finish, or need something, send a message to the orchestrator. A message you send is the only way other agents see your work's status.
`

const messageTools = `
Hypercube MCP tools:
- send_message(to, text) sends a message to an agent by ID or name. Messages arrive in your context as "Message from <name> (<id>):".
- list_agents() returns the team with each agent's activity.
`

// writeTeamwork writes the orchestrator's guidance on parallel work and the
// configured worker profiles.
func writeTeamwork(b *strings.Builder, profiles []harness.Profile) {
	b.WriteString(`
Run tasks that change the same subsystem one after another; run only independent work in parallel.

Worker profiles. Pass the name of the one that fits the task to create_agent as profile:
`)
	for _, p := range profiles {
		fmt.Fprintf(b, "- %s (%s, %s, %s): %s\n", p.Name, p.Harness, p.Model, p.Effort, p.Description)
	}
}
