// Command hypercube-harness-opencode drives a private OpenCode 2.x server.
// Native prompt admission supports steering without cancelling running tools.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

const startTimeout = 60 * time.Second

func main() {
	plugin.Plugin{Product: "OpenCode", Executable: "opencode", Serve: serve, Models: models}.Main()
}

func serve(in io.Reader, out io.Writer) error {
	return plugin.Serve(in, out, func(a harness.StartArgs, emit func(harness.Event)) (plugin.Agent, error) {
		ag, err := startAgent(a, emit)
		if err != nil {
			return plugin.Agent{}, err
		}
		started := plugin.Agent{
			Process: ag.server.proc, Conversation: ag.session,
			Configure: ag.configure, Input: ag.input, Interrupt: ag.interrupt, Answer: ag.answer, Stop: ag.stop,
		}
		if ag.selected != (harness.Selection{Model: a.Model, Effort: a.Effort}) {
			started.Selection = &ag.selected
		}
		return started, nil
	})
}

type agent struct {
	server         *server
	ctx            context.Context
	cancel         context.CancelFunc
	emit           func(harness.Event)
	streamDone     chan struct{}
	catalogChanged chan struct{}

	// mu keeps native events and user answers in transcript order. HTTP calls
	// never hold it: the stream can report execution before admission returns.
	mu       sync.Mutex
	session  string
	pending  map[string]bool
	open     map[string]harness.Item
	tools    map[string]harness.Item
	last     []string
	finish   string
	window   int
	used     int
	cost     float64
	requests plugin.Requests[request]
	errors   plugin.Errors

	selected harness.Selection // start and ordered configure commands own this
}

func newAgent(emit func(harness.Event)) *agent {
	ctx, cancel := context.WithCancel(context.Background())
	return &agent{ctx: ctx, cancel: cancel, emit: emit, catalogChanged: make(chan struct{}, 1),
		pending: map[string]bool{}, open: map[string]harness.Item{}, tools: map[string]harness.Item{}}
}

func startAgent(a harness.StartArgs, emit func(harness.Event)) (*agent, error) {
	if a.Fork != "" && a.Dir != a.ForkDir {
		return nil, errors.New("OpenCode forks require the parent's folder, and a fork of a session in its own worktree gets a new worktree; choose fresh context")
	}
	config := map[string]any{}
	if a.Instructions != "" {
		file, err := plugin.WriteFile(filepath.Join(a.StateDir, "opencode-instructions"), a.Session+".md", []byte(a.Instructions))
		if err != nil {
			return nil, err
		}
		config["instructions"] = []string{file}
	}
	// This server belongs to this launch, so its MCP environment cannot leak
	// into another session or change the user's saved OpenCode configuration.
	config["mcp"] = map[string]any{"servers": map[string]any{"hypercube": map[string]any{
		"type": "local", "command": []string{a.MCP}, "environment": a.Env,
	}}}
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	ag := newAgent(emit)
	ctx, cancel := context.WithTimeout(ag.ctx, startTimeout)
	defer cancel()
	ag.server, err = launch(ctx, a.Dir, "OPENCODE_CONFIG_CONTENT="+string(encoded))
	if err == nil {
		err = ag.begin(ctx, a)
	}
	if err != nil {
		ag.stop()
		return nil, err
	}
	return ag, nil
}

type sessionInfo struct {
	ID       string   `json:"id"`
	Model    modelRef `json:"model"`
	Location struct {
		Directory string `json:"directory"`
	} `json:"location"`
}

func sessionPath(id string) string { return "/api/session/" + id }

func (ag *agent) begin(ctx context.Context, a harness.StartArgs) error {
	var opened sessionInfo
	var err error
	switch {
	case a.Conversation != "":
		err = ag.server.call(ctx, "GET", sessionPath(a.Conversation), nil, &opened)
		if err != nil {
			return fmt.Errorf("OpenCode could not resume session %s: %w", a.Conversation, err)
		}
	case a.Fork != "":
		err = ag.server.call(ctx, "POST", sessionPath(a.Fork)+"/fork", map[string]any{}, &opened)
		if err != nil {
			return fmt.Errorf("OpenCode could not fork session %s: %w", a.Fork, err)
		}
		if opened.ID == a.Fork {
			return errors.New("OpenCode returned no independent fork")
		}
	default:
		err = ag.server.call(ctx, "POST", "/api/session", map[string]any{"location": map[string]string{"directory": a.Dir}}, &opened)
	}
	if err != nil {
		return err
	}
	if opened.ID == "" {
		return errors.New("OpenCode returned no conversation ID")
	}
	if opened.Location.Directory != a.Dir {
		return fmt.Errorf("OpenCode session is in %s, not %s", opened.Location.Directory, a.Dir)
	}
	ag.session = opened.ID
	ag.selected = harness.Selection{Model: opened.Model.key(), Effort: opened.Model.Variant}
	if err := ag.subscribe(ctx); err != nil {
		return err
	}
	if err := ag.awaitCatalog(ctx); err != nil {
		return err
	}
	if _, err := ag.selectSettings(ctx, harness.Selection{Model: a.Model, Effort: a.Effort}); err != nil {
		return err
	}
	return nil
}

func (ag *agent) stop() {
	ag.cancel()
	if ag.server != nil {
		ag.server.stop()
	}
	if ag.streamDone != nil {
		<-ag.streamDone
	}
}
