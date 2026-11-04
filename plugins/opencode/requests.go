package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// request is a form waiting for the user, which is how OpenCode asks the
// questions of its question tool.
type request struct {
	id      json.RawMessage // the form request's JSON-RPC ID; nil once OpenCode has withdrawn it
	fields  []field         // one per attention item
	content map[string]any
}

// field is a form field shown as one attention item.
type field struct {
	key     string
	array   bool
	options bool   // the field lists choices
	custom  string // the key of the field for a typed answer, if any
}

// formReply answers a form: "accept" with the answers, or "cancel".
type formReply struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

// closeRequests finishes the items of requests that nobody will answer, and
// cancels each request once.
func (ag *agent) closeRequests() []harness.Item {
	items, closed := ag.requests.Close(func(*request) bool { return true })
	for _, r := range closed {
		if r.id != nil {
			_ = ag.rpc.Respond(r.id, formReply{Action: "cancel"})
		}
	}
	return items
}

// withdraw refuses a form that OpenCode no longer waits for, with the error
// the ACP SDK sent for a cancelled request. Its questions stay open until the
// turn ends.
func (ag *agent) withdraw(params json.RawMessage) {
	var p struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(params, &p) != nil {
		return
	}
	r := ag.requests.Find(func(r *request) bool { return r.id != nil && bytes.Equal(r.id, p.RequestID) })
	if r != nil {
		_ = ag.rpc.RespondError(r.id, -32603, "Internal error")
		r.id = nil
	}
}

// permission approves a permission request once, as `opencode --auto` does.
// Permissions that OpenCode's configuration denies are never requested.
func permission(params json.RawMessage) (any, error) {
	var p struct {
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	for _, o := range p.Options {
		if o.Kind == "allow_once" {
			return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": o.OptionID}}, nil
		}
	}
	return nil, errors.New("permission request without an allow-once option")
}

// form turns the form of OpenCode's question tool into one attention item per
// question, and keeps the request's ID to answer once the user has. A
// question's typed answer has a companion field named <key>_custom. A form
// that cannot be shown, or that asks nothing, is answered at once.
func (ag *agent) form(ev *harness.Event, id, params json.RawMessage) {
	var p struct {
		Mode   string `json:"mode"`
		Schema struct {
			Properties json.RawMessage `json:"properties"`
		} `json:"requestedSchema"`
	}
	if json.Unmarshal(params, &p) != nil || p.Mode != "form" {
		_ = ag.rpc.RespondError(id, -32602, "unsupported request")
		return
	}
	keys, props, err := plugin.ParseForm(p.Schema.Properties)
	if err != nil {
		_ = ag.rpc.RespondError(id, -32602, err.Error())
		return
	}
	r := &request{id: id, content: map[string]any{}}
	var items []harness.Item
	for _, key := range keys {
		f := props[key]
		if base, ok := strings.CutSuffix(key, "_custom"); ok && props[base].Type != "" {
			continue
		}
		if f.Type != "string" && f.Type != "array" {
			// OpenCode ends the turn when a form is cancelled.
			ev.Items = append(ev.Items, ag.errors.Item(fmt.Sprintf("OpenCode asked for a %s value, which Hypercube cannot show.", f.Type)))
			_ = ag.rpc.Respond(id, formReply{Action: "cancel"})
			return
		}
		options := f.Options()
		text := f.Description
		if text == "" {
			text = f.Title
		}
		fld := field{key: key, array: f.Type == "array", options: len(options) > 0}
		if _, ok := props[key+"_custom"]; ok {
			fld.custom = key + "_custom"
		}
		item := harness.Item{ID: fmt.Sprintf("form:%d/%d", ag.forms, len(items)), Kind: harness.KindAttention, Free: !fld.options || fld.custom != ""}
		var notes []string
		for _, o := range options {
			item.Choices = append(item.Choices, o.Const)
			notes = append(notes, o.Description)
		}
		item.Text, item.Notes = text, plugin.Notes(notes)
		items = append(items, item)
		r.fields = append(r.fields, fld)
	}
	ag.forms++
	if len(items) == 0 {
		_ = ag.rpc.Respond(id, formReply{Action: "accept"})
		return
	}
	ag.requests.Add(r, items)
	ev.Items = append(ev.Items, items...)
}

// answer replies to an open question with a choice or text.
func (ag *agent) answer(item, choice, text string) error {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	open, ok := ag.requests.Get(item)
	if !ok {
		return fmt.Errorf("request %s is not open", item)
	}
	r := open.Request
	result := choice
	if result == "" {
		result = text
	}
	if result == "" {
		return errors.New("the answer is empty")
	}
	f := r.fields[open.Index]
	if choice == "" && f.options && f.custom == "" {
		return errors.New("the question needs one of its choices")
	}
	switch {
	case choice != "" && f.array:
		r.content[f.key] = []string{choice}
	case choice != "":
		r.content[f.key] = choice
	case f.custom != "":
		r.content[f.custom] = text
	default:
		r.content[f.key] = text
	}
	if open.Last && r.id != nil {
		_ = ag.rpc.Respond(r.id, formReply{Action: "accept", Content: r.content})
	}
	ag.emit(harness.Event{Items: []harness.Item{ag.requests.Done(item, result)}})
	return nil
}
