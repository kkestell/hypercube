// Command hypercube-harness-pi runs Pi as a Hypercube agent. `serve` keeps one
// `pi --mode rpc` process alive across turns, speaks its JSON Lines protocol,
// and translates its events into protocol events.
package main

import (
	"cmp"
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
	plugin.Plugin{Product: "Pi", Executable: "pi", Serve: serve, Models: models}.Main()
}

func serve(in io.Reader, out io.Writer) error {
	return plugin.Serve(in, out, func(a harness.StartArgs, emit func(harness.Event)) (plugin.Agent, error) {
		ag, err := startAgent(a, emit)
		if err != nil {
			return plugin.Agent{}, err
		}
		started := plugin.Agent{
			Process: ag.proc, Conversation: ag.conversation(),
			Configure: ag.configure, Input: ag.input, Interrupt: ag.interrupt, Answer: ag.answer, Stop: ag.stop,
		}
		if ag.started != (harness.Selection{Model: a.Model, Effort: a.Effort}) {
			started.Selection = &ag.started
		}
		return started, nil
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

	started harness.Selection // what Pi started with; set by connect
}

func startAgent(a harness.StartArgs, emit func(harness.Event)) (*agent, error) {
	provider, model, ok := strings.Cut(a.Model, "/")
	if !ok {
		return nil, fmt.Errorf("model %q is not provider/id; see list_models", a.Model)
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
	} else if a.Fork != "" {
		argv = append(argv, "--fork", a.Fork, "--session-id", a.Session)
	}
	command, args, err := plugin.MCPCommand(a)
	if err != nil {
		return nil, err
	}
	mcp := map[string]any{"command": command, "args": args, "env": a.Env, "exposure": "direct"}
	mcpJSON, err := json.Marshal(mcp)
	if err != nil {
		return nil, err
	}
	ag, err := launch(a.Dir, argv, emit, "HYPERCUBE_MCP="+string(mcpJSON))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ag.ctx, startTimeout)
	defer cancel()
	if err := ag.connect(ctx, a); err != nil {
		ag.stop()
		return nil, err
	}
	return ag, nil
}

// launch starts pi in dir with the arguments and environment.
func launch(dir string, argv []string, emit func(harness.Event), env ...string) (*agent, error) {
	cmd := exec.Command("pi", argv...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
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
	return ag, nil
}

// stop gives up the calls in flight and ends pi.
func (ag *agent) stop() {
	ag.cancel()
	ag.proc.Stop()
}

// connect checks what Pi started with: the model, which Pi replaces with
// another when the requested name is partial, the thinking level, which Pi
// lowers to one the model supports, and a resumed session, which Pi has
// saved once it holds a message.
func (ag *agent) connect(ctx context.Context, a harness.StartArgs) error {
	var state sessionState
	if err := ag.call(ctx, "get_state", nil, &state); err != nil {
		return err
	}
	if err := state.check(a.Model); err != nil {
		return err
	}
	if err := ag.checkLevel(ctx, a.Model, a.Effort); err != nil {
		return err
	}
	ag.started = harness.Selection{Model: a.Model, Effort: cmp.Or(state.ThinkingLevel, a.Effort)}
	window, session := state.Model.ContextWindow, state.SessionID
	var cost float64
	var contextTokens int
	announced := a.Conversation != "" || a.Fork != ""
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
