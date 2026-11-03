// Command hypercube-harness-claude runs Claude Code as a Hypercube agent.
// `serve` keeps one `claude -p` process with stream-JSON input and output
// alive across turns, and translates its stream into protocol events.
package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

const startTimeout = 60 * time.Second

func main() {
	if len(os.Args) != 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: hypercube-harness-claude serve")
		os.Exit(1)
	}
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-harness-claude:", err)
		os.Exit(1)
	}
}

func serve(in io.Reader, out io.Writer) error {
	return plugin.Serve(in, out, func(a harness.StartArgs, emit func(harness.Event)) (plugin.Agent, error) {
		c, err := startClaude(a, emit)
		if err != nil {
			return plugin.Agent{}, err
		}
		return plugin.Agent{
			Process: c.proc, Conversation: a.Conversation,
			Input: c.input, Interrupt: c.interrupt, Answer: c.answer, Stop: c.proc.Stop,
		}, nil
	})
}

// claude is a running `claude -p` process and what its stream has told us.
type claude struct {
	proc  *plugin.Process
	stdin io.Writer
	emit  func(harness.Event)

	writing sync.Mutex // serializes writes to stdin
	counter int        // control request IDs; guarded by writing

	// mu guards the translation state below. Events are emitted with it held,
	// so they keep the order of the stream and of answers.
	mu           sync.Mutex
	conversation string
	model        string
	blocks       map[string]int          // content blocks seen per assistant message
	open         map[string]harness.Item // streamed items that no complete block has finished
	tools        map[string]harness.Item // tool items, by tool use ID
	requests     plugin.Requests[request]
	errors       plugin.Errors
	usage        harness.Usage
	latest       answer   // the main agent's newest message
	final        []string // the text items a successful result confirmed, until idle reports them
}

// answer is the main agent's newest message and the text items its blocks
// became. A successful result settles it as the response to a turn.
type answer struct {
	message string
	texts   []string
	settled bool
}

// request is a can_use_tool control request waiting for the user: a tool
// permission, or the questions of AskUserQuestion.
type request struct {
	id        string          // the control request's ID
	input     json.RawMessage // the tool input, which an allowing reply carries back
	questions []string        // the text of each question, by item; nil for a permission
	answers   map[string]string
}

func startClaude(a harness.StartArgs, emit func(harness.Event)) (*claude, error) {
	mcp := map[string]any{"mcpServers": map[string]any{"hypercube": map[string]any{"command": a.Hypercube, "args": []string{"mcp"}, "env": a.Env}}}
	mcpJSON, err := json.Marshal(mcp)
	if err != nil {
		return nil, err
	}
	// The prompt snapshot would freeze the instructions of the first launch.
	argv := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--permission-prompt-tool", "stdio", "--permission-mode", "auto", "--system-prompt-snapshot", "off",
		"--mcp-config", string(mcpJSON), "--model", a.Model, "--effort", a.Effort}
	if a.Instructions != "" {
		argv = append(argv, "--append-system-prompt", a.Instructions)
	}
	if a.Conversation != "" {
		argv = append(argv, "--resume", a.Conversation)
	}
	cmd := exec.Command("claude", argv...)
	cmd.Dir = a.Dir
	// Claude reports idle and working states only to a host that asks for them.
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SDK_READS_SESSION_STATE=1")
	p, err := plugin.StartProcess("Claude Code", cmd)
	if err != nil {
		return nil, err
	}
	c := &claude{
		proc: p, stdin: p.Stdin, emit: emit, conversation: a.Conversation,
		blocks: map[string]int{}, open: map[string]harness.Item{}, tools: map[string]harness.Item{},
	}
	ready := make(chan error, 1)
	go c.read(ready)
	if err := c.send(map[string]any{"type": "control_request", "request_id": "initialize", "request": map[string]any{"subtype": "initialize"}}); err != nil {
		p.Stop()
		return nil, err
	}
	select {
	case err := <-ready:
		if err != nil {
			p.Stop()
			return nil, err
		}
	case <-time.After(startTimeout):
		p.Stop()
		return nil, errors.New("claude did not become ready")
	}
	return c, nil
}

// Startup states of claude's output.
const (
	starting = iota // waiting for the response to initialize
	started         // translating every line
	failed          // discarding every line
)

// read translates claude's output once claude has started, and sends on
// ready whether it did. During startup, a result line is the reason a start
// fails, such as a conversation that cannot be resumed, and is not
// translated.
func (c *claude) read(ready chan<- error) {
	state := starting
	var failure error
	err := c.proc.Read(func(line []byte) {
		switch state {
		case started:
			c.line(line)
			return
		case failed:
			return
		}
		var m message
		if json.Unmarshal(line, &m) != nil {
			c.line(line) // reports the unreadable line
			return
		}
		switch {
		case m.Type == "control_response" && m.Response.RequestID == "initialize":
			if m.Response.Subtype == "error" {
				failure = errors.New(m.Response.Error)
			}
			state = started
			if failure != nil {
				state = failed
			}
			ready <- failure
		case m.Type == "result":
			failure = errors.New(m.summary())
		default:
			c.line(line)
		}
	})
	if state == starting {
		if failure == nil {
			failure = err
		}
		ready <- failure
	}
}

func (c *claude) send(v any) error {
	c.writing.Lock()
	defer c.writing.Unlock()
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := c.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("claude is not running: %w", err)
	}
	return nil
}

// input sends a prompt: plain text, or content blocks when it has images.
func (c *claude) input(text string, images []harness.Image) error {
	var content any = text
	if len(images) > 0 {
		blocks := make([]map[string]any, 0, len(images)+1)
		if text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": text})
		}
		for _, img := range images {
			blocks = append(blocks, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": img.MIMEType, "data": img.Data}})
		}
		content = blocks
	}
	return c.send(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}})
}

func (c *claude) interrupt() error {
	c.writing.Lock()
	c.counter++
	id := fmt.Sprintf("interrupt-%d", c.counter)
	c.writing.Unlock()
	return c.send(map[string]any{"type": "control_request", "request_id": id, "request": map[string]any{"subtype": "interrupt"}})
}

const (
	allow = "Allow"
	deny  = "Deny"
)

// answer replies to an open request: the choice or text of a question, or
// Allow or Deny, with a reason as text, for a tool permission.
func (c *claude) answer(item, choice, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	open, ok := c.requests.Get(item)
	if !ok {
		return fmt.Errorf("request %s is not open", item)
	}
	if open.Request.questions != nil {
		return c.answerQuestion(open, choice, text)
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
func (c *claude) answerQuestion(open plugin.Open[request], choice, text string) error {
	r := open.Request
	result := cmp.Or(choice, text)
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

// message is the part of a stream line the translation reads.
type message struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	State     string          `json:"state"`
	SessionID string          `json:"session_id"`
	Parent    string          `json:"parent_tool_use_id"`
	Model     string          `json:"model"`
	APIID     string          `json:"api_message_id"`
	Event     json.RawMessage `json:"event"`
	Message   struct {
		ID      string          `json:"id"`
		Content json.RawMessage `json:"content"`
		Usage   struct {
			Input         int `json:"input_tokens"`
			CacheRead     int `json:"cache_read_input_tokens"`
			CacheCreation int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
	Response  struct {
		Subtype   string `json:"subtype"`
		RequestID string `json:"request_id"`
		Error     string `json:"error"`
	} `json:"response"`
	IsError        bool    `json:"is_error"`
	Result         string  `json:"result"`
	Errors         []any   `json:"errors"`
	TerminalReason string  `json:"terminal_reason"`
	Cost           float64 `json:"total_cost_usd"`
	ModelUsage     map[string]struct {
		ContextWindow int `json:"contextWindow"`
	} `json:"modelUsage"`
}

// block is a content block of an assistant or user message.
type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
	Source    struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source"`
}

// image is the picture of an image block, if it is an inline one.
func (b block) image() (harness.Image, bool) {
	if b.Type != "image" || b.Source.Type != "base64" || b.Source.Data == "" || !strings.HasPrefix(b.Source.MediaType, "image/") {
		return harness.Image{}, false
	}
	return harness.Image{MIMEType: b.Source.MediaType, Data: b.Source.Data}, true
}

// line translates one line of claude's output.
func (c *claude) line(line []byte) {
	var m message
	if err := json.Unmarshal(line, &m); err != nil {
		fmt.Fprintf(os.Stderr, "hypercube-harness-claude: unreadable claude output: %v\n", err)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var ev harness.Event
	switch m.Type {
	case "system":
		switch m.Subtype {
		case "init":
			c.model = m.Model
		case "session_state_changed":
			switch m.State {
			case "running":
				ev.State = harness.Working
			case "idle":
				ev.State, ev.Final = harness.Idle, c.final
				c.final = nil
			}
		}
	case "stream_event":
		if m.Parent == "" {
			c.mainMessage(m.APIID)
			c.stream(&ev, m)
		}
	case "assistant":
		c.assistant(&ev, m)
	case "user":
		c.toolResults(&ev, m)
	case "control_request":
		c.control(&ev, m)
	case "control_cancel_request":
		items, _ := c.requests.Close(func(r *request) bool { return r.id == m.RequestID })
		ev.Items = append(ev.Items, items...)
	case "result":
		c.result(&ev, m)
	}
	if ev.Conversation != "" || ev.State != "" || len(ev.Items) > 0 || ev.Usage != nil {
		c.emit(ev)
	}
}

// stream shows text and reasoning as they arrive. Claude's complete blocks
// follow and finish the items.
func (c *claude) stream(ev *harness.Event, m message) {
	var e struct {
		Type  string `json:"type"`
		Index int    `json:"index"`
		Delta struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"delta"`
	}
	if json.Unmarshal(m.Event, &e) != nil || e.Type != "content_block_delta" || m.APIID == "" {
		return
	}
	item := harness.Item{ID: fmt.Sprintf("%s:%d", m.APIID, e.Index), Append: true}
	switch e.Delta.Type {
	case "text_delta":
		item.Kind, item.Text = harness.KindText, e.Delta.Text
	case "thinking_delta":
		item.Kind, item.Text = harness.KindReasoning, e.Delta.Thinking
	}
	if item.Text == "" {
		return
	}
	c.open[item.ID] = harness.Item{ID: item.ID, Kind: item.Kind}
	ev.Items = append(ev.Items, item)
}

// mainMessage notes a record of the main agent's message. A new message
// supersedes a response that an earlier result confirmed.
func (c *claude) mainMessage(id string) {
	if id != "" && id != c.latest.message {
		c.latest = answer{message: id}
		c.final = nil
	}
}

func (c *claude) assistant(ev *harness.Event, m message) {
	var blocks []block
	if json.Unmarshal(m.Message.Content, &blocks) != nil {
		return
	}
	if m.Parent == "" {
		c.mainMessage(m.Message.ID)
	}
	for _, b := range blocks {
		id := fmt.Sprintf("%s:%d", m.Message.ID, c.blocks[m.Message.ID])
		c.blocks[m.Message.ID]++
		_, streamed := c.open[id]
		delete(c.open, id)
		switch b.Type {
		case "text":
			if streamed || strings.TrimSpace(b.Text) != "" {
				ev.Items = append(ev.Items, harness.Item{ID: id, Parent: m.Parent, Kind: harness.KindText, Text: b.Text, Done: true})
				if m.Parent == "" {
					c.latest.texts = append(c.latest.texts, id)
				}
			}
		case "thinking":
			if streamed || strings.TrimSpace(b.Thinking) != "" {
				ev.Items = append(ev.Items, harness.Item{ID: id, Parent: m.Parent, Kind: harness.KindReasoning, Text: b.Thinking, Done: true})
			}
		case "tool_use":
			// AskUserQuestion is shown as its attention requests.
			if b.Name == "AskUserQuestion" {
				continue
			}
			item := harness.Item{ID: b.ID, Parent: m.Parent, Kind: harness.KindTool, Input: b.Input}
			item.Server, item.Tool = plugin.MCPTool(b.Name)
			c.tools[b.ID] = item
			ev.Items = append(ev.Items, item)
		}
	}
	// A native subagent's requests do not count toward the agent's context.
	if u := m.Message.Usage; m.Parent == "" && u.Input+u.CacheRead+u.CacheCreation > 0 {
		c.usage.Context = u.Input + u.CacheRead + u.CacheCreation
		usage := c.usage
		ev.Usage = &usage
	}
}

func (c *claude) toolResults(ev *harness.Event, m message) {
	var blocks []block
	if json.Unmarshal(m.Message.Content, &blocks) != nil {
		return // a prompt, which Hypercube already shows
	}
	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
		}
		item, ok := c.tools[b.ToolUseID]
		if !ok {
			continue
		}
		delete(c.tools, b.ToolUseID)
		item.Output, item.Images = resultText(b.Content)
		item.Failed, item.Done = b.IsError, true
		ev.Items = append(ev.Items, item)
	}
}

// resultText is the text and the images of a tool result, which claude sends
// as a string or as blocks.
func resultText(raw json.RawMessage) (string, []harness.Image) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return string(raw), nil
	}
	var parts []string
	var images []harness.Image
	for _, b := range blocks {
		if img, ok := b.image(); ok {
			images = append(images, img)
		} else if b.Type == "text" {
			parts = append(parts, b.Text)
		} else {
			parts = append(parts, "["+b.Type+"]")
		}
	}
	return strings.Join(parts, "\n"), images
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
			item := harness.Item{ID: fmt.Sprintf("%s/%d", m.RequestID, i), Kind: harness.KindAttention, Text: q.Question, Free: true}
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

// summary is what a result line says about how its turn ended.
func (m message) summary() string {
	if m.Result != "" {
		return m.Result
	}
	if len(m.Errors) > 0 {
		return fmt.Sprint(m.Errors...)
	}
	return m.Subtype
}

// result ends a turn: nothing is left streaming or waiting, and the turn's
// usage is final.
func (c *claude) result(ev *harness.Event, m message) {
	// Claude names its session at startup but saves it only once a turn
	// runs, so the conversation is reported from a turn's result; an agent
	// that never took a turn has nothing to resume.
	if m.SessionID != "" && m.SessionID != c.conversation {
		c.conversation = m.SessionID
		ev.Conversation = m.SessionID
	}
	for id, it := range c.open {
		it.Append, it.Done = true, true
		ev.Items = append(ev.Items, it)
		delete(c.open, id)
	}
	items, _ := c.requests.Close(func(*request) bool { return true })
	ev.Items = append(ev.Items, items...)
	// A successful result confirms the newest main message as the response; a
	// result with no newer message, such as a bookkeeping one, leaves it.
	switch {
	case m.IsError:
		c.final = nil
		c.latest.settled = true
	case !c.latest.settled:
		c.final = slices.Clone(c.latest.texts)
		c.latest.settled = true
	}
	// An interrupted turn is the user's own doing.
	if m.IsError && !strings.HasPrefix(m.TerminalReason, "aborted") {
		ev.Items = append(ev.Items, c.errors.Item(m.summary()))
	}
	window := m.ModelUsage[c.model].ContextWindow
	if len(m.ModelUsage) == 1 {
		for _, u := range m.ModelUsage {
			window = u.ContextWindow
		}
	}
	if window > 0 {
		c.usage.Window = window
	}
	c.usage.Cost = m.Cost
	usage := c.usage
	ev.Usage = &usage
}
