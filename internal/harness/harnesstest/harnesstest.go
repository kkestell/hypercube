// Package harnesstest is a stand-in harness plugin for tests. The test binary
// runs as the plugin: a package's tests install it with Install and define
//
//	func TestStandIn(t *testing.T) { harnesstest.Run() }
//
// to give the binary its entry point.
//
// The stand-in logs each command it handles to $HYPERCUBE_TEST_LOGS/<agent>.log
// as JSON lines, which Log reads. It acts on a keyword written with a leading
// "!" in the text of an input:
//
//	!hold      no reply until an interrupt
//	!onclose   reply when input closes
//	!append    append to the streaming work item
//	!complete  finish the streaming work item
//	!reject    an error reply
//	!work      start a turn that does not end until "!finish"
//	!finish    end that turn with its text
//	!answer    a turn of progress text and a final answer
//	!stream    text and tool items that replace and extend each other
//	!ask       an attention request with the choices yes and no
//	!multiask  a question that permits several choices
//	!asyncask  another request, asked asynchronously while the turn goes on;
//	           its answer closes it without changing the work state
//	!question  the !ask request alone, without a work state
//	!approve   a tool call and an approval for the tool "Tool", choices Allow and Deny
//	!usage     a usage and limits event
//	!fail      an error item
//	!image     an assistant item that holds only an image
//	!child     start a child process in the plugin's group
//	!crash     reply, then exit
//
// Any other text is a quick turn. Starting with the conversation "unresumable"
// or the model "invalid" fails, and a start without an effort reports the
// effort "low". With HYPERCUBE_TEST_CRASH set the plugin exits shortly after
// starting.
//
// Its `describe` and `models` commands answer with Catalog's fixed models.
// With HYPERCUBE_TEST_MODELS_HOLD set, `models` waits until it is killed.
// HYPERCUBE_TEST_MODELS_LOG records each models request as a JSON line.
package harnesstest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

const marker = "HYPERCUBE_STANDIN"

// Exit codes of the stand-in when it cannot continue, and when a test asks it
// to crash.
const (
	exitLogFailed   = 90
	exitBadCommand  = 91
	exitChildFailed = 92
	exitCrash       = 3
	exitStartCrash  = 1
)

// startCrashDelay is how long a plugin started with HYPERCUBE_TEST_CRASH lives
// after its start reply. The app has to see the agent start before the exit,
// or it reports a failed start instead of an agent that ended.
const startCrashDelay = 100 * time.Millisecond

// Entry is one line of an agent's log. Which fields are set depends on Kind.
type Entry struct {
	Kind   string `json:"kind"`
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	// start
	Conversation string            `json:"conversation,omitempty"`
	Fork         string            `json:"fork,omitempty"`
	Dir          string            `json:"dir,omitempty"`
	Instructions string            `json:"instructions,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	Pgid         int               `json:"pgid,omitempty"`
	// start and child: the process ID of the plugin or of its child.
	Pid int `json:"pid,omitempty"`
	// input and answer
	Text string `json:"text,omitempty"`
	// input
	Images int `json:"images,omitempty"`
	// answer
	Request string   `json:"request,omitempty"`
	Choice  string   `json:"choice,omitempty"`
	Choices []string `json:"choices,omitempty"`
}

// Log returns the entries of the given kind that the agent's stand-in wrote
// to the log directory, oldest first.
func Log(t testing.TB, dir, agent, kind string) []Entry {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, agent+".log"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var entries []Entry
	for _, line := range bytes.Split(b, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("log of %s: %v: %s", agent, err, line)
		}
		if e.Kind == kind {
			entries = append(entries, e)
		}
	}
	return entries
}

// Install writes a plugin for the harness name into dir that runs this test
// binary as the stand-in.
func Install(t testing.TB, dir, name string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The race detector makes a process sleep a second before it exits.
	script := fmt.Sprintf("#!/bin/sh\nexport %s=1 GORACE=atexit_sleep_ms=0\nexec '%s' -test.run '^TestStandIn$' -- \"$@\"\n", marker, exe)
	if err := os.WriteFile(filepath.Join(dir, "hypercube-harness-"+name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// Run serves the protocol and exits when the process was started by an
// installed plugin script, and does nothing otherwise.
func Run() {
	if os.Getenv(marker) != "1" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "describe":
		_ = json.NewEncoder(os.Stdout).Encode(harness.Description{Name: "Stand-in", Installed: true})
	case "models":
		var req harness.ModelsRequest
		if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
			os.Exit(exitBadCommand)
		}
		if os.Getenv("HYPERCUBE_TEST_MODELS_HOLD") != "" {
			select {}
		}
		if path := os.Getenv("HYPERCUBE_TEST_MODELS_LOG"); path != "" {
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				os.Exit(exitLogFailed)
			}
			err = json.NewEncoder(f).Encode(req)
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				os.Exit(exitLogFailed)
			}
		}
		c, err := Catalog(req)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(c)
	default:
		serve(os.Stdin, os.Stdout)
	}
	os.Exit(0)
}

// Catalog is the stand-in's answer to `models`: the models "m", whose efforts
// are low and high, and "plain", which has no effort setting.
func Catalog(req harness.ModelsRequest) (harness.Catalog, error) {
	c := harness.Catalog{Models: []harness.Model{{ID: "m", Label: "Model M", Provider: "test"}, {ID: "plain"}}, Default: "m", Model: req.Model}
	switch req.Model {
	case "", "plain":
	case "m":
		c.Efforts, c.DefaultEffort = []string{"low", "high"}, "low"
	default:
		return c, fmt.Errorf("model %q is not offered", req.Model)
	}
	return c, nil
}

// standIn is the state of one plugin process.
type standIn struct {
	enc   *json.Encoder
	agent string
	// held holds the IDs of inputs that wait for an interrupt.
	onclose    []int64
	held       []int64
	heldConfig []harness.Reply
	turns      int
	// turn names the items of the input being handled.
	turn string
}

func serve(in io.Reader, out io.Writer) {
	p := &standIn{enc: json.NewEncoder(out)}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	var last int64
	for scanner.Scan() {
		var cmd harness.Command
		if err := json.Unmarshal(scanner.Bytes(), &cmd); err != nil {
			os.Exit(exitBadCommand)
		}
		if cmd.ID <= last {
			continue
		}
		last = cmd.ID
		switch cmd.Command {
		case harness.CmdStart:
			p.start(cmd)
		case harness.CmdInput:
			p.input(cmd)
		case harness.CmdInterrupt:
			p.interrupt(cmd)
		case harness.CmdAnswer:
			p.answer(cmd)
		case harness.CmdConfigure:
			p.configure(cmd)
		}
	}
	for _, id := range p.onclose {
		p.reply(id, "")
	}
	if delay, err := time.ParseDuration(os.Getenv("HYPERCUBE_TEST_END_DELAY")); err == nil {
		time.Sleep(delay)
	}
}

// log appends to the agent's log. Before the start command there is no agent
// to log for.
func (p *standIn) log(e Entry) {
	if p.agent == "" {
		return
	}
	b, _ := json.Marshal(e)
	f, err := os.OpenFile(filepath.Join(os.Getenv("HYPERCUBE_TEST_LOGS"), p.agent+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		os.Exit(exitLogFailed)
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

func (p *standIn) reply(id int64, problem string) {
	_ = p.enc.Encode(harness.Record{Reply: &harness.Reply{ID: id, Error: problem}})
}

func (p *standIn) event(ev harness.Event) { _ = p.enc.Encode(harness.Record{Event: &ev}) }

func (p *standIn) state(s string, items ...harness.Item) {
	p.event(harness.Event{State: s, Items: items})
}

func text(id, body string, done bool) harness.Item {
	return harness.Item{ID: id, Kind: harness.KindText, Text: body, Done: done}
}

func (p *standIn) start(cmd harness.Command) {
	s := cmd.Start
	p.agent = s.Session
	p.log(Entry{Kind: "start", Conversation: s.Conversation, Fork: s.Fork, Dir: s.Dir, Instructions: s.Instructions,
		Model: s.Model, Effort: s.Effort, Env: s.Env, Pid: os.Getpid(), Pgid: syscall.Getpgrp()})
	if s.Conversation == "unresumable" {
		p.reply(cmd.ID, "cannot resume conversation unresumable")
		return
	}
	if s.Model == "invalid" {
		p.reply(cmd.ID, "model invalid is not offered")
		return
	}
	if os.Getenv("HYPERCUBE_TEST_START_HOLD") == "1" {
		return
	}
	reply := harness.Reply{ID: cmd.ID}
	if s.Effort == "" {
		reply.Selection = &harness.Selection{Model: s.Model, Effort: "low"}
	}
	_ = p.enc.Encode(harness.Record{Reply: &reply})
	p.event(harness.Event{Conversation: p.agent + "-conversation", State: harness.Idle})
	if os.Getenv("HYPERCUBE_TEST_CRASH") != "" {
		time.Sleep(startCrashDelay)
		os.Exit(exitStartCrash)
	}
}

func (p *standIn) input(cmd harness.Command) {
	p.log(Entry{Kind: "input", Text: cmd.Text, Images: len(cmd.Images)})
	p.turns++
	p.turn = fmt.Sprintf("t%d", p.turns)
	has := func(word string) bool { return strings.Contains(cmd.Text, "!"+word) }
	switch {
	case strings.HasPrefix(cmd.Text, "!report "):
		var answer string
		if err := json.Unmarshal([]byte(strings.TrimPrefix(cmd.Text, "!report ")), &answer); err != nil {
			p.reply(cmd.ID, err.Error())
			return
		}
		p.reply(cmd.ID, "")
		p.state(harness.Working, text("progress", "looking", true))
		p.event(harness.Event{State: harness.Idle, Final: []string{p.turn}, Items: []harness.Item{text(p.turn, answer, true)}})
	case has("reject"):
		p.reply(cmd.ID, "rejected")
	case has("onclose"):
		p.onclose = append(p.onclose, cmd.ID)
	case has("hold"):
		p.held = append(p.held, cmd.ID)
	case has("accept"):
		p.reply(cmd.ID, "")
	case has("work"):
		p.reply(cmd.ID, "")
		p.state(harness.Working, text("w", "working", false))
	case has("append"):
		p.reply(cmd.ID, "")
		p.event(harness.Event{Items: []harness.Item{{ID: "w", Kind: harness.KindText, Append: true, Text: " more"}}})
	case has("complete"):
		p.reply(cmd.ID, "")
		p.state(harness.Idle, harness.Item{ID: "w", Kind: harness.KindText, Append: true, Text: " done", Done: true})
	case has("finish"):
		p.reply(cmd.ID, "")
		p.state(harness.Idle, text("w", "finished", true))
	case has("answer"):
		p.reply(cmd.ID, "")
		p.state(harness.Working, text("progress", "looking", true))
		p.event(harness.Event{State: harness.Idle, Final: []string{"answer"}, Items: []harness.Item{text("answer", "the answer", true)}})
	case has("stream"):
		p.reply(cmd.ID, "")
		p.stream()
	case has("asyncask"):
		p.reply(cmd.ID, "")
		p.state(harness.Working, harness.Item{ID: "aq", Kind: harness.KindAttention, Text: "Continue?", Choices: []string{"yes", "no"}, Async: true})
	case has("question"):
		p.reply(cmd.ID, "")
		p.state("", harness.Item{ID: "q", Kind: harness.KindAttention, Text: "Proceed?", Choices: []string{"yes", "no"}})
	case has("multiask"):
		p.reply(cmd.ID, "")
		p.state(harness.Working, harness.Item{ID: "q", Kind: harness.KindAttention, Text: "Choose several", Choices: []string{"yes", "no", "with, punctuation"}, Multi: true, Free: true})
	case has("ask"):
		p.reply(cmd.ID, "")
		p.state(harness.Working, harness.Item{ID: "q", Kind: harness.KindAttention, Text: "Proceed?", Choices: []string{"yes", "no"}})
	case has("approve"):
		p.reply(cmd.ID, "")
		p.state(harness.Working, harness.Item{ID: "tool-" + p.turn, Kind: harness.KindTool, Tool: "Tool"}, approval(p.turn, false, ""))
	case has("usage"):
		p.reply(cmd.ID, "")
		p.event(harness.Event{
			Usage:  &harness.Usage{Context: 45200, Window: 200000, Cost: 0.31},
			Limits: []harness.Limit{{Minutes: 300, Used: 42, Resets: 1793767800, At: 1793750712115}},
		})
	case has("fail"):
		p.reply(cmd.ID, "")
		p.state(harness.Idle, harness.Item{ID: p.turn, Kind: harness.KindError, Text: "test provider failure", Done: true})
	case has("image"):
		p.reply(cmd.ID, "")
		p.state(harness.Idle, harness.Item{ID: p.turn, Kind: harness.KindText, Images: []harness.Image{{MIMEType: "image/png", Data: "iVBORw0KGgo="}}, Done: true})
	case has("child"):
		p.child(cmd.ID)
	case has("crash"):
		p.reply(cmd.ID, "")
		os.Exit(exitCrash)
	default:
		p.reply(cmd.ID, "")
		p.state(harness.Working)
		p.state(harness.Idle, text(p.turn, "ack", true))
	}
}

// stream sends text and tool items that replace and extend each other.
func (p *standIn) stream() {
	tool := harness.Item{ID: "c", Parent: "s", Kind: harness.KindTool, Tool: "bash"}
	call, result := tool, tool
	call.Input = json.RawMessage(`{"command":"ls"}`)
	result.Output, result.Done = "ok", true
	p.state(harness.Working, text("s", "Hel", false))
	p.state("", harness.Item{ID: "s", Kind: harness.KindText, Append: true, Text: "lo"})
	p.state("", call)
	p.state("", result)
	p.state(harness.Idle, harness.Item{ID: "s", Kind: harness.KindText, Append: true, Text: "!", Done: true})
}

// child starts a process in the plugin's group.
func (p *standIn) child(id int64) {
	child := exec.Command("sleep", "300")
	if err := child.Start(); err != nil {
		os.Exit(exitChildFailed)
	}
	p.log(Entry{Kind: "child", Pid: child.Process.Pid})
	p.reply(id, "")
}

func (p *standIn) interrupt(cmd harness.Command) {
	p.log(Entry{Kind: "interrupt"})
	for _, id := range p.held {
		p.reply(id, "")
	}
	p.held = nil
	for _, reply := range p.heldConfig {
		_ = p.enc.Encode(harness.Record{Reply: &reply})
	}
	p.heldConfig = nil
	p.reply(cmd.ID, "")
	p.state(harness.Idle)
}

// approval is the stand-in's approval of the tool call in the turn.
func approval(turn string, done bool, answer string) harness.Item {
	return harness.Item{ID: "approval-" + turn, Kind: harness.KindAttention, Text: "Allow Tool?", Choices: []string{"Allow", "Deny"},
		Grant: "Tool", Done: done, Answer: answer}
}

func (p *standIn) answer(cmd harness.Command) {
	if len(cmd.Choices) > 0 {
		cmd.Choice = strings.Join(cmd.Choices, ", ")
	}
	p.log(Entry{Kind: "answer", Request: cmd.Request, Choice: cmd.Choice, Choices: cmd.Choices, Text: cmd.Text})
	if turn, ok := strings.CutPrefix(cmd.Request, "approval-"); ok {
		p.reply(cmd.ID, "")
		p.state(harness.Idle, approval(turn, true, cmd.Choice), harness.Item{ID: "tool-" + turn, Kind: harness.KindTool, Tool: "Tool", Output: cmd.Choice, Done: true})
		return
	}
	if cmd.Request == "aq" {
		p.reply(cmd.ID, "")
		p.state("", harness.Item{ID: "aq", Kind: harness.KindAttention, Text: "Continue?", Async: true, Done: true, Answer: cmd.Choice + cmd.Text})
		return
	}
	if cmd.Request != "q" {
		p.reply(cmd.ID, "no such request")
		return
	}
	p.reply(cmd.ID, "")
	p.state(harness.Idle, harness.Item{ID: "q", Kind: harness.KindAttention, Text: "Proceed?", Done: true, Answer: cmd.Choice + cmd.Text})
}

// configure accepts a selection, or simulates a native refusal or partial change.
func (p *standIn) configure(cmd harness.Command) {
	selected := *cmd.Selection
	p.log(Entry{Kind: "configure", Model: selected.Model, Effort: selected.Effort})
	reply := harness.Reply{ID: cmd.ID, Selection: &selected}
	switch selected.Model {
	case "reject":
		reply.Selection, reply.Error = nil, "model rejected"
	case "partial":
		selected.Effort, reply.Error = "low", "effort rejected"
	case "hold":
		p.heldConfig = append(p.heldConfig, reply)
		return
	}
	_ = p.enc.Encode(harness.Record{Reply: &reply})
}
