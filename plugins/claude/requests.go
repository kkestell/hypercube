package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// request is a can_use_tool control request waiting for the user: a tool
// permission, or the questions of AskUserQuestion.
type request struct {
	id        string          // the control request's ID
	input     json.RawMessage // the tool input, which an allowing reply carries back
	questions []string        // the text of each question, by item; nil for a permission
	answers   map[string]string
}

const (
	allow = "Allow"
	deny  = "Deny"
)

// answer replies to an open request: the choice or text of a question, or
// Allow or Deny, with a reason as text, for a tool permission.
func (c *claude) answer(item string, choices []string, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	open, ok := c.requests.Get(item)
	if !ok {
		return fmt.Errorf("request %s is not open", item)
	}
	if err := harness.CheckAnswer(open.Item, choices, text); err != nil {
		return err
	}
	if open.Request.questions != nil {
		return c.answerQuestion(open, choices, text)
	}
	choice := ""
	if len(choices) > 0 {
		choice = choices[0]
	}
	return c.answerPermission(open, choice, text)
}

func (c *claude) answerPermission(open plugin.Open[request], choice, text string) error {
	r := open.Request
	var response map[string]any
	var result string
	switch {
	case choice == allow:
		response = map[string]any{"behavior": "allow", "updatedInput": r.input}
		result = allow
	case choice == deny || choice == "" && text != "":
		message := text
		if message == "" {
			message = "The user denied this tool use."
		}
		response = map[string]any{"behavior": "deny", "message": message}
		result = deny
		if text != "" {
			result += ": " + text
		}
	case choice == "":
		return errors.New("the answer is empty")
	default:
		return fmt.Errorf("unknown choice %q", choice)
	}
	if err := c.respond(r.id, response); err != nil {
		return err
	}
	c.emit(harness.Event{Items: []harness.Item{c.requests.Done(open.Item.ID, result)}})
	return nil
}

// answerQuestion records the answer to one question. Claude is answered once
// every question of the request has an answer.
func (c *claude) answerQuestion(open plugin.Open[request], choices []string, text string) error {
	r := open.Request
	result := cmp.Or(strings.Join(choices, ", "), text)
	if open.Item.Multi && len(choices) > 0 && strings.TrimSpace(text) != "" {
		result += ", " + text
	}
	if result == "" {
		return errors.New("the answer is empty")
	}
	r.answers[r.questions[open.Index]] = result
	if open.Last {
		input, err := withAnswers(r.input, r.answers)
		if err != nil {
			return err
		}
		if err := c.respond(r.id, map[string]any{"behavior": "allow", "updatedInput": input}); err != nil {
			return err
		}
	}
	c.emit(harness.Event{Items: []harness.Item{c.requests.Done(open.Item.ID, result)}})
	return nil
}

// withAnswers is the question input with its answers added. Every other field
// stays as claude sent it.
func withAnswers(input json.RawMessage, answers map[string]string) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(answers)
	if err != nil {
		return nil, err
	}
	fields["answers"] = raw
	return json.Marshal(fields)
}

func (c *claude) respond(id string, response any) error {
	return c.send(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": id, "response": response}})
}

// control turns a permission request into attention items.
func (c *claude) control(ev *harness.Event, m message) {
	var req struct {
		Subtype     string          `json:"subtype"`
		Tool        string          `json:"tool_name"`
		DisplayName string          `json:"display_name"`
		Title       string          `json:"title"`
		Description string          `json:"description"`
		Input       json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(m.Request, &req); err != nil || req.Subtype != "can_use_tool" {
		_ = c.send(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": m.RequestID, "error": "unsupported request"}})
		return
	}
	r := &request{id: m.RequestID, input: req.Input}
	var items []harness.Item
	if req.Tool == "AskUserQuestion" {
		r.questions, r.answers = []string{}, map[string]string{}
		var in struct {
			Questions []struct {
				Question string `json:"question"`
				Multi    bool   `json:"multiSelect"`
				Options  []struct {
					Label       string `json:"label"`
					Description string `json:"description"`
				} `json:"options"`
			} `json:"questions"`
		}
		if err := json.Unmarshal(req.Input, &in); err != nil {
			_ = c.respond(m.RequestID, map[string]any{"behavior": "deny", "message": "The question could not be read: " + err.Error()})
			return
		}
		for i, q := range in.Questions {
			item := harness.Item{ID: fmt.Sprintf("%s/%d", m.RequestID, i), Kind: harness.KindAttention, Text: q.Question, Free: true, Multi: q.Multi}
			var notes []string
			for _, o := range q.Options {
				item.Choices = append(item.Choices, o.Label)
				notes = append(notes, o.Description)
			}
			item.Notes = plugin.Notes(notes)
			items = append(items, item)
			r.questions = append(r.questions, q.Question)
		}
		if len(items) == 0 {
			_ = c.respond(m.RequestID, map[string]any{"behavior": "deny", "message": "The question had no content."})
			return
		}
	} else {
		name := req.DisplayName
		if name == "" {
			name = req.Tool
		}
		text := req.Title
		if text == "" {
			text = "Allow " + name + "?"
		}
		if req.Description != "" {
			text += "\n\n" + req.Description
		}
		items = []harness.Item{{ID: m.RequestID, Kind: harness.KindAttention, Text: text, Choices: []string{allow, deny}, Free: true, Grant: req.Tool}}
	}
	c.requests.Add(r, items)
	ev.Items = append(ev.Items, items...)
}
