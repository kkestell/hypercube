// Package terminal runs one shell per session on a pseudo-terminal. A shell
// starts on demand, keeps running while no one is connected, and keeps a
// bounded buffer of its recent output for the next connection.
package terminal

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	// bufferSize is how much recent output a shell keeps.
	bufferSize = 256 << 10
	// stopWait is how long a shell has to end after SIGHUP before it is killed.
	stopWait = 2 * time.Second
	// A subscriber that falls this many chunks behind is dropped.
	backlog = 256
)

// Set holds the sessions' shells.
type Set struct {
	// Command is the program a shell runs. Empty means the user's login shell.
	Command []string

	mu     sync.Mutex
	shells map[string]*Shell
	closed bool
}

// Shell is one running shell.
type Shell struct {
	cmd  *exec.Cmd
	pty  *os.File
	done chan struct{}

	mu   sync.Mutex
	buf  []byte
	subs map[chan []byte]struct{}
	exit int
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

// Open returns the session's shell, starting it in dir at the given size if
// the session has none.
func (s *Set) Open(session, dir string, cols, rows int) (*Shell, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errClosed
	}
	if sh := s.shells[session]; sh != nil {
		return sh, nil
	}
	argv := s.command()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	sh := &Shell{cmd: cmd, pty: f, done: make(chan struct{}), subs: map[chan []byte]struct{}{}}
	if s.shells == nil {
		s.shells = map[string]*Shell{}
	}
	s.shells[session] = sh
	go func() {
		sh.run()
		s.mu.Lock()
		if s.shells[session] == sh {
			delete(s.shells, session)
		}
		s.mu.Unlock()
	}()
	return sh, nil
}

// Stop ends the session's shell, if it has one, and waits for it to end.
func (s *Set) Stop(session string) {
	s.mu.Lock()
	sh := s.shells[session]
	s.mu.Unlock()
	if sh != nil {
		sh.stop()
	}
}

// Close ends every shell and refuses new ones.
func (s *Set) Close() {
	s.mu.Lock()
	s.closed = true
	var all []*Shell
	for _, sh := range s.shells {
		all = append(all, sh)
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, sh := range all {
		wg.Go(sh.stop)
	}
	wg.Wait()
}

// run copies the shell's output to its buffer and subscribers until it ends.
func (sh *Shell) run() {
	chunk := make([]byte, 32<<10)
	for {
		n, err := sh.pty.Read(chunk)
		if n > 0 {
			sh.publish(append([]byte(nil), chunk[:n]...))
		}
		if err != nil {
			break
		}
	}
	_ = sh.cmd.Wait()
	_ = sh.pty.Close()
	sh.mu.Lock()
	sh.exit = sh.cmd.ProcessState.ExitCode()
	close(sh.done)
	for ch := range sh.subs {
		close(ch)
	}
	sh.subs = nil
	sh.mu.Unlock()
}

func (sh *Shell) publish(b []byte) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.buf = append(sh.buf, b...)
	if extra := len(sh.buf) - bufferSize; extra > 0 {
		sh.buf = append([]byte(nil), sh.buf[extra:]...)
	}
	for ch := range sh.subs {
		select {
		case ch <- b:
		default:
			close(ch)
			delete(sh.subs, ch)
		}
	}
}

func (sh *Shell) stop() {
	_ = syscall.Kill(-sh.cmd.Process.Pid, syscall.SIGHUP)
	select {
	case <-sh.done:
		return
	case <-time.After(stopWait):
	}
	_ = syscall.Kill(-sh.cmd.Process.Pid, syscall.SIGKILL)
	_ = sh.pty.Close()
	<-sh.done
}

// Subscribe returns the output so far and a channel of the output after it.
// The channel closes when the shell ends, or when the subscriber falls too far
// behind; Done tells which. Cancel releases the subscription.
func (sh *Shell) Subscribe() (replay []byte, out <-chan []byte, cancel func()) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	ch := make(chan []byte, backlog)
	if sh.subs == nil {
		close(ch)
	} else {
		sh.subs[ch] = struct{}{}
	}
	return append([]byte(nil), sh.buf...), ch, func() {
		sh.mu.Lock()
		defer sh.mu.Unlock()
		if _, ok := sh.subs[ch]; ok {
			delete(sh.subs, ch)
			close(ch)
		}
	}
}

// Write sends input to the shell.
func (sh *Shell) Write(p []byte) error {
	_, err := sh.pty.Write(p)
	return err
}

// Resize sets the shell's terminal size.
func (sh *Shell) Resize(cols, rows int) error {
	return pty.Setsize(sh.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

// Done is closed when the shell has ended.
func (sh *Shell) Done() <-chan struct{} { return sh.done }

// ExitCode is the shell's exit status once it has ended.
func (sh *Shell) ExitCode() int {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.exit
}
