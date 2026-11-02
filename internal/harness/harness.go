// Package harness runs agents through harness plugins. A plugin is an
// executable named hypercube-harness-<name> beside hypercube. Its `serve`
// command keeps a native harness alive across turns and translates between the
// harness and the protocol in protocol.go. A plugin builds the harness's
// command line from per-launch arguments, environment, and overrides, without
// changing the user's settings, credentials, or HOME.
package harness

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Find returns the path of the harness's plugin in dir, or beside the running
// executable when dir is empty.
func Find(dir, name string) (string, error) {
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		dir = filepath.Dir(exe)
	}
	path := filepath.Join(dir, "hypercube-harness-"+name)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("harness %q has no plugin: install %s", name, path)
	}
	return path, nil
}

// Client is a running plugin and its connection.
type Client struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	tail  *tailBuffer
	done  chan struct{}

	writing sync.Mutex // serializes writes to stdin

	mu      sync.Mutex
	next    int64
	pending map[string]chan Reply
	err     error // why the plugin ended; set before done closes
}

// Launch starts the plugin's `serve` command in its own process group.
// onEvent receives its events in order, on one goroutine. onExit follows the
// last event, once, with the reason the plugin ended: it exited, or it broke
// the protocol, which also kills it.
func Launch(path string, onEvent func(Event), onExit func(error)) (*Client, error) {
	cmd := exec.Command(path, "serve")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	// The plugin's descendants may keep the pipes open after it exits, so the
	// reader owns the read end and the group is killed when the plugin ends.
	read, write, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	tail := &tailBuffer{max: 2048}
	cmd.Stdout = write
	cmd.Stderr = io.MultiWriter(os.Stderr, tail)
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		read.Close()
		write.Close()
		return nil, err
	}
	write.Close()
	c := &Client{cmd: cmd, stdin: stdin, tail: tail, done: make(chan struct{}), pending: map[string]chan Reply{}}
	go c.run(read, filepath.Base(path), onEvent, onExit)
	return c, nil
}

func (c *Client) run(read *os.File, name string, onEvent func(Event), onExit func(error)) {
	waited := make(chan error, 1)
	go func() {
		err := c.cmd.Wait()
		c.killGroup()
		waited <- err
	}()
	failure := c.read(read, onEvent)
	if failure != nil {
		c.killGroup()
	}
	waitErr := <-waited
	read.Close()
	reason := failure
	if reason == nil {
		reason = fmt.Errorf("%s ended", name)
		if waitErr != nil {
			reason = fmt.Errorf("%s ended: %w", name, waitErr)
		}
	}
	if detail := c.tail.last(); detail != "" {
		reason = fmt.Errorf("%w: %s", reason, detail)
	}
	c.mu.Lock()
	c.err = reason
	c.mu.Unlock()
	c.stdin.Close()
	close(c.done)
	onExit(reason)
}

// read dispatches the plugin's records until its output ends. It returns why
// the connection is unusable, or nil at the end of the output.
func (c *Client) read(r io.Reader, onEvent func(Event)) error {
	lines := bufio.NewReader(r)
	for {
		line, err := lines.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			rec, derr := decode(line)
			if derr != nil {
				return derr
			}
			if rec.Event != nil {
				onEvent(*rec.Event)
			} else if !c.reply(*rec.Reply) {
				return fmt.Errorf("reply to unknown request %q", rec.Reply.ID)
			}
		}
		if err != nil {
			return nil
		}
	}
}

func (c *Client) reply(r Reply) bool {
	c.mu.Lock()
	ch, ok := c.pending[r.ID]
	delete(c.pending, r.ID)
	c.mu.Unlock()
	if ok {
		ch <- r
	}
	return ok
}

// Do sends the command and waits for its reply. A failed reply is an error.
// Giving up on ctx leaves the request open, so its late reply is still a
// known one.
func (c *Client) Do(ctx context.Context, cmd Command) error {
	c.mu.Lock()
	if c.err != nil {
		defer c.mu.Unlock()
		return c.err
	}
	c.next++
	cmd.ID = strconv.FormatInt(c.next, 10)
	reply := make(chan Reply, 1)
	c.pending[cmd.ID] = reply
	c.mu.Unlock()
	line, err := jsonLine(cmd)
	if err == nil {
		c.writing.Lock()
		_, err = c.stdin.Write(line)
		c.writing.Unlock()
	}
	if err != nil {
		c.mu.Lock()
		delete(c.pending, cmd.ID)
		c.mu.Unlock()
		// A write fails when the plugin has ended; if the reader has seen
		// that, its reason is the better explanation.
		select {
		case <-c.done:
			return c.exitErr()
		default:
			return err
		}
	}
	answer := func(r Reply) error {
		if r.Error != "" {
			return errors.New(r.Error)
		}
		return nil
	}
	select {
	case r := <-reply:
		return answer(r)
	case <-c.done:
		select {
		case r := <-reply:
			return answer(r)
		default:
			return c.exitErr()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) exitErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Shutdown asks the plugin to end, waits up to wait for it, and then kills its
// process group.
func (c *Client) Shutdown(wait time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	// A plugin that stops reading can leave the write blocked; killing the
	// process breaks the pipe and frees it.
	go c.Do(ctx, Command{Command: CmdShutdown})
	select {
	case <-c.done:
	case <-ctx.Done():
	}
	c.Kill()
}

// Kill kills the plugin's process group and waits for the connection to end.
func (c *Client) Kill() {
	c.killGroup()
	<-c.done
}

func (c *Client) killGroup() {
	_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
}

// tailBuffer keeps the end of the plugin's diagnostics for error messages.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

// last returns the final line of diagnostics.
func (t *tailBuffer) last() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(string(t.buf)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
