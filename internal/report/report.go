// Package report writes the text agents and the user read: the instructions
// that start an agent, attribution of queued messages, session titles, and
// failure reports. It applies protocol events to the stored agent.
package report

import (
	"fmt"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// Apply records an event's conversation, work state, and usage. A lead that
// finishes a turn has a reply for the user to see.
func Apply(a *store.Agent, s *store.Session, ev harness.Event) {
	if ev.Conversation != "" {
		a.Conversation = ev.Conversation
	}
	switch ev.State {
	case harness.Working:
		a.Activity = store.Busy
	case harness.Idle:
		if a.Activity == store.Busy && a.Role == store.Lead {
			s.Unseen = true
		}
		a.Activity = store.Idle
	}
	if ev.Usage != nil {
		a.Usage = store.Usage(*ev.Usage)
	}
}

// Prompt records the user's prompt: it releases the session's queued messages.
func Prompt(s *store.Session) {
	s.AwaitingUser = false
}

// Failure tells the lead that a worker's turn failed.
func Failure(tx *store.Tx, worker store.Agent, detail string) error {
	if worker.Role != store.Worker {
		return nil
	}
	lead, err := tx.Lead(worker.Session)
	if err != nil {
		return err
	}
	return tx.Queue(store.Message{From: worker.ID, To: lead.ID, Text: failure(detail)})
}

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

// Format renders messages as the model sees them.
// Senders cannot see the recipient's chat, so agent messages say how to reply.
func Format(tx *store.Tx, messages []store.Message) string {
	var parts []string
	for _, m := range messages {
		if a, err := tx.Agent(m.From); err == nil {
			parts = append(parts, fmt.Sprintf("Message from %s:\n%s\n\n%s cannot see your chat. To answer, use the hypercube send_message tool to %s.", a.Address(), m.Text, a.Address(), a.Address()))
		} else {
			parts = append(parts, fmt.Sprintf("Message from %s:\n%s", m.From, m.Text))
		}
	}
	return strings.Join(parts, "\n\n")
}

// Leads returns the leads of the workspace's sessions other than the given
// one, which leads can message, with their sessions' titles.
func Leads(tx *store.Tx, workspace, except string) ([]store.Agent, map[string]string, error) {
	sessions, err := tx.Sessions(workspace)
	if err != nil {
		return nil, nil, err
	}
	var leads []store.Agent
	titles := map[string]string{}
	for _, s := range sessions {
		titles[s.ID] = s.Title
		if s.ID == except {
			continue
		}
		lead, err := tx.Lead(s.ID)
		if err != nil {
			return nil, nil, err
		}
		leads = append(leads, lead)
	}
	return leads, titles, nil
}

// Instructions describe the agent's place in the team. The lead also gets
// the configured worker profiles.
func Instructions(tx *store.Tx, a store.Agent, profiles []harness.Profile) (string, error) {
	s, err := tx.Session(a.Session)
	if err != nil {
		return "", err
	}
	w, err := tx.Workspace(s.Workspace)
	if err != nil {
		return "", err
	}
	team, err := tx.Agents(a.Session)
	if err != nil {
		return "", err
	}
	others, titles, err := Leads(tx, s.Workspace, s.ID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, the %s in a Hypercube team of coding agents.\n", a.Address(), a.Role)
	fmt.Fprintf(&b, "Your Hypercube session is %q, titled %q. Agents are addressed as session/name, where session is the session's ID; a bare name means an agent in your session.\n", a.Session, titles[a.Session])
	if a.Role == store.Lead {
		b.WriteString("Worker tools apply only to this session. You can also message the leads of other sessions.\n")
	} else {
		b.WriteString("Messaging applies only to this session.\n")
	}
	b.WriteString("Reach Hypercube agents only with the hypercube tools. Any other agent tools you have manage only your own subagents. Messages from Hypercube agents arrive in your conversation by themselves, so end your turn instead of waiting for them.\n")
	fmt.Fprintf(&b, "Your working directory is %s%s.\n\nTeam:\n", a.Dir, onBranch(a.Branch))
	for _, t := range team {
		fmt.Fprintf(&b, "- %s: %s, %s, %s%s\n", t.Address(), t.Role, t.Harness, t.Dir, onBranch(t.Branch))
	}
	if a.Role == store.Lead {
		b.WriteString("\nOther Hypercube sessions in this workspace share your original checkout, files, Git index, and branch. Coordinate shared changes with their leads and the user; use worker worktrees for independent edits. list_agents shows the current leads.\n")
		var leads []string
		for _, l := range others {
			leads = append(leads, fmt.Sprintf("%s (%s)", l.Address(), titles[l.Session]))
		}
		if len(leads) > 0 {
			fmt.Fprintf(&b, "Other leads: %s\n", strings.Join(leads, ", "))
		}
	}
	switch {
	case a.Role == store.Lead && w.NoGit:
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
	return b.String(), nil
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
