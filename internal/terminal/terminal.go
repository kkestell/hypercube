// Package terminal attaches panel shells to terminal holders. Each connection
// reads retained output directly, so scrollback survives an app restart.
package terminal

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kkestell/hypercube/internal/hold"
)

type Set struct {
	Command []string
	// Dir is the holders' parent directory.
	Dir      string
	mu       sync.Mutex
	shells   map[string]*Shell
	closed   bool
	detached chan struct{}
}

type Shell struct {
	holder  *hold.Holder
	done    chan struct{}
	removed chan struct{}
	mu      sync.Mutex
	readers sync.WaitGroup
	ended   bool
	exit    int
}

var errClosed = errors.New("hypercube is shutting down")

func (s *Set) command() []string {
	if len(s.Command) > 0 {
		return s.Command
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	return []string{shell, "-l"}
}
func (s *Set) Open(session, dir string, cols, rows int) (*Shell, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errClosed
	}
	if sh := s.shells[session]; sh != nil {
		return sh, nil
	}
	h, err := hold.Start(filepath.Join(s.Dir, "shell-"+session), s.command(), append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor"), dir, &hold.Size{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	return s.adopt(session, h), nil
}
func (s *Set) Adopt(session, dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errClosed
	}
	if s.shells[session] != nil {
		return nil
	}
	h, err := hold.Open(dir)
	if err != nil {
		return err
	}
	if !h.Alive() {
		return h.Remove()
	}
	s.adopt(session, h)
	return nil
}
func (s *Set) adopt(session string, h *hold.Holder) *Shell {
	if s.shells == nil {
		s.shells = map[string]*Shell{}
	}
	if s.detached == nil {
		s.detached = make(chan struct{})
	}
	detached := s.detached
	sh := &Shell{holder: h, done: make(chan struct{}), removed: make(chan struct{})}
	s.shells[session] = sh
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for h.Alive() {
			select {
			case <-detached:
				return
			case <-ticker.C:
			}
		}
		sh.mu.Lock()
		sh.ended = true
		e, err := h.Exit()
		sh.exit = -1
		if err == nil {
			sh.exit = e.Code
		}
		close(sh.done)
		sh.mu.Unlock()
		// Readers finish the final segments before their files are removed.
		sh.readers.Wait()
		s.mu.Lock()
		defer s.mu.Unlock()
		select {
		case <-detached:
			return
		default:
		}
		_ = h.Remove()
		if s.shells[session] == sh {
			delete(s.shells, session)
		}
		close(sh.removed)
	}()
	return sh
}
func (s *Set) Stop(session string) {
	s.mu.Lock()
	sh := s.shells[session]
	s.mu.Unlock()
	if sh != nil {
		_ = sh.holder.Stop()
		<-sh.removed
	}
}
func (s *Set) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	var all []*Shell
	for _, sh := range s.shells {
		all = append(all, sh)
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, sh := range all {
		wg.Go(func() { _ = sh.holder.Stop(); <-sh.removed })
	}
	wg.Wait()
}
func (s *Set) Detach() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		if s.detached != nil {
			close(s.detached)
		}
	}
}

// Output is one connection's cursor through retained scrollback.
type Output struct {
	*hold.Output
	sh   *Shell
	once sync.Once
}

func (sh *Shell) Output() (*Output, error) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.ended {
		return nil, errors.New("shell ended")
	}
	output, err := sh.holder.Output(sh.holder.First())
	if err != nil {
		return nil, err
	}
	sh.readers.Add(1)
	return &Output{Output: output, sh: sh}, nil
}
func (o *Output) Close() error {
	o.once.Do(func() { _ = o.Output.Close(); o.sh.readers.Done() })
	return nil
}
func (sh *Shell) Write(p []byte) error        { _, err := sh.holder.Write(p); return err }
func (sh *Shell) Resize(cols, rows int) error { return sh.holder.Resize(cols, rows) }
func (sh *Shell) Done() <-chan struct{}       { return sh.done }
func (sh *Shell) ExitCode() int               { sh.mu.Lock(); defer sh.mu.Unlock(); return sh.exit }
