package plugin

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/kkestell/hypercube/internal/harness"
)

// Agent is a started harness, as the serve loop drives it. Idle reports the
// harness as waiting for input once start has replied; Stop ends it.
type Agent struct {
	Input     func(text string, images []harness.Image) error
	Interrupt func() error
	Answer    func(request, choice, text string) error
	Idle      func()
	Stop      func()
}

// Start starts the harness for the `start` command. The harness reports
// through emit and sends the reason on exited if it ends by itself.
type Start func(a harness.StartArgs, emit func(harness.Event), exited chan<- error) (Agent, error)

// Serve runs a plugin's `serve` loop: it reads commands from in, drives the
// harness that start returns, and writes replies and events to out. It returns
// when in closes, on `shutdown`, or when the harness ends.
func Serve(in io.Reader, out io.Writer, start Start) error {
	s := &server{enc: json.NewEncoder(out), exited: make(chan error, 1)}
	defer func() {
		if s.agent != nil {
			s.agent.Stop()
		}
	}()
	commands := make(chan harness.Command)
	failed := make(chan error, 1)
	go func() {
		defer close(commands)
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 1<<20), 64<<20)
		for scanner.Scan() {
			var cmd harness.Command
			if err := json.Unmarshal(scanner.Bytes(), &cmd); err != nil {
				failed <- fmt.Errorf("malformed command: %w", err)
				return
			}
			commands <- cmd
		}
		if err := scanner.Err(); err != nil {
			failed <- err
		}
	}()
	for {
		select {
		case cmd, ok := <-commands:
			if !ok {
				select {
				case err := <-failed:
					return err
				default:
					return nil
				}
			}
			if cmd.Command == harness.CmdShutdown {
				s.reply(cmd.ID, nil)
				return nil
			}
			err := s.handle(cmd, start)
			s.reply(cmd.ID, err)
			if err == nil && cmd.Command == harness.CmdStart {
				s.agent.Idle()
			}
		case err := <-s.exited:
			return err
		}
	}
}

type server struct {
	outMu sync.Mutex
	enc   *json.Encoder

	agent  *Agent // set by start
	exited chan error
}

func (s *server) write(r harness.Record) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_ = s.enc.Encode(r)
}

func (s *server) reply(id string, err error) {
	r := harness.Reply{ID: id}
	if err != nil {
		r.Error = err.Error()
	}
	s.write(harness.Record{Reply: &r})
}

func (s *server) handle(cmd harness.Command, start Start) error {
	if cmd.Command == harness.CmdStart {
		if s.agent != nil {
			return errors.New("already started")
		}
		if cmd.Start == nil {
			return errors.New("start without arguments")
		}
		a, err := start(*cmd.Start, func(ev harness.Event) { s.write(harness.Record{Event: &ev}) }, s.exited)
		if err != nil {
			return err
		}
		s.agent = &a
		return nil
	}
	if s.agent == nil {
		return errors.New("not started")
	}
	switch cmd.Command {
	case harness.CmdInput:
		return s.agent.Input(cmd.Text, cmd.Images)
	case harness.CmdInterrupt:
		return s.agent.Interrupt()
	case harness.CmdAnswer:
		return s.agent.Answer(cmd.Request, cmd.Choice, cmd.Text)
	}
	return fmt.Errorf("unknown command %q", cmd.Command)
}

// Tail keeps the end of a harness's diagnostics for error messages.
type Tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *Tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 2048 {
		t.buf = t.buf[len(t.buf)-2048:]
	}
	return len(p), nil
}

// Last returns the kept diagnostics.
func (t *Tail) Last() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}
