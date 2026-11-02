// Package terminal runs each session's panel shell under a terminal holder.
// Each connection reads the holder's retained output directly, so scrollback
// survives an app restart.
package terminal

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/kkestell/hypercube/internal/hold"
)

// Set holds one shell per session.
type Set struct {
	Command []string
	// Dir is the holders' parent directory.
	Dir    string
	mu     sync.Mutex
	shells map[string]*Shell
	closed bool
}

// Shell is a session's shell holder. It may have ended.
type Shell struct{ holder *hold.Holder }

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

// Name is the name of the session's shell holder directory in Dir.
func Name(session string) string { return "shell-" + session }

func (s *Set) dir(session string) string { return filepath.Join(s.Dir, Name(session)) }

// Open returns the session's running shell, or starts one in dir in place of
// one that ended.
func (s *Set) Open(session, dir string, cols, rows int) (*Shell, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errClosed
	}
	if sh := s.shells[session]; sh != nil && sh.holder.Alive() {
		return sh, nil
	}
	if err := os.RemoveAll(s.dir(session)); err != nil {
		return nil, err
	}
	h, err := hold.Start(s.dir(session), s.command(), append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor"), dir, &hold.Size{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	if s.shells == nil {
		s.shells = map[string]*Shell{}
	}
	sh := &Shell{holder: h}
	s.shells[session] = sh
	return sh, nil
}

// Adopt takes over the session's shell after an app restart, removing it if
// it has ended.
func (s *Set) Adopt(session string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errClosed
	}
	h, err := hold.Open(s.dir(session))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !h.Alive() {
		return h.Remove()
	}
	if s.shells == nil {
		s.shells = map[string]*Shell{}
	}
	s.shells[session] = &Shell{holder: h}
	return nil
}

// Stop ends the session's shell and removes its holder.
func (s *Set) Stop(session string) {
	s.mu.Lock()
	sh := s.shells[session]
	delete(s.shells, session)
	s.mu.Unlock()
	if sh != nil {
		sh.end()
	}
}

// Close ends every shell, for Quit, and opens no more.
func (s *Set) Close() {
	s.mu.Lock()
	s.closed = true
	shells := s.shells
	s.shells = nil
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, sh := range shells {
		wg.Go(sh.end)
	}
	wg.Wait()
}

// end stops the holder; removing it waits for it to exit.
func (sh *Shell) end() {
	_ = sh.holder.Stop()
	_ = sh.holder.Remove()
}

// Output reads the shell's retained output, then what it writes next.
func (sh *Shell) Output() (*hold.Output, error) {
	return sh.holder.Output(sh.holder.First())
}
func (sh *Shell) Write(p []byte) error        { _, err := sh.holder.Write(p); return err }
func (sh *Shell) Resize(cols, rows int) error { return sh.holder.Resize(cols, rows) }

// ExitCode is the ended shell's exit code, or -1 when it has none.
func (sh *Shell) ExitCode() int {
	e, err := sh.holder.Exit()
	if err != nil {
		return -1
	}
	return e.Code
}
