package plugin

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/kkestell/hypercube/internal/harness"
)

// Agent is a started harness, as the serve loop drives it. Stop ends it.
type Agent struct {
	Process      *Process // Serve returns Process.Err() when it ends by itself
	Conversation string   // reported with the idle event that follows start's reply
	Input        func(text string, images []harness.Image) error
	Interrupt    func() error
	Answer       func(request, choice, text string) error
	Stop         func()
}

// Start starts the harness for the `start` command. The harness reports
// through emit.
type Start func(a harness.StartArgs, emit func(harness.Event)) (Agent, error)

// Serve runs a plugin's `serve` loop: it reads commands from in, drives the
// harness that start returns, and writes replies and events to out. It returns
// when in closes or when the harness ends.
func Serve(in io.Reader, out io.Writer, start Start) error {
	s := &server{enc: json.NewEncoder(out)}
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
	var last int64
	var exited <-chan struct{} // the started process's Done
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
			if cmd.ID <= last {
				continue
			}
			last = cmd.ID
			err := s.handle(cmd, start)
			s.reply(cmd.ID, err)
			if err == nil && cmd.Command == harness.CmdStart {
				s.write(harness.Record{Event: &harness.Event{Conversation: s.agent.Conversation, State: harness.Idle}})
				exited = s.agent.Process.Done()
			}
		case <-exited:
			return s.agent.Process.Err()
		}
	}
}

type server struct {
	outMu sync.Mutex
	enc   *json.Encoder

	agent *Agent // set by start
}

func (s *server) write(r harness.Record) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_ = s.enc.Encode(r)
}

func (s *server) reply(id int64, err error) {
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
		a, err := start(*cmd.Start, func(ev harness.Event) { s.write(harness.Record{Event: &ev}) })
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
