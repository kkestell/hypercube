package harness_test

import (
	"errors"
	"fmt"
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
func until(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out: " + what)
}

type connection struct {
	*harness.Client
	logs    string
	mu      sync.Mutex
	got     []harness.Event
	next    int64
	pending map[int64]chan error
	exit    chan error
	ended   error
}

func (c *connection) events() []harness.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]harness.Event(nil), c.got...)
}
func connect(t *testing.T) *connection {
	t.Helper()
	dir, err := os.MkdirTemp("", "hc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	harnesstest.Install(t, dir, "test")
	t.Setenv("HYPERCUBE_TEST_LOGS", dir)
	client, err := harness.Launch(filepath.Join(dir, "run"), filepath.Join(dir, "hypercube-harness-test"))
	if err != nil {
		t.Fatal(err)
	}
	c := &connection{Client: client, logs: dir, exit: make(chan error, 1), pending: map[int64]chan error{}}
	go func() {
		err := client.Read(0, func(rec harness.Record, _ int64) error {
			c.mu.Lock()
			defer c.mu.Unlock()
			if rec.Event != nil {
				c.got = append(c.got, *rec.Event)
				return nil
			}
			ch := c.pending[rec.Reply.ID]
			if ch == nil {
				return fmt.Errorf("reply to unknown request %d", rec.Reply.ID)
			}
			delete(c.pending, rec.Reply.ID)
			var err error
			if rec.Reply.Error != "" {
				err = errors.New(rec.Reply.Error)
			}
			ch <- err
			return nil
		})
		c.mu.Lock()
		c.ended = err
		for _, ch := range c.pending {
			ch <- err
		}
		c.mu.Unlock()
		c.exit <- err
	}()
	t.Cleanup(func() { _ = client.Stop(); _ = client.Remove() })
	return c
}
func (c *connection) send(cmd harness.Command) error {
	line, err := cmd.Line()
	if err != nil {
		return err
	}
	return c.Send(line)
}
func (c *connection) do(cmd harness.Command) error {
	c.mu.Lock()
	if c.ended != nil {
		c.mu.Unlock()
		return c.ended
	}
	c.next++
	cmd.ID = c.next
	ch := make(chan error, 1)
	c.pending[cmd.ID] = ch
	c.mu.Unlock()
	if err := c.send(cmd); err != nil {
		return err
	}
	select {
	case err := <-ch:
		return err
	case <-time.After(5 * time.Second):
		return errors.New("reply timed out")
	}
}
func (c *connection) start() error {
	return c.do(harness.Command{Command: harness.CmdStart, Start: &harness.StartArgs{Session: "a"}})
}
func (c *connection) input(text string) error {
	return c.do(harness.Command{Command: harness.CmdInput, Text: text})
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
	until(t, "idle", func() bool { return len(c.events()) == 1 })
	if err := c.input("!early"); err != nil {
		t.Fatal(err)
	}
	until(t, "early events", func() bool { return len(c.events()) == 3 })
	if err := c.input("!reject"); err == nil || err.Error() != "rejected" {
		t.Fatal(err)
	}
	if err := c.input("again"); err != nil {
		t.Fatal(err)
	}
	until(t, "turn", func() bool { return len(c.events()) == 5 })
}
func TestStopEndsTheProcessGroup(t *testing.T) {
	c := connect(t)
	if err := c.start(); err != nil {
		t.Fatal(err)
	}
	if err := c.input("!child"); err != nil {
		t.Fatal(err)
	}
	child := harnesstest.Log(t, c.logs, "a", "child")[0].Pid
	_ = c.Stop()
	select {
	case <-c.exit:
	case <-time.After(5 * time.Second):
		t.Fatal("no exit")
	}
	until(t, "child gone", func() bool { return syscall.Kill(child, 0) == syscall.ESRCH })
}
func TestBrokenProtocolEndsTheConnection(t *testing.T) {
	for word, want := range map[string]string{"!malformed": "malformed record", "!stray": "reply to unknown request 999"} {
		t.Run(word, func(t *testing.T) {
			c := connect(t)
			if err := c.start(); err != nil {
				t.Fatal(err)
			}
			_ = c.input(word)
			select {
			case err := <-c.exit:
				if !strings.Contains(err.Error(), want) {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("connection stayed open")
			}
		})
	}
}
func TestPluginThatEndsReportsItsDiagnostics(t *testing.T) {
	dir, err := os.MkdirTemp("", "hc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "broken")
	if err = os.WriteFile(path, []byte("#!/bin/sh\necho unsupported >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	c, err := harness.Launch(filepath.Join(dir, "run"), path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Remove()
	err = c.Read(0, func(harness.Record, int64) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "exit status 1: unsupported") {
		t.Fatal(err)
	}
}
func TestStandInIgnoresResentCommands(t *testing.T) {
	c := connect(t)
	if err := c.start(); err != nil {
		t.Fatal(err)
	}
	if err := c.send(harness.Command{ID: 1, Command: harness.CmdStart, Start: &harness.StartArgs{Session: "a"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.input("one"); err != nil {
		t.Fatal(err)
	}
	if err := c.send(harness.Command{ID: 2, Command: harness.CmdInput, Text: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := c.input("two"); err != nil {
		t.Fatal(err)
	}
	if got := harnesstest.Log(t, c.logs, "a", "input"); len(got) != 2 {
		t.Fatal(got)
	}
}
