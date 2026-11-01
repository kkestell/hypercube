// Package harnesstest is a stand-in harness plugin for tests. The test binary
// runs as the plugin: a package's tests install it with Install and define
//
//	func TestStandIn(t *testing.T) { harnesstest.Run() }
//
// to give the binary its entry point.
//
// The stand-in logs each command it handles to $HYPERCUBE_TEST_LOGS/<agent>.log
// as JSON lines. It acts on a keyword written with a leading "!" in the text of
// an input:
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
//	!crash     reply, then exit
//	!malformed reply, then write a bad line
//	!stray     reply, then write a reply nobody asked for
//
// Any other text is a quick turn. Starting with the conversation "unresumable"
// fails. With HYPERCUBE_TEST_CRASH set the plugin exits shortly after starting.
package harnesstest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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

func serve(in io.Reader, out io.Writer) {
	enc := json.NewEncoder(out)
	var agent string
	var held []string
	var turns int
	log := func(kind string, fields map[string]any) {
		if agent == "" {
			return
		}
		if fields == nil {
			fields = map[string]any{}
		}
		fields["kind"] = kind
		b, _ := json.Marshal(fields)
		f, err := os.OpenFile(filepath.Join(os.Getenv("HYPERCUBE_TEST_LOGS"), agent+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			os.Exit(90)
		}
		_, _ = f.Write(append(b, '\n'))
		_ = f.Close()
	}
	reply := func(id, problem string) {
		_ = enc.Encode(harness.Record{Reply: &harness.Reply{ID: id, Error: problem}})
	}
	event := func(ev harness.Event) { _ = enc.Encode(harness.Record{Event: &ev}) }
	state := func(s string, items ...harness.Item) { event(harness.Event{State: s, Items: items}) }
	text := func(id, body string, done bool) harness.Item {
		return harness.Item{ID: id, Kind: harness.KindText, Text: body, Done: done}
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		var cmd harness.Command
		if err := json.Unmarshal(scanner.Bytes(), &cmd); err != nil {
			os.Exit(91)
		}
		switch cmd.Command {
		case harness.CmdStart:
			s := cmd.Start
			agent = s.Agent
			log("start", map[string]any{"conversation": s.Conversation, "role": s.Role, "dir": s.Dir, "instructions": s.Instructions,
				"env": s.Env, "pid": os.Getpid(), "pgid": syscall.Getpgrp()})
			if s.Conversation == "unresumable" {
				reply(cmd.ID, "cannot resume conversation unresumable")
				continue
			}
			reply(cmd.ID, "")
			event(harness.Event{Conversation: agent + "-conversation", State: harness.Idle})
			if os.Getenv("HYPERCUBE_TEST_CRASH") != "" {
				time.Sleep(100 * time.Millisecond)
				os.Exit(1)
			}
		case harness.CmdInput:
			body := cmd.Text
			log("input", map[string]any{"text": body, "images": len(cmd.Images)})
			turns++
			id := fmt.Sprintf("t%d", turns)
			has := func(word string) bool { return strings.Contains(body, "!"+word) }
			switch {
			case has("reject"):
				reply(cmd.ID, "rejected")
			case has("hold"):
				held = append(held, cmd.ID)
			case has("work"):
				reply(cmd.ID, "")
				state(harness.Working, text("w", "working", false))
			case has("finish"):
				reply(cmd.ID, "")
				state(harness.Idle, text("w", "finished", true))
			case has("stream"):
				reply(cmd.ID, "")
				state(harness.Working, text("s", "Hel", false))
				state("", harness.Item{ID: "s", Kind: harness.KindText, Append: true, Text: "lo"})
				state("", harness.Item{ID: "c", Parent: "s", Kind: harness.KindTool, Tool: "bash", Input: json.RawMessage(`{"command":"ls"}`)})
				state("", harness.Item{ID: "c", Parent: "s", Kind: harness.KindTool, Tool: "bash", Output: "ok", Done: true})
				state(harness.Idle, harness.Item{ID: "s", Kind: harness.KindText, Append: true, Text: "!", Done: true})
			case has("early"):
				state(harness.Working, text(id, "early", true))
				reply(cmd.ID, "")
				state(harness.Idle)
			case has("ask"):
				reply(cmd.ID, "")
				state(harness.Working, harness.Item{ID: "q", Kind: harness.KindAttention, Text: "Proceed?", Choices: []string{"yes", "no"}})
			case has("usage"):
				reply(cmd.ID, "")
				event(harness.Event{Usage: &harness.Usage{Context: 45200, Window: 200000, Cost: 0.31}})
			case has("fail"):
				reply(cmd.ID, "")
				state(harness.Idle, harness.Item{ID: id, Kind: harness.KindError, Text: "test provider failure", Done: true})
			case has("image"):
				reply(cmd.ID, "")
				state(harness.Idle, harness.Item{ID: id, Kind: harness.KindText, Images: []harness.Image{{MIMEType: "image/png", Data: "iVBORw0KGgo="}}, Done: true})
			case has("child"):
				child := exec.Command("sleep", "300")
				if err := child.Start(); err != nil {
					os.Exit(92)
				}
				log("child", map[string]any{"pid": child.Process.Pid})
				reply(cmd.ID, "")
			case has("crash"):
				reply(cmd.ID, "")
				os.Exit(3)
			case has("malformed"):
				reply(cmd.ID, "")
				fmt.Fprintln(out, "not json")
			case has("stray"):
				reply(cmd.ID, "")
				reply("999", "")
			default:
				reply(cmd.ID, "")
				state(harness.Working)
				state(harness.Idle, text(id, "ack", true))
			}
		case harness.CmdInterrupt:
			log("interrupt", nil)
			for _, id := range held {
				reply(id, "")
			}
			held = nil
			reply(cmd.ID, "")
			state(harness.Idle)
		case harness.CmdAnswer:
			log("answer", map[string]any{"request": cmd.Request, "choice": cmd.Choice, "text": cmd.Text})
			if cmd.Request != "q" {
				reply(cmd.ID, "no such request")
				continue
			}
			reply(cmd.ID, "")
			state(harness.Idle, harness.Item{ID: "q", Kind: harness.KindAttention, Text: "Proceed?", Done: true, Answer: cmd.Choice + cmd.Text})
		case harness.CmdShutdown:
			log("shutdown", nil)
			reply(cmd.ID, "")
			return
		}
	}
}
