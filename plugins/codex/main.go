// Command hypercube-harness-codex runs Codex as a Hypercube agent. `serve`
// keeps one `codex app-server` process alive across turns, speaks its
// JSON-RPC protocol, and translates its notifications into protocol events.
package main

import (
	"context"
	"encoding/json"
	"errors"
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
	stopTimeout  = 3 * time.Second
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: hypercube-harness-codex serve")
		os.Exit(1)
	}
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-harness-codex:", err)
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

// agent is a running `codex app-server` process and what its notifications
// have told us.
type agent struct {
	cmd    *exec.Cmd
	stdin  io.Closer
	rpc    *plugin.RPC
	emit   func(harness.Event)
	done   chan struct{} // closed when the process has ended
	ctx    context.Context
	cancel context.CancelFunc
	stderr plugin.Tail

	// mu guards the state below. Events are emitted with it held, so they keep
	// the order of the notifications and of answers.
	mu       sync.Mutex
	thread   string
	turn     string                  // the main turn in progress
	ended    string                  // the last main turn that completed
	finished map[string]bool         // items that have completed
	open     map[string]harness.Item // streamed items that have not completed
	running  map[string]harness.Item // tool items that have not completed
	children map[string]string       // subagent thread to the item that started it
	requests map[string]*request     // open attention items, by item ID
	errors   int
}

// request is a server request waiting for the user, with one attention item
// per question. parse converts an answer for the question at its index, if
// set. reply turns the converted answers into the response once every
// question has one; refuse is the response that gives up.
type request struct {
	id     json.RawMessage
	items  []harness.Item
	parse  []func(string) (any, error)
	values []any
	given  int
	reply  func(values []any) any
	refuse any
}

func startAgent(a harness.StartArgs, emit func(harness.Event), exited chan<- error) (*agent, error) {
	path, err := exec.LookPath("codex")
	if err != nil {
		return nil, errors.New("Codex is missing: install codex")
	}
	cmd := exec.Command(path, "app-server")
	cmd.Dir = a.Dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	ag := &agent{
		cmd: cmd, stdin: stdin, emit: emit, done: make(chan struct{}), finished: map[string]bool{},
		open: map[string]harness.Item{}, running: map[string]harness.Item{}, children: map[string]string{}, requests: map[string]*request{},
	}
	ag.ctx, ag.cancel = context.WithCancel(context.Background())
	cmd.Stderr = io.MultiWriter(os.Stderr, &ag.stderr)
	// Codex's children may keep its stderr open after it ends.
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex: %w", err)
	}
	ag.rpc = plugin.NewRPC(stdout, stdin, ag.handle)
	died := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		ag.cancel()
		close(ag.done)
		reason := "codex exited"
		if err != nil {
			reason = fmt.Sprintf("codex exited: %v", err)
		}
		if detail := ag.stderr.Last(); detail != "" {
			reason += ": " + detail
		}
		died <- errors.New(reason)
	}()
	ctx, cancel := context.WithTimeout(ag.ctx, startTimeout)
	defer cancel()
	if err := ag.connect(ctx, a); err != nil {
		ag.stop()
		return nil, err
	}
	go func() { exited <- <-died }()
	return ag, nil
}

// connect initializes the connection and starts or resumes the thread. The
// thread runs with an automatic approval reviewer in a workspace-write sandbox
// with network access, in a trusted folder, and with its Git directories
// writable so that a worker can commit.
func (ag *agent) connect(ctx context.Context, a harness.StartArgs) error {
	// Experimental methods include the questions Codex asks the user.
	if err := ag.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "hypercube", "version": "1"},
		"capabilities": map[string]any{"experimentalApi": true},
	}, nil); err != nil {
		return fmt.Errorf("incompatible Codex app-server; update Codex: %w", err)
	}
	if err := ag.rpc.Notify("initialized", map[string]any{}); err != nil {
		return err
	}
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
		"cwd": a.Dir, "model": a.Model, "approvalPolicy": "on-request", "approvalsReviewer": "auto_review", "sandbox": "workspace-write",
		"developerInstructions": a.Instructions,
		"config": map[string]any{
			"model_reasoning_effort":  a.Effort,
			"sandbox_workspace_write": sandbox,
			"projects":                map[string]any{wd: map[string]any{"trust_level": "trusted"}},
			"mcp_servers": map[string]any{"hypercube": map[string]any{
				"command": a.Hypercube, "args": []string{"mcp"}, "env": a.Env, "default_tools_approval_mode": "approve",
			}},
		},
	}
	method := "thread/start"
	if a.Conversation != "" {
		method = "thread/resume"
		params["threadId"] = a.Conversation
		params["excludeTurns"] = true
	}
	var opened struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := ag.call(ctx, method, params, &opened); err != nil {
		if a.Conversation != "" {
			return fmt.Errorf("Codex could not resume thread %s: %w", a.Conversation, err)
		}
		return err
	}
	if opened.Thread.ID == "" {
		return errors.New("Codex returned no thread")
	}
	ag.mu.Lock()
	defer ag.mu.Unlock()
	ag.thread = opened.Thread.ID
	ag.emit(harness.Event{Conversation: ag.thread})
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

// call makes a request and decodes its result. A connection that has ended is
// explained by the end of Codex's diagnostics.
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
	ag.emit(harness.Event{Conversation: ag.thread, State: harness.Idle})
}

// stop ends codex: closing its input lets it finish, and it is killed if it
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

// input steers the turn in progress, or starts a turn. Codex refuses a steer
// once the turn has ended, which can happen before its completion reaches us;
// the input then starts a turn.
func (ag *agent) input(text string, images []harness.Image) error {
	select {
	case <-ag.done:
		return errors.New("codex is not running")
	default:
	}
	ag.mu.Lock()
	thread, turn := ag.thread, ag.turn
	ag.mu.Unlock()
	if turn != "" {
		ctx, cancel := context.WithTimeout(ag.ctx, callTimeout)
		err := ag.call(ctx, "turn/steer", map[string]any{"threadId": thread, "expectedTurnId": turn, "input": userInput(text, images)}, nil)
		cancel()
		if err == nil {
			return nil
		}
		ag.mu.Lock()
		ended := ag.turn != turn || strings.Contains(err.Error(), "no active turn")
		ag.mu.Unlock()
		if !ended {
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
	if err := ag.call(ctx, "turn/start", map[string]any{"threadId": thread, "input": userInput(text, images)}, &started); err != nil {
		return err
	}
	ag.mu.Lock()
	defer ag.mu.Unlock()
	var ev harness.Event
	ag.startTurn(&ev, started.Turn.ID)
	ag.send(ev)
	return nil
}

func (ag *agent) send(ev harness.Event) {
	if len(ev.Items) > 0 || ev.State != "" || ev.Usage != nil {
		ag.emit(ev)
	}
}

func (ag *agent) interrupt() error {
	ag.mu.Lock()
	thread, turn := ag.thread, ag.turn
	var ev harness.Event
	// Codex waits for the answers of requests it has sent.
	ev.Items = ag.closeRequests(func(*request) bool { return true }, true)
	ag.send(ev)
	ag.mu.Unlock()
	if turn == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ag.ctx, callTimeout)
	defer cancel()
	return ag.call(ctx, "turn/interrupt", map[string]any{"threadId": thread, "turnId": turn}, nil)
}
