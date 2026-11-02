// Command hypercube-harness-pi runs Pi as a Hypercube agent. `serve` keeps one
// `pi --mode rpc` process alive across turns, speaks its JSON Lines protocol,
// and translates its events into protocol events.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

const (
	startTimeout = 60 * time.Second
	callTimeout  = 30 * time.Second
)

// extension registers the hypercube MCP server in Pi.
//
//go:embed pi.js
var extension []byte

func main() {
	if len(os.Args) != 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: hypercube-harness-pi serve")
		os.Exit(1)
	}
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-harness-pi:", err)
		os.Exit(1)
	}
}

func serve(in io.Reader, out io.Writer) error {
	return plugin.Serve(in, out, func(a harness.StartArgs, emit func(harness.Event)) (plugin.Agent, error) {
		ag, err := startAgent(a, emit)
		if err != nil {
			return plugin.Agent{}, err
		}
		return plugin.Agent{
			Process: ag.proc, Conversation: ag.conversation(),
			Input: ag.input, Interrupt: ag.interrupt, Answer: ag.answer, Stop: ag.proc.Stop,
		}, nil
	})
}

// agent is a running `pi --mode rpc` process and what its events have told us.
type agent struct {
	proc  *plugin.Process
	stdin io.Writer
	emit  func(harness.Event)
	ended chan struct{} // closed when pi's output has ended
	exit  error         // why it ended; set before ended is closed

	writing sync.Mutex // serializes writes to stdin
	callMu  sync.Mutex
	next    int
	calls   map[string]chan response

	// mu guards the translation state below. Events are emitted with it held,
	// so they keep the order of Pi's events and of answers.
	mu        sync.Mutex
	session   string
	announced bool // the conversation was reported; Pi saves it from its first message
	window    int
	context   int
	cost      float64
	messages  int                     // assistant messages seen
	blocks    map[string]*block       // text and reasoning of the message in progress
	tools     map[string]harness.Item // tool items that have not completed
	requests  map[string]*request     // open extension dialogs, by item ID
	last      assistant               // the latest assistant message of the run
	errs      plugin.Errors
}

// response is Pi's answer to a command.
type response struct {
	err  string
	data json.RawMessage
}

// block is a text or reasoning block of an assistant message. It is shown
// once it has text beyond whitespace.
type block struct {
	kind  string
	text  string
	shown bool
}

// assistant is what a finished assistant message says about how it ended.
type assistant struct{ stop, err string }

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

func startAgent(a harness.StartArgs, emit func(harness.Event)) (*agent, error) {
	provider, model, ok := strings.Cut(a.Model, "/")
	if !ok {
		return nil, fmt.Errorf("model %q is not provider/id; see pi --list-models", a.Model)
	}
	file, err := plugin.WriteFile(filepath.Join(a.StateDir, "pi-extension"), "index.js", extension)
	if err != nil {
		return nil, err
	}
	argv := []string{"--mode", "rpc", "--provider", provider, "--model", model, "-e", file}
	if a.Effort != "" {
		argv = append(argv, "--thinking", a.Effort)
	}
	if a.Instructions != "" {
		argv = append(argv, "--append-system-prompt", a.Instructions)
	}
	// Pi creates a new session under an ID it cannot find; connect checks that
	// a resumed one exists.
	if a.Conversation != "" {
		argv = append(argv, "--session-id", a.Conversation)
	}
	cmd := exec.Command("pi", argv...)
	cmd.Dir = a.Dir
	mcp := map[string]any{"command": a.Hypercube, "args": []string{"mcp"}, "env": a.Env, "exposure": "direct"}
	cmd.Env = append(os.Environ(), "HYPERCUBE_MCP="+plugin.JSONString(mcp))
	p, err := plugin.StartProcess("Pi", cmd)
	if err != nil {
		return nil, err
	}
	ag := &agent{
		proc: p, stdin: p.Stdin, emit: emit, ended: make(chan struct{}), calls: map[string]chan response{},
		blocks: map[string]*block{}, tools: map[string]harness.Item{}, requests: map[string]*request{},
	}
	go func() { ag.end(p.Read(ag.line)) }()
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	if err := ag.connect(ctx, a); err != nil {
		p.Stop()
		return nil, err
	}
	return ag, nil
}

// end fails the calls waiting on pi, and later ones, with the reason its
// output ended.
func (ag *agent) end(err error) {
	ag.exit = err
	close(ag.ended)
}

// connect checks what Pi started with: the model, which Pi replaces with
// another when the requested name is partial, and a resumed session, which
// Pi has saved once it holds a message.
func (ag *agent) connect(ctx context.Context, a harness.StartArgs) error {
	var state struct {
		Model *struct {
			Provider      string `json:"provider"`
			ID            string `json:"id"`
			ContextWindow int    `json:"contextWindow"`
		} `json:"model"`
		SessionFile string `json:"sessionFile"`
		SessionID   string `json:"sessionId"`
	}
	if err := ag.call(ctx, "get_state", nil, &state); err != nil {
		return err
	}
	selected := "no model"
	if state.Model != nil {
		selected = state.Model.Provider + "/" + state.Model.ID
	}
	if selected != a.Model {
		return fmt.Errorf("Pi selected %s instead of requested %s; use a full provider/model ID from pi --list-models", selected, a.Model)
	}
	window, session := state.Model.ContextWindow, state.SessionID
	var cost float64
	var contextTokens int
	announced := a.Conversation != ""
	if announced {
		if _, err := os.Stat(state.SessionFile); err != nil {
			return fmt.Errorf("Pi has no saved session %s", a.Conversation)
		}
		var stats struct {
			Cost         float64 `json:"cost"`
			ContextUsage *struct {
				Tokens int `json:"tokens"`
			} `json:"contextUsage"`
		}
		if err := ag.call(ctx, "get_session_stats", nil, &stats); err != nil {
			return err
		}
		cost = stats.Cost
		if stats.ContextUsage != nil {
			contextTokens = stats.ContextUsage.Tokens
		}
	}
	ag.mu.Lock()
	defer ag.mu.Unlock()
	ag.window, ag.session = window, session
	ag.cost, ag.announced, ag.context = cost, announced, contextTokens
	return nil
}

// conversation is the session to report once started. A new session is
// reported once Pi has saved it.
func (ag *agent) conversation() string {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	if !ag.announced {
		return ""
	}
	return ag.session
}

// write sends one record to pi.
func (ag *agent) write(v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ag.writing.Lock()
	defer ag.writing.Unlock()
	if _, err := ag.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("pi is not running: %w", err)
	}
	return nil
}

// call sends a command and decodes the data of its response. A failed write
// returns its error. A process that ends before it responds returns why it
// ended.
func (ag *agent) call(ctx context.Context, command string, params map[string]any, result any) error {
	reply := make(chan response, 1)
	ag.callMu.Lock()
	ag.next++
	id := strconv.Itoa(ag.next)
	ag.calls[id] = reply
	ag.callMu.Unlock()
	forget := func() {
		ag.callMu.Lock()
		delete(ag.calls, id)
		ag.callMu.Unlock()
	}
	rec := map[string]any{"id": id, "type": command}
	maps.Copy(rec, params)
	if err := ag.write(rec); err != nil {
		forget()
		return err
	}
	select {
	case r := <-reply:
		if r.err != "" {
			return fmt.Errorf("%s: %s", command, r.err)
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(r.data, result)
	case <-ag.ended:
		forget()
		return fmt.Errorf("%s: %w", command, ag.exit)
	case <-ctx.Done():
		forget()
		return fmt.Errorf("%s: %w", command, ctx.Err())
	}
}

// input sends the text and images as a prompt. Pi starts a turn, or, during
// one, queues them to steer it: Pi reads them before its next model request.
func (ag *agent) input(text string, images []harness.Image) error {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	params := map[string]any{"message": text, "streamingBehavior": "steer"}
	if len(images) > 0 {
		blocks := make([]map[string]string, len(images))
		for i, im := range images {
			blocks[i] = map[string]string{"type": "image", "data": im.Data, "mimeType": im.MIMEType}
		}
		params["images"] = blocks
	}
	return ag.call(ctx, "prompt", params, nil)
}

// interrupt gives up the open dialogs, which Pi would wait on, and aborts the
// run.
func (ag *agent) interrupt() error {
	ag.mu.Lock()
	ag.send(harness.Event{Items: ag.closeRequests(true)})
	ag.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	return ag.call(ctx, "abort", nil, nil)
}

func (ag *agent) send(ev harness.Event) {
	if ev.Conversation != "" || ev.State != "" || len(ev.Items) > 0 || ev.Usage != nil {
		ag.emit(ev)
	}
}

// answer replies to an open dialog: a choice, or typed text for a dialog that
// takes text.
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
	reply := map[string]any{"type": "extension_ui_response", "id": item}
	switch {
	case r.method == "confirm":
		if result != yes && result != no {
			return fmt.Errorf("unknown choice %q", result)
		}
		reply["confirmed"] = result == yes
	case len(r.item.Choices) > 0 && !slices.Contains(r.item.Choices, result):
		return fmt.Errorf("unknown choice %q", result)
	default:
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

// line translates one line of pi's output.
func (ag *agent) line(line []byte) {
	var h struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(line, &h); err != nil {
		fmt.Fprintf(os.Stderr, "hypercube-harness-pi: unreadable pi output: %v\n", err)
		return
	}
	if h.Type == "response" {
		ag.response(line, h.ID)
		return
	}
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	switch h.Type {
	case "agent_start":
		ev.State = harness.Working
	case "agent_settled":
		ag.settled(&ev, line)
	case "message_start":
		var m struct {
			Message struct{ Role string } `json:"message"`
		}
		if json.Unmarshal(line, &m) == nil && m.Message.Role == "assistant" {
			ag.messages++
		}
	case "message_update":
		ag.update(&ev, line)
	case "message_end":
		ag.messageEnd(&ev, line)
	case "tool_execution_start":
		ag.toolStart(&ev, line)
	case "tool_execution_end":
		ag.toolEnd(&ev, line)
	case "extension_ui_request":
		ag.dialog(&ev, line)
	case "extension_error":
		var m struct{ Error string }
		if json.Unmarshal(line, &m) == nil {
			ag.fail(&ev, "Extension error: "+m.Error)
		}
	}
	ag.send(ev)
}

// response hands a command's response to its caller.
func (ag *agent) response(line []byte, id string) {
	var m struct {
		Success bool            `json:"success"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(line, &m); err != nil {
		return
	}
	ag.callMu.Lock()
	reply := ag.calls[id]
	delete(ag.calls, id)
	ag.callMu.Unlock()
	if reply == nil {
		// A parse failure has no ID; a caller that gave up has no channel.
		if !m.Success {
			fmt.Fprintf(os.Stderr, "hypercube-harness-pi: pi rejected a command: %s\n", m.Error)
		}
		return
	}
	r := response{data: m.Data}
	if !m.Success {
		r.err = m.Error
		if r.err == "" {
			r.err = "failed"
		}
	}
	reply <- r
}

func (ag *agent) fail(ev *harness.Event, text string) {
	ev.Items = append(ev.Items, ag.errs.Item(text))
}

// settled ends a run: Pi has nothing more to do on its own. A run that ended
// in a provider error says so; an aborted one is the user's own doing.
func (ag *agent) settled(ev *harness.Event, line []byte) {
	var m struct{ Aborted bool }
	_ = json.Unmarshal(line, &m)
	last := ag.last
	ag.last = assistant{}
	if last.stop == "error" && !m.Aborted {
		text := last.err
		if text == "" {
			text = "The model request failed."
		}
		ag.fail(ev, text)
	}
	ev.State = harness.Idle
}

// update shows text and reasoning as they arrive.
func (ag *agent) update(ev *harness.Event, line []byte) {
	var m struct {
		Update struct {
			Type    string `json:"type"`
			Index   int    `json:"contentIndex"`
			Delta   string `json:"delta"`
			Content string `json:"content"`
		} `json:"assistantMessageEvent"`
	}
	if json.Unmarshal(line, &m) != nil {
		return
	}
	u := m.Update
	id := fmt.Sprintf("m%d:%d", ag.messages, u.Index)
	switch u.Type {
	case "text_start":
		ag.blocks[id] = &block{kind: harness.KindText}
	case "thinking_start":
		ag.blocks[id] = &block{kind: harness.KindReasoning}
	case "text_delta", "thinking_delta":
		b := ag.blocks[id]
		if b == nil {
			return
		}
		b.text += u.Delta
		if strings.TrimSpace(b.text) == "" {
			return
		}
		item := harness.Item{ID: id, Kind: b.kind, Text: u.Delta, Append: true}
		if !b.shown {
			item.Text, item.Append = b.text, false
			b.shown = true
		}
		ev.Items = append(ev.Items, item)
	case "text_end", "thinking_end":
		b := ag.blocks[id]
		if b == nil {
			return
		}
		delete(ag.blocks, id)
		if b.shown || strings.TrimSpace(u.Content) != "" {
			ev.Items = append(ev.Items, harness.Item{ID: id, Kind: b.kind, Text: u.Content, Done: true})
		}
	}
}

// messageEnd records a finished assistant message: how it ended, and the
// usage of its request. Pi saves the session from its first message.
func (ag *agent) messageEnd(ev *harness.Event, line []byte) {
	var m struct {
		Message struct {
			Role         string `json:"role"`
			StopReason   string `json:"stopReason"`
			ErrorMessage string `json:"errorMessage"`
			Usage        struct {
				TotalTokens int `json:"totalTokens"`
				Cost        struct {
					Total float64 `json:"total"`
				} `json:"cost"`
			} `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &m) != nil {
		return
	}
	switch m.Message.Role {
	case "user":
	case "assistant":
		// A message cut short has no end for its blocks.
		for id, b := range ag.blocks {
			if b.shown {
				ev.Items = append(ev.Items, harness.Item{ID: id, Kind: b.kind, Append: true, Done: true})
			}
			delete(ag.blocks, id)
		}
		ag.last = assistant{m.Message.StopReason, m.Message.ErrorMessage}
		ag.cost += m.Message.Usage.Cost.Total
		if t := m.Message.Usage.TotalTokens; t > 0 {
			ag.context = t
		}
		ev.Usage = &harness.Usage{Context: ag.context, Window: ag.window, Cost: ag.cost}
	default:
		return
	}
	if !ag.announced {
		ag.announced = true
		ev.Conversation = ag.session
	}
}

func (ag *agent) toolStart(ev *harness.Event, line []byte) {
	var m struct {
		ID   string          `json:"toolCallId"`
		Name string          `json:"toolName"`
		Args json.RawMessage `json:"args"`
	}
	if json.Unmarshal(line, &m) != nil {
		return
	}
	item := harness.Item{ID: m.ID, Kind: harness.KindTool, Input: m.Args}
	item.Server, item.Tool = plugin.MCPTool(m.Name)
	ag.tools[m.ID] = item
	ev.Items = append(ev.Items, item)
}

func (ag *agent) toolEnd(ev *harness.Event, line []byte) {
	var m struct {
		ID     string `json:"toolCallId"`
		Result struct {
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Data     string `json:"data"`
				MimeType string `json:"mimeType"`
			} `json:"content"`
		} `json:"result"`
		IsError bool `json:"isError"`
	}
	if json.Unmarshal(line, &m) != nil {
		return
	}
	item, ok := ag.tools[m.ID]
	if !ok {
		return
	}
	delete(ag.tools, m.ID)
	var parts []string
	for _, c := range m.Result.Content {
		switch c.Type {
		case "text":
			parts = append(parts, c.Text)
		case "image":
			item.Images = append(item.Images, harness.Image{MIMEType: c.MimeType, Data: c.Data})
		default:
			parts = append(parts, "["+c.Type+"]")
		}
	}
	item.Output, item.Failed, item.Done = strings.Join(parts, "\n"), m.IsError, true
	ev.Items = append(ev.Items, item)
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
