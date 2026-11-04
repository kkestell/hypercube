// Package report writes the text agents and the user read: the instructions
// that start an agent, attribution of queued messages, session titles, and
// failure reports.
package report

import (
	"fmt"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// maxTitle is the longest session title, in characters.
const maxTitle = 200

// Title makes a session title from its first prompt: the first line with text,
// with runs of whitespace collapsed and cut to maxTitle characters. A prompt
// without text is titled "Image".
func Title(prompt string) string {
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
	return "Image"
}

// Format renders a message as the model sees it, including how to reply.
func Format(tx *store.Tx, m store.Message) string {
	from := m.From
	s, err := tx.Session(m.From)
	if err != nil {
		return fmt.Sprintf("Message from %s:\n%s", m.From, m.Text)
	}
	if s.Parent == m.To {
		from = s.Name
	}
	sender := from
	if s.Parent == "" {
		if recipient, err := tx.Session(m.To); err == nil && recipient.Parent == "" {
			if w, err := tx.Workspace(s.Workspace); err == nil {
				sender = fmt.Sprintf("%s (%q, workspace %s)", s.ID, s.Title, w.Path)
			}
		}
	}
	return fmt.Sprintf("Message from %s:\n%s\n\n%s cannot see your chat. To answer, use the hypercube send_message tool to %s.", sender, m.Text, from, from)
}

// Instructions describe the session's place and tools.
func Instructions(tx *store.Tx, s store.Session, profiles []harness.Profile) (string, error) {
	var b strings.Builder
	if s.Parent != "" {
		parent, err := tx.Session(s.Parent)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "You are %s, a child session of Hypercube session %s, titled %q. Your parent created you and is the only session you can message: use send_message to %s.\n", s.Name, parent.ID, parent.Title, parent.ID)
	} else {
		fmt.Fprintf(&b, "You are the agent of Hypercube session %s, titled %q. Hypercube runs one coding agent per session.\n", s.ID, s.Title)
		b.WriteString("You can create child sessions with create_session to work in parallel, and address each by its name. Children can message only you. You end them with end_session. You can also message any other running top-level session across all workspaces in this app by its ID. Stopped and ended recipients are refused. Use list_sessions to find running addressable recipients, including their workspace paths and parent IDs.\n")
		b.WriteString("At the start of the session, call set_title with a short, meaningful title for the user. Until you do, the title is taken from the user's first message.\n")
	}
	b.WriteString("Do not use built-in subagent tools. Delegate work only to Hypercube workers (child sessions), managed through the hypercube tools. Messages from Hypercube sessions arrive in your conversation by themselves, so end your turn instead of waiting for them.\n")
	fmt.Fprintf(&b, "Your working directory is %s%s.\n", s.Dir, onBranch(s.Branch))
	if s.Parent != "" {
		if s.Branch != "" {
			b.WriteString("Commit your work on your branch. Your parent merges it from its checkout.")
		} else {
			b.WriteString("You work directly in your working directory. Handle branches and commits as your parent tells you.")
		}
		b.WriteString(reportBack)
		return b.String(), nil
	}
	children, err := tx.Children(s.ID)
	if err != nil {
		return "", err
	}
	b.WriteString("\nChildren:\n")
	if len(children) == 0 {
		b.WriteString("none yet\n")
	}
	for _, c := range children {
		ended := ""
		if c.Ended {
			ended = ", ended"
		}
		fmt.Fprintf(&b, "- %s (%s): %s, %s%s%s\n", c.Name, c.Title, c.Harness, c.Dir, onBranch(c.Branch), ended)
	}
	w, err := tx.Workspace(s.Workspace)
	if err != nil {
		return "", err
	}
	if s.Base == "" {
		b.WriteString("\nOther Hypercube top-level sessions in this workspace that run in the original checkout share your checkout, files, Git index, and branch. Coordinate shared changes with their agents and the user; use child session worktrees for independent edits.\n")
	}
	if w.NoGit {
		b.WriteString(`
Your working directory is not a Git checkout. Each child session works directly in the directory you give it, usually its own Git checkout. Tell each child session how to handle branches and commits in its directory. Child sessions given the same directory share one checkout. End a child session when its work is done. You decide how to divide the work, coordinate the children, and judge when it is done. The user talks to you in this session.
`)
	} else {
		if s.Base == "" {
			b.WriteString("\nYou work in the original checkout.")
		} else {
			fmt.Fprintf(&b, "\nYou work in your own worktree on branch %s, made from the original checkout at %s. Your child sessions' branches merge into your branch. Your work reaches the original checkout's branch when the user asks you to merge it.", s.Branch, w.Path)
		}
		b.WriteString(` Do the work yourself unless it splits into long, independent parts that gain from running in parallel, or the user asks for a team. A child session that changes files alongside other work gets its own worktree: omit dir. Any other child session runs in your checkout: pass dir ".". A child commits on its own branch. Merge child branches, and commit in your checkout, only when the user asks. A merge needs a clean checkout, so commit your own changes before merging a child's branch, and merge it with git merge. When a merge is blocked, resolve the cause. A child session with a worktree ends once its branch is merged into HEAD. End a child session when its work is merged or no longer needed. When the user asks for changes made directly in a directory, pass that directory, and tell the child session how to handle branches and commits there. Child sessions given the same directory share one checkout. The user talks to you in this session.
`)
	}
	writeTeamwork(&b, profiles)
	return b.String(), nil
}

func onBranch(branch string) string {
	if branch == "" {
		return ""
	}
	return " on branch " + branch
}

const reportBack = ` When you finish, or need something, send your parent a short summary: what you did, the result, and anything unresolved. Leave details in your commits or files.
`

// writeTeamwork writes the top-level session's guidance on parallel work and the
// configured profiles.
func writeTeamwork(b *strings.Builder, profiles []harness.Profile) {
	b.WriteString(`
Teamwork:
- Do small and tightly coupled work yourself. A child session costs a fresh context, and often a worktree and a build. A team uses several times the tokens of one agent.
- Run independent work in parallel and coupled work in sequence. Give each file that several tasks touch to one child session.
- A child session's first message states the objective, the expected result, where to start, and the boundaries: what to leave alone and when to stop and report.
- The user's choices of harness, model, or budget override the profile descriptions. Otherwise choose the cheapest profile that fits, and move to a stronger one when a result fails its checks.
- Match checks to the change. Run the build and tests before reporting work as done. A substantial change gets one review from a session that did not write it. A small change gets none.
- Ask child sessions for conclusions, not transcripts.

Profiles. Choose the profile whose description fits the task, and pass its name to create_session as profile:
`)
	for _, p := range profiles {
		fmt.Fprintf(b, "- %s (%s, %s, %s): %s\n", p.Name, p.Harness, p.Model, p.Effort, p.Description)
	}
}

// FailureMessage describes a child session's failed turn for its parent.
func FailureMessage(detail string) string {
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
