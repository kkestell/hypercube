// Package hold runs children independently of the app. Each holder owns a
// directory:
//
//   - lock: locked for the holder's whole life; Alive tests it.
//   - pid, and tty in terminal mode: the holder's process and terminal.
//   - in: the socket that carries input to the child.
//   - out-<offset>: retained output in segments named by their first byte's
//     offset. Terminal mode keeps the last two; pipe mode keeps them until
//     Discard.
//   - exit: the child's exit record, renamed into place after all output and
//     before the lock is released, so a dead holder without one was lost.
package hold

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const segmentSize = 256 << 10

var (
	ErrLost   = errors.New("holder died without an exit record")
	ErrClosed = errors.New("output closed")
)

type Size struct{ Cols, Rows int }
type Exit struct {
	Code   int    `json:"code"`
	Status string `json:"status"`
	Stderr string `json:"stderr,omitempty"`
}

type Holder struct {
	dir    string
	mu     sync.Mutex
	input  net.Conn
	closed bool
}

// Start runs holder, the hypercube-hold executable, detached, to start argv
// with the environment and working directory in the holder directory dir.
// size starts it on a terminal of that size.
func Start(holder, dir string, argv []string, env []string, cwd string, size *Size) (*Holder, error) {
	if len(filepath.Join(dir, "in")) > 103 {
		return nil, fmt.Errorf("holder socket path is longer than 103 bytes: %s", dir)
	}
	if len(argv) == 0 {
		return nil, errors.New("holder needs a command")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(dir, "in"), Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(false)
	defer listener.Close()
	socket, err := listener.File()
	if err != nil {
		return nil, err
	}
	defer socket.Close()
	var args []string
	if size != nil {
		args = append(args, "-terminal", "-cols", strconv.Itoa(size.Cols), "-rows", strconv.Itoa(size.Rows))
	}
	args = append(args, dir)
	args = append(args, argv...)
	cmd := exec.Command(holder, args...)
	cmd.Env, cmd.Dir, cmd.Stderr = env, cwd, os.Stderr
	cmd.ExtraFiles = []*os.File{lock, socket}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		os.Remove(filepath.Join(dir, "in"))
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	return &Holder{dir: dir}, nil
}

func Open(dir string) (*Holder, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}
	return &Holder{dir: dir}, nil
}
func (h *Holder) Write(p []byte) (int, error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return 0, net.ErrClosed
	}
	if h.input == nil {
		c, err := net.Dial("unix", filepath.Join(h.dir, "in"))
		if err != nil {
			h.mu.Unlock()
			return 0, err
		}
		h.input = c
	}
	conn := h.input
	h.mu.Unlock()
	// Close must be able to release a write whose child stopped reading.
	return conn.Write(p)
}

// Close releases this attachment's input connection without stopping the child.
func (h *Holder) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	if h.input != nil {
		err := h.input.Close()
		h.input = nil
		return err
	}
	return nil
}
func (h *Holder) Alive() bool {
	f, err := os.OpenFile(filepath.Join(h.dir, "lock"), os.O_RDWR, 0600)
	if err != nil {
		return false
	}
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	return errors.Is(err, syscall.EWOULDBLOCK)
}
func (h *Holder) Stop() error {
	for h.Alive() {
		b, err := os.ReadFile(filepath.Join(h.dir, "pid"))
		if errors.Is(err, os.ErrNotExist) {
			time.Sleep(25 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil {
			return err
		}
		if h.Alive() {
			return syscall.Kill(pid, syscall.SIGTERM)
		}
		return nil
	}
	return nil
}
func (h *Holder) Exit() (Exit, error) {
	var e Exit
	b, err := os.ReadFile(filepath.Join(h.dir, "exit"))
	if err == nil {
		err = json.Unmarshal(b, &e)
	}
	return e, err
}
func (h *Holder) Remove() error {
	for h.Alive() {
		time.Sleep(25 * time.Millisecond)
	}
	_ = h.Close()
	return os.RemoveAll(h.dir)
}
func (h *Holder) Resize(cols, rows int) error {
	b, err := os.ReadFile(filepath.Join(h.dir, "tty"))
	if err != nil {
		return err
	}
	f, err := os.OpenFile(string(b), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return pty.Setsize(f, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
func (h *Holder) segments() ([]int64, error) {
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		return nil, err
	}
	var positions []int64
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "out-") {
			n, err := strconv.ParseInt(strings.TrimPrefix(e.Name(), "out-"), 10, 64)
			if err != nil {
				return nil, err
			}
			positions = append(positions, n)
		}
	}
	slices.Sort(positions)
	return positions, nil
}
func (h *Holder) segment(position int64) string {
	return filepath.Join(h.dir, fmt.Sprintf("out-%d", position))
}
func (h *Holder) Discard(before int64) error {
	p, err := h.segments()
	if err != nil {
		return err
	}
	for i := 0; i+1 < len(p); i++ {
		if p[i+1] <= before {
			if err := os.Remove(h.segment(p[i])); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
