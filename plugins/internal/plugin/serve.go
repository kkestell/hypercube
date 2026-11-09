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
	// Selection is the model and effort the harness reports it started with,
	// when they differ from the requested ones. Start's reply carries it.
	Selection *harness.Selection
	Input     func(text string, images []harness.Image) error
	Interrupt func() error
	Answer    func(request string, choices []string, text string) error
	Configure func(harness.Selection) (*harness.Selection, error)
	Stop      func()
}

// Start starts the harness for the `start` command. The harness reports
// through emit.
type Start func(a harness.StartArgs, emit func(harness.Event)) (Agent, error)

// Serve runs a plugin's `serve` loop: it reads commands from in, drives the
// harness that start returns, and writes replies and events to out. It returns
// when in closes or when the harness ends.
func Serve(in io.Reader, out io.Writer, start Start) error {
	s := &server{enc: json.NewEncoder(out)}
	var inputDone <-chan error
	defer func() {
		if s.agent != nil {
			s.agent.Stop()
		}
		if inputDone != nil {
			<-inputDone
		}
	}()
	commands := make(chan harness.Command)
	failed := make(chan error, 1)
	stopped := make(chan struct{})
	defer close(stopped)
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
			select {
			case commands <- cmd:
			case <-stopped:
				return
			}
		}
		if err := scanner.Err(); err != nil {
			failed <- err
		}
	}()
	var last int64
	var exited <-chan struct{} // the started process's Done
	var inputID int64
	var queued []harness.Command
	for {
		if inputDone == nil && len(queued) > 0 {
			cmd := queued[0]
			queued = queued[1:]
			if cmd.Command == harness.CmdInput && s.agent != nil {
				// Only input waits off the command loop: native extensions may ask
				// for an answer before accepting it. Input and configuration stay
				// ordered; answers and interrupts can complete while input waits.
				done := make(chan error, 1)
				inputDone, inputID = done, cmd.ID
				go func() { done <- s.agent.Input(cmd.Text, cmd.Images) }()
				continue
			}
			selection, err := s.handle(cmd, start)
			s.reply(cmd.ID, selection, err)
			if err == nil && cmd.Command == harness.CmdStart {
				s.write(harness.Record{Event: &harness.Event{Conversation: s.agent.Conversation, State: harness.Idle}})
				exited = s.agent.Process.Done()
			}
			continue
		}
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
			if inputDone != nil && (cmd.Command == harness.CmdAnswer || cmd.Command == harness.CmdInterrupt) {
				selection, err := s.handle(cmd, start)
				s.reply(cmd.ID, selection, err)
				continue
			}
			queued = append(queued, cmd)
		case err := <-inputDone:
			s.reply(inputID, nil, err)
			inputDone = nil
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

func (s *server) reply(id int64, selection *harness.Selection, err error) {
	r := harness.Reply{ID: id, Selection: selection}
	if err != nil {
		r.Error = err.Error()
	}
	s.write(harness.Record{Reply: &r})
}

func (s *server) handle(cmd harness.Command, start Start) (*harness.Selection, error) {
	if cmd.Command == harness.CmdStart {
		if s.agent != nil {
			return nil, errors.New("already started")
		}
		if cmd.Start == nil {
			return nil, errors.New("start without arguments")
		}
		if cmd.Start.MCP == "" {
			return nil, errors.New("the start names no MCP server")
		}
		a, err := start(*cmd.Start, func(ev harness.Event) { s.write(harness.Record{Event: &ev}) })
		if err != nil {
			return nil, err
		}
		s.agent = &a
		return a.Selection, nil
	}
	if s.agent == nil {
		return nil, errors.New("not started")
	}
	switch cmd.Command {
	case harness.CmdConfigure:
		if cmd.Selection == nil {
			return nil, errors.New("configure without selection")
		}
		return s.agent.Configure(*cmd.Selection)
	case harness.CmdInput:
		return nil, s.agent.Input(cmd.Text, cmd.Images)
	case harness.CmdInterrupt:
		return nil, s.agent.Interrupt()
	case harness.CmdAnswer:
		return nil, s.agent.Answer(cmd.Request, cmd.Choices, cmd.Text)
	}
	return nil, fmt.Errorf("unknown command %q", cmd.Command)
}
