// Command hypercube-harness-opencode runs OpenCode 2.x as a Hypercube agent.
// `serve` keeps one `opencode acp` process alive across turns, speaks the Agent
// Client Protocol to it, and translates its updates into protocol events.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

const (
	startTimeout = 60 * time.Second
	stopTimeout  = 3 * time.Second

	// protocolVersion is the version of the Agent Client Protocol spoken here.
	protocolVersion = 1
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
	busy     bool  // a prompt is in flight
	queued   []any // prompt content that arrived during the prompt
	turn     int
	open     map[string]harness.Item // streamed items that the turn's end has not finished
	tools    map[string]*tool        // by tool call ID
	children map[string]string       // child session ID to the tool item that started it
	requests map[string]*request     // open attention items, by item ID
	forms    int
	errors   plugin.Errors
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
	id      json.RawMessage // the form request's JSON-RPC ID
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
	ag := newAgent(a, emit)
	ag.cmd, ag.stdin = cmd, stdin
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

func newAgent(a harness.StartArgs, emit func(harness.Event)) *agent {
	ag := &agent{
		emit: emit, done: make(chan struct{}), dir: a.Dir, model: a.Model, session: a.Conversation,
		open: map[string]harness.Item{}, tools: map[string]*tool{}, children: map[string]string{}, requests: map[string]*request{},
	}
	ag.ctx, ag.cancel = context.WithCancel(context.Background())
	return ag
}

// initializeResult is the part of OpenCode's answer to initialize that the
// plugin checks.
type initializeResult struct {
	ProtocolVersion   int `json:"protocolVersion"`
	AgentCapabilities struct {
		SessionCapabilities struct {
			Resume *struct{} `json:"resume"`
		} `json:"sessionCapabilities"`
	} `json:"agentCapabilities"`
}

// sessionResult is OpenCode's answer to a request that opens a session or
// changes its settings.
type sessionResult struct {
	SessionID     string         `json:"sessionId"`
	ConfigOptions []configOption `json:"configOptions"`
}

// configOption is a session setting. A setting with a list of values has the
// type "select".
type configOption struct {
	ID      string        `json:"id"`
	Type    string        `json:"type"`
	Options []configValue `json:"options"`
}

// configValue is one value of a setting, or a group of values.
type configValue struct {
	Value   string        `json:"value"`
	Options []configValue `json:"options"`
}

// envVariable is an environment variable of an MCP server.
type envVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// begin starts or resumes the session and selects the model and effort.
func (ag *agent) begin(ctx context.Context, a harness.StartArgs) error {
	var init initializeResult
	err := ag.call(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"clientInfo":      map[string]any{"name": "hypercube", "version": "1"},
		// A form request is how OpenCode asks the user a question.
		"clientCapabilities": map[string]any{"elicitation": map[string]any{"form": map[string]any{}}},
	}, &init)
	if err != nil {
		return err
	}
	if init.ProtocolVersion != protocolVersion || a.Conversation != "" && init.AgentCapabilities.SessionCapabilities.Resume == nil {
		return errors.New("this OpenCode does not support ACP 1 with session resume: OpenCode 2.x is required")
	}
	env := []envVariable{}
	for _, k := range slices.Sorted(maps.Keys(a.Env)) {
		env = append(env, envVariable{Name: k, Value: a.Env[k]})
	}
	servers := []any{map[string]any{"name": "hypercube", "command": a.Hypercube, "args": []string{"mcp"}, "env": env}}
	var opened sessionResult
	if a.Conversation != "" {
		err := ag.call(ctx, "session/resume", map[string]any{"sessionId": a.Conversation, "cwd": a.Dir, "mcpServers": servers}, &opened)
		if err != nil {
			return fmt.Errorf("OpenCode could not resume session %s: %w", a.Conversation, err)
		}
	} else {
		if err := ag.call(ctx, "session/new", map[string]any{"cwd": a.Dir, "mcpServers": servers}, &opened); err != nil {
			return err
		}
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

// findOption returns the session setting with a list of values that has the
// given ID.
func findOption(options []configOption, id string) *configOption {
	for i, o := range options {
		if o.Type == "select" && o.ID == id {
			return &options[i]
		}
	}
	return nil
}

// values lists a setting's values, whether listed directly or in groups.
func values(o *configOption) []string {
	var values []string
	for _, v := range o.Options {
		if v.Value != "" {
			values = append(values, v.Value)
		}
		for _, grouped := range v.Options {
			values = append(values, grouped.Value)
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
	if !slices.Contains(values(o), value) {
		hint := ""
		if id == "model" {
			hint = "; use a provider/model ID from `opencode models`"
		}
		return nil, fmt.Errorf("OpenCode does not offer %s %q%s", id, value, hint)
	}
	var result sessionResult
	err := ag.call(ctx, "session/set_config_option", map[string]any{"sessionId": ag.session, "configId": id, "value": value}, &result)
	return result.ConfigOptions, err
}

// call makes an ACP request and decodes its result into result, unless that
// is nil. A connection that has ended is explained by the end of opencode's
// diagnostics.
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
func (ag *agent) input(text string, images []harness.Image) error {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	select {
	case <-ag.rpc.Done():
		return errors.New("opencode is not running")
	default:
	}
	if ag.busy {
		ag.queued = append(ag.queued, blocks(text, images)...)
		return nil
	}
	ag.busy = true
	ag.emit(harness.Event{State: harness.Working})
	ag.prompt(blocks(text, images))
	return nil
}

// textBlock and imageBlock are the content blocks of a prompt.
type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type imageBlock struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

// blocks is a message as prompt content: its text, then its images.
func blocks(text string, images []harness.Image) []any {
	var out []any
	if text != "" || len(images) == 0 {
		out = append(out, textBlock{Type: "text", Text: text})
	}
	for _, im := range images {
		out = append(out, imageBlock{Type: "image", Data: im.Data, MIMEType: im.MIMEType})
	}
	return out
}

// prompt sends one turn made of content blocks. ag.mu is held.
func (ag *agent) prompt(blocks []any) {
	ag.turn++
	params := map[string]any{"sessionId": ag.session, "prompt": blocks}
	go func() {
		var res struct {
			StopReason string `json:"stopReason"`
		}
		err := ag.call(ag.ctx, "session/prompt", params, &res)
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
		ev.Items = append(ev.Items, ag.errors.Item(text))
	}
	if len(ag.queued) > 0 {
		blocks := ag.queued
		ag.queued = nil
		ag.prompt(blocks)
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
	if items := ag.closeRequests(); len(items) > 0 {
		ag.emit(harness.Event{Items: items})
	}
	return ag.rpc.Notify("session/cancel", map[string]any{"sessionId": ag.session})
}

// formReply answers a form: "accept" with the answers, or "cancel".
type formReply struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

// closeRequests finishes the items of requests that nobody will answer, and
// cancels each request once.
func (ag *agent) closeRequests() []harness.Item {
	var items []harness.Item
	cancelled := map[*request]bool{}
	for id, r := range ag.requests {
		if !cancelled[r] {
			cancelled[r] = true
			_ = ag.rpc.Respond(r.id, formReply{Action: "cancel"})
		}
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
// for the user. It runs on the connection's reading goroutine, so it answers
// what it can at once and leaves a form to be answered by the user.
func (ag *agent) handle(m plugin.Message) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	switch {
	case m.Method == "session/update":
		ag.update(&ev, m.Params)
	case m.ID == nil:
		// Other notifications carry nothing the plugin shows.
	case m.Method == "session/request_permission":
		result, err := permission(m.Params)
		if err != nil {
			_ = ag.rpc.RespondError(m.ID, -32602, err.Error())
		} else {
			_ = ag.rpc.Respond(m.ID, result)
		}
	case m.Method == "elicitation/create":
		ag.form(&ev, m.ID, m.Params)
	default:
		_ = ag.rpc.RespondError(m.ID, -32601, "Method not found")
	}
	if len(ev.Items) > 0 || ev.Usage != nil {
		ag.emit(ev)
	}
}

// childSession is OpenCode's note on an update from a native subagent's
// session.
type childSession struct {
	ID       string `json:"id"`
	ParentID string `json:"parentID"`
	Title    string `json:"title"`
}

// sessionUpdate is the part of a session/update notification that the plugin
// reads. Content is one content block for a chunk and a list of tool call
// content for a tool call.
type sessionUpdate struct {
	SessionID string `json:"sessionId"`
	Update    struct {
		SessionUpdate string          `json:"sessionUpdate"`
		MessageID     string          `json:"messageId"`
		Content       json.RawMessage `json:"content"`
		ToolCallID    string          `json:"toolCallId"`
		Name          string          `json:"name"`
		Status        string          `json:"status"`
		RawInput      json.RawMessage `json:"rawInput"`
		Used          int             `json:"used"`
		Size          int             `json:"size"`
		Cost          *struct {
			Amount float64 `json:"amount"`
		} `json:"cost"`
		Meta struct {
			Child *childSession `json:"opencode/child-session"`
		} `json:"_meta"`
	} `json:"update"`
}

// contentBlock is a block of a message or of a tool's result.
type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

// toolContent is an entry of a tool call's content: a content block, or
// another type such as a diff.
type toolContent struct {
	Type    string       `json:"type"`
	Content contentBlock `json:"content"`
}

func (ag *agent) update(ev *harness.Event, params json.RawMessage) {
	var p sessionUpdate
	if json.Unmarshal(params, &p) != nil || p.SessionID != ag.session {
		return
	}
	u := p.Update
	switch u.SessionUpdate {
	case "agent_message_chunk", "agent_thought_chunk":
		kind := harness.KindText
		if u.SessionUpdate == "agent_thought_chunk" {
			kind = harness.KindReasoning
		}
		var content contentBlock
		if json.Unmarshal(u.Content, &content) != nil {
			return
		}
		ag.chunk(ev, kind, u.MessageID, content, u.Meta.Child)
	case "tool_call", "tool_call_update":
		var content []toolContent
		if len(u.Content) > 0 && json.Unmarshal(u.Content, &content) != nil {
			return
		}
		ag.toolCall(ev, u.ToolCallID, u.Name, u.Status, u.RawInput, content, u.Meta.Child)
	case "usage_update":
		// A native subagent's requests do not count toward the agent's context.
		if u.Meta.Child != nil || u.Used == 0 {
			return
		}
		ag.usage = harness.Usage{Context: u.Used, Window: u.Size}
		if u.Cost != nil {
			ag.usage.Cost = u.Cost.Amount
		}
		usage := ag.usage
		ev.Usage = &usage
	}
}

// chunk extends a streamed message or reasoning item.
func (ag *agent) chunk(ev *harness.Event, kind, messageID string, content contentBlock, c *childSession) {
	if content.Type != "text" || content.Text == "" {
		return
	}
	id := fmt.Sprintf("%s:%d", kind, ag.turn)
	if messageID != "" {
		id = messageID
	}
	parent := ag.parent(c)
	ev.Items = append(ev.Items, harness.Item{ID: id, Parent: parent, Kind: kind, Text: content.Text, Append: true})
	ag.open[id] = harness.Item{ID: id, Parent: parent, Kind: kind}
}

// toolCall reports a tool call or an update to it. OpenCode names the tool in
// a field of its own, and its input is kept as OpenCode wrote it.
func (ag *agent) toolCall(ev *harness.Event, id, name, status string, input json.RawMessage, content []toolContent, c *childSession) {
	if id == "" {
		return
	}
	t := ag.tools[id]
	if t == nil {
		t = &tool{item: harness.Item{ID: id, Parent: ag.parent(c), Kind: harness.KindTool}, session: ag.session}
		if c != nil {
			t.session = c.ID
		}
		ag.tools[id] = t
	}
	if t.item.Done {
		return
	}
	if name != "" {
		t.item.Tool = name
		// OpenCode names an MCP tool <server>_<tool>. Only the server this
		// plugin configures has a known name to split at.
		if tool, ok := strings.CutPrefix(name, "hypercube_"); ok {
			t.item.Server, t.item.Tool = "hypercube", tool
		}
	}
	if len(input) > 0 {
		t.item.Input = input
	}
	if status == "completed" || status == "failed" {
		t.item.Done, t.item.Failed = true, status == "failed"
		t.item.Output, t.item.Images = toolOutput(content)
	}
	ev.Items = append(ev.Items, t.item)
}

// toolOutput is the text and images of a tool's result. An edit is shown as a
// diff, which has neither.
func toolOutput(content []toolContent) (string, []harness.Image) {
	var parts []string
	var images []harness.Image
	for _, c := range content {
		if c.Type != "content" {
			continue
		}
		switch b := c.Content; b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "image":
			images = append(images, harness.Image{MIMEType: b.MIMEType, Data: b.Data})
		}
	}
	return strings.Join(parts, "\n"), images
}

// parent is the tool item that started the subagent session an update came
// from. A subagent session is named after its tool call's description, and
// the call is still running when the session starts.
func (ag *agent) parent(c *childSession) string {
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
		item := harness.Item{ID: fmt.Sprintf("form:%d/%d", ag.forms, len(r.items)), Kind: harness.KindAttention, Free: !fld.options || fld.custom != ""}
		var notes []string
		for _, o := range options {
			item.Choices = append(item.Choices, o.Const)
			notes = append(notes, o.Description)
		}
		item.Text = plugin.OptionNotes(text, item.Choices, notes)
		r.items = append(r.items, item)
		r.fields = append(r.fields, fld)
	}
	ag.forms++
	if len(r.items) == 0 {
		_ = ag.rpc.Respond(id, formReply{Action: "accept"})
		return
	}
	for _, it := range r.items {
		ag.requests[it.ID] = r
	}
	ev.Items = append(ev.Items, r.items...)
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
		_ = ag.rpc.Respond(r.id, formReply{Action: "accept", Content: r.content})
	}
	done := r.items[index]
	done.Done, done.Answer = true, result
	delete(ag.requests, item)
	ag.emit(harness.Event{Items: []harness.Item{done}})
	return nil
}
