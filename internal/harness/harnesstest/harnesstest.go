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
//	!reject    an error reply
//	!work      start a turn that does not end until "!finish"
//	!finish    end that turn with its text
//	!stream    text and tool items that replace and extend each other
//	!early     events before the reply
//	!ask       an attention request with the choices yes and no
//	!usage     a usage event
//	!fail      an error item
//	!image     an assistant item that holds only an image
//	!child     start a child process in the plugin's group
//	!linger    take half a second to reply to the shutdown that follows
//	!crash     reply, then exit
//	!malformed reply, then write a bad line
//	!stray     reply, then write a reply nobody asked for
//
// Any other text is a quick turn. Starting with the conversation "unresumable"
// fails. With HYPERCUBE_TEST_CRASH set the plugin exits shortly after starting.
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
	Kind string `json:"kind"`
	// start
	Conversation string            `json:"conversation,omitempty"`
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
	Request string `json:"request,omitempty"`
	Choice  string `json:"choice,omitempty"`
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
	serve(os.Stdin, os.Stdout)
	os.Exit(0)
}

// standIn is the state of one plugin process.
type standIn struct {
	out   io.Writer
	enc   *json.Encoder
	agent string
	// held holds the IDs of inputs that wait for an interrupt.
	held  []string
	turns int
	// turn names the items of the input being handled.
	turn string
	// linger delays the reply to shutdown.
	linger bool
}

func serve(in io.Reader, out io.Writer) {
	p := &standIn{out: out, enc: json.NewEncoder(out)}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		var cmd harness.Command
		if err := json.Unmarshal(scanner.Bytes(), &cmd); err != nil {
			os.Exit(exitBadCommand)
		}
		switch cmd.Command {
		case harness.CmdStart:
			p.start(cmd)
		case harness.CmdInput:
			p.input(cmd)
		case harness.CmdInterrupt:
			p.interrupt(cmd)
		case harness.CmdAnswer:
			p.answer(cmd)
		case harness.CmdShutdown:
			p.log(Entry{Kind: "shutdown"})
			if p.linger {
				time.Sleep(500 * time.Millisecond)
			}
			p.reply(cmd.ID, "")
			return
		}
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

func (p *standIn) reply(id, problem string) {
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
	p.log(Entry{Kind: "start", Conversation: s.Conversation, Dir: s.Dir, Instructions: s.Instructions,
		Env: s.Env, Pid: os.Getpid(), Pgid: syscall.Getpgrp()})
	if s.Conversation == "unresumable" {
		p.reply(cmd.ID, "cannot resume conversation unresumable")
		return
	}
	p.reply(cmd.ID, "")
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
	case has("reject"):
		p.reply(cmd.ID, "rejected")
	case has("hold"):
		p.held = append(p.held, cmd.ID)
	case has("work"):
		p.reply(cmd.ID, "")
		p.state(harness.Working, text("w", "working", false))
	case has("finish"):
		p.reply(cmd.ID, "")
		p.state(harness.Idle, text("w", "finished", true))
	case has("stream"):
		p.reply(cmd.ID, "")
		p.stream()
	case has("early"):
		p.state(harness.Working, text(p.turn, "early", true))
		p.reply(cmd.ID, "")
		p.state(harness.Idle)
	case has("ask"):
		p.reply(cmd.ID, "")
		p.state(harness.Working, harness.Item{ID: "q", Kind: harness.KindAttention, Text: "Proceed?", Choices: []string{"yes", "no"}})
	case has("usage"):
		p.reply(cmd.ID, "")
		p.event(harness.Event{Usage: &harness.Usage{Context: 45200, Window: 200000, Cost: 0.31}})
	case has("fail"):
		p.reply(cmd.ID, "")
		p.state(harness.Idle, harness.Item{ID: p.turn, Kind: harness.KindError, Text: "test provider failure", Done: true})
	case has("image"):
		p.reply(cmd.ID, "")
		p.state(harness.Idle, harness.Item{ID: p.turn, Kind: harness.KindText, Images: []harness.Image{{MIMEType: "image/png", Data: "iVBORw0KGgo="}}, Done: true})
	case has("child"):
		p.child(cmd.ID)
	case has("linger"):
		p.linger = true
		p.reply(cmd.ID, "")
	case has("crash"):
		p.reply(cmd.ID, "")
		os.Exit(exitCrash)
	case has("malformed"):
		p.reply(cmd.ID, "")
		fmt.Fprintln(p.out, "not json")
	case has("stray"):
		p.reply(cmd.ID, "")
		p.reply("999", "")
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
func (p *standIn) child(id string) {
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
	p.reply(cmd.ID, "")
	p.state(harness.Idle)
}

func (p *standIn) answer(cmd harness.Command) {
	p.log(Entry{Kind: "answer", Request: cmd.Request, Choice: cmd.Choice, Text: cmd.Text})
	if cmd.Request != "q" {
		p.reply(cmd.ID, "no such request")
		return
	}
	p.reply(cmd.ID, "")
	p.state(harness.Idle, harness.Item{ID: "q", Kind: harness.KindAttention, Text: "Proceed?", Done: true, Answer: cmd.Choice + cmd.Text})
}
