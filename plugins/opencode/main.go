// Command hypercube-harness-opencode runs OpenCode 2.x as a Hypercube agent.
// `serve` keeps one `opencode acp` process alive across turns, speaks the Agent
// Client Protocol to it, and translates its updates into protocol events.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"

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
	conn   *acp.Connection
	taken  chan struct{} // the handler has taken the message it was given
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
	forms    int
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
	reply   chan acp.UnstableCreateElicitationResponse // receives the form's one answer
	items   []harness.Item                             // one per form field
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
	ag.connect(stdout, stdin)
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
		emit: emit, done: make(chan struct{}), taken: make(chan struct{}, 1), dir: a.Dir, model: a.Model, session: a.Conversation,
		open: map[string]harness.Item{}, tools: map[string]*tool{}, children: map[string]string{}, requests: map[string]*request{},
	}
	ag.ctx, ag.cancel = context.WithCancel(context.Background())
	return ag
}

// connect speaks ACP to opencode over its output and input.
func (ag *agent) connect(out io.Reader, in io.Writer) {
	ag.conn = acp.NewConnection(ag.handle, in, &inOrder{r: bufio.NewReaderSize(out, 1<<20), taken: ag.taken})
	// The connection's errors, such as a line that is not JSON, go to stderr.
	ag.conn.SetLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
}

// inOrder passes opencode's messages to the connection one at a time. The
// connection runs each request on a goroutine of its own, so a message waits
// until the agent has taken the request or notification before it. Updates,
// requests, and the events they make keep the order opencode sent them in.
type inOrder struct {
	r       *bufio.Reader
	taken   <-chan struct{}
	waiting bool // the agent has not yet taken the last message
	line    []byte
	err     error
}

func (o *inOrder) Read(p []byte) (int, error) {
	if len(o.line) == 0 {
		if o.err != nil {
			return 0, o.err
		}
		if o.waiting {
			<-o.taken
		}
		o.line, o.err = o.r.ReadBytes('\n')
		if len(o.line) == 0 {
			return 0, o.err
		}
		// The connection gives the agent every request and notification but
		// its own request cancellations.
		var m struct {
			ID     *json.RawMessage `json:"id"`
			Method string           `json:"method"`
		}
		o.waiting = json.Unmarshal(o.line, &m) == nil && m.Method != "" && (m.ID != nil || m.Method != "$/cancel_request")
	}
	n := copy(p, o.line)
	o.line = o.line[n:]
	return n, nil
}

// begin starts or resumes the session and selects the model and effort.
func (ag *agent) begin(ctx context.Context, a harness.StartArgs) error {
	init, err := call[acp.InitializeResponse](ctx, ag, acp.AgentMethodInitialize, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientInfo:      &acp.Implementation{Name: "hypercube", Version: "1"},
		// A form request is how OpenCode asks the user a question.
		ClientCapabilities: acp.ClientCapabilities{Elicitation: &acp.ElicitationCapabilities{Form: &acp.ElicitationFormCapabilities{}}},
	})
	if err != nil {
		return err
	}
	if init.ProtocolVersion != acp.ProtocolVersionNumber || a.Conversation != "" && init.AgentCapabilities.SessionCapabilities.Resume == nil {
		return errors.New("this OpenCode does not support ACP 1 with session resume: OpenCode 2.x is required")
	}
	env := []acp.EnvVariable{}
	for _, k := range sortedKeys(a.Env) {
		env = append(env, acp.EnvVariable{Name: k, Value: a.Env[k]})
	}
	servers := []acp.McpServer{{Stdio: &acp.McpServerStdio{Name: "hypercube", Command: a.Hypercube, Args: []string{"mcp"}, Env: env}}}
	var options []acp.SessionConfigOption
	if a.Conversation != "" {
		resumed, err := call[acp.ResumeSessionResponse](ctx, ag, acp.AgentMethodSessionResume, acp.ResumeSessionRequest{SessionId: acp.SessionId(a.Conversation), Cwd: a.Dir, McpServers: servers})
		if err != nil {
			return fmt.Errorf("OpenCode could not resume session %s: %w", a.Conversation, err)
		}
		options = resumed.ConfigOptions
	} else {
		opened, err := call[acp.NewSessionResponse](ctx, ag, acp.AgentMethodSessionNew, acp.NewSessionRequest{Cwd: a.Dir, McpServers: servers})
		if err != nil {
			return err
		}
		options = opened.ConfigOptions
		ag.mu.Lock()
		ag.session = string(opened.SessionId)
		ag.mu.Unlock()
	}
	options, err = ag.choose(ctx, options, "model", a.Model)
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
func findOption(options []acp.SessionConfigOption, id string) *acp.SessionConfigOptionSelect {
	for _, o := range options {
		if o.Select != nil && string(o.Select.Id) == id {
			return o.Select
		}
	}
	return nil
}

// values lists a setting's values, whether listed directly or in groups.
func values(o *acp.SessionConfigOptionSelect) []string {
	var values []string
	if o.Options.Ungrouped != nil {
		for _, v := range *o.Options.Ungrouped {
			values = append(values, string(v.Value))
		}
	}
	if o.Options.Grouped != nil {
		for _, g := range *o.Options.Grouped {
			for _, v := range g.Options {
				values = append(values, string(v.Value))
			}
		}
	}
	return values
}

// choose sets a session option to a value OpenCode offers and returns the
// options that follow, which depend on the model.
func (ag *agent) choose(ctx context.Context, options []acp.SessionConfigOption, id, value string) ([]acp.SessionConfigOption, error) {
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
	result, err := call[acp.SetSessionConfigOptionResponse](ctx, ag, acp.AgentMethodSessionSetConfigOption, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{SessionId: acp.SessionId(ag.session), ConfigId: acp.SessionConfigId(id), Value: acp.SessionConfigValueId(value)},
	})
	return result.ConfigOptions, err
}

// call makes an ACP request and returns its result. A connection that has
// ended is explained by the end of opencode's diagnostics.
func call[T any](ctx context.Context, ag *agent, method string, params any) (T, error) {
	result, err := acp.SendRequest[T](ag.conn, ctx, method, params)
	if err == nil {
		return result, nil
	}
	select {
	case <-ag.conn.Done():
		err = errors.New("connection closed")
		select {
		case <-ag.done:
		case <-time.After(stopTimeout):
		}
		if detail := ag.stderr.Last(); detail != "" {
			err = fmt.Errorf("%w: %s", err, detail)
		}
	default:
		var re *acp.RequestError
		if errors.As(err, &re) {
			err = fmt.Errorf("%s (code %d)", re.Message, re.Code)
		}
	}
	return result, fmt.Errorf("%s: %w", method, err)
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
	blocks := []acp.ContentBlock{}
	for _, t := range texts {
		blocks = append(blocks, acp.TextBlock(t))
	}
	session := acp.SessionId(ag.session)
	go func() {
		res, err := call[acp.PromptResponse](ag.ctx, ag, acp.AgentMethodSessionPrompt, acp.PromptRequest{SessionId: session, Prompt: blocks})
		ag.ended(res.StopReason, err)
	}()
}

// ended finishes a turn: nothing is left streaming or waiting. The agent is
// idle once no prompt is in flight and no input waits.
func (ag *agent) ended(stopReason acp.StopReason, err error) {
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
	case stopReason == acp.StopReasonMaxTokens:
		text = "The model stopped at its output limit."
	case stopReason == acp.StopReasonRefusal:
		text = "The model refused the request."
	}
	if text != "" {
		ev.Items = append(ev.Items, ag.errorItem(text))
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

// errorItem is a finished error item. ag.mu is held.
func (ag *agent) errorItem(text string) harness.Item {
	ag.errors++
	return harness.Item{ID: fmt.Sprintf("error-%d", ag.errors), Kind: harness.KindError, Text: text, Done: true}
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
			r.reply <- acp.UnstableCreateElicitationResponse{Cancel: &acp.UnstableCreateElicitationCancel{Action: "cancel"}}
		}
	}
	if items := ag.closeRequests(); len(items) > 0 {
		ag.emit(harness.Event{Items: items})
	}
	return ag.conn.SendNotification(ag.ctx, acp.AgentMethodSessionCancel, acp.CancelNotification{SessionId: acp.SessionId(ag.session)})
}

// closeRequests finishes the items of requests that nobody will answer.
// OpenCode no longer waits for them, so their requests stay unanswered.
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
// for the user. A form's answer is returned once the user has given it.
func (ag *agent) handle(ctx context.Context, method string, params json.RawMessage) (any, *acp.RequestError) {
	reply, result, err := ag.take(method, params)
	select {
	case ag.taken <- struct{}{}:
	default: // the reader waits for one message at a time
	}
	if reply == nil {
		return result, err
	}
	select {
	case r := <-reply:
		return r, nil
	case <-ctx.Done():
		return nil, acp.NewInternalError(map[string]any{"error": "the form was not answered"})
	}
}

// take handles a message as far as it can without waiting for the user. It
// returns the channel that receives a form's answer, or the message's result.
func (ag *agent) take(method string, params json.RawMessage) (<-chan acp.UnstableCreateElicitationResponse, any, *acp.RequestError) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	var reply <-chan acp.UnstableCreateElicitationResponse
	var result any
	var err *acp.RequestError
	switch method {
	case acp.ClientMethodSessionUpdate:
		ag.update(&ev, params)
	case acp.ClientMethodSessionRequestPermission:
		result, err = permission(params)
	case acp.ClientMethodElicitationCreate:
		reply, result, err = ag.form(&ev, params)
	default:
		err = acp.NewMethodNotFound(method)
	}
	if len(ev.Items) > 0 || ev.Usage != nil {
		ag.emit(ev)
	}
	return reply, result, err
}

// childSession is OpenCode's note on an update from a native subagent's
// session.
type childSession struct {
	ID       string `json:"id"`
	ParentID string `json:"parentID"`
	Title    string `json:"title"`
}

// child reads the child session from an update's _meta.
func child(meta map[string]any) *childSession {
	v, ok := meta["opencode/child-session"]
	if !ok {
		return nil
	}
	raw, err := json.Marshal(v)
	var c childSession
	if err != nil || json.Unmarshal(raw, &c) != nil {
		return nil
	}
	return &c
}

func (ag *agent) update(ev *harness.Event, params json.RawMessage) {
	var p acp.SessionNotification
	if json.Unmarshal(params, &p) != nil || string(p.SessionId) != ag.session {
		return
	}
	u := p.Update
	switch {
	case u.AgentMessageChunk != nil:
		c := u.AgentMessageChunk
		ag.chunk(ev, harness.KindText, c.MessageId, c.Content, c.Meta)
	case u.AgentThoughtChunk != nil:
		c := u.AgentThoughtChunk
		ag.chunk(ev, harness.KindReasoning, c.MessageId, c.Content, c.Meta)
	case u.ToolCall != nil:
		c := u.ToolCall
		status := c.Status
		ag.toolCall(ev, params, string(c.ToolCallId), &status, c.Content, c.Meta)
	case u.ToolCallUpdate != nil:
		c := u.ToolCallUpdate
		ag.toolCall(ev, params, string(c.ToolCallId), c.Status, c.Content, c.Meta)
	case u.UsageUpdate != nil:
		c := u.UsageUpdate
		// A native subagent's requests do not count toward the agent's context.
		if child(c.Meta) != nil || c.Used == 0 {
			return
		}
		ag.usage = harness.Usage{Context: c.Used, Window: c.Size}
		if c.Cost != nil {
			ag.usage.Cost = c.Cost.Amount
		}
		usage := ag.usage
		ev.Usage = &usage
	}
}

// chunk extends a streamed message or reasoning item.
func (ag *agent) chunk(ev *harness.Event, kind string, messageID *string, content acp.ContentBlock, meta map[string]any) {
	if content.Text == nil || content.Text.Text == "" {
		return
	}
	id := fmt.Sprintf("%s:%d", kind, ag.turn)
	if messageID != nil && *messageID != "" {
		id = *messageID
	}
	parent := ag.parent(child(meta))
	ev.Items = append(ev.Items, harness.Item{ID: id, Parent: parent, Kind: kind, Text: content.Text.Text, Append: true})
	ag.open[id] = harness.Item{ID: id, Parent: parent, Kind: kind}
}

// toolCall reports a tool call or an update to it. OpenCode names the tool in
// a field of its own, and its input is kept as OpenCode wrote it.
func (ag *agent) toolCall(ev *harness.Event, params json.RawMessage, id string, status *acp.ToolCallStatus, content []acp.ToolCallContent, meta map[string]any) {
	if id == "" {
		return
	}
	var raw struct {
		Update struct {
			Name     string          `json:"name"`
			RawInput json.RawMessage `json:"rawInput"`
		} `json:"update"`
	}
	_ = json.Unmarshal(params, &raw)
	c := child(meta)
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
	if raw.Update.Name != "" {
		t.item.Tool = raw.Update.Name
	}
	if len(raw.Update.RawInput) > 0 {
		t.item.Input = raw.Update.RawInput
	}
	if status != nil && (*status == acp.ToolCallStatusCompleted || *status == acp.ToolCallStatusFailed) {
		t.item.Done, t.item.Failed = true, *status == acp.ToolCallStatusFailed
		t.item.Output = toolOutput(content)
	}
	ev.Items = append(ev.Items, t.item)
}

// toolOutput is the text of a tool's result. An edit is shown as a diff, which
// has no text.
func toolOutput(content []acp.ToolCallContent) string {
	var parts []string
	for _, c := range content {
		if c.Content != nil && c.Content.Content.Text != nil {
			parts = append(parts, c.Content.Content.Text.Text)
		}
	}
	return strings.Join(parts, "\n")
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
func permission(params json.RawMessage) (any, *acp.RequestError) {
	var p acp.RequestPermissionRequest
	_ = json.Unmarshal(params, &p)
	for _, o := range p.Options {
		if o.Kind == acp.PermissionOptionKindAllowOnce {
			return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{
				Selected: &acp.RequestPermissionOutcomeSelected{Outcome: "selected", OptionId: o.OptionId},
			}}, nil
		}
	}
	return nil, &acp.RequestError{Code: -32602, Message: "permission request without an allow-once option"}
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
// question, and returns the channel that receives the form's answer. A
// question's typed answer has a companion field named <key>_custom.
func (ag *agent) form(ev *harness.Event, params json.RawMessage) (<-chan acp.UnstableCreateElicitationResponse, any, *acp.RequestError) {
	var req acp.UnstableCreateElicitationRequest
	if json.Unmarshal(params, &req) != nil || req.Form == nil {
		return nil, nil, &acp.RequestError{Code: -32602, Message: "unsupported request"}
	}
	props := map[string]formField{}
	for key, v := range req.Form.RequestedSchema.Properties {
		raw, err := json.Marshal(v)
		var f formField
		if err == nil {
			err = json.Unmarshal(raw, &f)
		}
		if err != nil {
			return nil, nil, &acp.RequestError{Code: -32602, Message: err.Error()}
		}
		props[key] = f
	}
	keys, err := fieldOrder(params)
	if err != nil || len(keys) != len(props) {
		return nil, nil, &acp.RequestError{Code: -32602, Message: "form without fields"}
	}
	r := &request{reply: make(chan acp.UnstableCreateElicitationResponse, 1), content: map[string]any{}}
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
			ev.Items = append(ev.Items, ag.errorItem(fmt.Sprintf("OpenCode asked for a %s value, which Hypercube cannot show.", f.Type)))
			return nil, acp.UnstableCreateElicitationResponse{Cancel: &acp.UnstableCreateElicitationCancel{Action: "cancel"}}, nil
		}
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
	ag.forms++
	if len(r.items) == 0 {
		return nil, acp.UnstableCreateElicitationResponse{Accept: &acp.UnstableCreateElicitationAccept{Action: "accept", Content: map[string]any{}}}, nil
	}
	for _, it := range r.items {
		ag.requests[it.ID] = r
	}
	ev.Items = append(ev.Items, r.items...)
	return r.reply, nil, nil
}

// fieldOrder returns the keys of a form's properties in the order the schema
// lists them, which is the order of the questions.
func fieldOrder(params json.RawMessage) ([]string, error) {
	var p struct {
		Schema struct {
			Properties json.RawMessage `json:"properties"`
		} `json:"requestedSchema"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(p.Schema.Properties))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("form without fields")
	}
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, err
		}
		keys = append(keys, key.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return keys, nil
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
		r.reply <- acp.UnstableCreateElicitationResponse{Accept: &acp.UnstableCreateElicitationAccept{Action: "accept", Content: r.content}}
	}
	done := r.items[index]
	done.Done, done.Answer = true, result
	delete(ag.requests, item)
	ag.emit(harness.Event{Items: []harness.Item{done}})
	return nil
}
