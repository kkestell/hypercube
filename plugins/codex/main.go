// Command hypercube-harness-codex runs Codex as a Hypercube agent. `serve`
// keeps one `codex app-server` process alive across turns, speaks its
// JSON-RPC protocol, and translates its notifications into protocol events.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// Older unprefixed references belong to the retired private home. The
	// native home may contain a stale source copy with the same thread ID.
	nativeConversationPrefix = "native:"
)

func main() {
	plugin.Plugin{Product: "Codex", Executable: "codex", Serve: serve, Models: models}.Main()
}

func serve(in io.Reader, out io.Writer) error {
	return plugin.Serve(in, out, func(a harness.StartArgs, emit func(harness.Event)) (plugin.Agent, error) {
		ag, err := startAgent(a, emit)
		if err != nil {
			return plugin.Agent{}, err
		}
		started := plugin.Agent{
			Process: ag.proc, Conversation: nativeConversationPrefix + ag.thread,
			Configure: ag.configure, Input: ag.input, Interrupt: ag.interrupt, Answer: ag.answer, Stop: ag.stop,
		}
		if ag.selection != (harness.Selection{Model: a.Model, Effort: a.Effort}) {
			started.Selection = &ag.selection
		}
		return started, nil
	})
}

// agent is a running `codex app-server` process and what its notifications
// have told us.
type agent struct {
	proc   *plugin.Process
	rpc    *plugin.RPC
	emit   func(harness.Event)
	ctx    context.Context
	cancel context.CancelFunc

	// mu guards the state below. Events are emitted with it held, so they keep
	// the order of the notifications and of answers.
	mu             sync.Mutex
	selection      harness.Selection
	thread         string
	turn           string                  // the main turn in progress
	starting       bool                    // turn/start for the main thread is in flight
	interruptStart bool                    // interrupt the main turn once turn/start returns its ID
	ended          string                  // the last main turn that completed
	final          []string                // the main turn's final answer messages so far
	finished       map[string]bool         // items that have completed
	open           map[string]harness.Item // streamed items that have not completed
	running        map[string]harness.Item // tool items that have not completed
	children       map[string]childThread  // native subagents, by thread ID
	limits         [2]harness.Limit        // general primary and secondary windows
	requests       plugin.Requests[request]
	answering      *request // async question whose answer is being sent
	errs           plugin.Errors
}

type childThread struct {
	parent string // the transcript item that started it
	turn   string // its current turn, if any
	ended  string // its most recently completed turn
}

// launch starts `codex app-server` in dir.
func launch(dir string, emit func(harness.Event)) (*agent, error) {
	cmd := exec.Command("codex", "app-server")
	cmd.Dir = dir
	p, err := plugin.StartProcess("Codex", cmd)
	if err != nil {
		return nil, err
	}
	ag := &agent{
		proc: p, emit: emit, finished: map[string]bool{},
		open: map[string]harness.Item{}, running: map[string]harness.Item{}, children: map[string]childThread{},
	}
	ag.ctx, ag.cancel = context.WithCancel(context.Background())
	ag.rpc = plugin.NewRPC(p.Stdin, ag.handle)
	go func() { ag.rpc.End(p.Read(ag.rpc.Line)) }()
	return ag, nil
}

func startAgent(a harness.StartArgs, emit func(harness.Event)) (*agent, error) {
	ag, err := launch(a.Dir, emit)
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

// connect initializes the connection and starts or resumes the thread. The
// thread runs with an automatic approval reviewer in a workspace-write sandbox
// with network access, in a trusted folder, and with its Git directories
// writable so that a child session can commit.
func (ag *agent) connect(ctx context.Context, a harness.StartArgs) error {
	thread := ""
	if a.Conversation != "" || a.Fork != "" {
		var native bool
		thread, native = strings.CutPrefix(cmp.Or(a.Conversation, a.Fork), nativeConversationPrefix)
		if !native {
			return errors.New("this saved Codex conversation predates the native storage integration and cannot resume safely; its transcript and files are preserved; start a new session")
		}
	}
	if err := ag.initialize(ctx); err != nil {
		return err
	}
	selection, err := ag.resolve(ctx, harness.Selection{Model: a.Model, Effort: a.Effort})
	if err != nil {
		return err
	}
	ag.mu.Lock()
	ag.selection = selection
	ag.mu.Unlock()
	// Codex names a folder by its resolved path when it asks whether to trust it.
	wd, err := filepath.EvalSymlinks(a.Dir)
	if err != nil {
		return err
	}
	sandbox := map[string]any{"network_access": true}
	if roots := gitDirs(a.Dir); len(roots) > 0 {
		sandbox["writable_roots"] = roots
	}
	params := map[string]any{
		"cwd": a.Dir, "model": selection.Model, "approvalPolicy": "on-request", "approvalsReviewer": "auto_review", "sandbox": "workspace-write",
		"developerInstructions": a.Instructions,
		"config": map[string]any{
			"sandbox_workspace_write": sandbox,
			"projects":                map[string]any{wd: map[string]any{"trust_level": "trusted"}},
			"mcp_servers": map[string]any{"hypercube": map[string]any{
				"command": a.Hypercube, "args": []string{"mcp"}, "env": a.Env, "default_tools_approval_mode": "approve",
			}},
		},
	}
	if selection.Effort != "" {
		params["config"].(map[string]any)["model_reasoning_effort"] = selection.Effort
	}
	method := "thread/start"
	if a.Conversation != "" {
		method = "thread/resume"
		params["threadId"] = thread
		params["excludeTurns"] = true
	} else if a.Fork != "" {
		method = "thread/fork"
		params["threadId"] = thread
		params["excludeTurns"] = true
	}
	var opened struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := ag.call(ctx, method, params, &opened); err != nil {
		if a.Conversation != "" {
			return fmt.Errorf("Codex could not resume thread %s: %w", thread, err)
		}
		return err
	}
	if opened.Thread.ID == "" {
		return errors.New("Codex returned no thread")
	}
	ag.mu.Lock()
	defer ag.mu.Unlock()
	ag.thread = opened.Thread.ID
	ag.emit(harness.Event{Conversation: nativeConversationPrefix + ag.thread})
	return nil
}

// gitDirs returns the Git directories of the repository that holds dir: the
// sandbox keeps Git metadata read-only unless it is a writable root itself. A
// worktree's metadata is in its own Git directory and in its parent
// checkout's.
func gitDirs(dir string) []string {
	cmd := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir", "--git-dir")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	dirs := strings.Split(strings.TrimSpace(string(out)), "\n")
	roots := dirs[:1]
	if len(dirs) > 1 && dirs[1] != dirs[0] {
		roots = append(roots, dirs[1])
	}
	return roots
}

// call makes a request and decodes its result.
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

// stop gives up the calls in flight and ends codex.
func (ag *agent) stop() {
	ag.cancel()
	ag.proc.Stop()
}

// userInput is a prompt's text, when it has any, and its images as data URLs.
func userInput(text string, images []harness.Image) []map[string]any {
	var input []map[string]any
	if text != "" || len(images) == 0 {
		input = append(input, map[string]any{"type": "text", "text": text})
	}
	for _, img := range images {
		input = append(input, map[string]any{"type": "image", "url": "data:" + img.MIMEType + ";base64," + img.Data})
	}
	return input
}

// configure retains the settings for the next turn/start; steering keeps the
// current turn's settings. An empty effort selects the model's default, since
// turn/start keeps the previous effort when it is omitted.
func (ag *agent) configure(selection harness.Selection) (*harness.Selection, error) {
	ctx, cancel := context.WithTimeout(ag.ctx, callTimeout)
	defer cancel()
	selection, err := ag.resolve(ctx, selection)
	if err != nil {
		return nil, err
	}
	ag.mu.Lock()
	ag.selection = selection
	ag.mu.Unlock()
	return &selection, nil
}

func (ag *agent) input(text string, images []harness.Image) error {
	ag.mu.Lock()
	thread := ag.thread
	ag.mu.Unlock()
	return ag.inputThread(thread, text, images)
}

// inputThread steers the named thread's turn in progress, or starts a turn. A
// steer can arrive after the turn has ended but before its completion reaches
// us; the input then starts a turn.
func (ag *agent) inputThread(thread, text string, images []harness.Image) error {
	select {
	case <-ag.rpc.Done():
		return errors.New("codex is not running")
	default:
	}
	ag.mu.Lock()
	main := thread == ag.thread
	turn, selection := ag.turn, ag.selection
	if !main {
		turn = ag.children[thread].turn
	}
	ag.mu.Unlock()
	if turn != "" {
		ctx, cancel := context.WithTimeout(ag.ctx, callTimeout)
		err := ag.call(ctx, "turn/steer", map[string]any{"threadId": thread, "expectedTurnId": turn, "input": userInput(text, images)}, nil)
		cancel()
		if err == nil {
			ag.mu.Lock()
			ag.send(harness.Event{Items: ag.closeAsyncQuestions(thread)})
			ag.mu.Unlock()
			return nil
		}
		// Codex answers a steer after the turn ended with the invalid request
		// error that its other refusals use too, so the thread's status decides.
		ag.mu.Lock()
		current := ag.turn
		if !main {
			current = ag.children[thread].turn
		}
		ag.mu.Unlock()
		if current == turn && !ag.threadIdle(thread) {
			return err
		}
	}
	var started struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	ctx, cancel := context.WithTimeout(ag.ctx, callTimeout)
	defer cancel()
	params := map[string]any{"threadId": thread, "input": userInput(text, images)}
	if main {
		params["model"] = selection.Model
		if selection.Effort != "" {
			params["effort"] = selection.Effort
		}
		ag.mu.Lock()
		ag.starting = true
		ag.mu.Unlock()
	}
	err := ag.call(ctx, "turn/start", params, &started)
	ag.mu.Lock()
	interrupted := false
	if main {
		interrupted = ag.interruptStart
		ag.starting, ag.interruptStart = false, false
	}
	if err != nil {
		ag.mu.Unlock()
		return err
	}
	var ev harness.Event
	ev.Items = ag.closeAsyncQuestions(thread)
	if main {
		ag.startTurn(&ev, started.Turn.ID)
	} else {
		child := ag.children[thread]
		if started.Turn.ID != child.ended {
			child.turn = started.Turn.ID
			ag.children[thread] = child
		}
	}
	ag.send(ev)
	ag.mu.Unlock()
	if interrupted {
		return ag.interrupt()
	}
	return nil
}

// threadIdle reports whether codex says the thread has no turn running. A
// thread that cannot be read is not idle.
func (ag *agent) threadIdle(thread string) bool {
	var read struct {
		Thread struct {
			Status struct {
				Type string `json:"type"`
			} `json:"status"`
		} `json:"thread"`
	}
	ctx, cancel := context.WithTimeout(ag.ctx, callTimeout)
	defer cancel()
	if err := ag.call(ctx, "thread/read", map[string]any{"threadId": thread}, &read); err != nil {
		return false
	}
	return read.Thread.Status.Type == "idle"
}

func (ag *agent) send(ev harness.Event) {
	if len(ev.Items) > 0 || ev.State != "" || ev.Usage != nil || len(ev.Limits) > 0 {
		ag.emit(ev)
	}
}

func (ag *agent) interrupt() error {
	ag.mu.Lock()
	thread, turn := ag.thread, ag.turn
	var ev harness.Event
	// Codex waits for the answers of requests it has sent.
	ev.Items = ag.closeRequests(blockingRequest, true)
	ag.send(ev)
	if ag.starting {
		ag.interruptStart = true
		ag.mu.Unlock()
		return nil
	}
	ag.mu.Unlock()
	if turn == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ag.ctx, callTimeout)
	defer cancel()
	return ag.call(ctx, "turn/interrupt", map[string]any{"threadId": thread, "turnId": turn}, nil)
}
