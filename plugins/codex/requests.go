package main

import (
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

// Choices for an elicitation that only carries a message.
const (
	accept  = "Accept"
	decline = "Decline"
)

// The server requests that ask the user.
const (
	methodCommand     = "item/commandExecution/requestApproval"
	methodFile        = "item/fileChange/requestApproval"
	methodPermissions = "item/permissions/requestApproval"
	methodQuestions   = "item/tool/requestUserInput"
	methodElicitation = "mcpServer/elicitation/request"
)

// request is a server request waiting for the user, with one attention item
// per question. answers holds the answer to each item. Once every item has one,
// response turns them into the reply to codex; refusal is the reply that gives
// up. The other fields are what a kind of request needs to build its response.
type request struct {
	id      json.RawMessage
	method  string
	items   []harness.Item
	answers []string
	given   int

	declinable  bool            // an approval that lets the user decline
	permissions json.RawMessage // the permissions asked for
	questionIDs []string        // the ID of each question, by item
	form        []formField     // the fields of a form, by item; nil without one
}

// formField is a field of an elicitation's form.
type formField struct {
	key   string
	field plugin.Field
}

// serverRequest turns a request from codex into attention items. The ones
// codex's automatic reviewer settles never arrive.
func (ag *agent) serverRequest(ev *harness.Event, m plugin.Message) {
	var build func(json.RawMessage) (*request, error)
	switch m.Method {
	case methodCommand:
		build = commandRequest
	case methodFile:
		build = fileRequest
	case methodPermissions:
		build = permissionsRequest
	case methodQuestions:
		build = questionsRequest
	case methodElicitation:
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
	r.id, r.method = m.ID, m.Method
	r.answers = make([]string, len(r.items))
	for i := range r.items {
		r.items[i].ID = fmt.Sprintf("request:%s/%d", strings.Trim(string(m.ID), `"`), i)
		r.items[i].Kind = harness.KindAttention
		ag.requests[r.items[i].ID] = r
	}
	ev.Items = append(ev.Items, r.items...)
}

// response is the reply to codex once every item has an answer.
func (r *request) response() (any, error) {
	switch r.method {
	case methodCommand, methodFile:
		return approvalResponse(r.answers[0], r.declinable), nil
	case methodPermissions:
		return permissionsResponse(r.answers[0], r.permissions), nil
	case methodQuestions:
		return questionsResponse(r.questionIDs, r.answers), nil
	default:
		return elicitationResponse(r.form, r.answers)
	}
}

// refusal is the reply that tells codex the user gave up.
func (r *request) refusal() any {
	switch r.method {
	case methodPermissions:
		return map[string]any{"permissions": map[string]any{}}
	case methodQuestions:
		return map[string]any{"answers": map[string]any{}}
	case methodElicitation:
		return map[string]any{"action": "cancel"}
	default:
		return map[string]any{"decision": "cancel"}
	}
}

// decisions offers the approval choices that codex accepts, and reports
// whether the user may decline. A request lists its available decisions; one
// that does not accepts them all. Codex offers no decline when only cancelling
// is possible, so Deny cancels then.
func decisions(available []json.RawMessage) (choices []string, declinable bool) {
	names := map[string]bool{}
	for _, raw := range available {
		var name string
		if json.Unmarshal(raw, &name) == nil {
			names[name] = true
		}
	}
	has := func(name string) bool { return len(available) == 0 || names[name] }
	choices = []string{allow}
	if has("acceptForSession") {
		choices = append(choices, allowSession)
	}
	return append(choices, deny), has("decline")
}

func approvalResponse(choice string, declinable bool) any {
	switch {
	case choice == allow:
		return map[string]any{"decision": "accept"}
	case choice == allowSession:
		return map[string]any{"decision": "acceptForSession"}
	case declinable:
		return map[string]any{"decision": "decline"}
	}
	return map[string]any{"decision": "cancel"}
}

func permissionsResponse(choice string, permissions json.RawMessage) any {
	if choice == deny {
		return map[string]any{"permissions": map[string]any{}}
	}
	scope := "turn"
	if choice == allowSession {
		scope = "session"
	}
	return map[string]any{"permissions": permissions, "scope": scope}
}

func questionsResponse(ids, answers []string) any {
	byID := map[string]any{}
	for i, id := range ids {
		byID[id] = map[string]any{"answers": []string{answers[i]}}
	}
	return map[string]any{"answers": byID}
}

func elicitationResponse(form []formField, answers []string) (any, error) {
	if form == nil {
		if answers[0] == accept {
			return map[string]any{"action": "accept", "content": map[string]any{}}, nil
		}
		return map[string]any{"action": "decline"}, nil
	}
	content := map[string]any{}
	for i, f := range form {
		value, err := formValue(f.field, answers[i])
		if err != nil {
			return nil, err
		}
		content[f.key] = value
	}
	return map[string]any{"action": "accept", "content": content}, nil
}

// formValue converts the answer to a form field to the value codex expects.
func formValue(f plugin.Field, answer string) (any, error) {
	switch f.Type {
	case "boolean":
		return answer == "Yes", nil
	case "number", "integer":
		n, err := strconv.ParseFloat(strings.TrimSpace(answer), 64)
		if err != nil {
			return nil, errors.New("the answer must be a number")
		}
		return n, nil
	case "array":
		return []string{answer}, nil
	}
	return answer, nil
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
	choices, declinable := decisions(p.Decisions)
	return &request{items: []harness.Item{{Text: text, Choices: choices}}, declinable: declinable}, nil
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
	choices, declinable := decisions(p.Decisions)
	return &request{items: []harness.Item{{Text: text, Choices: choices}}, declinable: declinable}, nil
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
	return &request{
		items:       []harness.Item{{Text: text, Choices: []string{allow, allowSession, deny}}},
		permissions: p.Permissions,
	}, nil
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
	r := &request{}
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
		item.Text = plugin.OptionNotes(item.Text, labels, descriptions)
		r.items = append(r.items, item)
		r.questionIDs = append(r.questionIDs, q.ID)
	}
	return r, nil
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
	var keys []string
	var fields map[string]plugin.Field
	switch p.Mode {
	case "url":
		text += "\n\n" + p.URL
	case "form":
		var err error
		if keys, fields, err = plugin.ParseForm(p.Schema.Properties); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("a %s request", p.Mode)
	}
	if len(keys) == 0 {
		return &request{items: []harness.Item{{Text: text, Choices: []string{accept, decline}}}}, nil
	}
	r := &request{}
	for i, key := range keys {
		f := fields[key]
		item := harness.Item{Text: f.Description}
		if item.Text == "" {
			item.Text = f.Title
		}
		if i == 0 {
			item.Text = text + "\n\n" + item.Text
		}
		switch f.Type {
		case "boolean":
			item.Choices = []string{"Yes", "No"}
		case "number", "integer":
			item.Free = true
		case "string", "array":
			var titles []string
			for _, o := range f.Options() {
				item.Choices, titles = append(item.Choices, o.Const), append(titles, o.Title)
			}
			item.Free = len(item.Choices) == 0
			item.Text = plugin.OptionNotes(item.Text, item.Choices, titles)
		default:
			return nil, fmt.Errorf("a %s field", f.Type)
		}
		r.items = append(r.items, item)
		r.form = append(r.form, formField{key: key, field: f})
	}
	return r, nil
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
	// A form answer that cannot be converted is refused at once.
	if r.form != nil {
		if _, err := formValue(r.form[i].field, answer); err != nil {
			return err
		}
	}
	r.answers[i] = answer
	if r.given+1 == len(r.items) {
		response, err := r.response()
		if err != nil {
			return err
		}
		if err := ag.rpc.Respond(r.id, response); err != nil {
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
			_ = ag.rpc.Respond(r.id, r.refusal())
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
