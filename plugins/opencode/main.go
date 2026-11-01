// Command hypercube-harness-opencode runs OpenCode 2.x as a Hypercube agent.
// `serve` keeps one `opencode acp` process alive across turns, speaks the Agent
// Client Protocol to it, and translates its updates into protocol events.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

const (
	startTimeout = 60 * time.Second
	stopTimeout  = 3 * time.Second
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: hypercube-harness-opencode serve")
		os.Exit(1)
	}
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-harness-opencode:", err)
		os.Exit(1)
	}
}

func serve(in io.Reader, out io.Writer) error {
	return plugin.Serve(in, out, func(a harness.StartArgs, emit func(harness.Event), exited chan<- error) (plugin.Agent, error) {
		ag, err := startAgent(a, emit, exited)
		if err != nil {
			return plugin.Agent{}, err
		}
		return plugin.Agent{Input: ag.input, Interrupt: ag.interrupt, Answer: ag.answer, Idle: ag.idle, Stop: ag.stop}, nil
	})
}

// agent is a running `opencode acp` process and what its updates have told us.
type agent struct {
	cmd    *exec.Cmd
	stdin  io.Closer
	rpc    *plugin.RPC
	emit   func(harness.Event)
	done   chan struct{} // closed when the process has ended
	ctx    context.Context
	cancel context.CancelFunc
	stderr plugin.Tail
	dir    string
	model  string

	// mu guards the state below. Events are emitted with it held, so they keep
	// the order of the updates and of answers.
	mu       sync.Mutex
	session  string
	busy     bool     // a prompt is in flight
	queued   []string // input that arrived during the prompt
	turn     int
	open     map[string]harness.Item // streamed items that the turn's end has not finished
	tools    map[string]*tool        // by tool call ID
	children map[string]string       // child session ID to the tool item that started it
	requests map[string]*request     // open attention items, by item ID
	errors   int
	usage    harness.Usage
}

// tool is a tool call and the session that made it.
type tool struct {
	item    harness.Item
	session string
}

// request is a form waiting for the user, which is how OpenCode asks the
// questions of its question tool.
type request struct {
	id      json.RawMessage // the JSON-RPC request's ID
	items   []harness.Item  // one per form field
	fields  []field
	content map[string]any
	answers int
}

// field is a form field shown as one attention item.
type field struct {
	key     string
	array   bool
	options bool   // the field lists choices
	custom  string // the key of the field for a typed answer, if any
}

func startAgent(a harness.StartArgs, emit func(harness.Event), exited chan<- error) (*agent, error) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return nil, errors.New("OpenCode is missing: install opencode")
	}
	config := map[string]any{}
	if a.Instructions != "" {
		file, err := plugin.WriteFile(filepath.Join(a.StateDir, "opencode-instructions"), a.Agent+".md", []byte(a.Instructions))
		if err != nil {
			return nil, err
		}
		config["instructions"] = []string{file}
	}
	cmd := exec.Command(path, "acp")
	cmd.Dir = a.Dir
	cmd.Env = append(os.Environ(), "OPENCODE_CONFIG_CONTENT="+plugin.JSONString(config))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	ag := &agent{
		cmd: cmd, stdin: stdin, emit: emit, done: make(chan struct{}), dir: a.Dir, model: a.Model, session: a.Conversation,
		open: map[string]harness.Item{}, tools: map[string]*tool{}, children: map[string]string{}, requests: map[string]*request{},
	}
	ag.ctx, ag.cancel = context.WithCancel(context.Background())
	cmd.Stderr = io.MultiWriter(os.Stderr, &ag.stderr)
	// OpenCode's children may keep its stderr open after it ends.
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start opencode: %w", err)
	}
	ag.rpc = plugin.NewRPC(stdout, stdin, ag.handle)
	died := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		ag.cancel()
		close(ag.done)
		reason := "opencode exited"
		if err != nil {
			reason = fmt.Sprintf("opencode exited: %v", err)
		}
		if detail := ag.stderr.Last(); detail != "" {
			reason += ": " + detail
		}
		died <- errors.New(reason)
	}()
	ctx, cancel := context.WithTimeout(ag.ctx, startTimeout)
	defer cancel()
	if err := ag.begin(ctx, a); err != nil {
		ag.stop()
		return nil, err
	}
	go func() { exited <- <-died }()
	return ag, nil
}

// begin starts or resumes the session and selects the model and effort.
func (ag *agent) begin(ctx context.Context, a harness.StartArgs) error {
	var init struct {
		ProtocolVersion   int `json:"protocolVersion"`
		AgentCapabilities struct {
			SessionCapabilities struct {
				Resume *json.RawMessage `json:"resume"`
			} `json:"sessionCapabilities"`
		} `json:"agentCapabilities"`
	}
	if err := ag.call(ctx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo":      map[string]any{"name": "hypercube", "version": "1"},
		// A form request is how OpenCode asks the user a question.
		"clientCapabilities": map[string]any{"elicitation": map[string]any{"form": map[string]any{}}},
	}, &init); err != nil {
		return err
	}
	if init.ProtocolVersion != 1 || a.Conversation != "" && init.AgentCapabilities.SessionCapabilities.Resume == nil {
		return errors.New("this OpenCode does not support ACP 1 with session resume: OpenCode 2.x is required")
	}
	env := []map[string]string{}
	for _, k := range sortedKeys(a.Env) {
		env = append(env, map[string]string{"name": k, "value": a.Env[k]})
	}
	params := map[string]any{"cwd": a.Dir, "mcpServers": []any{map[string]any{"name": "hypercube", "command": a.Hypercube, "args": []string{"mcp"}, "env": env}}}
	var opened struct {
		SessionID     string         `json:"sessionId"`
		ConfigOptions []configOption `json:"configOptions"`
	}
	method := "session/new"
	if a.Conversation != "" {
		method = "session/resume"
		params["sessionId"] = a.Conversation
	}
	if err := ag.call(ctx, method, params, &opened); err != nil {
		if a.Conversation != "" {
			return fmt.Errorf("OpenCode could not resume session %s: %w", a.Conversation, err)
		}
		return err
	}
	if a.Conversation == "" {
		ag.mu.Lock()
		ag.session = opened.SessionID
		ag.mu.Unlock()
	}
	options, err := ag.choose(ctx, opened.ConfigOptions, "model", a.Model)
	if err != nil {
		return err
	}
	// A model without effort levels has no effort option.
	if findOption(options, "effort") != nil || a.Effort != "default" {
		if _, err := ag.choose(ctx, options, "effort", a.Effort); err != nil {
			return fmt.Errorf("model %s: %w", a.Model, err)
		}
	}
	ag.mu.Lock()
	defer ag.mu.Unlock()
	ag.emit(harness.Event{Conversation: ag.session})
	return nil
}

// configOption is a session setting with a list of values. A value is either
// listed directly or inside a group.
type configOption struct {
	ID      string `json:"id"`
	Options []struct {
		Value   string `json:"value"`
		Options []struct {
			Value string `json:"value"`
		} `json:"options"`
	} `json:"options"`
}

func findOption(options []configOption, id string) *configOption {
	for i := range options {
		if options[i].ID == id {
			return &options[i]
		}
	}
	return nil
}

func (o configOption) values() []string {
	var values []string
	for _, v := range o.Options {
		if v.Value != "" {
			values = append(values, v.Value)
		}
		for _, g := range v.Options {
			values = append(values, g.Value)
		}
	}
	return values
}

// choose sets a session option to a value OpenCode offers and returns the
// options that follow, which depend on the model.
func (ag *agent) choose(ctx context.Context, options []configOption, id, value string) ([]configOption, error) {
	o := findOption(options, id)
	if o == nil {
		return nil, fmt.Errorf("OpenCode offers no %s setting", id)
	}
	if !slices.Contains(o.values(), value) {
		hint := ""
		if id == "model" {
			hint = "; use a provider/model ID from `opencode models`"
		}
		return nil, fmt.Errorf("OpenCode does not offer %s %q%s", id, value, hint)
	}
	var result struct {
		ConfigOptions []configOption `json:"configOptions"`
	}
	err := ag.call(ctx, "session/set_config_option", map[string]any{"sessionId": ag.session, "configId": id, "value": value}, &result)
	return result.ConfigOptions, err
}

// call makes an ACP request and decodes its result. A connection that has
// ended is explained by the end of opencode's diagnostics.
func (ag *agent) call(ctx context.Context, method string, params, result any) error {
	raw, err := ag.rpc.Call(ctx, method, params)
	if err != nil {
		select {
		case <-ag.rpc.Done():
			select {
			case <-ag.done:
			case <-time.After(stopTimeout):
			}
			if detail := ag.stderr.Last(); detail != "" {
				err = fmt.Errorf("%w: %s", err, detail)
			}
		default:
		}
		return fmt.Errorf("%s: %w", method, err)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(raw, result)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// idle reports the started agent as waiting for input.
func (ag *agent) idle() {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	ag.emit(harness.Event{Conversation: ag.session, State: harness.Idle})
}

// stop ends opencode: closing its input lets it finish, and it is killed if it
// does not. Its children share the plugin's process group, which Hypercube
// kills.
func (ag *agent) stop() {
	ag.cancel()
	_ = ag.stdin.Close()
	select {
	case <-ag.done:
	case <-time.After(stopTimeout):
		_ = ag.cmd.Process.Kill()
	}
}

// input starts a turn. OpenCode refuses a prompt while one is running, so
// input that arrives during a turn waits and starts the next turn when the
// current one ends.
func (ag *agent) input(text string) error {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	select {
	case <-ag.done:
		return errors.New("opencode is not running")
	default:
	}
	if ag.busy {
		ag.queued = append(ag.queued, text)
		return nil
	}
	ag.busy = true
	ag.emit(harness.Event{State: harness.Working})
	ag.prompt([]string{text})
	return nil
}

// prompt sends one turn made of texts. ag.mu is held.
func (ag *agent) prompt(texts []string) {
	ag.turn++
	blocks := []map[string]string{}
	for _, t := range texts {
		blocks = append(blocks, map[string]string{"type": "text", "text": t})
	}
	session := ag.session
	go func() {
		var res struct {
			StopReason string `json:"stopReason"`
		}
		err := ag.call(ag.ctx, "session/prompt", map[string]any{"sessionId": session, "prompt": blocks}, &res)
		ag.ended(res.StopReason, err)
	}()
}

// ended finishes a turn: nothing is left streaming or waiting. The agent is
// idle once no prompt is in flight and no input waits.
func (ag *agent) ended(stopReason string, err error) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	for id, it := range ag.open {
		it.Append, it.Done = true, true
		ev.Items = append(ev.Items, it)
		delete(ag.open, id)
	}
	ev.Items = append(ev.Items, ag.closeRequests()...)
	// A cancelled turn is the user's own doing.
	text := ""
	switch {
	case ag.ctx.Err() != nil:
		// The agent is stopping.
	case err != nil:
		text = err.Error()
	case stopReason == "max_tokens":
		text = "The model stopped at its output limit."
	case stopReason == "refusal":
		text = "The model refused the request."
	}
	if text != "" {
		ag.errors++
		ev.Items = append(ev.Items, harness.Item{ID: fmt.Sprintf("error-%d", ag.errors), Kind: harness.KindError, Text: text, Done: true})
	}
	if len(ag.queued) > 0 {
		texts := ag.queued
		ag.queued = nil
		ag.prompt(texts)
	} else {
		ag.busy = false
		ev.State = harness.Idle
	}
	ag.emit(ev)
}

func (ag *agent) interrupt() error {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	if !ag.busy {
		return nil
	}
	// OpenCode waits for the answers of forms it has sent.
	answered := map[*request]bool{}
	for _, r := range ag.requests {
		if !answered[r] {
			answered[r] = true
			_ = ag.rpc.Respond(r.id, map[string]any{"action": "cancel"})
		}
	}
	if items := ag.closeRequests(); len(items) > 0 {
		ag.emit(harness.Event{Items: items})
	}
	return ag.rpc.Notify("session/cancel", map[string]any{"sessionId": ag.session})
}

// closeRequests finishes the items of requests that nobody will answer.
func (ag *agent) closeRequests() []harness.Item {
	var items []harness.Item
	for id, r := range ag.requests {
		for _, it := range r.items {
			if it.ID == id {
				it.Done = true
				items = append(items, it)
			}
		}
	}
	clear(ag.requests)
	return items
}

// handle takes what opencode sends: updates, permission requests, and questions
// for the user.
func (ag *agent) handle(m plugin.Message) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	switch m.Method {
	case "session/update":
		ag.update(&ev, m.Params)
	case "session/request_permission":
		ag.permission(m)
	case "elicitation/create":
		ag.form(&ev, m)
	default:
		if len(m.ID) > 0 {
			_ = ag.rpc.RespondError(m.ID, -32601, "method not found")
		}
	}
	if len(ev.Items) > 0 || ev.Usage != nil {
		ag.emit(ev)
	}
}

// update is the part of a session update the translation reads.
type update struct {
	Kind       string          `json:"sessionUpdate"`
	MessageID  string          `json:"messageId"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"toolCallId"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	RawInput   json.RawMessage `json:"rawInput"`
	Used       int             `json:"used"`
	Size       int             `json:"size"`
	Cost       struct {
		Amount float64 `json:"amount"`
	} `json:"cost"`
	Meta struct {
		Child *struct {
			ID       string `json:"id"`
			ParentID string `json:"parentID"`
			Title    string `json:"title"`
		} `json:"opencode/child-session"`
	} `json:"_meta"`
}

// contentBlock is a block of message or tool content. Tool content wraps its
// block, and shows an edit as a diff, which has no text.
type contentBlock struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Content *struct {
		Text string `json:"text"`
	} `json:"content"`
}

func (ag *agent) update(ev *harness.Event, params json.RawMessage) {
	var p struct {
		SessionID string `json:"sessionId"`
		Update    update `json:"update"`
	}
	if json.Unmarshal(params, &p) != nil || p.SessionID != ag.session {
		return
	}
	u := p.Update
	switch u.Kind {
	case "agent_message_chunk", "agent_thought_chunk":
		var c contentBlock
		if json.Unmarshal(u.Content, &c) != nil || c.Type != "text" || c.Text == "" {
			return
		}
		kind := harness.KindText
		if u.Kind == "agent_thought_chunk" {
			kind = harness.KindReasoning
		}
		id := u.MessageID
		if id == "" {
			id = fmt.Sprintf("%s:%d", kind, ag.turn)
		}
		item := harness.Item{ID: id, Parent: ag.parent(u), Kind: kind, Text: c.Text, Append: true}
		ag.open[id] = harness.Item{ID: id, Parent: item.Parent, Kind: kind}
		ev.Items = append(ev.Items, item)
	case "tool_call", "tool_call_update":
		if u.ToolCallID == "" {
			return
		}
		t := ag.tools[u.ToolCallID]
		if t == nil {
			t = &tool{item: harness.Item{ID: u.ToolCallID, Parent: ag.parent(u), Kind: harness.KindTool}}
			if u.Meta.Child != nil {
				t.session = u.Meta.Child.ID
			} else {
				t.session = ag.session
			}
			ag.tools[u.ToolCallID] = t
		}
		if t.item.Done {
			return
		}
		if u.Name != "" {
			t.item.Tool = u.Name
		}
		if len(u.RawInput) > 0 {
			t.item.Input = u.RawInput
		}
		if u.Status == "completed" || u.Status == "failed" {
			t.item.Done, t.item.Failed = true, u.Status == "failed"
			t.item.Output = toolOutput(u.Content)
		}
		ev.Items = append(ev.Items, t.item)
	case "usage_update":
		// A native subagent's requests do not count toward the agent's context.
		if u.Meta.Child != nil || u.Used == 0 {
			return
		}
		ag.usage = harness.Usage{Context: u.Used, Window: u.Size, Cost: u.Cost.Amount}
		usage := ag.usage
		ev.Usage = &usage
	}
}

// toolOutput is the text of a tool's result.
func toolOutput(raw json.RawMessage) string {
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		switch {
		case b.Type == "content" && b.Content != nil:
			parts = append(parts, b.Content.Text)
		case b.Type == "text":
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// parent is the tool item that started the subagent session an update came
// from. A subagent session is named after its tool call's description, and
// the call is still running when the session starts.
func (ag *agent) parent(u update) string {
	c := u.Meta.Child
	if c == nil {
		return ""
	}
	if id, ok := ag.children[c.ID]; ok {
		return id
	}
	var found string
	for id, t := range ag.tools {
		var in struct {
			Description string `json:"description"`
		}
		if t.session == c.ParentID && !t.item.Done && json.Unmarshal(t.item.Input, &in) == nil && in.Description == c.Title && c.Title != "" {
			found = id
		}
	}
	ag.children[c.ID] = found
	return found
}

// permission approves a permission request once, as `opencode --auto` does.
// Permissions that OpenCode's configuration denies are never requested.
func (ag *agent) permission(m plugin.Message) {
	var p struct {
		Options []struct {
			ID   string `json:"optionId"`
			Kind string `json:"kind"`
		} `json:"options"`
	}
	_ = json.Unmarshal(m.Params, &p)
	for _, o := range p.Options {
		if o.Kind == "allow_once" {
			_ = ag.rpc.Respond(m.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": o.ID}})
			return
		}
	}
	_ = ag.rpc.RespondError(m.ID, -32602, "permission request without an allow-once option")
}

// formField is a property of a form's schema.
type formField struct {
	Type        string       `json:"type"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	OneOf       []formOption `json:"oneOf"`
	Items       struct {
		AnyOf []formOption `json:"anyOf"`
	} `json:"items"`
}

type formOption struct {
	Const       string `json:"const"`
	Description string `json:"description"`
}

// form turns the form of OpenCode's question tool into one attention item per
// question. A question's typed answer has a companion field named
// <key>_custom.
func (ag *agent) form(ev *harness.Event, m plugin.Message) {
	keys, props, err := parseForm(m.Params)
	if err != nil {
		_ = ag.rpc.RespondError(m.ID, -32602, err.Error())
		return
	}
	r := &request{id: m.ID, content: map[string]any{}}
	for _, key := range keys {
		f := props[key]
		if base, ok := strings.CutSuffix(key, "_custom"); ok && props[base].Type != "" {
			continue
		}
		options := f.OneOf
		if f.Type == "array" {
			options = f.Items.AnyOf
		}
		if f.Type != "string" && f.Type != "array" {
			// OpenCode ends the turn when a form is cancelled.
			_ = ag.rpc.Respond(m.ID, map[string]any{"action": "cancel"})
			ag.errors++
			ev.Items = append(ev.Items, harness.Item{ID: fmt.Sprintf("error-%d", ag.errors), Kind: harness.KindError, Text: fmt.Sprintf("OpenCode asked for a %s value, which Hypercube cannot show.", f.Type), Done: true})
			return
		}
		text := f.Description
		if text == "" {
			text = f.Title
		}
		fld := field{key: key, array: f.Type == "array", options: len(options) > 0}
		if _, ok := props[key+"_custom"]; ok {
			fld.custom = key + "_custom"
		}
		item := harness.Item{ID: fmt.Sprintf("form:%s/%d", m.ID, len(r.items)), Kind: harness.KindAttention, Free: !fld.options || fld.custom != ""}
		var notes []string
		for _, o := range options {
			item.Choices = append(item.Choices, o.Const)
			if o.Description != "" {
				notes = append(notes, fmt.Sprintf("- %s: %s", o.Const, o.Description))
			}
		}
		if len(notes) > 0 {
			text += "\n\n" + strings.Join(notes, "\n")
		}
		item.Text = text
		r.items = append(r.items, item)
		r.fields = append(r.fields, fld)
	}
	if len(r.items) == 0 {
		_ = ag.rpc.Respond(m.ID, map[string]any{"action": "accept", "content": map[string]any{}})
		return
	}
	for _, it := range r.items {
		ag.requests[it.ID] = r
	}
	ev.Items = append(ev.Items, r.items...)
}

// parseForm reads a form request and returns its properties in the order the
// schema lists them.
func parseForm(params json.RawMessage) ([]string, map[string]formField, error) {
	var p struct {
		Mode   string `json:"mode"`
		Schema struct {
			Properties json.RawMessage `json:"properties"`
		} `json:"requestedSchema"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Mode != "form" {
		return nil, nil, errors.New("unsupported request")
	}
	dec := json.NewDecoder(bytes.NewReader(p.Schema.Properties))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, nil, errors.New("form without fields")
	}
	var keys []string
	props := map[string]formField{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		var f formField
		if err := dec.Decode(&f); err != nil {
			return nil, nil, err
		}
		keys = append(keys, key.(string))
		props[key.(string)] = f
	}
	return keys, props, nil
}

// answer replies to an open question with a choice or text.
func (ag *agent) answer(item, choice, text string) error {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	r := ag.requests[item]
	if r == nil {
		return fmt.Errorf("request %s is not open", item)
	}
	result := choice
	if result == "" {
		result = text
	}
	if result == "" {
		return errors.New("the answer is empty")
	}
	index := 0
	for i, it := range r.items {
		if it.ID == item {
			index = i
		}
	}
	f := r.fields[index]
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
	r.answers++
	if r.answers == len(r.fields) {
		if err := ag.rpc.Respond(r.id, map[string]any{"action": "accept", "content": r.content}); err != nil {
			return err
		}
	}
	done := r.items[index]
	done.Done, done.Answer = true, result
	delete(ag.requests, item)
	ag.emit(harness.Event{Items: []harness.Item{done}})
	return nil
}
