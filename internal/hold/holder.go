package hold

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Main runs one child. Files 3 and 4 are the lock and input listener inherited
// from Start, so neither ownership nor readiness depends on the app staying up.
func Main(args []string) error {
	flags := flag.NewFlagSet("hold", flag.ContinueOnError)
	terminal := flags.Bool("terminal", false, "")
	cols := flags.Int("cols", 80, "")
	rows := flags.Int("rows", 24, "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	args = flags.Args()
	if len(args) < 2 {
		return fmt.Errorf("usage: hypercube hold [-terminal -cols N -rows N] <dir> <command> [args...]")
	}
	h := &Holder{dir: args[0]}
	lock := os.NewFile(3, "lock")
	defer lock.Close()
	syscall.CloseOnExec(3)
	if err := h.publish("pid", []byte(strconv.Itoa(os.Getpid()))); err != nil {
		return err
	}
	socket := os.NewFile(4, "in")
	listener, err := net.FileListener(socket)
	socket.Close()
	if err != nil {
		return err
	}
	defer listener.Close()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	defer signal.Stop(signals)
	var size *Size
	if *terminal {
		size = &Size{Cols: *cols, Rows: *rows}
	}
	child, err := startChild(args[1:], h.dir, size)
	if err != nil {
		return h.writeExit(Exit{Code: -1, Status: err.Error()})
	}
	defer child.output.Close()
	go serveInput(listener, child.input, *terminal)
	copied := make(chan error, 1)
	go func() { copied <- h.copyOutput(child.output, *terminal) }()
	waited := make(chan error, 1)
	go func() { waited <- child.wait(signals, *terminal) }()
	var waitErr error
	select {
	case err = <-copied:
		if err != nil {
			_ = syscall.Kill(-child.cmd.Process.Pid, syscall.SIGKILL)
		}
		waitErr = <-waited
	case waitErr = <-waited:
		// A descendant can keep the output open after the child exits; stop
		// copying after a second rather than wait for it.
		select {
		case err = <-copied:
		case <-time.After(time.Second):
			child.output.Close()
			err = <-copied
		}
	}
	_ = syscall.Kill(-child.cmd.Process.Pid, syscall.SIGKILL)
	<-child.stderrDone
	status := ""
	if waitErr != nil {
		status = waitErr.Error()
	}
	if err != nil {
		status = err.Error()
	}
	return h.writeExit(Exit{Code: child.cmd.ProcessState.ExitCode(), Status: status, Stderr: child.tail.String()})
}

// child owns the pipes or terminal for the one process being held.
type child struct {
	cmd        *exec.Cmd
	input      io.WriteCloser
	output     *os.File
	tail       diagnostics
	stderrDone chan struct{}
}

func startChild(argv []string, dir string, size *Size) (*child, error) {
	c := &child{cmd: exec.Command(argv[0], argv[1:]...), stderrDone: make(chan struct{})}
	var err error
	if size != nil {
		err = c.startTerminal(dir, *size)
	} else {
		err = c.startPipes()
	}
	if err != nil {
		if c.input != nil {
			c.input.Close()
		}
		if c.output != nil {
			c.output.Close()
		}
		return nil, err
	}
	return c, nil
}

func (c *child) startTerminal(dir string, size Size) error {
	master, tty, err := pty.Open()
	if err != nil {
		return err
	}
	defer tty.Close()
	c.input, c.output = master, master
	if err = pty.Setsize(master, &pty.Winsize{Cols: uint16(size.Cols), Rows: uint16(size.Rows)}); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "tty"), []byte(tty.Name()), 0600); err != nil {
		return err
	}
	c.cmd.Stdin, c.cmd.Stdout, c.cmd.Stderr = tty, tty, tty
	c.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	close(c.stderrDone)
	return c.cmd.Start()
}

func (c *child) startPipes() error {
	c.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var err error
	c.input, err = c.cmd.StdinPipe()
	if err != nil {
		return err
	}
	var write *os.File
	c.output, write, err = os.Pipe()
	if err != nil {
		return err
	}
	defer write.Close()
	stderr, stderrWrite, err := os.Pipe()
	if err != nil {
		return err
	}
	defer stderrWrite.Close()
	c.cmd.Stdout, c.cmd.Stderr = write, stderrWrite
	if err = c.cmd.Start(); err != nil {
		stderr.Close()
		return err
	}
	go func() { _, _ = io.Copy(&c.tail, stderr); stderr.Close(); close(c.stderrDone) }()
	return nil
}

func (c *child) wait(signals <-chan os.Signal, terminal bool) error {
	waited := make(chan error, 1)
	go func() { waited <- c.cmd.Wait() }()
	select {
	case err := <-waited:
		return err
	case <-signals:
		if terminal {
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGHUP)
		} else {
			_ = c.input.Close()
		}
		// Give the child a moment to exit cleanly before killing its group.
		select {
		case err := <-waited:
			return err
		case <-time.After(3 * time.Second):
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
			return <-waited
		}
	}
}

func serveInput(listener net.Listener, input io.Writer, terminal bool) {
	var writing sync.Mutex
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			if terminal {
				_, _ = io.Copy(input, conn)
				return
			}
			lines := bufio.NewReader(conn)
			for {
				line, err := lines.ReadBytes('\n')
				if err != nil {
					return
				}
				writing.Lock()
				_, err = input.Write(line)
				writing.Unlock()
				if err != nil {
					return
				}
			}
		}()
	}
}

func (h *Holder) writeExit(e Exit) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return h.publish("exit", data)
}

// publish writes the file name in the holder's directory whole, so a reader
// sees it complete or not at all.
func (h *Holder) publish(name string, data []byte) error {
	path := filepath.Join(h.dir, name)
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
func (h *Holder) copyOutput(in io.Reader, terminal bool) error {
	var position int64
	f, err := os.OpenFile(h.segment(0), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { f.Close() }()
	buf := make([]byte, 32<<10)
	size := 0
	for {
		n, readErr := in.Read(buf)
		p := buf[:n]
		for len(p) > 0 {
			if size == segmentSize {
				f.Close()
				f, err = os.OpenFile(h.segment(position), os.O_CREATE|os.O_WRONLY, 0600)
				if err != nil {
					return err
				}
				size = 0
				if terminal {
					positions, err := h.segments()
					if err != nil {
						return err
					}
					if len(positions) > 2 {
						if err = h.Discard(positions[len(positions)-2]); err != nil {
							return err
						}
					}
				}
			}
			count := min(len(p), segmentSize-size)
			written, err := f.Write(p[:count])
			if err != nil {
				return err
			}
			size += written
			position += int64(written)
			p = p[written:]
		}
		if readErr != nil {
			return nil
		}
	}
}

type diagnostics struct {
	mu  sync.Mutex
	buf []byte
}

func (d *diagnostics) Write(p []byte) (int, error) {
	_, _ = os.Stderr.Write(p)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.buf = append(d.buf, p...)
	if len(d.buf) > 2048 {
		d.buf = d.buf[len(d.buf)-2048:]
	}
	return len(p), nil
}
func (d *diagnostics) String() string { d.mu.Lock(); defer d.mu.Unlock(); return string(d.buf) }
