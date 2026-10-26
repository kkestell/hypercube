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
			var models string
			if a.Role == store.Orchestrator {
				cfg, err := harness.LoadConfig()
				if err != nil {
					return err
				}
				models = cfg.Models
			}
			resp = context(event, facts(s, a, models))
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
		return nil
	})
	if err != nil || resp == nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(resp)
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
// the configured models text.
func facts(s *store.State, a *store.Agent, models string) string {
	if s.NoGit {
		return dirFacts(s, a, models)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s (%s), the %s in a Hypercube team of coding agents.\n", a.Name, a.ID, a.Role)
	fmt.Fprintf(&b, "Your working directory is %s, on branch %s.\n\nTeam:\n", a.Dir, a.Branch)
	for _, t := range s.Agents {
		fmt.Fprintf(&b, "- %s (%s): %s, %s, %s on %s\n", t.Name, t.ID, t.Role, t.Harness, t.Dir, t.Branch)
	}
	b.WriteString(messageTools + `- create_agent(name, harness?, model?, effort?, message?) creates a worker with its own branch and worktree from the orchestrator's HEAD. The optional message is delivered on the worker's first turn. Orchestrator only.
- remove_agent(id) removes a worker's worktree, branch, and pane. Git refuses dirty worktrees and unmerged branches. Orchestrator only.
`)
	if a.Role == store.Orchestrator {
		b.WriteString(`
You work in the original checkout. Each worker commits on its own branch in its own worktree. Merge a worker's branch into your branch from your checkout, then remove the worker when its work is merged. You decide how to divide the work, coordinate the workers, and judge when it is done. The user talks to you in this session.
`)
		writeTeamwork(&b, models)
	} else {
		b.WriteString(`
Commit your work on your branch. The orchestrator merges it from its checkout. When you finish, or need something, send a message to the orchestrator. A message you send is the only way other agents see your work's status.
`)
	}
	return b.String()
}

// dirFacts is facts for a workspace outside Git, whose workers work directly
// in existing directories.
func dirFacts(s *store.State, a *store.Agent, models string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s (%s), the %s in a Hypercube team of coding agents.\n", a.Name, a.ID, a.Role)
	fmt.Fprintf(&b, "Your working directory is %s.\n\nTeam:\n", a.Dir)
	for _, t := range s.Agents {
		fmt.Fprintf(&b, "- %s (%s): %s, %s, %s\n", t.Name, t.ID, t.Role, t.Harness, t.Dir)
	}
	b.WriteString(messageTools + `- create_agent(name, dir, harness?, model?, effort?, message?) creates a worker that works directly in dir, an existing directory given as an absolute path, a path starting with ~/, or a path relative to the orchestrator's working directory. Hypercube creates no branch or worktree. The optional message is delivered on the worker's first turn. Orchestrator only.
- remove_agent(id) closes a worker's pane and removes it from the team. Its directory is left as it is. Orchestrator only.
`)
	if a.Role == store.Orchestrator {
		b.WriteString(`
Your working directory is not a Git checkout. Each worker works directly in the directory you give it, usually its own Git checkout, and Hypercube creates no branches or worktrees. Tell each worker how to handle branches and commits in its directory. Workers given the same directory share one checkout. Remove a worker when its work is done. You decide how to divide the work, coordinate the workers, and judge when it is done. The user talks to you in this session.
`)
		writeTeamwork(&b, models)
	} else {
		b.WriteString(`
You work directly in your working directory. Hypercube created no branch or worktree for you. Handle branches and commits as the orchestrator tells you. When you finish, or need something, send a message to the orchestrator. A message you send is the only way other agents see your work's status.
`)
	}
	return b.String()
}

const messageTools = `
Hypercube MCP tools:
- send_message(to, text) sends a message to an agent by ID or name. Messages arrive in your context as "Message from <name> (<id>):".
- list_agents() returns the team with each agent's activity.
`

// writeTeamwork writes the orchestrator's guidance on parallel work and the
// configured models text.
func writeTeamwork(b *strings.Builder, models string) {
	b.WriteString(`
Run tasks that change the same subsystem one after another; run only independent work in parallel.
`)
	if models != "" {
		b.WriteString("\nModels for workers. Pass the harness, model, and effort of the one that fits the task to create_agent:\n")
		b.WriteString(strings.TrimRight(models, "\n") + "\n")
	}
}
