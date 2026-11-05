package harness_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	t       *testing.T
	logs    string
	records chan harness.Record
	exit    chan error
}

func connect(t *testing.T, handle func(harness.Record) error) *connection {
	t.Helper()
	// Keep holder socket paths below macOS's length limit.
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
	c := &connection{Client: client, t: t, logs: dir, records: make(chan harness.Record, 8), exit: make(chan error, 1)}
	go func() {
		defer close(c.exit)
		c.exit <- client.Read(0, func(rec harness.Record, _ int64) error {
			if handle != nil {
				if err := handle(rec); err != nil {
					return err
				}
			}
			c.records <- rec
			return nil
		})
	}()
	t.Cleanup(func() {
		_ = client.Stop()
		_ = client.Close()
		select {
		case <-c.exit:
		case <-time.After(5 * time.Second):
			t.Error("client reader did not stop")
		}
		_ = client.Remove()
	})
	return c
}

func (c *connection) send(cmd harness.Command) {
	c.t.Helper()
	line, err := cmd.Line()
	if err != nil {
		c.t.Fatal(err)
	}
	if err := c.Send(line); err != nil {
		c.t.Fatal(err)
	}
}

func (c *connection) next() harness.Record {
	c.t.Helper()
	select {
	case rec := <-c.records:
		return rec
	case <-time.After(5 * time.Second):
		c.t.Fatal("timed out waiting for a record")
		return harness.Record{}
	}
}

func (c *connection) reply(id int64, problem string) {
	c.t.Helper()
	if rec := c.next(); rec.Reply == nil || rec.Reply.ID != id || rec.Reply.Error != problem {
		c.t.Fatalf("record = %+v, want reply %d with error %q", rec, id, problem)
	}
}

func (c *connection) state(want string) {
	c.t.Helper()
	if rec := c.next(); rec.Event == nil || rec.Event.State != want {
		c.t.Fatalf("record = %+v, want state %q", rec, want)
	}
}

func (c *connection) start() {
	c.t.Helper()
	c.send(harness.Command{ID: 1, Command: harness.CmdStart, Start: &harness.StartArgs{Session: "a"}})
	c.reply(1, "")
	c.state(harness.Idle)
}

func TestFindNeedsPlugin(t *testing.T) {
	_, err := harness.Find(t.TempDir(), "pi")
	if err == nil || !strings.Contains(err.Error(), "hypercube-harness-pi") {
		t.Fatal(err)
	}
}
func TestConversation(t *testing.T) {
	c := connect(t, nil)
	c.start()
	c.send(harness.Command{ID: 2, Command: harness.CmdInput, Text: "!early"})
	c.state(harness.Working)
	c.reply(2, "")
	c.state(harness.Idle)
	c.send(harness.Command{ID: 3, Command: harness.CmdInput, Text: "!reject"})
	c.reply(3, "rejected")
	c.send(harness.Command{ID: 4, Command: harness.CmdInput, Text: "again"})
	c.reply(4, "")
	c.state(harness.Working)
	c.state(harness.Idle)
}

func TestStopEndsTheProcessGroup(t *testing.T) {
	c := connect(t, nil)
	c.start()
	c.send(harness.Command{ID: 2, Command: harness.CmdInput, Text: "!child"})
	c.reply(2, "")
	child := harnesstest.Log(t, c.logs, "a", "child")[0].Pid
	_ = c.Stop()
	select {
	case <-c.exit:
	case <-time.After(5 * time.Second):
		t.Fatal("no exit")
	}
	until(t, "child gone", func() bool { return syscall.Kill(child, 0) == syscall.ESRCH })
}

func TestReadStopsOnProtocolOrHandlerError(t *testing.T) {
	refused := errors.New("handler refused the record")
	for _, tc := range []struct {
		name   string
		input  string
		handle func(harness.Record) error
		want   string
	}{
		{name: "malformed record", input: "!malformed", want: "malformed record"},
		{name: "handler error", input: "again", want: refused.Error(), handle: func(rec harness.Record) error {
			if rec.Reply != nil && rec.Reply.ID == 2 {
				return refused
			}
			return nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := connect(t, tc.handle)
			c.start()
			c.send(harness.Command{ID: 2, Command: harness.CmdInput, Text: tc.input})
			select {
			case err := <-c.exit:
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("Read error = %v, want %q", err, tc.want)
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
