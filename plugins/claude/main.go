// Command hypercube-harness-claude runs Claude Code as a Hypercube agent.
// `serve` keeps one `claude -p` process with stream-JSON input and output
// alive across turns, and translates its stream into protocol events.
package main

import (
	"bufio"
	"bytes"
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

const (
	startTimeout = 60 * time.Second
	stopTimeout  = 3 * time.Second
)

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
	return plugin.Serve(in, out, func(a harness.StartArgs, emit func(harness.Event), exited chan<- error) (plugin.Agent, error) {
		c, err := startClaude(a, emit, exited)
		if err != nil {
			return plugin.Agent{}, err
		}
		return plugin.Agent{Input: c.input, Interrupt: c.interrupt, Answer: c.answer, Idle: c.idle, Stop: c.stop}, nil
	})
}

// claude is a running `claude -p` process and what its stream has told us.
type claude struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	emit  func(harness.Event)
	done  chan struct{} // closed when the process has ended

	writing sync.Mutex // serializes writes to stdin
	counter int        // control request IDs; guarded by writing

	// mu guards the translation state below. Events are emitted with it held,
	// so they keep the order of the stream and of answers.
	mu           sync.Mutex
	ready        chan struct{} // closed by the response to initialize
	initError    string        // why initialize failed
	resultText   string        // text of an error result before ready
	conversation string
	model        string
	blocks       map[string]int          // content blocks seen per assistant message
	open         map[string]harness.Item // streamed items that no complete block has finished
	tools        map[string]harness.Item // tool items, by tool use ID
	requests     map[string]*request     // open permission requests, by item ID
	errors       plugin.Errors
	usage        harness.Usage
}

// request is a can_use_tool control request waiting for the user: a tool
// permission, or the questions of AskUserQuestion.
type request struct {
	id        string          // the control request's ID
	input     json.RawMessage // the tool input, which an allowing reply carries back
	items     []harness.Item  // one per question, or the one permission prompt
	questions []string
	answers   map[string]string
}

func startClaude(a harness.StartArgs, emit func(harness.Event), exited chan<- error) (*claude, error) {
	path, err := exec.LookPath("claude")
	if err != nil {
		return nil, errors.New("Claude Code is missing: install claude")
	}
	mcp := map[string]any{"mcpServers": map[string]any{"hypercube": map[string]any{"command": a.Hypercube, "args": []string{"mcp"}, "env": a.Env}}}
	// The prompt snapshot would freeze the instructions of the first launch.
	argv := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--permission-prompt-tool", "stdio", "--permission-mode", "auto", "--system-prompt-snapshot", "off",
		"--mcp-config", plugin.JSONString(mcp), "--model", a.Model, "--effort", a.Effort}
	if a.Instructions != "" {
		argv = append(argv, "--append-system-prompt", a.Instructions)
	}
	if a.Conversation != "" {
		argv = append(argv, "--resume", a.Conversation)
	}
	cmd := exec.Command(path, argv...)
	cmd.Dir = a.Dir
	// Claude reports idle and working states only to a host that asks for them.
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SDK_READS_SESSION_STATE=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	// Claude's children may keep its pipes open after it ends, so the reader
	// owns the read end and gives up shortly after claude exits.
	stdout, pipe, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	var stderr plugin.Tail
	cmd.Stdout = pipe
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	cmd.WaitDelay = time.Second
	c := &claude{
		cmd: cmd, stdin: stdin, emit: emit, done: make(chan struct{}), ready: make(chan struct{}), conversation: a.Conversation,
		blocks: map[string]int{}, open: map[string]harness.Item{}, tools: map[string]harness.Item{},
		requests: map[string]*request{},
	}
	if err := cmd.Start(); err != nil {
		stdout.Close()
		pipe.Close()
		return nil, fmt.Errorf("start claude: %w", err)
	}
	pipe.Close()
	read := make(chan struct{})
	go func() {
		defer close(read)
		reader := bufio.NewReaderSize(stdout, 1<<20)
		for {
			line, err := reader.ReadBytes('\n')
			if len(bytes.TrimSpace(line)) > 0 {
				c.line(line)
			}
			if err != nil {
				return
			}
		}
	}()
	died := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = stdout.SetReadDeadline(time.Now().Add(time.Second))
		<-read
		stdout.Close()
		close(c.done)
		reason := "claude exited"
		if err != nil {
			reason = fmt.Sprintf("claude exited: %v", err)
		}
		if detail := stderr.Last(); detail != "" {
			reason += ": " + detail
		}
		died <- errors.New(reason)
	}()
	if err := c.send(map[string]any{"type": "control_request", "request_id": "initialize", "request": map[string]any{"subtype": "initialize"}}); err != nil {
		c.stop()
		return nil, err
	}
	select {
	case <-c.ready:
		if reason := c.failure(); reason != "" {
			c.stop()
			return nil, errors.New(reason)
		}
	case err := <-died:
		if text := c.failure(); text != "" {
			err = errors.New(text)
		}
		return nil, err
	case <-time.After(startTimeout):
		c.stop()
		return nil, errors.New("claude did not become ready")
	}
	go func() { exited <- <-died }()
	return c, nil
}

// idle reports the started harness as waiting for input.
func (c *claude) idle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.emit(harness.Event{Conversation: c.conversation, State: harness.Idle})
}

func (c *claude) failure() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.initError != "" {
		return c.initError
	}
	return c.resultText
}

// stop ends claude: closing its input lets it finish, and it is killed if it
// does not. Its children share the plugin's process group, which Hypercube
// kills.
func (c *claude) stop() {
	c.writing.Lock()
	_ = c.stdin.Close()
	c.writing.Unlock()
	select {
	case <-c.done:
	case <-time.After(stopTimeout):
		_ = c.cmd.Process.Kill()
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
	r := c.requests[item]
	if r == nil {
		return fmt.Errorf("request %s is not open", item)
	}
	if r.questions != nil {
		return c.answerQuestion(r, item, choice, text)
	}
	return c.answerPermission(r, choice, text)
}

func (c *claude) answerPermission(r *request, choice, text string) error {
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
	c.finish(r, 0, result)
	return nil
}

// answerQuestion records the answer to one question. Claude is answered once
// every question of the request has an answer.
func (c *claude) answerQuestion(r *request, item, choice, text string) error {
	result := cmp.Or(choice, text)
	if result == "" {
		return errors.New("the answer is empty")
	}
	index := slices.IndexFunc(r.items, func(it harness.Item) bool { return it.ID == item })
	r.answers[r.questions[index]] = result
	if len(r.answers) == len(r.questions) {
		input, err := withAnswers(r.input, r.answers)
		if err != nil {
			return err
		}
		if err := c.respond(r.id, map[string]any{"behavior": "allow", "updatedInput": input}); err != nil {
			return err
		}
	}
	c.finish(r, index, result)
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

// finish shows an item of a request as answered and stops tracking it.
func (c *claude) finish(r *request, index int, result string) {
	done := r.items[index]
	done.Done, done.Answer = true, result
	delete(c.requests, done.ID)
	c.emit(harness.Event{Items: []harness.Item{done}})
}

func (c *claude) respond(id string, response any) error {
	return c.send(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": id, "response": response}})
}

// closeRequests finishes the items of requests claude no longer waits on.
func (c *claude) closeRequests(match func(*request) bool) []harness.Item {
	var items []harness.Item
	for id, r := range c.requests {
		if !match(r) {
			continue
		}
		for _, it := range r.items {
			if it.ID == id {
				it.Done = true
				items = append(items, it)
			}
		}
		delete(c.requests, id)
	}
	return items
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
	// Claude names its session at startup but saves it only once a turn
	// runs, so the conversation is reported from a turn's result; an agent
	// that never took a turn has nothing to resume.
	if m.Type == "result" && m.SessionID != "" && m.SessionID != c.conversation {
		c.conversation = m.SessionID
		ev.Conversation = m.SessionID
	}
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
				ev.State = harness.Idle
			}
		}
	case "stream_event":
		if m.Parent == "" {
			c.stream(&ev, m)
		}
	case "assistant":
		c.assistant(&ev, m)
	case "user":
		c.toolResults(&ev, m)
	case "control_request":
		c.control(&ev, m)
	case "control_cancel_request":
		ev.Items = append(ev.Items, c.closeRequests(func(r *request) bool { return r.id == m.RequestID })...)
	case "control_response":
		if m.Response.RequestID == "initialize" {
			if m.Response.Subtype == "error" {
				c.initError = m.Response.Error
			}
			close(c.ready)
		}
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

func (c *claude) assistant(ev *harness.Event, m message) {
	var blocks []block
	if json.Unmarshal(m.Message.Content, &blocks) != nil {
		return
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
			item.Text = plugin.OptionNotes(q.Question, item.Choices, notes)
			r.items = append(r.items, item)
			r.questions = append(r.questions, q.Question)
		}
		if len(r.items) == 0 {
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
		r.items = []harness.Item{{ID: m.RequestID, Kind: harness.KindAttention, Text: text, Choices: []string{allow, deny}, Free: true}}
	}
	for _, it := range r.items {
		c.requests[it.ID] = r
	}
	ev.Items = append(ev.Items, r.items...)
}

// result ends a turn: nothing is left streaming or waiting, and the turn's
// usage is final.
func (c *claude) result(ev *harness.Event, m message) {
	text := m.Result
	if text == "" && len(m.Errors) > 0 {
		text = fmt.Sprint(m.Errors...)
	}
	if text == "" {
		text = m.Subtype
	}
	select {
	case <-c.ready:
	default:
		// A failed start, such as a conversation that cannot be resumed, is
		// reported by the reply to start.
		c.resultText = text
		return
	}
	for id, it := range c.open {
		it.Append, it.Done = true, true
		ev.Items = append(ev.Items, it)
		delete(c.open, id)
	}
	ev.Items = append(ev.Items, c.closeRequests(func(*request) bool { return true })...)
	// An interrupted turn is the user's own doing.
	if m.IsError && !strings.HasPrefix(m.TerminalReason, "aborted") {
		ev.Items = append(ev.Items, c.errors.Item(text))
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
