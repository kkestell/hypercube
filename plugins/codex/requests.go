package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// Choices for an approval.
const (
	allow        = "Allow"
	allowSession = "Allow for this session"
	deny         = "Deny"
)

// serverRequest turns a request from codex into attention items. The ones
// codex's automatic reviewer settles never arrive.
func (ag *agent) serverRequest(ev *harness.Event, m plugin.Message) {
	var build func(json.RawMessage) (*request, error)
	switch m.Method {
	case "item/commandExecution/requestApproval":
		build = commandRequest
	case "item/fileChange/requestApproval":
		build = fileRequest
	case "item/permissions/requestApproval":
		build = permissionsRequest
	case "item/tool/requestUserInput":
		build = questionsRequest
	case "mcpServer/elicitation/request":
		build = elicitationRequest
	default:
		_ = ag.rpc.RespondError(m.ID, -32601, "method not found")
		return
	}
	r, err := build(m.Params)
	if err != nil {
		_ = ag.rpc.RespondError(m.ID, -32602, err.Error())
		ag.fail(ev, "Codex asked for something Hypercube cannot show: "+err.Error())
		return
	}
	r.id = m.ID
	r.values = make([]any, len(r.items))
	for i := range r.items {
		r.items[i].ID = fmt.Sprintf("request:%s/%d", strings.Trim(string(m.ID), `"`), i)
		r.items[i].Kind = harness.KindAttention
		ag.requests[r.items[i].ID] = r
	}
	ev.Items = append(ev.Items, r.items...)
}

// approval asks for one of the choices; reply maps the choice to the response.
func approval(text string, choices []string, reply func(choice string) any, refuse any) *request {
	return &request{
		items:  []harness.Item{{Text: text, Choices: choices}},
		reply:  func(values []any) any { return reply(values[0].(string)) },
		refuse: refuse,
	}
}

// decisions offers the approval choices that codex accepts. A request lists
// its available decisions; one that does not accepts them all. Codex offers no
// decline when only cancelling is possible, so Deny cancels then.
func decisions(available []json.RawMessage) ([]string, func(string) any) {
	names := map[string]bool{}
	for _, raw := range available {
		var name string
		if json.Unmarshal(raw, &name) == nil {
			names[name] = true
		}
	}
	has := func(name string) bool { return len(available) == 0 || names[name] }
	choices := []string{allow}
	if has("acceptForSession") {
		choices = append(choices, allowSession)
	}
	choices = append(choices, deny)
	return choices, func(choice string) any {
		switch {
		case choice == allow:
			return map[string]any{"decision": "accept"}
		case choice == allowSession:
			return map[string]any{"decision": "acceptForSession"}
		case has("decline"):
			return map[string]any{"decision": "decline"}
		}
		return map[string]any{"decision": "cancel"}
	}
}

func codeBlock(s string) string { return "```\n" + s + "\n```" }

func commandRequest(params json.RawMessage) (*request, error) {
	var p struct {
		Command   string            `json:"command"`
		Reason    string            `json:"reason"`
		Kind      string            `json:"kind"`
		Decisions []json.RawMessage `json:"availableDecisions"`
		Network   *struct {
			Host string `json:"host"`
		} `json:"networkApprovalContext"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	text := "Run this command?"
	switch {
	case p.Network != nil:
		text = "Allow network access to " + p.Network.Host + "?"
	case p.Kind == "writeStdin":
		text = "Send input to a running command?"
	}
	if p.Command != "" {
		text += "\n\n" + codeBlock(p.Command)
	}
	if p.Reason != "" {
		text += "\n\n" + p.Reason
	}
	choices, reply := decisions(p.Decisions)
	return approval(text, choices, reply, map[string]any{"decision": "cancel"}), nil
}

func fileRequest(params json.RawMessage) (*request, error) {
	var p struct {
		Reason    string            `json:"reason"`
		GrantRoot string            `json:"grantRoot"`
		Decisions []json.RawMessage `json:"availableDecisions"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	text := "Apply these file changes?"
	if p.GrantRoot != "" {
		text = "Allow writes under " + p.GrantRoot + "?"
	}
	if p.Reason != "" {
		text += "\n\n" + p.Reason
	}
	choices, reply := decisions(p.Decisions)
	return approval(text, choices, reply, map[string]any{"decision": "cancel"}), nil
}

func permissionsRequest(params json.RawMessage) (*request, error) {
	var p struct {
		Permissions json.RawMessage `json:"permissions"`
		Reason      string          `json:"reason"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	text := "Grant these permissions?\n\n" + codeBlock(string(p.Permissions))
	if p.Reason != "" {
		text += "\n\n" + p.Reason
	}
	none := map[string]any{"permissions": map[string]any{}}
	return approval(text, []string{allow, allowSession, deny}, func(choice string) any {
		if choice == deny {
			return none
		}
		scope := "turn"
		if choice == allowSession {
			scope = "session"
		}
		return map[string]any{"permissions": p.Permissions, "scope": scope}
	}, none), nil
}

// optionNotes appends the options' descriptions to a question.
func optionNotes(text string, labels, descriptions []string) string {
	var notes []string
	for i, d := range descriptions {
		if d != "" {
			notes = append(notes, fmt.Sprintf("- %s: %s", labels[i], d))
		}
	}
	if len(notes) > 0 {
		text += "\n\n" + strings.Join(notes, "\n")
	}
	return text
}

// questionsRequest makes one attention item per question. Codex receives the
// answers once every question has one.
func questionsRequest(params json.RawMessage) (*request, error) {
	var p struct {
		Questions []struct {
			ID       string `json:"id"`
			Header   string `json:"header"`
			Question string `json:"question"`
			Other    bool   `json:"isOther"`
			Options  []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if len(p.Questions) == 0 {
		return nil, errors.New("a request without questions")
	}
	r := &request{refuse: map[string]any{"answers": map[string]any{}}}
	var ids []string
	for _, q := range p.Questions {
		item := harness.Item{Text: q.Question, Free: q.Other || len(q.Options) == 0}
		if q.Header != "" {
			item.Text = q.Header + "\n\n" + q.Question
		}
		var labels, descriptions []string
		for _, o := range q.Options {
			labels, descriptions = append(labels, o.Label), append(descriptions, o.Description)
		}
		item.Choices = labels
		item.Text = optionNotes(item.Text, labels, descriptions)
		r.items = append(r.items, item)
		ids = append(ids, q.ID)
	}
	r.reply = func(values []any) any {
		answers := map[string]any{}
		for i, id := range ids {
			answers[id] = map[string]any{"answers": []string{values[i].(string)}}
		}
		return map[string]any{"answers": answers}
	}
	return r, nil
}

// field is a property of an elicitation form's schema.
type field struct {
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Enum        []string `json:"enum"`
	OneOf       []option `json:"oneOf"`
	Items       struct {
		Enum  []string `json:"enum"`
		AnyOf []option `json:"anyOf"`
	} `json:"items"`
}

type option struct {
	Const string `json:"const"`
	Title string `json:"title"`
}

// elicitationRequest shows an MCP server's request. A request that only
// carries a message, as an MCP tool's approval does, is accepted or declined;
// a form becomes one attention item per field.
func elicitationRequest(params json.RawMessage) (*request, error) {
	var p struct {
		Server  string `json:"serverName"`
		Message string `json:"message"`
		Mode    string `json:"mode"`
		URL     string `json:"url"`
		Schema  struct {
			Properties json.RawMessage `json:"properties"`
		} `json:"requestedSchema"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	text := p.Server + ": " + p.Message
	refuse := map[string]any{"action": "cancel"}
	var keys []string
	var fields map[string]field
	switch p.Mode {
	case "url":
		text += "\n\n" + p.URL
	case "form":
		var err error
		if keys, fields, err = parseForm(p.Schema.Properties); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("a %s request", p.Mode)
	}
	if len(keys) == 0 {
		const accept, decline = "Accept", "Decline"
		return &request{
			items: []harness.Item{{Text: text, Choices: []string{accept, decline}}},
			reply: func(values []any) any {
				if values[0] == accept {
					return map[string]any{"action": "accept", "content": map[string]any{}}
				}
				return map[string]any{"action": "decline"}
			},
			refuse: refuse,
		}, nil
	}
	r := &request{refuse: refuse}
	for i, key := range keys {
		f := fields[key]
		item := harness.Item{Text: f.Description}
		if item.Text == "" {
			item.Text = f.Title
		}
		if i == 0 {
			item.Text = text + "\n\n" + item.Text
		}
		var parse func(string) (any, error)
		switch {
		case f.Type == "boolean":
			item.Choices = []string{"Yes", "No"}
			parse = func(s string) (any, error) { return s == "Yes", nil }
		case f.Type == "number" || f.Type == "integer":
			item.Free = true
			parse = func(s string) (any, error) {
				n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
				if err != nil {
					return nil, errors.New("the answer must be a number")
				}
				return n, nil
			}
		case f.Type == "string" || f.Type == "array":
			choices, labels := f.choices()
			item.Choices = choices
			item.Free = len(choices) == 0
			if f.Type == "array" {
				parse = func(s string) (any, error) { return []string{s}, nil }
			}
			item.Text = optionNotes(item.Text, choices, labels)
		default:
			return nil, fmt.Errorf("a %s field", f.Type)
		}
		r.items = append(r.items, item)
		r.parse = append(r.parse, parse)
	}
	r.reply = func(values []any) any {
		content := map[string]any{}
		for i, key := range keys {
			content[key] = values[i]
		}
		return map[string]any{"action": "accept", "content": content}
	}
	return r, nil
}

// choices returns the values a field offers and their titles.
func (f field) choices() (values, titles []string) {
	options := f.OneOf
	if f.Type == "array" {
		options = f.Items.AnyOf
		for _, v := range f.Items.Enum {
			options = append(options, option{Const: v})
		}
	}
	for _, v := range f.Enum {
		options = append(options, option{Const: v})
	}
	for _, o := range options {
		values, titles = append(values, o.Const), append(titles, o.Title)
	}
	return values, titles
}

// parseForm reads a form's properties in the order the schema lists them.
func parseForm(properties json.RawMessage) ([]string, map[string]field, error) {
	dec := json.NewDecoder(bytes.NewReader(properties))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, nil, errors.New("a form without fields")
	}
	var keys []string
	fields := map[string]field{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		var f field
		if err := dec.Decode(&f); err != nil {
			return nil, nil, err
		}
		keys = append(keys, key.(string))
		fields[key.(string)] = f
	}
	return keys, fields, nil
}

// answer replies to an open request with a choice or text.
func (ag *agent) answer(item, choice, text string) error {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	r := ag.requests[item]
	if r == nil {
		return fmt.Errorf("request %s is not open", item)
	}
	i := slices.IndexFunc(r.items, func(it harness.Item) bool { return it.ID == item })
	answer := choice
	switch {
	case choice != "":
		if !slices.Contains(r.items[i].Choices, choice) {
			return fmt.Errorf("unknown choice %q", choice)
		}
	case text == "":
		return errors.New("the answer is empty")
	case !r.items[i].Free:
		return errors.New("the question needs one of its choices")
	default:
		answer = text
	}
	var value any = answer
	if r.parse != nil && r.parse[i] != nil {
		var err error
		if value, err = r.parse[i](answer); err != nil {
			return err
		}
	}
	r.values[i] = value
	if r.given+1 == len(r.items) {
		if err := ag.rpc.Respond(r.id, r.reply(r.values)); err != nil {
			return err
		}
	}
	r.given++
	done := r.items[i]
	done.Done, done.Answer = true, answer
	delete(ag.requests, item)
	ag.emit(harness.Event{Items: []harness.Item{done}})
	return nil
}

// closeRequests finishes the items of the requests that match, which nobody
// will answer. With refuse, codex is told the user gave up.
func (ag *agent) closeRequests(match func(*request) bool, refuse bool) []harness.Item {
	var items []harness.Item
	closed := map[*request]bool{}
	for id, r := range ag.requests {
		if !match(r) {
			continue
		}
		if refuse && !closed[r] {
			_ = ag.rpc.Respond(r.id, r.refuse)
		}
		closed[r] = true
		for _, it := range r.items {
			if it.ID == id {
				it.Done = true
				items = append(items, it)
			}
		}
		delete(ag.requests, id)
	}
	return items
}
