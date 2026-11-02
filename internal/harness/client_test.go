package harness_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
)

func TestStandIn(t *testing.T) { harnesstest.Run() }

// connection is a client to the stand-in plugin, with the events and exit it
// reported.
type connection struct {
	*harness.Client
	logs string
	mu   sync.Mutex
	got  []harness.Event
	exit chan error
}

func (c *connection) events() []harness.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]harness.Event(nil), c.got...)
}

// until polls for a condition the plugin makes true.
func until(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func connect(t *testing.T) *connection {
	t.Helper()
	dir := t.TempDir()
	harnesstest.Install(t, dir, "test")
	c := &connection{logs: t.TempDir(), exit: make(chan error, 1)}
	t.Setenv("HYPERCUBE_TEST_LOGS", c.logs)
	path, err := harness.Find(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	c.Client, err = harness.Launch(path, func(ev harness.Event) {
		c.mu.Lock()
		c.got = append(c.got, ev)
		c.mu.Unlock()
	}, func(err error) { c.exit <- err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Kill)
	return c
}

func (c *connection) do(cmd harness.Command) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.Do(ctx, cmd)
}

func (c *connection) input(text string) error {
	return c.do(harness.Command{Command: harness.CmdInput, Text: text})
}

func (c *connection) start() error {
	return c.do(harness.Command{Command: harness.CmdStart, Start: &harness.StartArgs{Agent: "a", Role: "worker"}})
}

func TestFindNeedsPlugin(t *testing.T) {
	_, err := harness.Find(t.TempDir(), "pi")
	if err == nil || !strings.Contains(err.Error(), "hypercube-harness-pi") {
		t.Fatal(err)
	}
}

func TestConversation(t *testing.T) {
	c := connect(t)
	if err := c.start(); err != nil {
		t.Fatal(err)
	}
	// Events arrive in the order the plugin wrote them, before and after the
	// reply they surround.
	until(t, "the start event", func() bool { return len(c.events()) == 1 })
	if ev := c.events()[0]; ev.Conversation != "a-conversation" || ev.State != harness.Idle {
		t.Fatalf("start event = %+v", ev)
	}
	if err := c.input("!early"); err != nil {
		t.Fatal(err)
	}
	until(t, "the early turn", func() bool { return len(c.events()) == 3 })
	if err := c.input("hello"); err != nil {
		t.Fatal(err)
	}
	until(t, "the quick turn", func() bool { return len(c.events()) == 5 })
	var states []string
	for _, ev := range c.events() {
		states = append(states, ev.State)
	}
	if got := strings.Join(states, ","); got != "idle,working,idle,working,idle" {
		t.Fatalf("states = %s", got)
	}
	if items := c.events()[4].Items; len(items) != 1 || items[0].Text != "ack" || !items[0].Done {
		t.Fatalf("items = %+v", items)
	}
	// A refused input is an error, and the connection stays usable.
	if err := c.input("!reject"); err == nil || err.Error() != "rejected" {
		t.Fatalf("refused input: %v", err)
	}
	if err := c.input("again"); err != nil {
		t.Fatal(err)
	}
	if err := c.do(harness.Command{Command: harness.CmdAnswer, Request: "nothing"}); err == nil || err.Error() != "no such request" {
		t.Fatalf("answer to an unknown request: %v", err)
	}
	// An attention request is answered by its ID.
	if err := c.input("!ask"); err != nil {
		t.Fatal(err)
	}
	if err := c.do(harness.Command{Command: harness.CmdAnswer, Request: "q", Choice: "yes"}); err != nil {
		t.Fatal(err)
	}
	until(t, "the answered request", func() bool {
		evs := c.events()
		last := evs[len(evs)-1]
		return len(last.Items) == 1 && last.Items[0].Done && last.Items[0].Answer == "yes"
	})
}

func TestShutdownEndsTheProcessGroup(t *testing.T) {
	c := connect(t)
	if err := c.start(); err != nil {
		t.Fatal(err)
	}
	if err := c.input("!child"); err != nil {
		t.Fatal(err)
	}
	var child int
	until(t, "the child", func() bool {
		for _, e := range harnesstest.Log(t, c.logs, "a", "child") {
			child = e.Pid
		}
		return child != 0
	})
	c.Shutdown(5 * time.Second)
	select {
	case err := <-c.exit:
		if err == nil {
			t.Fatal("a plugin's end has a reason")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no exit reported")
	}
	until(t, "the child to end", func() bool { return syscall.Kill(child, 0) == syscall.ESRCH })
	if err := c.input("late"); err == nil {
		t.Fatal("input reached an ended plugin")
	}
}

func TestBrokenProtocolEndsTheConnection(t *testing.T) {
	for word, want := range map[string]string{
		"!malformed": "malformed record",
		"!stray":     `reply to unknown request "999"`,
	} {
		t.Run(word, func(t *testing.T) {
			c := connect(t)
			if err := c.start(); err != nil {
				t.Fatal(err)
			}
			_ = c.input(word)
			select {
			case err := <-c.exit:
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("exit = %v, want %q", err, want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("the connection stayed open")
			}
			if err := c.input("more"); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("input after the break: %v", err)
			}
		})
	}
}

func TestCrashFailsWaitingRequests(t *testing.T) {
	c := connect(t)
	if err := c.start(); err != nil {
		t.Fatal(err)
	}
	held := make(chan error, 1)
	go func() { held <- c.input("!hold") }()
	until(t, "the held request", func() bool { return len(harnesstest.Log(t, c.logs, "a", "input")) == 1 })
	// The crash reply arrives; the plugin's exit then fails the held request.
	if err := c.input("!crash"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-held:
		if err == nil || !strings.Contains(err.Error(), "exit status 3") {
			t.Fatalf("held request: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the held request never ended")
	}
}

// Giving up on a request leaves its reply known, so a late reply is not a
// protocol error.
func TestLateReplyIsKnown(t *testing.T) {
	c := connect(t)
	if err := c.start(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.Do(ctx, harness.Command{Command: harness.CmdInput, Text: "!hold"}); err != context.DeadlineExceeded {
		t.Fatalf("held input: %v", err)
	}
	if err := c.do(harness.Command{Command: harness.CmdInterrupt}); err != nil {
		t.Fatal(err)
	}
	if err := c.input("after"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-c.exit:
		t.Fatalf("the late reply ended the connection: %v", err)
	default:
	}
}

func TestPluginThatEndsReportsItsDiagnostics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hypercube-harness-broken")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'unsupported harness: not here' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	exit := make(chan error, 1)
	c, err := harness.Launch(path, func(harness.Event) {}, func(err error) { exit <- err })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = c.Do(ctx, harness.Command{Command: harness.CmdStart, Start: &harness.StartArgs{}})
	if err == nil || !strings.Contains(err.Error(), "exit status 1") || !strings.Contains(err.Error(), "unsupported harness: not here") {
		t.Fatalf("start of an ended plugin: %v", err)
	}
	if got := <-exit; got == nil || got.Error() != err.Error() {
		t.Fatalf("exit = %v", got)
	}
}
