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
	return fmt.Sprintf("Message from %s:\n%s\n\n%s cannot see your chat. To answer, use the hypercube send_message tool to %s.", from, m.Text, from, from)
}

// Peers returns the workspace's other top-level sessions.
func Peers(tx *store.Tx, workspace, except string) ([]store.Session, error) {
	sessions, err := tx.Sessions(workspace)
	if err != nil {
		return nil, err
	}
	var peers []store.Session
	for _, s := range sessions {
		if s.ID != except {
			peers = append(peers, s)
		}
	}
	return peers, nil
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
		b.WriteString("You can create child sessions with create_session to work in parallel, and address each by its name. Children can message only you. You end them with end_session. You can also message the other top-level sessions in this workspace by their IDs, and list_sessions lists the current ones.\n")
	}
	b.WriteString("Reach Hypercube sessions only with the hypercube tools. Any other agent tools you have manage only your own subagents. Messages from Hypercube sessions arrive in your conversation by themselves, so end your turn instead of waiting for them.\n")
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
		fmt.Fprintf(&b, "- %s: %s, %s%s%s\n", c.Name, c.Harness, c.Dir, onBranch(c.Branch), ended)
	}
	peers, err := Peers(tx, s.Workspace, s.ID)
	if err != nil {
		return "", err
	}
	var names []string
	for _, p := range peers {
		names = append(names, fmt.Sprintf("%s (%s)", p.ID, p.Title))
	}
	if len(names) == 0 {
		names = append(names, "none yet")
	}
	fmt.Fprintf(&b, "Other top-level sessions: %s\n", strings.Join(names, ", "))
	b.WriteString("\nOther Hypercube top-level sessions in this workspace share your original checkout, files, Git index, and branch. Coordinate shared changes with their agents and the user; use child session worktrees for independent edits. list_sessions shows the current sessions.\n")
	w, err := tx.Workspace(s.Workspace)
	if err != nil {
		return "", err
	}
	if w.NoGit {
		b.WriteString(`
Your working directory is not a Git checkout. Each child session works directly in the directory you give it, usually its own Git checkout. Tell each child session how to handle branches and commits in its directory. Child sessions given the same directory share one checkout. End a child session when its work is done. You decide how to divide the work, coordinate the children, and judge when it is done. The user talks to you in this session.
`)
	} else {
		b.WriteString(`
You work in the original checkout. Unless the user says otherwise, give a child session its own worktree, by omitting dir, when it will change files. It commits on its own branch in its worktree. Merge its branch into your branch from your checkout, then end the child session when its work is merged. A child session that will not change files, such as one doing research or review, needs no worktree: pass dir "." to run it in your checkout, and end it when it is done. When the user asks for changes made directly in a directory, pass that directory, and tell the child session how to handle branches and commits there. Child sessions given the same directory share one checkout. You decide how to divide the work, coordinate the children, and judge when it is done. The user talks to you in this session.
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

const reportBack = ` Use the hypercube set_status tool often to keep the user up to date: when you start, when you move to a new step, and when you find something that changes the plan, say in a sentence or two what you are doing now. When you finish, or need something, send your parent a short summary: what you did, the result, and anything unresolved. Leave details in your commits or files. set_status reaches the user in your parent's session, not your parent's agent. A message you send is the only way your parent's agent sees your work's status.
`

// writeTeamwork writes the top-level session's guidance on parallel work and the
// configured child session profiles.
func writeTeamwork(b *strings.Builder, profiles []harness.Profile) {
	b.WriteString(`
Teamwork:
- Do small or tightly coupled tasks yourself. Create child sessions when the work splits into long, self-contained parts. A team uses several times the tokens of a single agent.
- Run independent work in parallel and coupled work in sequence. Give each file that several tasks touch to one child session.
- A child session's first message states the objective, the expected result, the files and sources to start from, and the boundaries: what to leave alone and when to stop and report.
- Choose the cheapest profile that fits the task. When a child session's result fails its checks, give the task to a stronger profile.
- Before you merge a change, run the build and tests. A non-trivial change also needs a review by a child session that did not write it. A trivial change needs none.
- Ask child sessions for conclusions, not transcripts, to keep your context for coordination.

Child profiles. The role says which work each profile suits: plan, work, or review. Pass the name of the one that fits the task to create_session as profile:
`)
	for _, p := range profiles {
		fmt.Fprintf(b, "- %s (%s; %s, %s, %s): %s\n", p.Name, p.Role, p.Harness, p.Model, p.Effort, p.Description)
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
