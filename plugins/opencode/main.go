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
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

const (
	startTimeout = 60 * time.Second

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
	return plugin.Serve(in, out, func(a harness.StartArgs, emit func(harness.Event)) (plugin.Agent, error) {
		ag, err := startAgent(a, emit)
		if err != nil {
			return plugin.Agent{}, err
		}
		return plugin.Agent{
			Process: ag.proc, Conversation: ag.session,
			Input: ag.input, Interrupt: ag.interrupt, Answer: ag.answer, Stop: ag.stop,
		}, nil
	})
}

// agent is a running `opencode acp` process and what its updates have told us.
type agent struct {
	proc   *plugin.Process
	rpc    *plugin.RPC
	emit   func(harness.Event)
	ctx    context.Context
	cancel context.CancelFunc

	// mu guards the state below. Events are emitted with it held, so they keep
	// the order of the updates and of answers.
	mu       sync.Mutex
	session  string
	busy     bool  // a prompt is in flight
	queued   []any // prompt content that arrived during the prompt
	turn     int
	last     string                  // the main session's latest text message, unless a tool call has followed it
	open     map[string]harness.Item // streamed items that the turn's end has not finished
	tools    map[string]*tool        // by tool call ID
	children map[string]string       // child session ID to the tool item that started it
	requests plugin.Requests[request]
	forms    int
	errors   plugin.Errors
}

func startAgent(a harness.StartArgs, emit func(harness.Event)) (*agent, error) {
	config := map[string]any{}
	if a.Instructions != "" {
		file, err := plugin.WriteFile(filepath.Join(a.StateDir, "opencode-instructions"), a.Session+".md", []byte(a.Instructions))
		if err != nil {
			return nil, err
		}
		config["instructions"] = []string{file}
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("opencode", "acp")
	cmd.Dir = a.Dir
	cmd.Env = append(os.Environ(), "OPENCODE_CONFIG_CONTENT="+string(configJSON))
	p, err := plugin.StartProcess("OpenCode", cmd)
	if err != nil {
		return nil, err
	}
	ag := newAgent(a, emit)
	ag.proc = p
	ag.rpc = plugin.NewRPC(p.Stdin, ag.handle)
	go func() { ag.rpc.End(p.Read(ag.rpc.Line)) }()
	ctx, cancel := context.WithTimeout(ag.ctx, startTimeout)
	defer cancel()
	if err := ag.begin(ctx, a); err != nil {
		ag.stop()
		return nil, err
	}
	return ag, nil
}

func newAgent(a harness.StartArgs, emit func(harness.Event)) *agent {
	ag := &agent{
		emit: emit, session: a.Conversation,
		open: map[string]harness.Item{}, tools: map[string]*tool{}, children: map[string]string{},
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
// is nil.
func (ag *agent) call(ctx context.Context, method string, params, result any) error {
	raw, err := ag.rpc.Call(ctx, method, params)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(raw, result)
}

// stop gives up the prompt in flight and ends opencode.
func (ag *agent) stop() {
	ag.cancel()
	ag.proc.Stop()
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
	ag.last = ""
	params := map[string]any{"sessionId": ag.session, "prompt": blocks}
	go func() {
		var res struct {
			StopReason string `json:"stopReason"`
		}
		err := ag.call(ag.ctx, "session/prompt", params, &res)
		ag.ended(res.StopReason, err)
	}()
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
