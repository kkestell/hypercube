package session

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// queueReport uses only the final response the harness identified, never its
// progress text or a native subagent's output. It shares the event checkpoint.
func (r *Runtime) queueReport(tx *write, child store.Session, final []string, prefix string) error {
	var texts []string
	for _, id := range final {
		e, found, err := tx.Entry(child.ID, prefix+id)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		var item harness.Item
		if err := json.Unmarshal(e.Data, &item); err != nil {
			return err
		}
		if item.Kind == harness.KindText && item.Parent == "" && item.Text != "" {
			texts = append(texts, item.Text)
		}
	}
	text := strings.Join(texts, "\n\n")
	if text == "" {
		return nil
	}
	chars := []rune(text)
	if len(chars) > 400 {
		text = string(chars[:400]) + "…"
	}
	text = fmt.Sprintf("%s finished a turn:\n%s", child.Name, text)
	return tx.Queue(store.Message{From: child.ID, To: child.Parent, Text: text, Report: true})
}

// reportsReady describes the family's observed work state. A child whose
// process exited and has no successor cannot hold up the surviving children.
func (r *Runtime) reportsReady(tx *store.Tx, parent store.Session) (bool, error) {
	if parent.Activity != store.Idle || parent.Attention || parent.InputPending {
		return false, nil
	}
	for _, c := range r.live[parent.ID].commands {
		if c.Command == harness.CmdInput {
			return false, nil
		}
	}
	children, err := tx.Children(parent.ID)
	if err != nil {
		return false, err
	}
	for _, child := range children {
		l := r.live[child.ID]
		if child.Ended || l == nil || l.stopping {
			continue
		}
		if l.ready == 0 || child.InputPending || child.Activity != store.Idle || child.Attention {
			return false, nil
		}
		queued, err := tx.Queued(child.ID)
		if err != nil {
			return false, err
		}
		if len(queued) > 0 || l.delivering() {
			return false, nil
		}
		for _, c := range l.commands {
			if c.Command == harness.CmdInput {
				return false, nil
			}
		}
	}
	return true, nil
}

// reportInput takes a snapshot of pending reports. Delivery records that
// snapshot before any later child completion can add another report.
func reportInput(queued []store.Message) (out outgoing) {
	var text strings.Builder
	const heading = "All children are idle or have stopped. Continue with their reports:"
	text.WriteString(heading)
	for _, m := range queued {
		if !m.Report {
			continue
		}
		if out.message == 0 {
			out.message = m.ID
		}
		out.reports = append(out.reports, m.ID)
		// Sender names were captured when the report was queued; deleting
		// an ended child's session cannot make its pending report unreadable.
		text.WriteString("\n\n" + strings.TrimPrefix(m.Text, heading+"\n\n"))
	}
	out.text = text.String()
	out.item = harness.Item{Kind: harness.KindUser, Text: out.text, Automatic: true, Done: true}
	return out
}
