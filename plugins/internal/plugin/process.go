package plugin

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const stopTimeout = 3 * time.Second

// ReadLines calls handle for each non-empty line of r, including a last line
// without a newline, and returns the error that ended r.
func ReadLines(r io.Reader, handle func(line []byte)) error {
	reader := bufio.NewReaderSize(r, 1<<20)
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			handle(line)
		}
		if err != nil {
			return err
		}
	}
}

// Process is a running harness CLI. It owns the CLI's pipes, reading its
// output, the reason it ended, and stopping it.
type Process struct {
	// Stdin writes to the CLI's input. A failed write means the CLI has closed
	// its input or exited, so Write waits for the CLI to end and returns the
	// exit reason instead of the pipe's error.
	Stdin io.WriteCloser

	cmd    *exec.Cmd
	stdout *os.File
	stderr tail
	exited chan struct{} // closed when the CLI has exited; err is set
	done   chan struct{} // closed when Read has handled the last line
	err    error
}

// StartProcess starts cmd. product is the harness's name for a user who has not
// installed it.
func StartProcess(product string, cmd *exec.Cmd) (*Process, error) {
	name := cmd.Args[0]
	if errors.Is(cmd.Err, exec.ErrNotFound) {
		return nil, fmt.Errorf("%s is missing: install %s", product, name)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	// The process owns the read end of stdout instead of using StdoutPipe,
	// which Wait closes as soon as the CLI exits, dropping output not yet
	// read. A CLI that fails to resume prints why and exits at once.
	stdout, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	p := &Process{cmd: cmd, stdout: stdout, exited: make(chan struct{}), done: make(chan struct{})}
	p.Stdin = stdinPipe{WriteCloser: stdin, p: p}
	cmd.Stdout = w
	cmd.Stderr = io.MultiWriter(os.Stderr, &p.stderr)
	// The CLI's children may keep its stderr open after it ends.
	cmd.WaitDelay = time.Second
	err = cmd.Start()
	w.Close()
	if err != nil {
		stdout.Close()
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	go func() {
		err := cmd.Wait()
		reason := name + " exited"
		if err != nil {
			reason += ": " + err.Error()
		}
		if detail := p.stderr.last(); detail != "" {
			reason += ": " + detail
		}
		p.err = errors.New(reason)
		// Children that keep stdout open cannot hold the reader for long.
		_ = stdout.SetReadDeadline(time.Now().Add(time.Second))
		close(p.exited)
	}()
	return p, nil
}

// Read calls handle for each line of the output until the output ends, then
// returns the exit reason. Done closes after the last handle call returns.
// Call it once.
func (p *Process) Read(handle func(line []byte)) error {
	_ = ReadLines(p.stdout, handle)
	<-p.exited
	p.stdout.Close()
	close(p.done)
	return p.err
}

// stdinPipe is the CLI's input.
type stdinPipe struct {
	io.WriteCloser
	p *Process
}

func (s stdinPipe) Write(b []byte) (int, error) {
	n, err := s.WriteCloser.Write(b)
	if err != nil {
		<-s.p.exited
		return n, s.p.err
	}
	return n, nil
}

// Done is closed when the CLI has exited and Read has handled its output.
func (p *Process) Done() <-chan struct{} { return p.done }

// Err says why the CLI ended. It is valid once Done is closed.
func (p *Process) Err() error { return p.err }

// Stop ends the CLI: closing its input lets it finish, and it is killed if it
// does not. Its children share the plugin's process group, which Hypercube
// kills.
func (p *Process) Stop() {
	_ = p.Stdin.Close()
	select {
	case <-p.done:
	case <-time.After(stopTimeout):
		_ = p.cmd.Process.Kill()
	}
}

// tail keeps the end of a CLI's diagnostics for its exit reason.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 2048 {
		t.buf = t.buf[len(t.buf)-2048:]
	}
	return len(p), nil
}

func (t *tail) last() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}
