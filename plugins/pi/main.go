// Command hypercube-harness-pi runs Pi as a Hypercube agent. `serve` keeps one
// `pi --mode rpc` process alive across turns, speaks its JSON Lines protocol,
// and translates its events into protocol events.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
			Configure: ag.configure, Input: ag.input, Interrupt: ag.interrupt, Answer: ag.answer, Stop: ag.stop,
		}, nil
	})
}

// agent is a running `pi --mode rpc` process and what its events have told us.
type agent struct {
	proc   *plugin.Process
	stdin  io.Writer
	emit   func(harness.Event)
	ctx    context.Context // ends when pi is stopped
	cancel context.CancelFunc
	ended  chan struct{} // closed when pi's output has ended
	exit   error         // why it ended; set before ended is closed

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
	texts     []string                // text items of the current assistant message
	blocks    map[string]*block       // text and reasoning of the message in progress
	tools     map[string]harness.Item // tool items that have not completed
	requests  map[string]*request     // open extension dialogs, by item ID
	last      assistant               // the latest assistant message of the run
	errs      plugin.Errors
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
	mcpJSON, err := json.Marshal(mcp)
	if err != nil {
		return nil, err
	}
	cmd.Env = append(os.Environ(), "HYPERCUBE_MCP="+string(mcpJSON))
	p, err := plugin.StartProcess("Pi", cmd)
	if err != nil {
		return nil, err
	}
	ag := &agent{
		proc: p, stdin: p.Stdin, emit: emit, ended: make(chan struct{}), calls: map[string]chan response{},
		blocks: map[string]*block{}, tools: map[string]harness.Item{}, requests: map[string]*request{},
	}
	ag.ctx, ag.cancel = context.WithCancel(context.Background())
	go func() { ag.end(p.Read(ag.line)) }()
	ctx, cancel := context.WithTimeout(ag.ctx, startTimeout)
	defer cancel()
	if err := ag.connect(ctx, a); err != nil {
		ag.stop()
		return nil, err
	}
	return ag, nil
}

// stop gives up the calls in flight and ends pi.
func (ag *agent) stop() {
	ag.cancel()
	ag.proc.Stop()
}

// connect checks what Pi started with: the model, which Pi replaces with
// another when the requested name is partial, and a resumed session, which
// Pi has saved once it holds a message.
func (ag *agent) connect(ctx context.Context, a harness.StartArgs) error {
	var state sessionState
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
		var stats sessionStats
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

// input sends the text and images as a prompt. Pi starts a turn, or, during
// one, queues them to steer it: Pi reads them before its next model request.
func (ag *agent) input(text string, images []harness.Image) error {
	ctx, cancel := context.WithTimeout(ag.ctx, callTimeout)
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
	ctx, cancel := context.WithTimeout(ag.ctx, callTimeout)
	defer cancel()
	return ag.call(ctx, "abort", nil, nil)
}
