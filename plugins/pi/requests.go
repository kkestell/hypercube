package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

// request is an extension dialog waiting for the user.
type request struct {
	method string
	item   harness.Item
	timer  *time.Timer // ends the item when Pi gives up on the dialog
}

func (r *request) stop() {
	if r.timer != nil {
		r.timer.Stop()
	}
}

// answer replies to an open dialog: a choice, or typed text for a dialog that
// takes text.
func (ag *agent) answer(item string, choices []string, text string) error {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	r := ag.requests[item]
	if r == nil {
		return fmt.Errorf("request %s is not open", item)
	}
	if err := harness.CheckAnswer(r.item, choices, text); err != nil {
		return err
	}
	result := text
	if len(choices) == 1 {
		result = choices[0]
	}
	reply := map[string]any{"type": "extension_ui_response", "id": item}
	if r.method == "confirm" {
		reply["confirmed"] = result == yes
	} else {
		reply["value"] = result
	}
	if err := ag.write(reply); err != nil {
		return err
	}
	r.stop()
	delete(ag.requests, item)
	done := r.item
	done.Done, done.Answer = true, result
	ag.emit(harness.Event{Items: []harness.Item{done}})
	return nil
}

const (
	yes = "Yes"
	no  = "No"
)

// closeRequests finishes the items of the open dialogs, and with cancel tells
// Pi the user gave them up.
func (ag *agent) closeRequests(cancel bool) []harness.Item {
	var items []harness.Item
	for id, r := range ag.requests {
		r.stop()
		if cancel {
			_ = ag.write(map[string]any{"type": "extension_ui_response", "id": id, "cancelled": true})
		}
		r.item.Done = true
		items = append(items, r.item)
		delete(ag.requests, id)
	}
	return items
}

// dialog turns an extension's request for the user into an attention item.
// Notifications of errors are shown; other interface requests are ignored.
func (ag *agent) dialog(ev *harness.Event, line []byte) {
	var m struct {
		ID         string   `json:"id"`
		Method     string   `json:"method"`
		Title      string   `json:"title"`
		Options    []string `json:"options"`
		Message    string   `json:"message"`
		Prefill    string   `json:"prefill"`
		Timeout    int      `json:"timeout"`
		NotifyType string   `json:"notifyType"`
	}
	if json.Unmarshal(line, &m) != nil {
		return
	}
	item := harness.Item{ID: m.ID, Kind: harness.KindAttention, Text: m.Title}
	switch m.Method {
	case "select":
		item.Choices, item.Free = m.Options, len(m.Options) == 0
	case "confirm":
		item.Choices = []string{yes, no}
		if m.Message != "" {
			item.Text += "\n\n" + m.Message
		}
	case "input":
		item.Free = true
	case "editor":
		item.Free = true
		if m.Prefill != "" {
			item.Text += "\n\n" + m.Prefill
		}
	case "notify":
		if m.NotifyType == "error" {
			ag.fail(ev, m.Message)
		}
		return
	default:
		return
	}
	r := &request{method: m.Method, item: item}
	// Pi resolves a dialog with a timeout by itself.
	if m.Timeout > 0 {
		r.timer = time.AfterFunc(time.Duration(m.Timeout)*time.Millisecond, func() {
			ag.mu.Lock()
			defer ag.mu.Unlock()
			if ag.requests[m.ID] == r {
				delete(ag.requests, m.ID)
				done := r.item
				done.Done = true
				ag.emit(harness.Event{Items: []harness.Item{done}})
			}
		})
	}
	ag.requests[m.ID] = r
	ev.Items = append(ev.Items, item)
}
