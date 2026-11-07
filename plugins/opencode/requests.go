package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

type form struct {
	ID        string  `json:"id"`
	SessionID string  `json:"sessionID"`
	Title     string  `json:"title"`
	Fields    []field `json:"fields"`
}

type field struct {
	Key         string            `json:"key"`
	Type        string            `json:"type"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Custom      bool              `json:"custom"`
	Hidden      bool              `json:"hidden"`
	Default     json.RawMessage   `json:"default"`
	When        []json.RawMessage `json:"when"`
	Options     []struct {
		Value       string `json:"value"`
		Label       string `json:"label"`
		Description string `json:"description"`
	} `json:"options"`
}

type request struct {
	id       string
	session  string
	fields   []field
	answers  map[string]any
	replying bool
}

func (ag *agent) form(f form) error {
	r := &request{id: f.ID, session: f.SessionID, answers: map[string]any{}}
	var items []harness.Item
	for _, fld := range f.Fields {
		if fld.Hidden && len(fld.Default) > 0 {
			r.answers[fld.Key] = fld.Default
			continue
		}
		if fld.Hidden || len(fld.When) > 0 || fld.Type != "string" && fld.Type != "multiselect" {
			ag.mu.Lock()
			ag.emit(harness.Event{Items: []harness.Item{ag.errors.Item(fmt.Sprintf("OpenCode asked for a %s value, which Hypercube cannot show.", fld.Type))}})
			ag.mu.Unlock()
			if err := ag.server.call(ag.ctx, "DELETE", sessionPath(f.SessionID)+"/form/"+f.ID, nil, nil); err != nil {
				return err
			}
			return ag.server.call(ag.ctx, "POST", sessionPath(f.SessionID)+"/interrupt?resume=false", nil, nil)
		}
		text := fld.Description
		if text == "" {
			text = fld.Title
		}
		if text == "" {
			text = f.Title
		}
		it := harness.Item{ID: fmt.Sprintf("form:%s/%d", f.ID, len(items)), Kind: harness.KindAttention, Text: text, Multi: fld.Type == "multiselect", Free: len(fld.Options) == 0 || fld.Custom}
		var notes []string
		for _, option := range fld.Options {
			it.Choices = append(it.Choices, option.Label)
			notes = append(notes, option.Description)
		}
		it.Notes = plugin.Notes(notes)
		r.fields = append(r.fields, fld)
		items = append(items, it)
	}
	if len(items) == 0 {
		return ag.server.call(ag.ctx, "POST", sessionPath(f.SessionID)+"/form/"+f.ID+"/reply", map[string]any{"answer": r.answers}, nil)
	}
	ag.mu.Lock()
	defer ag.mu.Unlock()
	ag.requests.Add(r, items)
	ag.emit(harness.Event{Items: items})
	return nil
}

func (ag *agent) answer(item string, choices []string, text string) error {
	ag.mu.Lock()
	open, ok := ag.requests.Get(item)
	if !ok {
		ag.mu.Unlock()
		return fmt.Errorf("request %s is not open", item)
	}
	if err := harness.CheckAnswer(open.Item, choices, text); err != nil {
		ag.mu.Unlock()
		return err
	}
	r := open.Request
	if r.replying {
		ag.mu.Unlock()
		return errors.New("the answer is being sent")
	}
	f := r.fields[open.Index]
	var values []string
	for _, choice := range choices {
		for _, option := range f.Options {
			if choice == option.Label {
				values = append(values, option.Value)
				break
			}
		}
	}
	if strings.TrimSpace(text) != "" {
		values = append(values, text)
	}
	if f.Type == "multiselect" {
		r.answers[f.Key] = values
	} else {
		r.answers[f.Key] = values[0]
	}
	display := append([]string{}, choices...)
	if text != "" && (len(choices) == 0 || f.Type == "multiselect") {
		display = append(display, text)
	}
	answer := strings.Join(display, ", ")
	if !open.Last {
		ag.emit(harness.Event{Items: []harness.Item{ag.requests.Done(item, answer)}})
		ag.mu.Unlock()
		return nil
	}
	r.replying = true
	ag.mu.Unlock()
	ctx, cancel := context.WithTimeout(ag.ctx, startTimeout)
	defer cancel()
	err := ag.server.call(ctx, "POST", sessionPath(r.session)+"/form/"+r.id+"/reply", map[string]any{"answer": r.answers}, nil)
	ag.mu.Lock()
	defer ag.mu.Unlock()
	r.replying = false
	if err != nil {
		return err
	}
	if _, ok := ag.requests.Get(item); ok {
		ag.emit(harness.Event{Items: []harness.Item{ag.requests.Done(item, answer)}})
	}
	return nil
}
