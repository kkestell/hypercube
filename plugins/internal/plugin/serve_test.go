package plugin

import (
	"bufio"
	"encoding/json"
	"io"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

// standIn starts a shell script as the harness. Each line it prints becomes a
// text event, and the agent records the calls it gets.
type standIn struct {
	script string

	mu    sync.Mutex
	calls []string
}

func (s *standIn) record(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

func (s *standIn) recorded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

func (s *standIn) start(a harness.StartArgs, emit func(harness.Event)) (Agent, error) {
	p, err := StartProcess("Shell", exec.Command("sh", "-c", s.script))
	if err != nil {
		return Agent{}, err
	}
	go p.Read(func(line []byte) {
		var m struct{ Text string }
		_ = json.Unmarshal(line, &m)
		emit(harness.Event{Items: []harness.Item{{ID: m.Text, Kind: harness.KindText, Text: m.Text, Done: true}}})
	})
	return Agent{
		Process:      p,
		Conversation: "conversation-" + a.Session,
		Input: func(text string, _ []harness.Image) error {
			s.record("input " + text)
			_, err := io.WriteString(p.Stdin, text+"\n")
			return err
		},
		Interrupt: func() error { s.record("interrupt"); return nil },
		Answer: func(request string, choices []string, text string) error {
			s.record("answer " + request + " " + strings.Join(choices, ", ") + " " + text)
			return nil
		},
		Stop: func() { s.record("stop"); p.Stop() },
	}, nil
}

// serving is a test's end of Serve.
type serving struct {
	t       *testing.T
	in      *io.PipeWriter
	records chan harness.Record
	done    chan error
}

func serveStandIn(t *testing.T, s *standIn) *serving {
	t.Helper()
	return serveStart(t, s.start)
}

func serveStart(t *testing.T, start Start) *serving {
	t.Helper()
	inRead, inWrite := io.Pipe()
	outRead, outWrite := io.Pipe()
	c := &serving{t: t, in: inWrite, records: make(chan harness.Record, 64), done: make(chan error, 1)}
	go func() {
		err := Serve(inRead, outWrite, start)
		outWrite.Close()
		c.done <- err
	}()
	go func() {
		defer close(c.records)
		scanner := bufio.NewScanner(outRead)
		for scanner.Scan() {
			var r harness.Record
			if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
				t.Error(err)
			}
			c.records <- r
		}
	}()
	t.Cleanup(func() { inWrite.Close() })
	return c
}

func (c *serving) send(line string) {
	c.t.Helper()
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		c.t.Fatal(err)
	}
}

func (c *serving) command(cmd harness.Command) {
	c.t.Helper()
	b, _ := json.Marshal(cmd)
	c.send(string(b))
}

func (c *serving) next() harness.Record {
	c.t.Helper()
	select {
	case r, ok := <-c.records:
		if !ok {
			c.t.Fatal("the output closed")
		}
		return r
	case <-time.After(10 * time.Second):
		c.t.Fatal("timed out")
	}
	return harness.Record{}
}

// reply sends a command and returns the error of its reply.
func (c *serving) reply(cmd harness.Command) string {
	c.t.Helper()
	c.command(cmd)
	r := c.next()
	if r.Reply == nil || r.Reply.ID != cmd.ID {
		c.t.Fatalf("got %+v, not the reply to %d", r, cmd.ID)
	}
	return r.Reply.Error
}

// wait returns what Serve returned.
func (c *serving) wait() error {
	c.t.Helper()
	select {
	case err := <-c.done:
		return err
	case <-time.After(10 * time.Second):
		c.t.Fatal("Serve did not return")
	}
	return nil
}

var startArgs = harness.StartArgs{Session: "a1"}

func (c *serving) start() {
	c.t.Helper()
	if err := c.reply(harness.Command{ID: 1, Command: harness.CmdStart, Start: &startArgs}); err != "" {
		c.t.Fatal(err)
	}
	r := c.next()
	if r.Event == nil || r.Event.State != harness.Idle || r.Event.Conversation != "conversation-a1" {
		c.t.Fatalf("%+v is not the idle event", r)
	}
}

func TestServeDrivesTheAgent(t *testing.T) {
	s := &standIn{script: "cat >/dev/null"}
	c := serveStandIn(t, s)
	c.start()
	if err := c.reply(harness.Command{ID: 2, Command: harness.CmdStart, Start: &startArgs}); err != "already started" {
		t.Error(err)
	}
	if err := c.reply(harness.Command{ID: 3, Command: harness.CmdStart}); err != "already started" {
		t.Error(err)
	}
	if err := c.reply(harness.Command{ID: 4, Command: "dance"}); err != `unknown command "dance"` {
		t.Error(err)
	}
	for _, cmd := range []harness.Command{
		{ID: 5, Command: harness.CmdInput, Text: "hello"},
		{ID: 6, Command: harness.CmdInterrupt},
		{ID: 7, Command: harness.CmdAnswer, Request: "r1", Choice: "Yes", Text: "why"},
	} {
		if err := c.reply(cmd); err != "" {
			t.Error(err)
		}
	}
	c.in.Close()
	if err := c.wait(); err != nil {
		t.Fatal(err)
	}
	want := []string{"input hello", "interrupt", "answer r1 Yes why", "stop"}
	if got := s.recorded(); !slices.Equal(got, want) {
		t.Fatal(got)
	}
}

func TestServeBeforeStart(t *testing.T) {
	c := serveStandIn(t, &standIn{script: "cat >/dev/null"})
	if err := c.reply(harness.Command{ID: 2, Command: harness.CmdInput, Text: "hello"}); err != "not started" {
		t.Error(err)
	}
	if err := c.reply(harness.Command{ID: 3, Command: harness.CmdStart}); err != "start without arguments" {
		t.Error(err)
	}
}

func TestServeMalformedCommand(t *testing.T) {
	c := serveStandIn(t, &standIn{script: "cat >/dev/null"})
	c.send("not json")
	if err := c.wait(); err == nil || !strings.HasPrefix(err.Error(), "malformed command") {
		t.Fatal(err)
	}
}

// When the harness ends by itself, Serve returns why, after the event of the
// harness's last line, which has no newline.
func TestServeEndsWithTheHarness(t *testing.T) {
	c := serveStandIn(t, &standIn{script: `read line; echo '{"text":"first"}'; printf '{"text":"last"}'; echo bye >&2; exit 3`})
	c.start()
	// The harness's events may come before the reply to input.
	c.command(harness.Command{ID: 2, Command: harness.CmdInput, Text: "go"})
	err := c.wait()
	var texts []string
	for r := range c.records {
		if r.Event != nil {
			for _, it := range r.Event.Items {
				texts = append(texts, it.Text)
			}
		}
	}
	if err == nil || err.Error() != "sh exited: exit status 3: bye" {
		t.Fatal(err)
	}
	if !slices.Equal(texts, []string{"first", "last"}) {
		t.Fatal(texts)
	}
}

func TestServeIgnoresResentCommands(t *testing.T) {
	s := &standIn{script: "cat >/dev/null"}
	c := serveStandIn(t, s)
	c.start()
	c.command(harness.Command{ID: 1, Command: harness.CmdStart, Start: &startArgs})
	cmd := harness.Command{ID: 2, Command: harness.CmdInput, Text: "once"}
	if err := c.reply(cmd); err != "" {
		t.Fatal(err)
	}
	c.command(cmd)
	c.command(harness.Command{ID: 1, Command: harness.CmdInput, Text: "old"})
	if err := c.reply(harness.Command{ID: 3, Command: harness.CmdInterrupt}); err != "" {
		t.Fatal(err)
	}
	c.in.Close()
	if err := c.wait(); err != nil {
		t.Fatal(err)
	}
	if got := s.recorded(); !slices.Equal(got, []string{"input once", "interrupt", "stop"}) {
		t.Fatal(got)
	}
}

func TestServeAnswersWhileInputPendingAndKeepsEffectsOrdered(t *testing.T) {
	s := &standIn{script: "cat >/dev/null"}
	accepted := make(chan struct{})
	release := sync.OnceFunc(func() { close(accepted) })
	defer release()
	started := make(chan struct{})
	c := serveStart(t, func(args harness.StartArgs, emit func(harness.Event)) (Agent, error) {
		ag, err := s.start(args, emit)
		ag.Input = func(text string, _ []harness.Image) error {
			s.record("input " + text)
			if text == "first" {
				close(started)
				<-accepted
				s.record("accepted first")
			}
			return nil
		}
		ag.Answer = func(request string, choices []string, _ string) error {
			s.record("answer " + strings.Join(choices, ", "))
			emit(harness.Event{Items: []harness.Item{{ID: request, Kind: harness.KindAttention, Done: true}}})
			return nil
		}
		ag.Configure = func(selection harness.Selection) (*harness.Selection, error) {
			s.record("configure " + selection.Model)
			return &selection, nil
		}
		return ag, err
	})
	c.start()
	c.command(harness.Command{ID: 2, Command: harness.CmdInput, Text: "first"})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("input did not start")
	}
	c.command(harness.Command{ID: 3, Command: harness.CmdInput, Text: "second"})
	c.command(harness.Command{ID: 4, Command: harness.CmdConfigure, Selection: &harness.Selection{Model: "next"}})
	c.command(harness.Command{ID: 5, Command: harness.CmdInput, Text: "third"})
	c.command(harness.Command{ID: 6, Command: harness.CmdAnswer, Request: "dialog", Choices: []string{"a", "b"}})
	r := c.next()
	if r.Event == nil || len(r.Event.Items) != 1 || r.Event.Items[0].ID != "dialog" || !r.Event.Items[0].Done {
		t.Fatalf("dialog completion: %+v", r)
	}
	r = c.next()
	if r.Reply == nil || r.Reply.ID != 6 || r.Reply.Error != "" {
		t.Fatalf("answer reply while input waits: %+v", r)
	}
	if calls := s.recorded(); !slices.Equal(calls, []string{"input first", "answer a, b"}) {
		t.Fatal(calls)
	}
	release()
	for _, id := range []int64{2, 3, 4, 5} {
		r := c.next()
		if r.Reply == nil || r.Reply.ID != id || r.Reply.Error != "" {
			t.Fatalf("reply to %d: %+v", id, r)
		}
	}
	c.in.Close()
	if err := c.wait(); err != nil {
		t.Fatal(err)
	}
	if calls := s.recorded(); !slices.Equal(calls, []string{"input first", "answer a, b", "accepted first", "input second", "configure next", "input third", "stop"}) {
		t.Fatal(calls)
	}
}

func TestServeInterruptsWhileInputPending(t *testing.T) {
	s := &standIn{script: "cat >/dev/null"}
	accepted := make(chan struct{})
	release := sync.OnceFunc(func() { close(accepted) })
	defer release()
	started := make(chan struct{})
	c := serveStart(t, func(args harness.StartArgs, emit func(harness.Event)) (Agent, error) {
		ag, err := s.start(args, emit)
		ag.Input = func(string, []harness.Image) error {
			s.record("input")
			close(started)
			<-accepted
			return nil
		}
		return ag, err
	})
	c.start()
	c.command(harness.Command{ID: 2, Command: harness.CmdInput, Text: "pending"})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("input did not start")
	}
	if err := c.reply(harness.Command{ID: 3, Command: harness.CmdInterrupt}); err != "" {
		t.Fatal(err)
	}
	if calls := s.recorded(); !slices.Equal(calls, []string{"input", "interrupt"}) {
		t.Fatal(calls)
	}
	release()
	r := c.next()
	if r.Reply == nil || r.Reply.ID != 2 || r.Reply.Error != "" {
		t.Fatalf("input reply: %+v", r)
	}
	c.in.Close()
	if err := c.wait(); err != nil {
		t.Fatal(err)
	}
}

func TestServeStopsAndJoinsPendingInput(t *testing.T) {
	s := &standIn{script: "cat >/dev/null"}
	started, stopped, joined := make(chan struct{}), make(chan struct{}), make(chan struct{})
	c := serveStart(t, func(args harness.StartArgs, emit func(harness.Event)) (Agent, error) {
		ag, err := s.start(args, emit)
		ag.Input = func(string, []harness.Image) error {
			close(started)
			<-stopped
			close(joined)
			return nil
		}
		stop := ag.Stop
		ag.Stop = func() { close(stopped); stop() }
		return ag, err
	})
	c.start()
	c.command(harness.Command{ID: 2, Command: harness.CmdInput, Text: "pending"})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("input did not start")
	}
	c.in.Close()
	if err := c.wait(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-joined:
	default:
		t.Fatal("Serve returned before input ended")
	}
}
